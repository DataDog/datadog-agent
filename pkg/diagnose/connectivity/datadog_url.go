// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/version"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"

	"crypto/tls"
)

type method string
type contentType string
type separator string

// URL types for different services
const (
	head method = "HEAD"
	post method = "POST"
	get  method = "GET"

	json      contentType = "application/json"
	multipart contentType = "multipart/form-data"

	dot  separator = "."
	dash separator = "-"
)

type endpointDescription struct {
	method            method
	route             string
	prefix            string
	path              string
	separator         separator
	contentType       contentType
	limitRedirect     bool
	additionalHeaders map[string]string
	versioned         bool
}

func getEndpointsDescriptions(cfg model.Reader) []endpointDescription {
	return []endpointDescription{
		{prefix: "install", method: head},
		{prefix: "yum", method: head},
		{prefix: "apt", method: head},
		{prefix: "keys", method: head},
		{prefix: "process", path: "probe", method: get},
		{route: helpers.GetFlareEndpoint(cfg), method: head, limitRedirect: true},
		{prefix: "orchestrator", path: "api/v2/orch", method: post, contentType: json},
		{prefix: "llmobs-intake.", path: "api/v2/llmobs", method: post, contentType: json},
		{prefix: "intake.synthetics.", path: "api/v2/synthetics", method: post, contentType: json},
		{prefix: "ndm-intake.", path: "api/v2/ndm", method: post, contentType: json},
		{prefix: "snmp-traps-intake.", path: "api/v2/ndmtraps", method: post, contentType: json},
		{prefix: "ndmflow-intake.", path: "api/v2/ndmflow", method: post, contentType: json},
		{prefix: "netpath-intake.", path: "api/v2/netpath", method: post, contentType: json},
		{prefix: "contlcycle-intake.", path: "api/v2/contlcycle", method: post, contentType: json},
		{prefix: "browser-intake-", path: "api/v2/logs", method: post, contentType: json, separator: dash},
		{prefix: "agent-http-intake.logs.", path: "api/v2/logs", method: post, contentType: json, separator: dot},
		{prefix: "trace.agent", path: "_health", method: get},
		{prefix: "config", path: "_health", method: get},
		{prefix: "instrumentation-telemetry-intake", path: "api/v2/apmtelemetry", method: post, contentType: json, additionalHeaders: map[string]string{
			"DD-Telemetry-Product": "agent",
		}},
		{prefix: "intake.profile", path: "api/v2/profile", method: post, contentType: multipart},
		{prefix: "app", path: "api/v1/validate", versioned: true, method: post, contentType: json, additionalHeaders: map[string]string{
			"User-Agent": "datadog-agent/<version>",
		}},
	}
}

type endpoint struct {
	url               string
	method            method
	contentType       contentType
	apiKey            string
	limitRedirect     bool
	additionalHeaders map[string]string
}

func (e *endpointDescription) buildEndpoints(domains map[string]domain) []endpoint {
	// if route is set -> There's only one possible url
	if e.route != "" {
		return []endpoint{
			{
				url:           e.route,
				method:        e.method,
				contentType:   e.contentType,
				apiKey:        domains["main"].apiKey,
				limitRedirect: e.limitRedirect,
			},
		}
	}
	routes := []endpoint{}
	if e.separator == "" {
		e.separator = dot
	}

	for _, domain := range domains {
		routes = append(routes, endpoint{
			url:               e.buildRoute(domain),
			method:            e.method,
			contentType:       e.contentType,
			apiKey:            domain.apiKey,
			limitRedirect:     e.limitRedirect,
			additionalHeaders: e.additionalHeaders,
		})
	}
	return routes
}

type domain struct {
	site          string
	apiKey        string
	infraEndpoint string
}

func getDomains(cfg model.Reader) map[string]domain {
	domains := map[string]domain{}

	site := pkgconfigsetup.DefaultSite

	if cfg.GetString("site") != "" {
		site = cfg.GetString("site")
	}

	domains["main"] = domain{
		site:          site,
		apiKey:        cfg.GetString("api_key"),
		infraEndpoint: utils.GetInfraEndpoint(cfg),
	}

	if cfg.GetBool("multi_region_failover.enabled") {
		if mrfEndpoint, err := utils.GetMRFInfraEndpoint(cfg); err == nil {
			domains["MRF"] = domain{
				site:          cfg.GetString("multi_region_failover.site"),
				apiKey:        cfg.GetString("multi_region_failover.apikey"),
				infraEndpoint: mrfEndpoint,
			}
		}
	}

	return domains
}

