// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runner implements the connectivity checker component
package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	httputil "github.com/DataDog/datadog-agent/pkg/util/http"

	"crypto/tls"
)

type method string
type separator string

// URL types for different services
const (
	head method = "HEAD"
	get  method = "GET"

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

type endpoint struct {
	url           string
	base          string
	method        method
	apiKey        string
	limitRedirect bool
}

func (e *endpointDescription) buildEndpoints(cfg model.Reader, domains map[string]domain) []endpoint {
	// if route is set -> There's only one possible url
	if e.route != "" {
		return []endpoint{
			{
				url:           e.route,
				base:          e.route,
				method:        e.method,
				apiKey:        getAPIKey(cfg, e.configPrefix, domains["main"].mainAPIKey, false),
				limitRedirect: e.limitRedirect,
			},
		}
	}
	routes := []endpoint{}
	if e.separator == "" {
		e.separator = dot
	}

	for _, domain := range domains {
		base, url := e.buildRoute(domain)
		routes = append(routes, endpoint{
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

func diagnoseConnectivity(cfg model.Reader) []DiagnosisPayload {
	endpointsDescription := getEndpointsDescriptions(cfg)
	domains := getDomains(cfg)

	diagnoses := []DiagnosisPayload{}

	for _, ed := range endpointsDescription {
		endpoints := ed.buildEndpoints(cfg, domains)
		for _, endpoint := range endpoints {
			description := "Ping: " + endpoint.base
			diagnosis, err := endpoint.checkServiceConnectivity(cfg)

			if err != nil {
				diagnoses = append(diagnoses, DiagnosisPayload{
					Status:      failure,
					Description: description,
					Error:       diagnosis,
					Metadata: map[string]string{
						"endpoint":  endpoint.url,
						"raw_error": err.Error(),
					},
				})
			} else {
				diagnoses = append(diagnoses, DiagnosisPayload{
					Status:      success,
					Description: description,
					Metadata: map[string]string{
						"endpoint": endpoint.url,
					},
				})
			}
		}
	}

	return diagnoses
}

func (e endpoint) checkServiceConnectivity(cfg model.Reader) (string, error) {
	// Build URL based on service type
	switch e.method {
	case head:
		return e.checkHead(cfg)
	case get:
		return e.checkGet(cfg)
	default:
		return "Unknown Method", fmt.Errorf("unknown Method for service %s", e.url)
	}
}

func (e endpoint) checkHead(cfg model.Reader) (string, error) {
	return executeRequest(e, cfg, map[string]string{
		"DD-API-KEY": e.apiKey,
	})
}

func (e endpoint) checkGet(cfg model.Reader) (string, error) {
	return executeRequest(e, cfg, map[string]string{
		"DD-API-KEY": e.apiKey,
	})
}

func executeRequest(endpoint endpoint, cfg model.Reader, headers map[string]string) (string, error) {
	url := endpoint.url
	var httpTraces []string
	ctx := httptrace.WithClientTrace(context.Background(), createDiagnoseTraces(&httpTraces))

	options := []func(*http.Client){}

	if endpoint.limitRedirect {
		options = append(options, withOneRedirect())
	}
	client := getClient(cfg, options...)

	req, err := http.NewRequestWithContext(ctx, string(endpoint.method), url, nil)
	if err != nil {
		return "configuration issue", fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	for k, v := range headers {
		v = strings.Replace(v, "<version>", version.AgentVersion, 1)
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Include HTTP traces in error message if available
		if len(httpTraces) > 0 {
			return "Failed to connect", fmt.Errorf("%w\nTraces:\n%s", err, strings.Join(httpTraces, "\n"))
		}
		return "Failed to connect", err
	}
	defer resp.Body.Close()

	return validateStatusCode(endpoint, resp.StatusCode)
}

func validateStatusCode(endpoint endpoint, statusCode int) (string, error) {
	if !isSuccessStatusCode(endpoint, statusCode) {
		return "invalid status code", fmt.Errorf("invalid status code: %d", statusCode)
	}
	return "Success", nil
}

func isSuccessStatusCode(endpoint endpoint, statusCode int) bool {
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

func withOneRedirect() func(*http.Client) {
	return func(client *http.Client) {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
}

func getClient(cfg model.Reader, clientOptions ...func(*http.Client)) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          2,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if proxies := cfg.GetProxies(); proxies != nil {
		transport.Proxy = httputil.GetProxyTransportFunc(proxies, cfg)
	}

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}

	for _, clientOption := range clientOptions {
		clientOption(client)
	}

	return client
}

// createDiagnoseTraces creates a httptrace.ClientTrace containing functions that collects
// additional information when a http.Client is sending requests
// During a request, the http.Client will call the functions of the ClientTrace at specific moments
// This is useful to get extra information about what is happening and if there are errors during
// connection establishment, DNS resolution or TLS handshake for instance
func createDiagnoseTraces(httpTraces *[]string) *httptrace.ClientTrace {
	hooks := &httpTraceContext{
		httpTraces: httpTraces,
	}

	return &httptrace.ClientTrace{
		ConnectDone:      hooks.connectDoneHook,
		DNSDone:          hooks.dnsDoneHook,
		TLSHandshakeDone: hooks.tlsHandshakeDoneHook,
	}
}

// httpTraceContext collect reported HTTP traces into its holding array
// to be retrieved later by client
type httpTraceContext struct {
	httpTraces *[]string
}

// connectDoneHook is called when the new connection to 'addr' completes
// It collects the error message if there is one and indicates if this step was successful
func (c *httpTraceContext) connectDoneHook(_, _ string, err error) {
	if err != nil {
		*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("Connect: FAIL -  %v", scrubber.ScrubLine(err.Error())))
	}
}

// dnsDoneHook is called after the DNS lookup
// It collects the error message if there is one and indicates if this step was successful
func (c *httpTraceContext) dnsDoneHook(di httptrace.DNSDoneInfo) {
	if di.Err != nil {
		*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("DNS Lookup: FAIL -  %v", scrubber.ScrubLine(di.Err.Error())))
	}
}

// tlsHandshakeDoneHook is called after the TLS Handshake
// It collects the error message if there is one and indicates if this step was successful
func (c *httpTraceContext) tlsHandshakeDoneHook(_ tls.ConnectionState, err error) {
	if err != nil {
		trace := fmt.Sprintf("TLS Handshake: FAIL - %v", scrubber.ScrubLine(err.Error()))
		if strings.Contains(err.Error(), "first record does not look like a TLS handshake") {
			trace = fmt.Sprintf("%s - Hint: Endpoint is not configured for HTTPS", trace)
		}
		*(c.httpTraces) = append(*(c.httpTraces), trace)
	}
}
