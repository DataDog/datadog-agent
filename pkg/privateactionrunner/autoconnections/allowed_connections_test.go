// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConnectionDefinition(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		expectedOk     bool
		expectedBundle string
	}{
		{
			name:           "valid key: http",
			key:            "http",
			expectedOk:     true,
			expectedBundle: "com.datadoghq.http",
		},
		{
			name:           "valid key: kubernetes",
			key:            "kubernetes",
			expectedOk:     true,
			expectedBundle: "com.datadoghq.kubernetes",
		},
		{
			name:           "valid key: script",
			key:            "script",
			expectedOk:     true,
			expectedBundle: "com.datadoghq.script",
		},
		{
			name:       "invalid key",
			key:        "invalid",
			expectedOk: false,
		},
		{
			name:       "empty key",
			key:        "",
			expectedOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := GetConnectionDefinition(tt.key)

			assert.Equal(t, tt.expectedOk, ok)

			if tt.expectedOk {
				require.NotEmpty(t, def.BundleID)
				assert.Equal(t, tt.expectedBundle, def.BundleID)
				assert.NotEmpty(t, def.IntegrationType)
				assert.NotEmpty(t, def.Credentials.Type)
			} else {
				assert.Equal(t, ConnectionDefinition{}, def)
			}
		})
	}
}

func TestGetBundleKeys(t *testing.T) {
	keys := GetBundleKeys()

	require.Len(t, keys, 3)

	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}

	assert.True(t, keyMap["http"])
	assert.True(t, keyMap["kubernetes"])
	assert.True(t, keyMap["script"])
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		bundleID string
		expected bool
	}{
		{
			name:     "exact match",
			pattern:  "com.datadoghq.script",
			bundleID: "com.datadoghq.script",
			expected: true,
		},
		{
			name:     "wildcard match",
			pattern:  "com.datadoghq.http.*",
			bundleID: "com.datadoghq.http",
			expected: true,
		},
		{
			name:     "prefix match - action pattern",
			pattern:  "com.datadoghq.kubernetes.get_pods",
			bundleID: "com.datadoghq.kubernetes",
			expected: true,
		},
		{
			name:     "universal wildcard",
			pattern:  "com.datadoghq.*",
			bundleID: "com.datadoghq.http",
			expected: true,
		},
		{
			name:     "exact match without wildcard",
			pattern:  "com.datadoghq.http",
			bundleID: "com.datadoghq.http",
			expected: true,
		},
		{
			name:     "no match - different bundle",
			pattern:  "com.datadoghq.http",
			bundleID: "com.datadoghq.kubernetes",
			expected: false,
		},
		{
			name:     "no match - different prefix",
			pattern:  "com.example.*",
			bundleID: "com.datadoghq.http",
			expected: false,
		},
		{
			name:     "wildcard matches kubernetes",
			pattern:  "com.datadoghq.*",
			bundleID: "com.datadoghq.kubernetes",
			expected: true,
		},
		{
			name:     "wildcard matches script",
			pattern:  "com.datadoghq.*",
			bundleID: "com.datadoghq.script",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPattern(tt.pattern, tt.bundleID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineConnectionsToCreate_AllBundles(t *testing.T) {
	allowlist := []string{"com.datadoghq.http.*", "com.datadoghq.kubernetes.*", "com.datadoghq.script"}

	definitions := DetermineConnectionsToCreate(allowlist)

	assert.Len(t, definitions, 3)

	defMap := make(map[string]ConnectionDefinition)
	for _, def := range definitions {
		defMap[def.BundleID] = def
	}

	httpDef, ok := defMap["com.datadoghq.http"]
	assert.True(t, ok)
	assert.Equal(t, "com.datadoghq.http", httpDef.BundleID)
	assert.Equal(t, "HTTP", httpDef.IntegrationType)
	assert.Equal(t, "HTTPNoAuth", httpDef.Credentials.Type)

	k8sDef, ok := defMap["com.datadoghq.kubernetes"]
	assert.True(t, ok)
	assert.Equal(t, "com.datadoghq.kubernetes", k8sDef.BundleID)
	assert.Equal(t, "Kubernetes", k8sDef.IntegrationType)
	assert.Equal(t, "KubernetesServiceAccount", k8sDef.Credentials.Type)

	scriptDef, ok := defMap["com.datadoghq.script"]
	assert.True(t, ok)
	assert.Equal(t, "com.datadoghq.script", scriptDef.BundleID)
	assert.Equal(t, "Script", scriptDef.IntegrationType)
	assert.Equal(t, "Script", scriptDef.Credentials.Type)
	assert.NotNil(t, scriptDef.Credentials.AdditionalFields)
	assert.Equal(t, "/etc/dd-action-runner/config/credentials/script.yaml",
		scriptDef.Credentials.AdditionalFields["configFileLocation"])
}

func TestDetermineConnectionsToCreate_SingleBundle(t *testing.T) {
	tests := []struct {
		name           string
		allowlist      []string
		expectedBundle string
	}{
		{
			name:           "http bundle",
			allowlist:      []string{"com.datadoghq.http.request"},
			expectedBundle: "com.datadoghq.http",
		},
		{
			name:           "kubernetes bundle",
			allowlist:      []string{"com.datadoghq.kubernetes.get_pods"},
			expectedBundle: "com.datadoghq.kubernetes",
		},
		{
			name:           "script bundle",
			allowlist:      []string{"com.datadoghq.script"},
			expectedBundle: "com.datadoghq.script",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definitions := DetermineConnectionsToCreate(tt.allowlist)

			assert.Len(t, definitions, 1)
			assert.Equal(t, tt.expectedBundle, definitions[0].BundleID)
		})
	}
}

func TestDetermineConnectionsToCreate_MultipleBundles(t *testing.T) {
	allowlist := []string{"com.datadoghq.http.*", "com.datadoghq.script"}

	definitions := DetermineConnectionsToCreate(allowlist)

	assert.Len(t, definitions, 2)

	bundleIDs := make(map[string]bool)
	for _, def := range definitions {
		bundleIDs[def.BundleID] = true
	}

	assert.True(t, bundleIDs["com.datadoghq.http"])
	assert.True(t, bundleIDs["com.datadoghq.script"])
	assert.False(t, bundleIDs["com.datadoghq.kubernetes"])
}

func TestDetermineConnectionsToCreate_NoRelevantBundles(t *testing.T) {
	allowlist := []string{"com.datadoghq.gitlab.*"}

	definitions := DetermineConnectionsToCreate(allowlist)

	assert.Len(t, definitions, 0)
}

func TestDetermineConnectionsToCreate_EmptyAndNilAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		allowlist []string
	}{
		{
			name:      "nil allowlist",
			allowlist: nil,
		},
		{
			name:      "empty slice",
			allowlist: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definitions := DetermineConnectionsToCreate(tt.allowlist)

			assert.Len(t, definitions, 0)
			assert.NotNil(t, definitions)
		})
	}
}

