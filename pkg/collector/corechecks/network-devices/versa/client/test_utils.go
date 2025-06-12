// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client/fixtures"
)

// mockTimeNow mocks time.Now
// var mockTimeNow = func() time.Time {
// 	layout := "2006-01-02 15:04:05"
// 	str := "2000-01-01 00:00:00"
// 	t, _ := time.Parse(layout, str)
// 	return t
// }

// func emptyHandler(w http.ResponseWriter, _ *http.Request) {
// 	w.WriteHeader(http.StatusOK)
// 	w.Write([]byte{})
// }

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
	return NewClient(serverURL(server), "testuser", "testpass", true)
}

//	type handler struct {
//		Func  http.HandlerFunc
//		Calls *atomic.Int32
//	}
func setupCommonServerMux() *http.ServeMux {
	// // Middleware to count the number of calls to a given test endpoint
	// func newHandler(handlerFunc func(w http.ResponseWriter, r *http.Request, called int32)) handler {
	// 	calls := atomic.NewInt32(0)
	// 	return handler{
	// 		Calls: calls,
	// 		Func: func(writer http.ResponseWriter, request *http.Request) {
	// 			calls.Inc()
	// 			handlerFunc(writer, request, calls.Load())
	// 		},
	// 	}
	// }
	// func (h handler) numberOfCalls() int {
	// 	return int(h.Calls.Load())
	// }
	mux := http.NewServeMux()
	return mux
}

// func setupCommonServerMuxWithFixture(path string, payload string) (*http.ServeMux, handler) {
// 	mux := setupCommonServerMux()
// SetupMockAPIServer starts a mock API server
// 	handler := newHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
// 		w.WriteHeader(http.StatusOK)
// 		w.Write([]byte(payload))
// 	})
// 	mux.HandleFunc(path, handler.Func)
// 	return mux, handler
// }

// SLAMetricsURL holds the API endpoint for Versa Analytics SLA metrics
var SLAMetricsURL = "/versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN"

// SetupMockAPIServer starts a mock API server
func SetupMockAPIServer() *httptest.Server {
	mux := setupCommonServerMux()

	mux.HandleFunc("/vnms/organization/orgs", fixtureHandler(fixtures.GetOrganizations))
	//mux.HandleFunc("/vnms/dashboard/childAppliancesDetail/", fixtureHandler(fixtures.GetChildAppliancesDetail))
	mux.HandleFunc("/vnms/dashboard/vdStatus", fixtureHandler(fixtures.GetDirectorStatus))
	mux.HandleFunc(SLAMetricsURL, fixtureHandler(fixtures.GetSLAMetrics))

	return httptest.NewServer(mux)
}
