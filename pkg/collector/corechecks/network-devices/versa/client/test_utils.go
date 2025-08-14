// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func serverURL(server *httptest.Server) (string, string, error) {
	splitURL := strings.Split(strings.TrimPrefix(server.URL, "http://"), ":")
	if len(splitURL) < 2 {
		return "", "", fmt.Errorf("failed to parse test server URL: %s", server.URL)
	}
	return splitURL[0], splitURL[1], nil
}

func testClient(server *httptest.Server) (*Client, error) {
	host, portStr, err := serverURL(server)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(host, port, "https://10.0.0.1:8443", "testuser", "testpass", true)
	if err != nil {
		return nil, err
	}
	client.directorAPIPort = port
	return client, err
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

	// mock session auth
	mux.HandleFunc("/versa/analytics/auth/user", fixtureHandler("{}"))
	mux.HandleFunc("/versa/j_spring_security_check", fixtureHandler("{}"))
	mux.HandleFunc("/versa/analytics/login", fixtureHandler("{}"))

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

// AnalyticsSDWANMetricsURL holds the API endpoint for Versa Analytics SDWAN metrics
var AnalyticsSDWANMetricsURL = "/versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN"

// AnalyticsSystemMetricsURL holds the API endpoint for Versa Analytics System metrics
var AnalyticsSystemMetricsURL = "/versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SYSTEM"

// SetupMockAPIServer starts a mock API server
func SetupMockAPIServer() *httptest.Server {
	mux := setupCommonServerMux()

	mux.HandleFunc("/vnms/organization/orgs", fixtureHandler(fixtures.GetOrganizations))
	//mux.HandleFunc("/vnms/dashboard/childAppliancesDetail/", fixtureHandler(fixtures.GetChildAppliancesDetail))
	mux.HandleFunc("/vnms/dashboard/vdStatus", fixtureHandler(fixtures.GetDirectorStatus))
	mux.HandleFunc(AnalyticsSDWANMetricsURL, func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a SLA metrics request or Link Extended metrics request
		queryParams := r.URL.Query()
		query := queryParams.Get("q")
		if strings.Contains(query, "slam(") {
			fixtureHandler(fixtures.GetSLAMetrics)(w, r)
		} else if strings.Contains(query, "linkusage(site,site.address") {
			fixtureHandler(fixtures.GetSiteMetrics)(w, r)
		} else if strings.Contains(query, "linkusage(") {
			fixtureHandler(fixtures.GetLinkUsageMetrics)(w, r)
		} else if strings.Contains(query, "linkstatus(") {
			fixtureHandler(fixtures.GetLinkStatusMetrics)(w, r)
		} else if strings.Contains(query, "app(") {
			fixtureHandler(fixtures.GetApplicationsByApplianceMetrics)(w, r)
		} else if strings.Contains(query, "appUser(") {
			fixtureHandler(fixtures.GetTopUsers)(w, r)
		} else if strings.Contains(query, "cos(") {
			fixtureHandler(fixtures.GetPathQoSMetrics)(w, r)
		} else if strings.Contains(query, "usage(") {
			fixtureHandler(fixtures.GetDIAMetrics)(w, r)
		} else {
			http.Error(w, "Unknown query type", http.StatusBadRequest)
		}
	})

	// Handle tunnel metrics from SYSTEM feature
	mux.HandleFunc(AnalyticsSystemMetricsURL, func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		query := queryParams.Get("q")
		if strings.Contains(query, "tunnelstats(") {
			fixtureHandler(fixtures.GetTunnelMetrics)(w, r)
		} else {
			http.Error(w, "Unknown query type", http.StatusBadRequest)
		}
	})

	return httptest.NewServer(mux)
}

