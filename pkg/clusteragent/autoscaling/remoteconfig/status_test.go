// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderedText(t *testing.T, info map[string]interface{}) string {
	t.Helper()
	var buffer bytes.Buffer
	require.NoError(t, renderText(&buffer, info))
	return buffer.String()
}

func renderedHTML(t *testing.T, info map[string]interface{}) string {
	t.Helper()
	var buffer bytes.Buffer
	require.NoError(t, renderHTML(&buffer, info))
	return buffer.String()
}

// TestStatusNotConfiguredRendersNothing is what keeps the Cluster Agent status
// output identical to today for every cluster that does not set the endpoint.
// An empty section buffer makes the status renderer skip the header entirely.
func TestStatusNotConfiguredRendersNothing(t *testing.T) {
	provider := NewProvider(newTestConfig(t, "", ""), NewEndpointStatus())

	assert.Nil(t, provider.statusInfo())
	assert.Empty(t, renderedText(t, provider.statusInfo()))
	assert.Empty(t, renderedHTML(t, provider.statusInfo()))
}

// TestStatusConfiguredButNotStarted covers an endpoint that is set with no client
// built, e.g. autoscaling disabled: it must still be visible.
// TestProviderIsInTheAutoscalingGroup pins where this entry lands: the status
// component groups by Section and orders by Name, so the name also has to sort
// before "Workload Autoscaling".
func TestProviderIsInTheAutoscalingGroup(t *testing.T) {
	assert.Equal(t, "Autoscaling", Provider{}.Section())
	assert.Equal(t, "Configuration Endpoint", Provider{}.Name())
	assert.Less(t, Provider{}.Name(), "Workload Autoscaling")
}

func TestStatusConfiguredButNotStarted(t *testing.T) {
	// A status with nothing reported into it: the service was never built.
	provider := NewProvider(newTestConfig(t, "https://config.datad0g.com", testAPIKey), NewEndpointStatus())

	info := provider.statusInfo()
	require.NotNil(t, info)
	assert.Equal(t, "https://config.datad0g.com", info["Endpoint"])
	assert.Equal(t, false, info["Started"])

	out := renderedText(t, info)
	assert.Contains(t, out, "Endpoint: https://config.datad0g.com")
	assert.Contains(t, out, "Client: not started")
	assert.NotContains(t, out, "Organization enabled")
}

func TestStatusConfiguredAndHealthy(t *testing.T) {
	// Driven through the StatusInstance interface, so this also covers
	// EndpointStatus implementing it the way the service expects.
	endpointStatus := NewEndpointStatus()
	endpointStatus.SetOrgEnabled(true)
	endpointStatus.SetAPIKeyAuthorized(true)
	endpointStatus.SetLastError("")
	endpointStatus.MarkStarted()

	provider := NewProvider(newTestConfig(t, "https://config.datad0g.com", testAPIKey), endpointStatus)

	info := provider.statusInfo()
	require.NotNil(t, info)
	assert.Equal(t, true, info["Started"])
	assert.Equal(t, true, info["OrgEnabled"])
	assert.Equal(t, true, info["APIKeyAuthorized"])

	out := renderedText(t, info)
	assert.Contains(t, out, "Endpoint: https://config.datad0g.com")
	assert.Contains(t, out, "Organization enabled: True")
	assert.Contains(t, out, "API Key: Authorized")
	assert.Contains(t, out, "Last error: None")

	assert.Contains(t, renderedHTML(t, info), "Organization enabled: True")
}

func TestStatusConfiguredAndBroken(t *testing.T) {
	endpointStatus := NewEndpointStatus()
	endpointStatus.SetOrgEnabled(false)
	endpointStatus.SetAPIKeyAuthorized(false)
	endpointStatus.SetLastError("api: 403 Forbidden")
	endpointStatus.MarkStarted()

	provider := NewProvider(newTestConfig(t, "https://config.datad0g.com", testAPIKey), endpointStatus)

	info := provider.statusInfo()

	out := renderedText(t, info)
	assert.Contains(t, out, "Organization enabled: False")
	assert.Contains(t, out, "API Key: Not authorized")
	assert.Contains(t, out, "Last error: api: 403 Forbidden")
}