func TestDetermineConnectionsToCreate_UniversalWildcard(t *testing.T) {
	allowlist := []string{"com.datadoghq.*"}

	definitions := DetermineConnectionsToCreate(allowlist)

	assert.Len(t, definitions, 3)

	bundleIDs := make(map[string]bool)
	for _, def := range definitions {
		bundleIDs[def.BundleID] = true
	}

	assert.True(t, bundleIDs["com.datadoghq.http"])
	assert.True(t, bundleIDs["com.datadoghq.kubernetes"])
	assert.True(t, bundleIDs["com.datadoghq.script"])
}

func TestGenerateConnectionName(t *testing.T) {
	httpDef := ConnectionDefinition{
		BundleID:        "com.datadoghq.http",
		IntegrationType: "HTTP",
		Credentials: CredentialConfig{
			Type: "HTTPNoAuth",
		},
	}

	runnerID := "runner-abc123"

	name := GenerateConnectionName(httpDef, runnerID)

	assert.Equal(t, "HTTP (runner-abc123)", name)
}

func TestExtractRunnerIDFromURN(t *testing.T) {
	tests := []struct {
		name        string
		urn         string
		expectedID  string
		expectError bool
	}{
		{
			name:        "valid URN - us1",
			urn:         "us1:12345:runner-abc123",
			expectedID:  "runner-abc123",
			expectError: false,
		},
		{
			name:        "valid URN - eu1",
			urn:         "eu1:67890:runner-xyz789",
			expectedID:  "runner-xyz789",
			expectError: false,
		},
		{
			name:        "invalid URN - single part",
			urn:         "invalid",
			expectedID:  "",
			expectError: true,
		},
		{
			name:        "empty URN",
			urn:         "",
			expectedID:  "",
			expectError: true,
		},
		{
			name:        "incomplete URN - two parts",
			urn:         "us1:12345",
			expectedID:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnerID, err := extractRunnerIDFromURN(tt.urn)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedID, runnerID)
			}
		})
	}
}
