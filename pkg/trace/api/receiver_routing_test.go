// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	traceconfigimpl "github.com/DataDog/datadog-agent/comp/trace/config/impl"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/require"
)

// These core fixtures are checked against the renderer by the framework's
// TestReceiverRoutingTraceFixtureConformance. Keep this Go test module independent
// of the E2E framework: only the fixture data crosses that boundary.
//
//go:embed receiver_routing_http.yaml
var receiverRoutingHTTP string

//go:embed receiver_routing_https.yaml
var receiverRoutingHTTPS string

const receiverRoutingDummyKey = "00000000000000000000000000000000"

// Exercise the real trace loader, handlers and URL-replacing transports using
// recording fakes. No external network or E2E framework imports are used here.
func loadReceiverRoutingTraceConfig(t *testing.T, endpoint string) *traceconfig.AgentConfig {
	t.Helper()
	raw := receiverRoutingHTTP
	if strings.HasPrefix(endpoint, "https:") {
		raw = receiverRoutingHTTPS
	}
	core := configcomp.NewMockFromYAML(t, raw)
	cfg, err := traceconfigimpl.LoadConfigFile("", core, taggerfxmock.SetupFakeTagger(t), ipcmock.New(t))
	require.NoError(t, err)
	require.True(t, cfg.Enabled, "routing must not disable trace collection")
	require.Equal(t, endpoint, cfg.Endpoints[0].Host)
	return cfg
}

func TestReceiverRoutingDisablesUnsupportedAPMProxies(t *testing.T) {
	cfg := loadReceiverRoutingTraceConfig(t, "http://receiver.invalid:8080")
	require.False(t, cfg.EVPProxy.Enabled)
	require.False(t, cfg.OpenLineageProxy.Enabled)
	// A leak into native handler construction or sending must fail even if it
	// happens to return the same HTTP error status as the disabled handler.
	transports, dials := 0, 0
	cfg.HTTPTransportFunc = func() *http.Transport {
		transports++
		return &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
			dials++
			return nil, errors.New("network forbidden in receiver test")
		}}
	}
	receiver := newTestReceiverFromConfig(cfg)
	// NewHTTPReceiver constructs the supported instrumentation-telemetry client.
	// Only subsequent EVP/OpenLineage handler construction is under test here.
	transports = 0
	for _, tc := range []struct {
		handler http.Handler
		path    string
		status  int
	}{
		{receiver.evpProxyHandler(2), "/evp_proxy/v2/api/v2/citestcycle", http.StatusMethodNotAllowed},
		{receiver.openLineageProxyHandler(), "/openlineage/api/v1/lineage", http.StatusInternalServerError},
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader("dummy event"))
		req.Header.Set("X-Datadog-EVP-Subdomain", "citestcycle-intake")
		tc.handler.ServeHTTP(rr, req)
		require.Equal(t, tc.status, rr.Code)
	}
	require.Zero(t, transports, "disabled proxies must not construct native transports")
	require.Zero(t, dials)
}

func TestReceiverRoutingAPMProxyRequestPaths(t *testing.T) {
	for _, origin := range []string{"http://receiver.invalid:8080", "https://receiver.invalid:8443"} {
		t.Run(origin, func(t *testing.T) {
			cfg := loadReceiverRoutingTraceConfig(t, origin)
			cfg.HTTPTransportFunc = func() *http.Transport {
				return &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
					return nil, errors.New("network forbidden in receiver test")
				}}
			}
			receiver := newTestReceiverFromConfig(cfg)
			for _, tc := range []struct {
				name    string
				handler http.Handler
				path    string
			}{
				{"profiling", receiver.profileProxyHandler(), traceconfig.ProfilingEndpointPath},
				{"debugger logs", receiver.debuggerLogsProxyHandler(), traceconfig.DebuggerLogsEndpointPath},
				{"debugger diagnostics", receiver.debuggerDiagnosticsProxyHandler(), traceconfig.DebuggerIntakeEndpointPath},
				{"symbol database", receiver.symDBProxyHandler(), traceconfig.DebuggerIntakeEndpointPath},
			} {
				t.Run(tc.name, func(t *testing.T) {
					calls := 0
					recorder := roundTripperMock(func(req *http.Request) (*http.Response, error) {
						calls++
						expected, err := url.Parse(origin)
						require.NoError(t, err)
						require.Equal(t, expected.Scheme, req.URL.Scheme)
						require.Equal(t, expected.Host, req.URL.Host)
						require.Equal(t, tc.path, req.URL.Path)
						require.Equal(t, receiverRoutingDummyKey, req.Header.Get("DD-API-KEY"))
						return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
					})
					proxy, ok := tc.handler.(*httputil.ReverseProxy)
					require.True(t, ok)
					switch transport := proxy.Transport.(type) {
					case *multiTransport:
						transport.rt = recorder
					case *measuringTransport:
						forwarding, ok := transport.rt.(*forwardingTransport)
						require.True(t, ok)
						forwarding.rt = recorder
					default:
						t.Fatalf("unexpected proxy transport %T", transport)
					}
					rr := httptest.NewRecorder()
					tc.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/incoming/proxy/path", strings.NewReader("dummy payload")))
					require.Equal(t, http.StatusOK, rr.Code)
					require.Equal(t, 1, calls)
				})
			}
		})
	}
}

// Manual/instruction-triggered diagnostics are explicitly outside the initial
// telemetry-routing contract. This test documents that exclusion without ever
// contacting the native backend or weakening telemetry forwarding assertions.
func TestReceiverRoutingTracerFlareDiagnosticExclusion(t *testing.T) {
	cfg := loadReceiverRoutingTraceConfig(t, "http://receiver.invalid:8080")
	receiver := newTestReceiverFromConfig(cfg)
	handler := receiver.tracerFlareHandler().(*httputil.ReverseProxy)
	transport := handler.Transport.(*tracerFlareTransport)
	calls := 0
	transport.rt = roundTripperMock(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "https", req.URL.Scheme)
		require.True(t, strings.HasSuffix(req.URL.Host, ".datadoghq.com"))
		require.NotEqual(t, "receiver.invalid:8080", req.URL.Host)
		require.Equal(t, serverlessFlareEndpointPath, req.URL.Path)
		require.Equal(t, receiverRoutingDummyKey, req.Header.Get("DD-API-KEY"))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tracer_flare/v1", strings.NewReader("dummy diagnostic")))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, 1, calls)
}
