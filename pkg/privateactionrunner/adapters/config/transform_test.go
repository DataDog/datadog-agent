// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestGetBundleInheritedAllowedActions(t *testing.T) {
	tests := []struct {
		name                     string
		actionsAllowlist         map[string]sets.Set[string]
		expectedInheritedActions map[string]sets.Set[string]
	}{
		{
			name: "returns special actions for existing bundle",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("testConnection", "enrichScript"),
			},
		},
		{
			name: "returns empty when bundle not in allowlist",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns empty when bundle has empty set",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string](),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name:                     "returns empty for empty allowlist",
			actionsAllowlist:         map[string]sets.Set[string]{},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns special actions for multiple matching bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle":              sets.New[string]("otherAction"),
				"com.datadoghq.script":          sets.New[string]("action1"),
				"com.datadoghq.gitlab.users":    sets.New[string]("action2"),
				"com.datadoghq.kubernetes.core": sets.New[string]("action3"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script":          sets.New[string]("testConnection", "enrichScript"),
				"com.datadoghq.gitlab.users":    sets.New[string]("testConnection"),
				"com.datadoghq.kubernetes.core": sets.New[string]("testConnection"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBundleInheritedAllowedActions(tt.actionsAllowlist)
			assert.Equal(t, tt.expectedInheritedActions, result)
		})
	}
}

func TestGetDatadogHost(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "removes https prefix and trailing dot",
			endpoint: "https://api.datadoghq.com.",
			expected: "api.datadoghq.com",
		},
		{
			name:     "removes https prefix only",
			endpoint: "https://api.datadoghq.com",
			expected: "api.datadoghq.com",
		},
		{
			name:     "removes trailing dot only",
			endpoint: "api.datadoghq.com.",
			expected: "api.datadoghq.com",
		},
		{
			name:     "handles clean URL without modifications",
			endpoint: "api.datadoghq.com",
			expected: "api.datadoghq.com",
		},
		{
			name:     "handles empty string",
			endpoint: "",
			expected: "",
		},
		{
			name:     "handles EU site with https and trailing dot",
			endpoint: "https://api.datadoghq.eu.",
			expected: "api.datadoghq.eu",
		},
		{
			name:     "handles gov site",
			endpoint: "https://api.ddog-gov.com.",
			expected: "api.ddog-gov.com",
		},
		{
			name:     "handles custom domain",
			endpoint: "https://custom.endpoint.example.com.",
			expected: "custom.endpoint.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDatadogHost(tt.endpoint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFromDDConfig(t *testing.T) {
	tests := []struct {
		name           string
		site           string
		ddURL          string
		expectedDDHost string
		expectedDDSite string
	}{
		{
			name:           "US site (datadoghq.com)",
			site:           "datadoghq.com",
			ddURL:          "",
			expectedDDHost: "api.datadoghq.com",
			expectedDDSite: "datadoghq.com",
		},
		{
			name:           "EU site (datadoghq.eu)",
			site:           "datadoghq.eu",
			ddURL:          "",
			expectedDDHost: "api.datadoghq.eu",
			expectedDDSite: "datadoghq.eu",
		},
		{
			name:           "Gov site (ddog-gov.com)",
			site:           "ddog-gov.com",
			ddURL:          "",
			expectedDDHost: "api.ddog-gov.com",
			expectedDDSite: "ddog-gov.com",
		},
		{
			name:           "dd_url overrides site",
			site:           "datadoghq.com",
			ddURL:          "https://api.datadoghq.eu.",
			expectedDDHost: "api.datadoghq.eu",
			expectedDDSite: "datadoghq.eu",
		},
		{
			name:           "custom domain via dd_url",
			site:           "",
			ddURL:          "https://custom.endpoint.example.com.",
			expectedDDHost: "custom.endpoint.example.com",
			expectedDDSite: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)

			// Set required configuration values
			if tt.site != "" {
				mockConfig.SetWithoutSource("site", tt.site)
			}
			if tt.ddURL != "" {
				mockConfig.SetWithoutSource("dd_url", tt.ddURL)
			}

			// Set minimal required PAR config to avoid errors
			mockConfig.SetWithoutSource(setup.PARPrivateKey, "")
			mockConfig.SetWithoutSource(setup.PARUrn, "")

			// Call FromDDConfig
			cfg, err := FromDDConfig(mockConfig)
			require.NoError(t, err)

			// Verify both DDHost and DatadogSite are set correctly
			assert.Equal(t, tt.expectedDDHost, cfg.DDHost, "DDHost mismatch")
			assert.Equal(t, tt.expectedDDSite, cfg.DatadogSite, "DatadogSite mismatch")

			// Verify DDApiHost is also set to the same value as DDHost
			assert.Equal(t, tt.expectedDDHost, cfg.DDApiHost, "DDApiHost should match DDHost")
		})
	}
}
