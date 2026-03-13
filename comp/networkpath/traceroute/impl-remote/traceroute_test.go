// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remoteimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
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

type mockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestGetTraceroute(t *testing.T) {
	hostnameComponent, _ := hostnameinterface.NewMock("test-agent-hostname")

	expectedDest := payload.NetworkPathDestination{
		Hostname: "example.com",
		Port:     80,
	}

	expectedPath := payload.NetworkPath{
		Timestamp:   1234567890,
		Protocol:    payload.ProtocolTCP,
		Destination: expectedDest,
	}

	jsonBytes, err := json.Marshal(expectedPath)
	require.NoError(t, err)

	// Update expectedPath with the expected source hostname
	expectedPath.Source.Hostname = "test-agent-hostname"

	client := &http.Client{
		Transport: &mockTransport{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				assert.Contains(t, req.URL.Path, "/traceroute/example.com")
				assert.Equal(t, clientID, req.URL.Query().Get("client_id"))
				assert.Equal(t, "80", req.URL.Query().Get("port"))
				assert.Equal(t, "TCP", req.URL.Query().Get("protocol"))

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	cfg := config.Config{
		DestHostname: "example.com",
		DestPort:     80,
		Protocol:     payload.ProtocolTCP,
		Timeout:      5 * time.Second,
		MaxTTL:       30,
	}

	rt := &remoteTraceroute{sysprobeClient: client, log: logmock.New(t), hostname: hostnameComponent}
	path, err := rt.Run(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, expectedPath, path)
}

func TestGetTracerouteError(t *testing.T) {
	hostnameComponent, _ := hostnameinterface.NewMock("test-agent-hostname")

	client := &http.Client{
		Transport: &mockTransport{
			RoundTripFunc: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewReader([]byte("internal server error"))),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	cfg := config.Config{
		DestHostname: "example.com",
		DestPort:     80,
		Protocol:     payload.ProtocolTCP,
	}

	rt := &remoteTraceroute{sysprobeClient: client, log: logmock.New(t), hostname: hostnameComponent}
	_, err := rt.Run(context.Background(), cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "traceroute request failed")
}

func TestGetTracerouteInvalidJSON(t *testing.T) {
	hostnameComponent, _ := hostnameinterface.NewMock("test-agent-hostname")

	client := &http.Client{
		Transport: &mockTransport{
			RoundTripFunc: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte("invalid json"))),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	cfg := config.Config{
		DestHostname: "example.com",
		DestPort:     80,
		Protocol:     payload.ProtocolTCP,
	}

	rt := &remoteTraceroute{sysprobeClient: client, log: logmock.New(t), hostname: hostnameComponent}
	_, err := rt.Run(context.Background(), cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshalling response")
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
		disableWindowsDriver      bool
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
			disableWindowsDriver:      false,
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
				"disable_windows_driver":        "false",
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

			hostnameComponent, _ := hostnameinterface.NewMock("test-agent-hostname")
			rt := &remoteTraceroute{sysprobeClient: mockClient, log: logmock.New(t), hostname: hostnameComponent}
			_, err := rt.getTracerouteFromSysProbe(
				context.TODO(),
				tt.clientID,
				tt.host,
				tt.port,
				tt.protocol,
				tt.tcpMethod,
				tt.tcpSynParisTracerouteMode,
				tt.disableWindowsDriver,
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
