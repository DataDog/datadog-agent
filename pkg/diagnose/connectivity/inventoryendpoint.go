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

type separator string

// URL types for different services
const (
	dot  separator = "."
	dash separator = "-"
)

type endpointDescription struct {
	method        method
	route         string
	routePrefix   string
	routePath     string
	configPrefix  string
	separator     separator
	limitRedirect bool
	versioned     bool
}

func getEndpointsDescriptions(cfg model.Reader) []endpointDescription {
	return []endpointDescription{
		{route: "https://install.datadoghq.com", method: head},
		{route: "https://yum.datadoghq.com", method: head},
		{route: "https://apt.datadoghq.com", method: head},
		{route: "https://keys.datadoghq.com", method: head},
		{routePrefix: "process", routePath: "probe", method: get},
		{route: helpers.GetFlareEndpoint(cfg), method: head, limitRedirect: true},
		{routePrefix: "orchestrator", routePath: "probe", method: get},
		{routePrefix: "llmobs-intake.", routePath: "probe", method: get},
		{routePrefix: "intake.synthetics.", routePath: "probe", method: get},
		{routePrefix: "ndm-intake.", routePath: "probe", method: get, configPrefix: "network_devices.metadata."},
		{routePrefix: "snmp-traps-intake.", routePath: "probe", method: get, configPrefix: "network_devices.snmp_traps.forwarder."},
		{routePrefix: "ndmflow-intake.", routePath: "probe", method: get, configPrefix: "network_devices.netflow.forwarder."},
		{routePrefix: "netpath-intake.", routePath: "probe", method: get, configPrefix: "network_path.forwarder."},
		{routePrefix: "contlcycle-intake.", routePath: "probe", method: get, configPrefix: "container_lifecycle.forwarder."},
		{routePrefix: "browser-intake-", routePath: "probe", method: get, separator: dash},
		{routePrefix: "agent-http-intake.logs.", routePath: "probe", method: get, configPrefix: "logs_config."},
		{routePrefix: "trace.agent", routePath: "_health", method: get},
		{routePrefix: "config", routePath: "_health", method: get},
		{routePrefix: "instrumentation-telemetry-intake", routePath: "probe", method: get, configPrefix: "service_discovery.forwarder."},
		{routePrefix: "intake.profile", routePath: "probe", method: get},
		{routePrefix: "app", routePath: "probe", versioned: true, method: get},
	}
}

type resolvedEndpoint struct {
	url           string
	base          string
	method        method
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
	if e.separator == "" {
		e.separator = dot
	}

	for _, domain := range domains {
		base, url := e.buildRoute(domain)
		routes = append(routes, resolvedEndpoint{
			url:           url,
			base:          base,
			method:        e.method,
			apiKey:        getAPIKey(cfg, e.configPrefix, domain.mainAPIKey, domain.useCustomAPIKey),
			limitRedirect: e.limitRedirect,
		})
	}
	return routes
}

func getAPIKey(cfg model.Reader, configPrefix string, defaultAPIKey string, useCustomAPIKey bool) string {
	if !useCustomAPIKey {
		return defaultAPIKey
	}
	if apiKey := cfg.GetString(configPrefix + "api_key"); apiKey != "" {
		return apiKey
	}
	return defaultAPIKey
}

type domain struct {
	site            string
	mainAPIKey      string
	infraEndpoint   string
	useCustomAPIKey bool
}

func getDomains(cfg model.Reader) map[string]domain {
	domains := map[string]domain{}

	site := pkgconfigsetup.DefaultSite

	if cfg.GetString("site") != "" {
		site = cfg.GetString("site")
	}

	domains["main"] = domain{
		site:            site,
		mainAPIKey:      cfg.GetString("api_key"),
		infraEndpoint:   utils.GetInfraEndpoint(cfg),
		useCustomAPIKey: true,
	}

	if cfg.GetBool("multi_region_failover.enabled") {
		if mrfEndpoint, err := utils.GetMRFEndpoint(cfg, utils.InfraURLPrefix, "multi_region_failover.dd_url"); err == nil {
			domains["MRF"] = domain{
				site:            cfg.GetString("multi_region_failover.site"),
				mainAPIKey:      cfg.GetString("multi_region_failover.api_key"),
				infraEndpoint:   mrfEndpoint,
				useCustomAPIKey: false,
			}
		}
	}

	return domains
}

func (e *endpointDescription) buildRoute(domain domain) (string, string) {
	prefix := e.routePrefix
	path := e.routePath
	separator := e.separator

	base := ""
	if e.versioned {
		base, _ = utils.AddAgentVersionToDomain(domain.infraEndpoint, e.routePrefix)
	} else {
		if !strings.HasSuffix(prefix, string(separator)) {
			prefix = prefix + string(separator)
		}
		base = fmt.Sprintf("https://%s%s", prefix, domain.site)
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base, base + path
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
	case head:
		return e.checkHead(ctx, client)
	case get:
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
	switch endpoint.method {
	case head:
		if statusCode == http.StatusTemporaryRedirect || statusCode == http.StatusPermanentRedirect {
			return endpoint.limitRedirect
		}
		return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
	default:
		return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
	}
}
