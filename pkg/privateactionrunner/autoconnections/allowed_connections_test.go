// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetermineConnectionsToCreate_AllBundles(t *testing.T) {
	allowlist := []string{"com.datadoghq.http.request", "com.datadoghq.kubernetes.core.getPod", "com.datadoghq.script.runPredefinedScript"}

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
			allowlist:      []string{"com.datadoghq.kubernetes.core.getPod"},
			expectedBundle: "com.datadoghq.kubernetes",
		},
		{
			name:           "script bundle",
			allowlist:      []string{"com.datadoghq.script.runPredefinedScript"},
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

func TestDetermineConnectionsToCreate_NoRelevantBundles(t *testing.T) {
	allowlist := []string{"com.datadoghq.gitlab.issues.createIssue"}

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
