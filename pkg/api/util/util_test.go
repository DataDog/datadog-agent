// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"crypto/tls"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput bool
	}{
		{
			name:           "IPv4",
			input:          "192.168.0.1",
			expectedOutput: false,
		},
		{
			name:           "IPv6",
			input:          "2600:1f19:35d4:b900:527a:764f:e391:d369",
			expectedOutput: true,
		},
		{
			name:           "zero compressed IPv6",
			input:          "2600:1f19:35d4:b900::1",
			expectedOutput: true,
		},
		{
			name:           "IPv6 loopback",
			input:          "::1",
			expectedOutput: true,
		},
		{
			name:           "short hostname with only hexadecimal digits",
			input:          "cafe",
			expectedOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, IsIPv6(tt.input), tt.expectedOutput)
		})
	}
}

func TestStartingServerClientWithUninitializedTLS(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	tlsListener := tls.NewListener(l, GetTLSServerConfig())

	go server.Serve(tlsListener) //nolint:errcheck
	defer server.Close()

	// create a http client with the provided tls client config
	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: GetTLSClientConfig(),
		},
	}

	// make a request to the server
	resp, err := client.Get("https://localhost:" + port)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
