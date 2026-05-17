// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

func TestURLAllowlistClient(t *testing.T) {
	tests := []struct {
		name          string
		allowlist     []string
		requestURL    string
		expectAllowed bool
	}{
		{
			name:          "blocks disallowed URL",
			allowlist:     []string{"allowed.example.com"},
			requestURL:    "http://blocked.example.com/test",
			expectAllowed: false,
		},
		{
			name:          "allows matching URL",
			allowlist:     []string{"allowed.example.com"},
			requestURL:    "http://allowed.example.com/test",
			expectAllowed: true,
		},
		{
			name:          "allows all when no allowlist configured",
			allowlist:     nil,
			requestURL:    "http://any-url.example.com/test",
			expectAllowed: true,
		},
		{
			name:          "blocks all when allowlist is empty",
			allowlist:     []string{},
			requestURL:    "http://any-url.example.com/test",
			expectAllowed: false,
		},
		{
			name:          "allows URL matching one of multiple allowlist entries",
			allowlist:     []string{"first.example.com", "second.example.com"},
			requestURL:    "http://second.example.com/path",
			expectAllowed: true,
		},
		{
			name:          "blocks URL not in multi-entry allowlist",
			allowlist:     []string{"first.example.com", "second.example.com"},
			requestURL:    "http://third.example.com/path",
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Allowlist: tt.allowlist}
			inner := &fakeHTTPClient{response: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}}
			client := &urlAllowlistClient{inner: inner, config: cfg}

			req, _ := http.NewRequest("GET", tt.requestURL, nil)
			resp, err := client.Do(req)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}

			if tt.expectAllowed {
				require.NoError(t, err)
				assert.Equal(t, 200, resp.StatusCode)
				assert.True(t, inner.called, "inner client should be called for allowed URLs")
			} else {
				assert.Nil(t, resp)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "request url is not allowed by runner policy")
				assert.False(t, inner.called, "inner client should not be called for blocked URLs")
			}
		})
	}
}

func TestNewDefaultProvider_EnforcesAllowlistByDefault(t *testing.T) {
	cfg := &config.Config{Allowlist: []string{"allowed.example.com"}}
	provider := NewDefaultProvider(cfg)

	client, err := provider.NewDefaultClient()
	require.NoError(t, err)

	_, isWrapped := client.(*urlAllowlistClient)
	assert.True(t, isWrapped, "client should be wrapped with urlAllowlistClient by default")
}

func TestNewDefaultProvider_WithURLAllowlistDisabled(t *testing.T) {
	cfg := &config.Config{Allowlist: []string{"allowed.example.com"}}
	provider := NewDefaultProvider(cfg, WithURLAllowlistDisabled())

	client, err := provider.NewDefaultClient()
	require.NoError(t, err)

	_, isWrapped := client.(*urlAllowlistClient)
	assert.False(t, isWrapped, "client should not be wrapped when WithURLAllowlistDisabled is used")
}

type fakeHTTPClient struct {
	response *http.Response
	called   bool
}

func (f *fakeHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	f.called = true
	return f.response, nil
}
