// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package intakeroutingimpl

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
)

// newTestClient returns a client that sends every request to srv, whatever its host, so tests can
// use real Datadog hostnames.
func newTestClient(t *testing.T, srv *httptest.Server) *httpRoutingClient {
	addr := srv.Listener.Addr().String()
	tlsConfig := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	// The test certificate is issued for example.com.
	tlsConfig.ServerName = "example.com"
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
		TLSClientConfig: tlsConfig,
	}
	t.Cleanup(transport.CloseIdleConnections)
	return newHTTPRoutingClientWithTransport(transport)
}

// hostOf returns the host a request was sent to, without the trailing dot of FQDN sites.
func hostOf(r *http.Request) string {
	return strings.TrimSuffix(r.Host, ".")
}

func TestQueryRequest(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "api.us3.datadoghq.com", hostOf(r))
		assert.Equal(t, routingPath, r.URL.Path)
		assert.Empty(t, r.URL.RawQuery, "the query does not select telemetry types")
		assert.Equal(t, testKey, r.Header.Get(apiKeyHeader))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.True(t, strings.HasPrefix(r.Header.Get("User-Agent"), "datadog-agent/"))
		_, _ = w.Write([]byte(`{"routes": {"logs": {"override": true, "host": "acme.agent-http-intake.logs.us3.datadoghq.com", "port": 443}}}`))
	}))
	defer srv.Close()

	res := newTestClient(t, srv).query(context.Background(), "us3.datadoghq.com", testKey)
	require.NoError(t, res.err)
	assert.Equal(t, statusSuccess, res.status)
	assert.Equal(t, map[intakerouting.TelemetryType]routeAnswer{
		intakerouting.Logs: {override: true, host: "acme.agent-http-intake.logs.us3.datadoghq.com", port: 443},
	}, res.routes)
}

func TestQueryResponses(t *testing.T) {
	tests := []struct {
		name           string
		code           int
		body           string
		expectedStatus string
		expectedRoutes map[intakerouting.TelemetryType]routeAnswer
	}{
		{
			name:           "no override",
			code:           200,
			body:           `{"routes": {"logs": {"override": false}}}`,
			expectedStatus: statusSuccess,
			expectedRoutes: map[intakerouting.TelemetryType]routeAnswer{intakerouting.Logs: {}},
		},
		{
			name:           "unknown types are kept for the router to ignore",
			code:           200,
			body:           `{"routes": {"future": {"override": true, "host": "x"}}, "extra": 1}`,
			expectedStatus: statusSuccess,
			expectedRoutes: map[intakerouting.TelemetryType]routeAnswer{"future": {override: true, host: "x"}},
		},
		{
			name:           "no entries",
			code:           200,
			body:           `{"routes": {}}`,
			expectedStatus: statusSuccess,
			expectedRoutes: map[intakerouting.TelemetryType]routeAnswer{},
		},
		{name: "not offered", code: 404, expectedStatus: statusNotOffered},
		{name: "unauthorized", code: 401, expectedStatus: statusHTTPError},
		{name: "forbidden", code: 403, expectedStatus: statusHTTPError},
		{name: "server error", code: 503, expectedStatus: statusHTTPError},
		{name: "no content", code: 204, expectedStatus: statusInvalidResponse},
		{name: "not JSON", code: 200, body: `not json`, expectedStatus: statusInvalidResponse},
		{name: "null", code: 200, body: `null`, expectedStatus: statusInvalidResponse},
		{name: "no routes", code: 200, body: `{}`, expectedStatus: statusInvalidResponse},
		{name: "wrong port type", code: 200, body: `{"routes": {"logs": {"override": true, "host": "x", "port": "443"}}}`, expectedStatus: statusInvalidResponse},
		{name: "too large", code: 200, body: `{"routes": {}}` + strings.Repeat(" ", maxResponseBytes), expectedStatus: statusInvalidResponse},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			res := newTestClient(t, srv).query(context.Background(), "datadoghq.com", testKey)
			assert.Equal(t, tc.expectedStatus, res.status)
			assert.Equal(t, tc.code, res.code)
			assert.Equal(t, tc.expectedRoutes, res.routes)
		})
	}
}

func TestQueryTransportErrors(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		defer srv.Close()
		defer close(release)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		res := newTestClient(t, srv).query(ctx, "datadoghq.com", testKey)
		assert.Equal(t, statusTimeout, res.status)
	})

	t.Run("network error", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.NotFoundHandler())
		client := newTestClient(t, srv)
		srv.Close()

		res := client.query(context.Background(), "datadoghq.com", testKey)
		assert.Equal(t, statusNetworkError, res.status)
		assert.Error(t, res.err)
	})
}

func TestQueryRedirects(t *testing.T) {
	const answer = `{"routes": {}}`
	tests := []struct {
		name           string
		location       string
		expectedStatus string
		expectedHosts  []string
	}{
		{
			name:           "to an official Datadog host",
			location:       "https://api.us5.datadoghq.com/final",
			expectedStatus: statusSuccess,
			expectedHosts:  []string{"api.datadoghq.com", "api.us5.datadoghq.com"},
		},
		{
			name:           "to another domain",
			location:       "https://api.example.com/final",
			expectedStatus: statusHTTPError,
			expectedHosts:  []string{"api.datadoghq.com"},
		},
		{
			name:           "to plain HTTP",
			location:       "http://api.us5.datadoghq.com/final",
			expectedStatus: statusHTTPError,
			expectedHosts:  []string{"api.datadoghq.com"},
		},
		{
			name:           "too many",
			location:       "https://api.us5.datadoghq.com/loop",
			expectedStatus: statusHTTPError,
			expectedHosts:  []string{"api.datadoghq.com", "api.us5.datadoghq.com", "api.us5.datadoghq.com", "api.us5.datadoghq.com"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var hosts []string
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				hosts = append(hosts, hostOf(r))
				mu.Unlock()
				assert.Equal(t, testKey, r.Header.Get(apiKeyHeader))
				switch r.URL.Path {
				case "/final":
					_, _ = w.Write([]byte(answer))
				case "/loop":
					http.Redirect(w, r, "https://api.us5.datadoghq.com/loop", http.StatusFound)
				default:
					http.Redirect(w, r, tc.location, http.StatusFound)
				}
			}))
			defer srv.Close()

			res := newTestClient(t, srv).query(context.Background(), "datadoghq.com", testKey)
			assert.Equal(t, tc.expectedStatus, res.status, "%v", res.err)
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, tc.expectedHosts, hosts)
		})
	}
}

func TestPreflightRequest(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     intakerouting.Endpoint
		code         int
		expectedHost string
		expectError  bool
	}{
		{"success", enhancedEndpoint, 200, enhancedHost, false},
		{"accepted", enhancedEndpoint, 202, enhancedHost, false},
		{"custom port", intakerouting.Endpoint{Host: enhancedHost, Port: 8443}, 200, enhancedHost + ":8443", false},
		{"failure", enhancedEndpoint, 503, enhancedHost, true},
		{"forbidden", enhancedEndpoint, 403, enhancedHost, true},
		{"redirect is refused", enhancedEndpoint, 302, enhancedHost, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var hits []string
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits = append(hits, r.URL.Path)
				assert.Equal(t, tc.expectedHost, r.Host)
				assert.Equal(t, testKey, r.Header.Get(apiKeyHeader))
				if tc.code == http.StatusFound {
					http.Redirect(w, r, "https://"+otherEnhancedHost+"/probe", tc.code)
					return
				}
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()

			err := newTestClient(t, srv).preflight(context.Background(), tc.endpoint, testKey)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, []string{preflightPath}, hits)
		})
	}
}
