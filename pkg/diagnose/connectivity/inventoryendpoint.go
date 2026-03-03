// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivity implements the connectivity checker component
package connectivity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

type endpointDescription struct {
	name              string
	method            string
	route             string
	prefix            string
	routePath         string
	configPrefix      string
	altURLOverrideKey string
	handlesFailover   bool
}

func getEndpointsDescriptions() []endpointDescription {
	// TODO: find a way to check process, orchestrator, profiling
	return []endpointDescription{
		{name: "Agent installation", route: "https://install.datadoghq.com", method: http.MethodHead, altURLOverrideKey: "installer.registry.url"},
		{name: "Agent package yum", route: "https://yum.datadoghq.com", method: http.MethodHead},
		{name: "Agent package apt", route: "https://apt.datadoghq.com", method: http.MethodHead},
		{name: "Agent keys", route: "https://keys.datadoghq.com", method: http.MethodHead},
		{name: "APM traces", prefix: "trace.agent", routePath: "_health", method: http.MethodGet, altURLOverrideKey: "apm_config.apm_dd_url"},
		{name: "LLM obs", prefix: "llmobs-intake", routePath: "api/v2/llmobs", method: http.MethodPost},
		{name: "Remote configuration", prefix: "config", configPrefix: "remote_configuration", altURLOverrideKey: "remote_configuration.rc_dd_url", handlesFailover: true, routePath: "_health", method: http.MethodGet},
	}
}

type resolvedEndpoint struct {
	name       string
	url        string
	method     string
	apiKey     string
	isFailover bool
}

func (e *endpointDescription) buildEndpoints(cfg model.Reader, domains []domain) []resolvedEndpoint {
	// if route is set -> There's only one possible url
	if e.route != "" {
		mainDomain := domains[0]
		route := e.route
		if e.altURLOverrideKey != "" {
			urlOverrideKey := getURLOverrideKey(e.altURLOverrideKey, false)
			if cfg.IsKnown(urlOverrideKey) {
				if overrideRoute := cfg.GetString(urlOverrideKey); overrideRoute != "" {
					route = overrideRoute
				}
			}
		}

		return []resolvedEndpoint{
			{
				name:   e.name,
				url:    route,
				method: e.method,
				apiKey: getAPIKey(cfg, e.configPrefix, mainDomain.defaultAPIKey, false),
			},
		}
	}
	routes := []resolvedEndpoint{}

	for _, domain := range domains {
		if domain.isFailover && !e.handlesFailover {
			continue
		}

		url := e.buildRoute(cfg, domain)
		routes = append(routes, resolvedEndpoint{
			name:       e.name,
			url:        url,
			method:     e.method,
			apiKey:     getAPIKey(cfg, e.configPrefix, domain.defaultAPIKey, domain.useAltAPIKey),
			isFailover: domain.isFailover,
		})
	}
	return routes
}

func getAPIKey(cfg model.Reader, configPrefix string, defaultAPIKey string, altAPIKey bool) string {
	if !altAPIKey {
		return defaultAPIKey
	}

	if configPrefix == "" {
		return defaultAPIKey
	}

	configAPIKey := joinSuffix(configPrefix, ".") + "api_key"
	if !cfg.IsKnown(configAPIKey) {
		return defaultAPIKey
	}
	if apiKey := cfg.GetString(configAPIKey); apiKey != "" {
		return apiKey
	}
	return defaultAPIKey
}

type domain struct {
	site          string
	defaultAPIKey string
	infraEndpoint string
	useAltAPIKey  bool
	isFailover    bool
}

func getDomains(cfg model.Reader) []domain {
	domains := []domain{}

	mainSite := pkgconfigsetup.DefaultSite
	if cfg.GetString("site") != "" {
		mainSite = cfg.GetString("site")
	}

	domains = append(domains, domain{
		site:          mainSite,
		defaultAPIKey: cfg.GetString("api_key"),
		infraEndpoint: utils.GetInfraEndpoint(cfg),
		useAltAPIKey:  true,
		isFailover:    false,
	})

	if cfg.GetBool("multi_region_failover.enabled") {
		if mrfEndpoint, err := utils.GetMRFEndpoint(cfg, utils.InfraURLPrefix, "multi_region_failover.dd_url"); err == nil {
			domains = append(domains, domain{
				site:          cfg.GetString("multi_region_failover.site"),
				defaultAPIKey: cfg.GetString("multi_region_failover.api_key"),
				infraEndpoint: mrfEndpoint,
				useAltAPIKey:  false,
				isFailover:    true,
			})
		}
	}

	return domains
}

