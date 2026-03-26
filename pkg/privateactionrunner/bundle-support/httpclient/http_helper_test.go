// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

func TestURLAllowlistClient_BlocksDisallowedURL(t *testing.T) {
	cfg := &config.Config{Allowlist: []string{"allowed.example.com"}}
	inner := &fakeHTTPClient{response: &http.Response{StatusCode: 200}}
	client := &urlAllowlistClient{inner: inner, config: cfg}

	req, _ := http.NewRequest("GET", "http://blocked.example.com/test", nil)
	resp, err := client.Do(req)

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request url is not allowed by runner policy")
	assert.False(t, inner.called, "inner client should not be called for blocked URLs")
}

func TestURLAllowlistClient_AllowsAllowedURL(t *testing.T) {
	cfg := &config.Config{Allowlist: []string{"allowed.example.com"}}
	inner := &fakeHTTPClient{response: &http.Response{StatusCode: 200}}
	client := &urlAllowlistClient{inner: inner, config: cfg}

	req, _ := http.NewRequest("GET", "http://allowed.example.com/test", nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, inner.called, "inner client should be called for allowed URLs")
}

func TestURLAllowlistClient_AllowsAllWhenNoAllowlist(t *testing.T) {
	cfg := &config.Config{} // nil Allowlist = allow all
	inner := &fakeHTTPClient{response: &http.Response{StatusCode: 200}}
	client := &urlAllowlistClient{inner: inner, config: cfg}

	req, _ := http.NewRequest("GET", "http://any-url.example.com/test", nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, inner.called)
}

func TestNewDefaultProvider_EnforcesAllowlistByDefault(t *testing.T) {
	cfg := &config.Config{Allowlist: []string{"allowed.example.com"}}
	provider := NewDefaultProvider(cfg)

	client, err := provider.NewDefaultClient()
	require.NoError(t, err)

	_, isWrapped := client.(*urlAllowlistClient)
	assert.True(t, isWrapped, "client should be wrapped with urlAllowlistClient by default")
}

func TestNewDefaultProvider_WithoutURLAllowlist(t *testing.T) {
	cfg := &config.Config{Allowlist: []string{"allowed.example.com"}}
	provider := NewDefaultProvider(cfg, WithoutURLAllowlist())

	client, err := provider.NewDefaultClient()
	require.NoError(t, err)

	_, isWrapped := client.(*urlAllowlistClient)
	assert.False(t, isWrapped, "client should not be wrapped when WithoutURLAllowlist is used")
}

type fakeHTTPClient struct {
	response *http.Response
	called   bool
}

func (f *fakeHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	f.called = true
	return f.response, nil
}
