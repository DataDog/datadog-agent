// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package traceroute

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

type mockRoundTripper struct {
	statusCode      int
	capturedRequest *http.Request
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.capturedRequest = req

	return &http.Response{
		StatusCode: m.statusCode,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

func TestGetTracerouteURL(t *testing.T) {
	tests := []struct {
		name                      string
		host                      string
		clientID                  string
		port                      uint16
		protocol                  payload.Protocol
		tcpMethod                 payload.TCPMethod
		tcpSynParisTracerouteMode bool
		reverseDNS                bool
		maxTTL                    uint8
		timeout                   time.Duration
		tracerouteQueries         int
		e2eQueries                int
		expectedParams            map[string]string
	}{
		{
			name:                      "validate URL",
			host:                      "google.com",
			clientID:                  "test-client",
			port:                      80,
			protocol:                  payload.ProtocolTCP,
			tcpMethod:                 payload.TCPConfigPreferSACK,
			tcpSynParisTracerouteMode: true,
			reverseDNS:                true,
			maxTTL:                    30,
			timeout:                   5 * time.Second,
			tracerouteQueries:         3,
			e2eQueries:                50,
			expectedParams: map[string]string{
				"client_id":                     "test-client",
				"port":                          "80",
				"max_ttl":                       "30",
				"timeout":                       "5000000000",
				"protocol":                      "TCP",
				"tcp_method":                    "prefer_sack",
				"tcp_syn_paris_traceroute_mode": "true",
				"reverse_dns":                   "true",
				"traceroute_queries":            "3",
				"e2e_queries":                   "50",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock HTTP client that captures the request
			mockTransport := &mockRoundTripper{
				statusCode: http.StatusOK,
			}
			mockClient := &http.Client{
				Transport: mockTransport,
			}

			_, err := getTraceroute(
				mockClient,
				tt.clientID,
				tt.host,
				tt.port,
				tt.protocol,
				tt.tcpMethod,
				tt.tcpSynParisTracerouteMode,
				tt.reverseDNS,
				tt.maxTTL,
				tt.timeout,
				tt.tracerouteQueries,
				tt.e2eQueries,
			)

			require.NoError(t, err)
			require.NotNil(t, mockTransport.capturedRequest, "HTTP request should have been captured")

			capturedURL := mockTransport.capturedRequest.URL
			require.NotNil(t, capturedURL, "Captured request should have a URL")

			expectedPathPrefix := "/traceroute/" + tt.host
			assert.Contains(t, capturedURL.Path, expectedPathPrefix, "URL path should contain the host")
			assert.Equal(t, "GET", mockTransport.capturedRequest.Method, "Should use GET method")
			assert.Equal(t, "application/json", mockTransport.capturedRequest.Header.Get("Accept"), "Should set Accept header")

			query := capturedURL.Query()

			expectedParamCount := len(tt.expectedParams)
			actualParamCount := len(query)
			assert.Equal(t, expectedParamCount, actualParamCount, "Should have exactly %d query parameters", expectedParamCount)

			for key, expectedValue := range tt.expectedParams {
				actualValue := query.Get(key)
				assert.Equal(t, expectedValue, actualValue, "Query parameter %s should match expected value", key)
			}
		})
	}
}
