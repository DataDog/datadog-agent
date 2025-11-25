// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMetric is a simple test type for testing getPaginatedAnalytics
type TestMetric struct {
	ID    string  `json:"id"`
	Value float64 `json:"value"`
}

// parseTestMetrics parses test data for getPaginatedAnalytics tests
func parseTestMetrics(data [][]interface{}) ([]TestMetric, error) {
	var metrics []TestMetric
	for _, row := range data {
		if len(row) != 2 {
			return nil, fmt.Errorf("expected 2 columns, got %d", len(row))
		}

		id, ok := row[0].(string)
		if !ok {
			return nil, errors.New("expected string for ID")
		}

		value, ok := row[1].(float64)
		if !ok {
			return nil, errors.New("expected float64 for Value")
		}

		metrics = append(metrics, TestMetric{ID: id, Value: value})
	}
	return metrics, nil
}

func TestGetPaginatedAnalytics(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *httptest.Server
		setupClient    func(*Client)
		expectedResult func(*testing.T, []TestMetric, error)
	}{
		{
			name: "happy_path_pagination",
			setupServer: func() *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/data/provider/tenants/test/features/TEST", func(w http.ResponseWriter, r *http.Request) {
					queryParams := r.URL.Query()
					fromCount := queryParams.Get("from-count")

					fromCountInt := 0
					if fromCount != "" {
						if parsed, err := strconv.Atoi(fromCount); err == nil {
							fromCountInt = parsed
						}
					}

					var response string
					if fromCountInt == 0 {
						// First page - return 2 items
						response = `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 3, "iTotalRecords": 3, "aaData": [["item1", 10.5], ["item2", 20.5]]}`
					} else if fromCountInt == 2 {
						// Second page - return 1 item (less than maxCount, so should stop)
						response = `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 1, "iTotalRecords": 3, "aaData": [["item3", 30.5]]}`
					} else {
						// No more data
						response = `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 0, "iTotalRecords": 3, "aaData": []}`
					}

					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response))
				})

				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.maxCount = "2"
				client.maxPages = 5
			},
			expectedResult: func(t *testing.T, result []TestMetric, err error) {
				require.NoError(t, err)
				require.Len(t, result, 3)
				require.Equal(t, "item1", result[0].ID)
				require.Equal(t, 10.5, result[0].Value)
				require.Equal(t, "item2", result[1].ID)
				require.Equal(t, 20.5, result[1].Value)
				require.Equal(t, "item3", result[2].ID)
				require.Equal(t, 30.5, result[2].Value)
			},
		},
		{
			name: "empty_response",
			setupServer: func() *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/data/provider/tenants/test/features/TEST", func(w http.ResponseWriter, _ *http.Request) {
					response := `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 0, "iTotalRecords": 0, "aaData": []}`
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response))
				})

				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.maxCount = "2"
				client.maxPages = 5
			},
			expectedResult: func(t *testing.T, result []TestMetric, err error) {
				require.NoError(t, err)
				require.Len(t, result, 0)
			},
		},
		{
			name: "single_page_response",
			setupServer: func() *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/data/provider/tenants/test/features/TEST", func(w http.ResponseWriter, _ *http.Request) {
					// Return less data than the page size
					response := `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 1, "iTotalRecords": 1, "aaData": [["single_item", 100.5]]}`
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response))
				})

				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.maxCount = "10" // Large page size
				client.maxPages = 5
			},
			expectedResult: func(t *testing.T, result []TestMetric, err error) {
				require.NoError(t, err)
				require.Len(t, result, 1)
				require.Equal(t, "single_item", result[0].ID)
				require.Equal(t, 100.5, result[0].Value)
			},
		},
		{
			name: "max_pages_limit",
			setupServer: func() *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/data/provider/tenants/test/features/TEST", func(w http.ResponseWriter, r *http.Request) {
					queryParams := r.URL.Query()
					fromCount := queryParams.Get("from-count")

					fromCountInt := 0
					if fromCount != "" {
						if parsed, err := strconv.Atoi(fromCount); err == nil {
							fromCountInt = parsed
						}
					}

					// Always return full pages to test maxPages limit
					var response string
					if fromCountInt == 0 {
						response = `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 2, "iTotalRecords": 1000, "aaData": [["page1_item1", 10.5], ["page1_item2", 20.5]]}`
					} else if fromCountInt == 2 {
						response = `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 2, "iTotalRecords": 1000, "aaData": [["page2_item1", 30.5], ["page2_item2", 40.5]]}`
					} else {
						response = `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 2, "iTotalRecords": 1000, "aaData": [["page3_item1", 50.5], ["page3_item2", 60.5]]}`
					}

					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response))
				})

				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.maxCount = "2"
				client.maxPages = 2 // Limit to 2 pages
			},
			expectedResult: func(t *testing.T, result []TestMetric, err error) {
				require.NoError(t, err)
				require.Len(t, result, 4) // Should only get 2 pages * 2 items = 4 items
				require.Equal(t, "page1_item1", result[0].ID)
				require.Equal(t, "page1_item2", result[1].ID)
				require.Equal(t, "page2_item1", result[2].ID)
				require.Equal(t, "page2_item2", result[3].ID)
			},
		},
		{
			name: "api_error",
			setupServer: func() *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/data/provider/tenants/test/features/TEST", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Internal Server Error"))
				})

				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.maxCount = "2"
				client.maxPages = 5
			},
			expectedResult: func(t *testing.T, result []TestMetric, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "failed to get analytics metrics page 1")
				require.Nil(t, result)
			},
		},
		{
			name: "parser_error",
			setupServer: func() *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/data/provider/tenants/test/features/TEST", func(w http.ResponseWriter, _ *http.Request) {
					// Return data with wrong number of columns for our test parser
					response := `{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": 1, "iTotalRecords": 1, "aaData": [["item1"]]}`
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response))
				})

				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.maxCount = "2"
				client.maxPages = 5
			},
			expectedResult: func(t *testing.T, result []TestMetric, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "failed to parse analytics metrics page 1")
				require.Contains(t, err.Error(), "expected 2 columns, got 1")
				require.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client, err := testClient(server)
			require.NoError(t, err)

			tt.setupClient(client)
			client.directorEndpoint = server.URL

			result, err := getPaginatedAnalytics(
				client,
				"test",
				"TEST",
				"30minutesAgo",
				"test(id,value)",
				"",
				"",
				[]string{"metric1", "metric2"},
				parseTestMetrics,
			)

			tt.expectedResult(t, result, err)
		})
	}
}