// TestStatusDropsCredentials covers the configured value being echoed before a
// client exists. The status output ends up in flares, so anything that could
// carry a credential is dropped rather than scrubbed.
func TestStatusDropsCredentials(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"userinfo", "https://user:SECRET@config.datad0g.com", "https://config.datad0g.com"},
		{"query", "https://config.datad0g.com/path?token=SECRET", "https://config.datad0g.com/path"},
		{"fragment", "https://config.datad0g.com#token=SECRET", "https://config.datad0g.com"},
		{"nothing to strip", "https://config.datad0g.com", "https://config.datad0g.com"},
		// IsConfigured and Load both trim, so a padded value reaches the status
		// as a healthy endpoint and must render as one
		{"padded", "  https://config.datad0g.com  ", "https://config.datad0g.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(newTestConfig(t, tt.url, testAPIKey), NewEndpointStatus())
			info := provider.statusInfo()

			assert.Equal(t, tt.want, info["Endpoint"])
			out := renderedText(t, info)
			assert.NotContains(t, out, "SECRET")
			assert.Contains(t, out, tt.want)
		})
	}
}

// TestStatusUnknownUntilEndpointAnswers covers an unreachable endpoint. Both org
// fields are false until the first answer, so reporting them unconditionally
// reads as "organization disabled, key not authorized" -- the misdiagnosis this
// section exists to prevent.
func TestStatusUnknownUntilEndpointAnswers(t *testing.T) {
	endpointStatus := NewEndpointStatus()
	endpointStatus.MarkStarted()
	provider := NewProvider(newTestConfig(t, "https://config.datad0g.com", testAPIKey), endpointStatus)

	info := provider.statusInfo()
	require.NotNil(t, info)
	assert.Equal(t, false, info["Reported"])
	assert.NotContains(t, info, "OrgEnabled")
	assert.NotContains(t, info, "APIKeyAuthorized")

	out := renderedText(t, info)
	assert.Contains(t, out, "no answer from the endpoint yet")
	// The reason never reaches the status, so the entry names what to check.
	// A wrong URL and an unscoped key are indistinguishable here, so both.
	assert.Contains(t, out, URLSetting)
	assert.Contains(t, out, APIKeySetting)
	assert.Contains(t, out, "Remote Configuration Read permission")
	assert.NotContains(t, out, "Organization enabled:")
	assert.Contains(t, renderedHTML(t, info), "no answer from the endpoint yet")

	// A disabled organization is a real answer and must still render as one
	endpointStatus.SetOrgEnabled(false)
	endpointStatus.SetAPIKeyAuthorized(false)

	out = renderedText(t, provider.statusInfo())
	assert.Contains(t, out, "Organization enabled: False")
	assert.Contains(t, out, "Not authorized")
	assert.NotContains(t, out, "no answer from the endpoint yet")
}

// TestStatusEachOrgFieldCountsAsAnAnswer pins the flag on both setters
// separately: the poller writes both, so a test driving both cannot tell which
// one records the answer.
func TestStatusEachOrgFieldCountsAsAnAnswer(t *testing.T) {
	for name, report := range map[string]func(*EndpointStatus){
		"org enabled":        func(s *EndpointStatus) { s.SetOrgEnabled(true) },
		"api key authorized": func(s *EndpointStatus) { s.SetAPIKeyAuthorized(true) },
	} {
		t.Run(name, func(t *testing.T) {
			endpointStatus := NewEndpointStatus()
			endpointStatus.MarkStarted()
			require.False(t, endpointStatus.snapshot().reported)

			report(endpointStatus)
			assert.True(t, endpointStatus.snapshot().reported)
		})
	}
}
