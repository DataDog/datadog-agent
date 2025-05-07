// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package client

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client/fixtures"
	"go.uber.org/atomic"
	"net/http"
	"net/http/httptest"
	"strings"
)

// TODO: taken a lot from cisco_sdwan, need to refactor or move to a common package

func emptyHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
}

func tokenHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("testtoken"))
}

func fixtureHandler(payload string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(payload))
	}
}

func serverURL(server *httptest.Server) string {
	return strings.TrimPrefix(server.URL, "http://")
}

func testClient(server *httptest.Server) (*Client, error) {
	return NewClient(serverURL(server), serverURL(server), "testuser", "testpass", true)
}

type handler struct {
	Func  http.HandlerFunc
	Calls *atomic.Int32
}

// Middleware to count the number of calls to a given test endpoint
func newHandler(handlerFunc func(w http.ResponseWriter, r *http.Request, called int32)) handler {
	calls := atomic.NewInt32(0)
	return handler{
		Calls: calls,
		Func: func(writer http.ResponseWriter, request *http.Request) {
			calls.Inc()
			handlerFunc(writer, request, calls.Load())
		},
	}
}

func (h handler) numberOfCalls() int {
	return int(h.Calls.Load())
}

func setupCommonServerMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/versa/j_spring_security_check", emptyHandler)
	mux.HandleFunc("/versa/analytics/login", tokenHandler)
	return mux
}

func setupCommonServerMuxWithFixture(path string, payload string) (*http.ServeMux, handler) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(payload))
	})

	mux.HandleFunc(path, handler.Func)

	return mux, handler
}

// TODO: move to a better location for shared usage
// TODO: figure out how to cleanly handle fixtures that use the same base URL but different params

// SLAMetricsURL holds the API endpoint for Versa Analytics SLA metrics
var SLAMetricsURL = "/versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN"

// SetupMockAPIServer starts a mock API server
func SetupMockAPIServer() *httptest.Server {
	mux := setupCommonServerMux()

	mux.HandleFunc(SLAMetricsURL, fixtureHandler(fixtures.GetSLAMetrics))
	return httptest.NewServer(mux)
}
