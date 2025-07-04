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
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

type endpointDescription struct {
	method        string
	route         string
	prefix        string
	dashPrefix    string
	routePath     string
	configPrefix  string
	limitRedirect bool
	versioned     bool
}

func getEndpointsDescriptions(cfg model.Reader) []endpointDescription {
	return []endpointDescription{
		{route: "https://install.datadoghq.com", method: http.MethodHead},
		{route: "https://yum.datadoghq.com", method: http.MethodHead},
		{route: "https://apt.datadoghq.com", method: http.MethodHead},
		{route: "https://keys.datadoghq.com", method: http.MethodHead},
		{prefix: "process", routePath: "probe", method: http.MethodGet},
		{route: helpers.GetFlareEndpoint(cfg), method: http.MethodHead, limitRedirect: true},
		{prefix: "orchestrator", routePath: "probe", method: http.MethodGet},
		{prefix: "llmobs-intake", routePath: "probe", method: http.MethodGet},
		{prefix: "intake.synthetics", routePath: "probe", method: http.MethodGet},
		{prefix: "ndm-intake", routePath: "probe", method: http.MethodGet, configPrefix: "network_devices.metadata"},
		{prefix: "snmp-traps-intake", routePath: "probe", method: http.MethodGet, configPrefix: "network_devices.snmp_traps.forwarder"},
		{prefix: "ndmflow-intake", routePath: "probe", method: http.MethodGet, configPrefix: "network_devices.netflow.forwarder"},
		{prefix: "netpath-intake", routePath: "probe", method: http.MethodGet, configPrefix: "network_path.forwarder"},
		{prefix: "contlcycle-intake", routePath: "probe", method: http.MethodGet, configPrefix: "container_lifecycle.forwarder"},
		{dashPrefix: "browser-intake", routePath: "probe", method: http.MethodGet},
		{prefix: "agent-http-intake.logs", routePath: "probe", method: http.MethodGet, configPrefix: "logs_config"},
		{prefix: "trace.agent", routePath: "_health", method: http.MethodGet},
		{prefix: "config", routePath: "_health", method: http.MethodGet},
		{prefix: "instrumentation-telemetry-intake", routePath: "probe", method: http.MethodGet, configPrefix: "service_discovery.forwarder"},
		{prefix: "intake.profile", routePath: "probe", method: http.MethodGet},
		{prefix: "app", routePath: "probe", versioned: true, method: http.MethodGet},
	}
}

type resolvedEndpoint struct {
	url           string
	base          string
	method        string
	apiKey        string
	limitRedirect bool
}

func (e *endpointDescription) buildEndpoints(cfg model.Reader, domains map[string]domain) []resolvedEndpoint {
	// if route is set -> There's only one possible url
	if e.route != "" {
		return []resolvedEndpoint{
			{
				url:           e.route,
				base:          e.route,
				method:        e.method,
				apiKey:        getAPIKey(cfg, e.configPrefix, domains["main"].mainAPIKey, false),
				limitRedirect: e.limitRedirect,
			},
		}
	}
	routes := []resolvedEndpoint{}

	for _, domain := range domains {
		base, url := e.buildRoute(domain)
		routes = append(routes, resolvedEndpoint{
			url:           url,
			base:          base,
			method:        e.method,
			apiKey:        getAPIKey(cfg, e.configPrefix, domain.mainAPIKey, domain.useAltAPIKey),
			limitRedirect: e.limitRedirect,
		})
	}
	return routes
}

func getAPIKey(cfg model.Reader, configPrefix string, defaultAPIKey string, useCustomAPIKey bool) string {
	if !useCustomAPIKey {
		return defaultAPIKey
	}
	if !strings.HasSuffix(configPrefix, ".") {
		configPrefix = configPrefix + "."
	}
	if apiKey := cfg.GetString(configPrefix + "api_key"); apiKey != "" {
		return apiKey
	}
	return defaultAPIKey
}

type domain struct {
	site          string
	mainAPIKey    string
	infraEndpoint string
	useAltAPIKey  bool
}

