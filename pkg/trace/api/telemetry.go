// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
)

// telemetryMultiTransport sends HTTP requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// The target hostname
// When multiple endpoints are in use the response from the main endpoint
// is proxied back to the client, while for all aditional endpoints the
// response is discarded. There is no de-duplication done between endpoint
// hosts or api keys.
//
// Could be extended in the future to allow supporting more product endpoints
// by simply parametrizing metric tags, and logger names
type telemetryMultiTransport struct {
	Transport http.RoundTripper
	Endpoints []*config.Endpoint
}

// telemetryProxyHandler parses returns a new HTTP handler which will proxy requests to the configured intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) telemetryProxyHandler() http.Handler {
	// extract and validate Hostnames from configured endpoints
	var endpoints []*config.Endpoint
	for _, endpoint := range r.conf.TelemetryConfig.Endpoints {
		u, err := url.Parse(endpoint.Host)
		if err != nil {
			log.Errorf("Error parsing apm_config.telemetry endpoint %q: %v", endpoint.Host, err)
			continue
		}
		if u.Host != "" {
			endpoint.Host = u.Host
		}

		endpoints = append(endpoints, endpoint)
	}

	if len(endpoints) == 0 {
		log.Error("None of the configured apm_config.telemetry endpoints are valid. Telemetry proxy is off")
		return http.NotFoundHandler()
	}

	transport := telemetryMultiTransport{
		Transport: r.conf.NewHTTPTransport(),
		Endpoints: endpoints,
	}
	limitedLogger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	logger := stdlog.New(limitedLogger, "telemetry.Proxy: ", 0)
	director := func(req *http.Request) {
		req.Header.Set("Via", fmt.Sprintf("trace-agent %s", info.Version))
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to the default value
			// that net/http gives it: Go-http-client/1.1
			// See https://codereview.appspot.com/7532043
			req.Header.Set("User-Agent", "")
		}
		req.Header.Set("DD-Agent-Hostname", r.conf.Hostname)
		req.Header.Set("DD-Agent-Env", r.conf.DefaultEnv)
	}
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  logger,
		Transport: &transport,
	}
}

// RoundTrip sends request first to Endpoint[0], then sends a copy of main request to every configurged
// additional endpoint.
//
// All requests will be sent irregardless of any errors
// If any request fails, the error will be logged. Only main target's
// error will be propagated via return value
func (m *telemetryMultiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(m.Endpoints) == 1 {
		return m.roundTrip(req, m.Endpoints[0])
	}
	slurp, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	newreq := req.Clone(req.Context())
	newreq.Body = ioutil.NopCloser(bytes.NewReader(slurp))
	// despite the number of endpoints, we always return the response of the first
	rresp, rerr := m.roundTrip(newreq, m.Endpoints[0])
	for _, endpoint := range m.Endpoints[1:] {
		newreq := req.Clone(req.Context())
		newreq.Body = ioutil.NopCloser(bytes.NewReader(slurp))
		if resp, err := m.roundTrip(newreq, endpoint); err == nil {
			// we discard responses for all subsequent requests
			io.Copy(ioutil.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}

func (m *telemetryMultiTransport) roundTrip(req *http.Request, endpoint *config.Endpoint) (*http.Response, error) {
	tags := []string{
		fmt.Sprintf("endpoint:%s", endpoint.Host),
	}
	defer func(now time.Time) {
		metrics.Timing("datadog.trace_agent.telemetry_proxy.roundtrip_ms", time.Since(now), tags, 1)
	}(time.Now())

	req.Host = endpoint.Host
	req.URL.Host = endpoint.Host
	req.URL.Scheme = "https"
	req.Header.Set("DD-API-KEY", endpoint.APIKey)

	resp, err := m.Transport.RoundTrip(req)
	if err != nil {
		metrics.Count("datadog.trace_agent.telemetry_proxy.error", 1, tags, 1)
	}
	return resp, err
}