// SetupPaginationMockAPIServer creates a mock server that handles pagination for analytics endpoints
func SetupPaginationMockAPIServer() *httptest.Server {
	mux := setupCommonServerMux()

	// Setup pagination-aware analytics endpoint
	mux.HandleFunc(AnalyticsSDWANMetricsURL, func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		query := queryParams.Get("q")
		fromCount := queryParams.Get("from-count")
		count := queryParams.Get("count")

		// Only handle SLA metrics pagination for now - can be extended for other types
		if strings.Contains(query, "slam(") {
			// Parse pagination parameters
			fromCountInt := 0
			if fromCount != "" {
				if parsed, err := strconv.Atoi(fromCount); err == nil {
					fromCountInt = parsed
				}
			}

			countInt := 2000 // default
			if count != "" {
				if parsed, err := strconv.Atoi(count); err == nil {
					countInt = parsed
				}
			}

			// Return different pages based on from-count and count
			if fromCountInt == 0 {
				if countInt >= 3 {
					// Large page size - return all data in one page by combining fixtures
					combinedData, err := combineAnalyticsFixtures(fixtures.GetSLAMetricsPage1, fixtures.GetSLAMetricsPage2)
					if err != nil {
						http.Error(w, fmt.Sprintf("Failed to combine fixtures: %v", err), http.StatusInternalServerError)
						return
					}
					fixtureHandler(combinedData)(w, r)
				} else {
					// Small page size - return first page (2 items)
					fixtureHandler(fixtures.GetSLAMetricsPage1)(w, r)
				}
			} else if fromCountInt == 2 && countInt < 3 {
				// Second page for small page size (1 item)
				fixtureHandler(fixtures.GetSLAMetricsPage2)(w, r)
			} else {
				// No more data - return empty page
				emptyData, _ := combineAnalyticsFixtures() // Empty fixtures list returns empty response
				fixtureHandler(emptyData)(w, r)
			}
		} else {
			// For other query types, use default fixtures
			if strings.Contains(query, "linkusage(site,site.address") {
				fixtureHandler(fixtures.GetSiteMetrics)(w, r)
			} else if strings.Contains(query, "linkusage(") {
				fixtureHandler(fixtures.GetLinkUsageMetrics)(w, r)
			} else if strings.Contains(query, "linkstatus(") {
				fixtureHandler(fixtures.GetLinkStatusMetrics)(w, r)
			} else if strings.Contains(query, "app(") {
				fixtureHandler(fixtures.GetApplicationsByApplianceMetrics)(w, r)
			} else if strings.Contains(query, "appUser(") {
				fixtureHandler(fixtures.GetTopUsers)(w, r)
			} else if strings.Contains(query, "cos(") {
				fixtureHandler(fixtures.GetPathQoSMetrics)(w, r)
			} else if strings.Contains(query, "usage(") {
				fixtureHandler(fixtures.GetDIAMetrics)(w, r)
			} else {
				http.Error(w, "Unknown query type", http.StatusBadRequest)
			}
		}
	})

	server := httptest.NewServer(mux)
	return server
}

// combineAnalyticsFixtures combines multiple analytics fixture responses into a single response
func combineAnalyticsFixtures(fixtures ...string) (string, error) {
	if len(fixtures) == 0 {
		return `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 0, "iTotalRecords": 0, "aaData": []}`, nil
	}

	var combinedAaData [][]interface{}
	var totalRecords int

	for _, fixtureData := range fixtures {
		var response struct {
			AaData [][]interface{} `json:"aaData"`
		}

		if err := json.Unmarshal([]byte(fixtureData), &response); err != nil {
			return "", fmt.Errorf("failed to parse fixture: %v", err)
		}

		combinedAaData = append(combinedAaData, response.AaData...)
		totalRecords += len(response.AaData)
	}

	combinedResponse := struct {
		QTime                int             `json:"qTime"`
		SEcho                int             `json:"sEcho"`
		ITotalDisplayRecords int             `json:"iTotalDisplayRecords"`
		ITotalRecords        int             `json:"iTotalRecords"`
		AaData               [][]interface{} `json:"aaData"`
	}{
		QTime:                1,
		SEcho:                0,
		ITotalDisplayRecords: totalRecords,
		ITotalRecords:        totalRecords,
		AaData:               combinedAaData,
	}

	combinedBytes, err := json.Marshal(combinedResponse)
	if err != nil {
		return "", fmt.Errorf("failed to marshal combined response: %v", err)
	}

	return string(combinedBytes), nil
}