func getDomains(cfg model.Reader) map[string]domain {
	domains := map[string]domain{}

	site := pkgconfigsetup.DefaultSite

	if cfg.GetString("site") != "" {
		site = cfg.GetString("site")
	}

	domains["main"] = domain{
		site:          site,
		mainAPIKey:    cfg.GetString("api_key"),
		infraEndpoint: utils.GetInfraEndpoint(cfg),
		useAltAPIKey:  true,
	}

	if cfg.GetBool("multi_region_failover.enabled") {
		if mrfEndpoint, err := utils.GetMRFEndpoint(cfg, utils.InfraURLPrefix, "multi_region_failover.dd_url"); err == nil {
			domains["MRF"] = domain{
				site:          cfg.GetString("multi_region_failover.site"),
				mainAPIKey:    cfg.GetString("multi_region_failover.api_key"),
				infraEndpoint: mrfEndpoint,
				useAltAPIKey:  false,
			}
		}
	}

	return domains
}

func (e *endpointDescription) buildRoute(domain domain) (baseURL string, route string) {
	if e.versioned {
		baseURL, _ = utils.AddAgentVersionToDomain(domain.infraEndpoint, e.prefix)
	} else {
		if e.dashPrefix != "" {
			baseURL = fmt.Sprintf("https://%s", joinPrefix(e.dashPrefix, "-", domain.site))
		} else {
			baseURL = fmt.Sprintf("https://%s", joinPrefix(e.prefix, ".", domain.site))
		}
	}

	if !strings.HasPrefix(e.routePath, "/") {
		e.routePath = "/" + e.routePath
	}
	return baseURL, baseURL + e.routePath
}

func joinPrefix(prefix, separator, domain string) string {
	if !strings.HasSuffix(prefix, separator) {
		prefix = prefix + separator
	}
	return prefix + domain
}

const (
	maxParallelWorkers = 3
	httpClientTimeout  = 10 * time.Second
)

// DiagnoseInventory checks the connectivity of the endpoints
func DiagnoseInventory(ctx context.Context, cfg config.Component, log log.Component) ([]diagnose.Diagnosis, error) {
	endpointsDescription := getEndpointsDescriptions(cfg)
	domains := getDomains(cfg)

	// Collect all endpoints to check
	var allEndpoints []resolvedEndpoint
	for _, ed := range endpointsDescription {
		endpoints := ed.buildEndpoints(cfg, domains)
		allEndpoints = append(allEndpoints, endpoints...)
	}

	// Create HTTP client for workers
	client := getClient(cfg, min(maxParallelWorkers, len(allEndpoints)), log, withOneRedirect(), withTimeout(httpClientTimeout))

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

				description := "Ping: " + endpoint.base
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
	default:
		return "Unknown Method", fmt.Errorf("unknown Method for service %s", e.url)
	}
}

func (e resolvedEndpoint) checkHead(ctx context.Context, client *http.Client) (string, error) {
	if e.limitRedirect {
		withOneRedirect()(client)
	}
	statusCode, _, err := sendHead(ctx, client, e.url)
	if e.limitRedirect {
		client.CheckRedirect = nil
	}
	if err != nil {
		return "Failed to connect", err
	}
	return validateStatusCode(e, statusCode)
}

func (e resolvedEndpoint) checkGet(ctx context.Context, client *http.Client) (string, error) {
	httpTraces := []string{}
	ctx = httptrace.WithClientTrace(ctx, createDiagnoseTraces(&httpTraces, true))
	statusCode, _, _, err := sendGet(ctx, client, e.url, map[string]string{
		"DD-API-KEY": e.apiKey,
	})
	if err != nil {
		return "Failed to connect", fmt.Errorf("%s\n%w", strings.Join(httpTraces, "\n"), err)
	}
	return validateStatusCode(e, statusCode)
}

func validateStatusCode(endpoint resolvedEndpoint, statusCode int) (string, error) {
	if !isSuccessStatusCode(endpoint, statusCode) {
		return "invalid status code", fmt.Errorf("invalid status code: %d", statusCode)
	}
	return "Success", nil
}

func isSuccessStatusCode(endpoint resolvedEndpoint, statusCode int) bool {
	if statusCode == http.StatusTemporaryRedirect || statusCode == http.StatusPermanentRedirect {
		return endpoint.limitRedirect
	}
	return statusCode >= 200 && statusCode < 300
}
