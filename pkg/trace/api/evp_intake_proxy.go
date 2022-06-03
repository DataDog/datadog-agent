// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
)

const (
	validSubdomainSymbols       = "_-."
	validPathSymbols            = "/_-+"
	validPathQueryStringSymbols = "/_-+@?&=.:\""
)

func isValidSubdomain(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune(validSubdomainSymbols, c) {
			return false
		}
	}
	return true
}

func isValidPath(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune(validPathSymbols, c) {
			return false
		}
	}
	return true
}

func isValidQueryString(s string) bool {
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune(validPathQueryStringSymbols, c) {
			return false
		}
	}
	return true
}

// evpIntakeEndpointsFromConfig returns the configured list of endpoints to forward payloads to.
func evpIntakeEndpointsFromConfig(conf *config.AgentConfig) []config.Endpoint {
	apiKey := conf.EVPIntakeProxy.APIKey
	if apiKey == "" {
		apiKey = conf.APIKey()
	}
	endpoint := conf.EVPIntakeProxy.DDURL
	if endpoint == "" {
		endpoint = conf.Site
	}
	mainEndpoint := config.Endpoint{Host: endpoint, APIKey: apiKey}
	endpoints := []config.Endpoint{mainEndpoint}
	for host, keys := range conf.EVPIntakeProxy.AdditionalEndpoints {
		for _, key := range keys {
			endpoints = append(endpoints, config.Endpoint{
				Host:   host,
				APIKey: key,
			})
		}
	}
	return endpoints
}

// evpIntakeHandler returns an HTTP handler for the /evp_proxy API.
// Depending on the config, this is a proxying handler or a noop handler.
func (r *HTTPReceiver) evpIntakeHandler() http.Handler {
	// r.conf is populated by cmd/trace-agent/config/config.go
	if !r.conf.EVPIntakeProxy.Enabled {
		return evpIntakeErrorHandler("Has been disabled in config")
	}
	endpoints := evpIntakeEndpointsFromConfig(r.conf)
	transport := r.conf.NewHTTPTransport()
	logger := stdlog.New(log.NewThrottled(5, 10*time.Second), "EVPIntakeProxy: ", 0) // limit to 5 messages every 10 seconds
	handler := evpIntakeReverseProxyHandler(r.conf, endpoints, transport, logger)
	return http.StripPrefix("/evp_proxy/v1/input", handler)
}

// evpIntakeErrorHandler returns an HTTP handler that will always return
// http.StatusMethodNotAllowed along with a clarification.
func evpIntakeErrorHandler(message string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("EVPIntakeProxy is disabled: %v", message)
		http.Error(w, msg, http.StatusMethodNotAllowed)
	})
}

// evpIntakeReverseProxyHandler creates an http.ReverseProxy which can forward payloads
// to one or more endpoints, based on the request received and the Agent configuration.
// Headers are not proxied, instead we add our own known set of headers.
// See also evpIntakeProxyTransport below.
func evpIntakeReverseProxyHandler(conf *config.AgentConfig, endpoints []config.Endpoint, transport http.RoundTripper, logger *stdlog.Logger) http.Handler {
	director := func(req *http.Request) {

		containerID := req.Header.Get("Datadog-Container-ID")
		contentType := req.Header.Get("Content-Type")
		userAgent := req.Header.Get("User-Agent")

		// Clear all received headers
		req.Header = http.Header{}

		// Standard headers
		req.Header.Set("Via", fmt.Sprintf("trace-agent %s", info.Version))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("User-Agent", userAgent) // Set even if an empty string so Go doesn't set its default
		req.Header["X-Forwarded-For"] = nil     // Prevent setting X-Forwarded-For

		// Datadog headers
		if ctags := getContainerTags(conf.ContainerTags, containerID); ctags != "" {
			req.Header.Set("X-Datadog-Container-Tags", ctags)
		}
		req.Header.Set("X-Datadog-Hostname", conf.Hostname)
		req.Header.Set("X-Datadog-AgentDefaultEnv", conf.DefaultEnv)

		// URL, Host and the API key header are set in the transport for each outbound request
	}

	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  logger,
		Transport: &evpIntakeProxyTransport{transport, endpoints},
	}
}

// evpIntakeProxyTransport sends HTTPS requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// When multiple endpoints are in use the response from the first endpoint
// is proxied back to the client, while for all aditional endpoints the
// response is discarded.
type evpIntakeProxyTransport struct {
	transport http.RoundTripper
	endpoints []config.Endpoint
}

func (t *evpIntakeProxyTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {

	// Metrics with stats for debugging
	beginTime := time.Now()
	metricTags := []string{}
	defer func() {
		metrics.Count("datadog.trace_agent.evp_proxy.request", 1, metricTags, 1)
		metrics.Count("datadog.trace_agent.evp_proxy.request_bytes", req.ContentLength, metricTags, 1)
		metrics.Timing("datadog.trace_agent.evp_proxy.request_duration_ms", time.Since(beginTime), metricTags, 1)
		if rerr != nil {
			metrics.Count("datadog.trace_agent.evp_proxy.request_error", 1, metricTags, 1)
		}
	}()

	// Parse request path: The first component is the target subdomain, the rest is the target path.
	inputPath := req.URL.Path
	parts := strings.SplitN(inputPath, "/", 3)
	if len(parts) != 3 || parts[0] != "" || parts[1] == "" || parts[2] == "" {
		return nil, fmt.Errorf("EVPIntakeProxy: invalid path: '%s'", inputPath)
	}
	subdomain := parts[1]
	req.URL.Path = strings.TrimPrefix(req.URL.Path, "/"+subdomain)

	// Sanitize the input
	if !isValidSubdomain(subdomain) {
		return nil, fmt.Errorf("EVPIntakeProxy: invalid subdomain: %s", subdomain)
	}
	metricTags = append(metricTags, "subdomain:"+subdomain)
	if !isValidPath(req.URL.Path) {
		return nil, fmt.Errorf("EVPIntakeProxy: invalid target path: %s", req.URL.Path)
	}
	if !isValidQueryString(req.URL.RawQuery) {
		return nil, fmt.Errorf("EVPIntakeProxy: invalid query string: %s", req.URL.RawQuery)
	}

	req.URL.Scheme = "https"
	setTarget := func(r *http.Request, host, apiKey string) {
		targetHost := subdomain + "." + host
		r.Host = targetHost
		r.URL.Host = targetHost
		r.Header.Set("DD-API-KEY", apiKey)
	}

	if len(t.endpoints) == 1 {
		setTarget(req, t.endpoints[0].Host, t.endpoints[0].APIKey)
		return t.transport.RoundTrip(req)
	}

	// There's more than one destination endpoint

	var slurp *[]byte
	if req.Body != nil {
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		slurp = &body
	}
	for i, endpointDomain := range t.endpoints {
		newreq := req.Clone(req.Context())
		if slurp != nil {
			newreq.Body = ioutil.NopCloser(bytes.NewReader(*slurp))
		}
		setTarget(newreq, endpointDomain.Host, endpointDomain.APIKey)
		if i == 0 {
			// given the way we construct the list of targets the main endpoint
			// will be the first one called, we return its response and error
			rresp, rerr = t.transport.RoundTrip(newreq)
			continue
		}

		if resp, err := t.transport.RoundTrip(newreq); err == nil {
			// we discard responses for all subsequent requests
			io.Copy(ioutil.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}