func (e *endpointDescription) buildRoute(domain domain) string {
	prefix := e.prefix
	path := e.path
	separator := e.separator

	route := ""
	if e.versioned {
		route, _ = utils.AddAgentVersionToDomain(domain.infraEndpoint, e.prefix)
	} else {
		if !strings.HasSuffix(prefix, string(separator)) {
			prefix = prefix + string(separator)
		}
		route = fmt.Sprintf("https://%s%s", prefix, domain.site)
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return route + path
}

// DiagnoseTest checks connectivity to a single endpoint
func DiagnoseTest(cfg model.Reader, endpointDescription endpointDescription) []diagnose.Diagnosis {
	endpoints := endpointDescription.buildEndpoints(getDomains(cfg))

	diagnoses := []diagnose.Diagnosis{}

	for _, endpoint := range endpoints {
		diagnosis, err := checkServiceConnectivity(endpoint)
		if err != nil {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:    diagnose.DiagnosisFail,
				Diagnosis: diagnosis,
				Name:      endpoint.url,
				RawError:  err.Error(),
			})
		} else {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:    diagnose.DiagnosisSuccess,
				Diagnosis: diagnosis,
				Name:      endpoint.url,
			})
		}
	}

	return diagnoses
}

// DiagnoseDatadogURL checks connectivity to Datadog endpoints
func DiagnoseDatadogURL(cfg model.Reader) []diagnose.Diagnosis {
	endpointsDescription := getEndpointsDescriptions(cfg)
	domains := getDomains(cfg)

	diagnoses := []diagnose.Diagnosis{}

	for _, ed := range endpointsDescription {
		endpoints := ed.buildEndpoints(domains)
		for _, endpoint := range endpoints {
			diagnosis, err := checkServiceConnectivity(endpoint)
			if err != nil {
				diagnoses = append(diagnoses, diagnose.Diagnosis{
					Status:    diagnose.DiagnosisFail,
					Diagnosis: diagnosis,
					Name:      endpoint.url,
					RawError:  err.Error(),
				})
			} else {
				diagnoses = append(diagnoses, diagnose.Diagnosis{
					Status:    diagnose.DiagnosisSuccess,
					Diagnosis: diagnosis,
					Name:      endpoint.url,
				})
			}
		}
	}

	return diagnoses
}

// checkServiceConnectivity checks connectivity for a specific service
func checkServiceConnectivity(endpoint endpoint) (string, error) {
	// Build URL based on service type
	switch endpoint.method {
	case head:
		return endpoint.checkHead()
	case post:
		return endpoint.checkPost()
	case get:
		return endpoint.checkGet()
	default:
		return "Unknown URL Type", fmt.Errorf("unknown URL type for service %s", endpoint.url)
	}
}

// checkHead verifies if an HTTP URL is accessible
func (e endpoint) checkHead() (string, error) {
	return executeRequest(e, map[string]string{})
}

func (e endpoint) checkGet() (string, error) {
	headers := map[string]string{
		"DD-API-KEY": e.apiKey,
	}

	return executeRequest(e, headers)
}

// checkHead verifies if an HTTP URL is accessible
func (e endpoint) checkPost() (string, error) {
	headers := map[string]string{
		"Content-Type": string(json),
		"DD-API-KEY":   e.apiKey,
	}

	return executeRequest(e, headers)
}

func executeRequest(endpoint endpoint, headers map[string]string) (string, error) {
	url := endpoint.url
	var httpTraces []string
	ctx := httptrace.WithClientTrace(context.Background(), createDiagnoseTraces(&httpTraces))

	options := []func(*http.Client){}

	if endpoint.limitRedirect {
		options = append(options, withOneRedirect())
	}
	client := getClient(options...)

	req, err := http.NewRequestWithContext(ctx, string(endpoint.method), url, nil)
	if err != nil {
		return "configuration issue", fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	if endpoint.additionalHeaders != nil {
		for k, v := range endpoint.additionalHeaders {
			headers[k] = v
		}
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

func getClient(clientOptions ...func(*http.Client)) *http.Client {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
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
		},
	}

	for _, clientOption := range clientOptions {
		clientOption(client)
	}

	return client
}
