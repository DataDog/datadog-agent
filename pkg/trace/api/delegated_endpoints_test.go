// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestProxyDelegatedEndpointUpdates(t *testing.T) {
	for _, setting := range []string{
		"apm_config.debugger_additional_endpoints",
		"apm_config.debugger_diagnostics_additional_endpoints",
		"apm_config.symdb_additional_endpoints",
		"apm_config.additional_endpoints",
	} {
		for _, initiallyResolved := range []bool{false, true} {
			name := setting + "/pending"
			if initiallyResolved {
				name = setting + "/resolved_before_startup"
			}
			t.Run(name, func(t *testing.T) {
				var mu sync.Mutex
				var received []string
				intake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					key := req.Header.Get("DD-API-KEY")
					assert.NotEmpty(t, key)
					assert.NotContains(t, key, "DELA[")
					mu.Lock()
					received = append(received, key)
					mu.Unlock()
					_, _ = io.Copy(io.Discard, req.Body)
					if key == "main-key" {
						w.WriteHeader(http.StatusOK)
					} else {
						w.WriteHeader(http.StatusCreated)
					}
				}))
				defer intake.Close()
				conf := newTestReceiverConfigNoPort()
				conf.Endpoints[0] = &config.Endpoint{Host: "https://main.datadoghq.com", APIKey: "main-key"}
				conf.Site = "datadoghq.com"
				conf.DebuggerLogsEnabled = true
				transport := intake.Client().Transport.(*http.Transport).Clone()
				transport.TLSClientConfig.ServerName = "example.com"
				transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, intake.Listener.Addr().String())
				}
				defer transport.CloseIdleConnections()
				conf.HTTPTransportFunc = func() *http.Transport { return transport }
				host := "https://secondary.datadoghq.eu"
				initial := []string{"DELA[org-a, aws]", "static-key", "DELA[org-b, aws]"}
				if initiallyResolved {
					initial = []string{"initial-a", "static-key", "initial-b"}
				}
				var state atomic.Value
				state.Store(initial)
				conf.ReadAdditionalEndpointKey = func(gotSetting, gotHost string, index int) (string, bool) {
					assert.Equal(t, setting, gotSetting)
					assert.Equal(t, host, gotHost)
					keys := state.Load().([]string)
					if index >= len(keys) || keys[index] == "" || strings.HasPrefix(keys[index], "DELA[") {
						return "", false
					}
					return keys[index], true
				}
				extra := map[string][]string{host: initial}
				receiver := newTestReceiverFromConfig(conf)
				var handler http.Handler
				switch setting {
				case "apm_config.debugger_additional_endpoints":
					conf.DebuggerProxy.AdditionalEndpoints = extra
					handler = receiver.debuggerLogsProxyHandler()
				case "apm_config.debugger_diagnostics_additional_endpoints":
					conf.DebuggerIntakeProxy.AdditionalEndpoints = extra
					handler = receiver.debuggerDiagnosticsProxyHandler()
				case "apm_config.symdb_additional_endpoints":
					conf.SymDBProxy.AdditionalEndpoints = extra
					handler = receiver.symDBProxyHandler()
				case "apm_config.additional_endpoints":
					for i, key := range initial {
						conf.Endpoints = append(conf.Endpoints, conf.AdditionalEndpoint(setting, host, key, i))
					}
					handler = receiver.pipelineStatsProxyHandler()
				}
				send := func(expected []string) {
					mu.Lock()
					received = nil
					mu.Unlock()
					req := httptest.NewRequest(http.MethodPost, "http://agent/evp_proxy/v2/test?foo=bar", strings.NewReader("payload"))
					req.Header.Set("X-Datadog-EVP-Subdomain", "citestcycle-intake")
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, req)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					mu.Lock()
					defer mu.Unlock()
					assert.ElementsMatch(t, expected, received)
				}
				want := []string{"main-key", "static-key"}
				if initiallyResolved {
					want = append(want, "initial-a", "initial-b")
				}
				send(want)
				state.Store([]string{"resolved-a", "static-key", "resolved-b"})
				send([]string{"main-key", "static-key", "resolved-a", "resolved-b"})
				state.Store([]string{"rotated-a", "static-key", "rotated-b"})
				send([]string{"main-key", "static-key", "rotated-a", "rotated-b"})
				state.Store([]string{})
				send([]string{"main-key"})
			})
		}
	}
}

func TestTelemetryProxyDelegatedEndpointUpdates(t *testing.T) {
	var mu sync.Mutex
	var received []string
	intake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		received = append(received, req.Header.Get("DD-API-KEY"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer intake.Close()
	conf := getTestConfig(intake.URL)
	var key atomic.Value
	key.Store("")
	conf.ReadAdditionalEndpointKey = func(setting, host string, index int) (string, bool) {
		assert.Equal(t, "apm_config.telemetry.additional_endpoints", setting)
		assert.Equal(t, intake.URL, host, "transport host rewriting must not affect lookup")
		assert.Zero(t, index)
		value := key.Load().(string)
		return value, value != ""
	}
	conf.TelemetryConfig.Endpoints = append(conf.TelemetryConfig.Endpoints,
		conf.AdditionalEndpoint("apm_config.telemetry.additional_endpoints", intake.URL, "DELA[org-a, aws]", 0),
		&config.Endpoint{Host: intake.URL, APIKey: "\tDELA[org-b, aws] "})
	forwarder := NewTelemetryForwarder(conf, NewContainerIDProviderFromConfig(conf), &statsd.NoOpClient{})
	defer forwarder.Stop()
	assert.Equal(t, intake.URL, conf.TelemetryConfig.Endpoints[0].Host, "constructor must preserve the shared config URL")
	for _, value := range []string{"", "resolved-key", "rotated-key"} {
		key.Store(value)
		mu.Lock()
		received = nil
		mu.Unlock()
		req := httptest.NewRequest(http.MethodPost, "http://agent/path", nil)
		req.RequestURI = ""
		forwarder.forwardTelemetry(forwardedRequest{req: req, body: []byte(`{}`)})
		want := []string{"test_apikey"}
		if value != "" {
			want = append(want, value)
		}
		mu.Lock()
		assert.Equal(t, want, received)
		mu.Unlock()
	}
}
