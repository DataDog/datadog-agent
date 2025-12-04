// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package traceroute

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

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
)

type mockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestGetTraceroute(t *testing.T) {
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

	client := &http.Client{
		Transport: &mockTransport{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				assert.Contains(t, req.URL.Path, "/traceroute/example.com")
				assert.Equal(t, "client-id", req.URL.Query().Get("client_id"))
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

	path, err := getTraceroute(context.Background(), client, "client-id", cfg)
	require.NoError(t, err)
	assert.Equal(t, expectedPath, path)
}

func TestGetTracerouteError(t *testing.T) {
	// Setup config for hostname
	pkgconfigsetup.Datadog().SetWithoutSource("hostname", "test-agent-hostname")

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

	_, err := getTraceroute(context.Background(), client, "client-id", cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "traceroute request failed")
}

func TestGetTracerouteInvalidJSON(t *testing.T) {
	// Setup config for hostname
	pkgconfigsetup.Datadog().SetWithoutSource("hostname", "test-agent-hostname")

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

	_, err := getTraceroute(context.Background(), client, "client-id", cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshalling response")
}