func (e *endpointDescription) buildRoute(cfg model.Reader, domain domain) string {
	baseURL := ""
	urlOverrideKey := getURLOverrideKey(e.altURLOverrideKey, domain.isFailover)
	if domain.isFailover {
		baseURL, _ = utils.GetMRFEndpoint(cfg, joinSuffix(e.prefix, "."), urlOverrideKey)
	} else {
		baseURL = utils.GetMainEndpoint(cfg, joinSuffix(e.prefix, "."), urlOverrideKey)
	}
	if !strings.HasPrefix(baseURL, "https://") && !strings.HasPrefix(baseURL, "http://") {
		baseURL = "https://" + baseURL
	}

	path := e.routePath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func getURLOverrideKey(altKey string, isFailover bool) string {
	if !isFailover {
		return altKey
	}
	return "multi_region_failover." + altKey
}

func joinSuffix(prefix, separator string) string {
	if strings.HasSuffix(prefix, separator) {
		return prefix
	}
	return prefix + separator
}

const (
	maxParallelWorkers = 3
	httpClientTimeout  = 10 * time.Second
)

// DiagnoseInventory checks the connectivity of the endpoints
func DiagnoseInventory(ctx context.Context, cfg config.Component, log log.Component) ([]diagnose.Diagnosis, error) {
	endpointsDescription := getEndpointsDescriptions()
	domains := getDomains(cfg)

	// Collect all endpoints to check
	var allEndpoints []resolvedEndpoint
	for _, ed := range endpointsDescription {
		endpoints := ed.buildEndpoints(cfg, domains)
		allEndpoints = append(allEndpoints, endpoints...)
	}

	// Create HTTP clients for workers
	client := getClient(cfg, min(maxParallelWorkers, len(allEndpoints)), log, withTimeout(httpClientTimeout)) // unlimited redirects

	return checkEndpoints(ctx, allEndpoints, client)
}

// checkEndpoints checks the connectivity of the provided endpoints in parallel
func checkEndpoints(ctx context.Context, endpoints []resolvedEndpoint, client *http.Client) ([]diagnose.Diagnosis, error) {
	workerCount := min(maxParallelWorkers, len(endpoints))

	// Create channels for work distribution and results collection
	endpointChan := make(chan resolvedEndpoint, len(endpoints))
	resultChan := make(chan diagnose.Diagnosis, len(endpoints))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for endpoint := range endpointChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				description := endpoint.name
				if endpoint.isFailover {
					description += " - failover"
				}

				diagnosis, err := endpoint.checkServiceConnectivity(ctx, client)

				var result diagnose.Diagnosis
				if err != nil {
					result = diagnose.Diagnosis{
						Status:    diagnose.DiagnosisFail,
						Name:      description,
						Diagnosis: diagnosis,
						Metadata: map[string]string{
							"endpoint":  endpoint.url,
							"raw_error": err.Error(),
						},
					}
				} else {
					result = diagnose.Diagnosis{
						Status: diagnose.DiagnosisSuccess,
						Name:   description,
						Metadata: map[string]string{
							"endpoint": endpoint.url,
						},
					}
				}

				select {
				case resultChan <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Send all endpoints to workers
	go func() {
		defer close(endpointChan)
		for _, endpoint := range endpoints {
			select {
			case endpointChan <- endpoint:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var diagnoses []diagnose.Diagnosis
	for result := range resultChan {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			diagnoses = append(diagnoses, result)
		}
	}

	return diagnoses, nil
}

func (e resolvedEndpoint) checkServiceConnectivity(ctx context.Context, client *http.Client) (string, error) {
	switch e.method {
	case http.MethodHead:
		return e.checkHead(ctx, client)
	case http.MethodGet:
		return e.checkGet(ctx, client)
	case http.MethodPost:
		return e.checkPost(ctx, client)
	default:
		return "Unknown Method", fmt.Errorf("unknown Method for service %s", e.url)
	}
}

func (e resolvedEndpoint) checkHead(ctx context.Context, client *http.Client) (string, error) {
	statusCode, _, err := sendHead(ctx, client, e.url)
	if err != nil {
		return "Failed to connect", err
	}
	return validateStatusCode(statusCode)
}

func (e resolvedEndpoint) checkGet(ctx context.Context, client *http.Client) (string, error) {
	httpTraces := []string{}
	ctx = httptrace.WithClientTrace(ctx, createDiagnoseTraces(&httpTraces, true))
	statusCode, _, _, err := sendGet(ctx, client, e.url, map[string]string{
		"DD-API-KEY":   e.apiKey,
		"Content-Type": "application/json",
	})
	if err != nil {
		return "Failed to connect", fmt.Errorf("%s\n%w", strings.Join(httpTraces, "\n"), err)
	}
	return validateStatusCode(statusCode)
}

func (e resolvedEndpoint) checkPost(ctx context.Context, client *http.Client) (string, error) {
	httpTraces := []string{}
	ctx = httptrace.WithClientTrace(ctx, createDiagnoseTraces(&httpTraces, true))
	statusCode, _, _, err := sendPost(ctx, client, e.url, nil, map[string]string{
		"DD-API-KEY":   e.apiKey,
		"Content-Type": "application/json",
	})
	if err != nil {
		return "Failed to connect", fmt.Errorf("%s\n%w", strings.Join(httpTraces, "\n"), err)
	}
	return validateStatusCode(statusCode)
}

func validateStatusCode(statusCode int) (string, error) {
	if !isSuccessStatusCode(statusCode) {
		return "invalid status code", fmt.Errorf("invalid status code: %d", statusCode)
	}
	return "Success", nil
}

func isSuccessStatusCode(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}
