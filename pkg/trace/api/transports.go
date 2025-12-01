// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// measuringTransport is a transport that emits count and timing metrics
// prefixed with a prefix and decorated with tags.
type measuringTransport struct {
	rt     http.RoundTripper
	prefix string
	tags   []string
	statsd statsd.ClientInterface
}

// newMeasuringTransport creates a measuringTransport wrapping another round
// tripper emitting metrics.
func newMeasuringTransport(rt http.RoundTripper, prefix string, tags []string, statsd statsd.ClientInterface) *measuringTransport {
	return &measuringTransport{rt, prefix, tags, statsd}
}

// RoundTrip makes an HTTP round trip measuring request count and timing.
func (m *measuringTransport) RoundTrip(req *http.Request) (rres *http.Response, rerr error) {
	defer func(start time.Time) {
		_ = m.statsd.Count(m.prefix+".proxy_request", 1, m.tags, 1)
		_ = m.statsd.Timing(m.prefix+".proxy_request_duration_ms", time.Since(start), m.tags, 1)
		if rerr != nil {
			tags := append(m.tags, "error:"+fmt.Sprintf("%T", rerr))
			_ = m.statsd.Count(m.prefix+".proxy_request_error", 1, tags, 1)
		}
	}(time.Now())
	return m.rt.RoundTrip(req)
}

// forwardingTransport is an HTTP transport wraps another transport that
// forwards a request to multiple endpoints. The first target in the targets
// slice is considered the main endpoint. Only the main endpoints response will
// be returned to the client. Responses of additional endpoints in the targets
// slice are dropped. Errors on additional endpoints will be logged.
type forwardingTransport struct {
	rt      http.RoundTripper
	targets []*url.URL
	keys    []string
	logger  *log.ThrottledLogger
}

// newForwardingTransport creates a new forwardingTransport, wrapping another
// round tripper with a main endpoint and additional endpoints to forwards the
// request to.
func newForwardingTransport(
	rt http.RoundTripper,
	mainEndpoint *url.URL,
	mainEndpointKey string,
	additionalEndpoints map[string][]string,
) *forwardingTransport {
	targets := []*url.URL{mainEndpoint}
	apiKeys := []string{mainEndpointKey}
	for endpoint, keys := range additionalEndpoints {
		u, err := url.Parse(endpoint)
		if err != nil {
			log.Errorf("Error parsing additional intake URL %s: %v", endpoint, err)
			continue
		}
		for _, key := range keys {
			targets = append(targets, u)
			apiKeys = append(apiKeys, strings.TrimSpace(key))
		}
	}
	return &forwardingTransport{rt, targets, apiKeys, log.NewThrottled(10, 10*time.Second)}
}

// RoundTrip makes an HTTP round trip forwarding one request to multiple
// additional endpoints.
func (m *forwardingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	setTarget := func(r *http.Request, u *url.URL, apiKey string) {
		q := r.URL.Query()
		u.RawQuery = q.Encode()
		r.Host = u.Host
		r.URL = u
		r.Header.Set("DD-API-KEY", apiKey)
	}
	if len(m.targets) == 1 {
		setTarget(req, m.targets[0], m.keys[0])
		return m.rt.RoundTrip(req)
	}

	var body []byte
	if req.Body != nil {
		slurp, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		body = slurp
	}

	roundTripAdditional := func(req *http.Request) {
		resp, err := m.rt.RoundTrip(req)
		if err == nil {
			// we discard responses for all subsequent requests
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
		} else {
			m.logger.Error("error forwarding request to %s: %v", req.URL, err)
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}
	var wg sync.WaitGroup
	for i, u := range m.targets {
		if i == 0 {
			continue
		}
		newreq := req.Clone(req.Context())
		if body != nil {
			newreq.Body = io.NopCloser(bytes.NewReader(body))
		}
		setTarget(newreq, u, m.keys[i])
		wg.Add(1)
		go func() {
			defer wg.Done()
			roundTripAdditional(newreq)
		}()
	}
	setTarget(req, m.targets[0], m.keys[0])
	if body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	res, err := m.rt.RoundTrip(req)
	wg.Wait()
	return res, err
}

// newMeasuringForwardingTransport creates a forwardingTransport wrapped in a measuringTransport.
func newMeasuringForwardingTransport(
	rt http.RoundTripper,
	mainEndpoint *url.URL,
	mainEndpointKey string,
	additionalEndpoints map[string][]string,
	metricPrefix string,
	metricTags []string,
	statsd statsd.ClientInterface,
) http.RoundTripper {
	forwardingTransport := newForwardingTransport(rt, mainEndpoint, mainEndpointKey, additionalEndpoints)
	return newMeasuringTransport(forwardingTransport, metricPrefix, metricTags, statsd)
}
