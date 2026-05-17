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
	actionsAllowlist := []string{"com.datadoghq.http.request", "com.datadoghq.kubernetes.core.getPods", "com.datadoghq.script.runPredefinedScript"}

	definitions := DetermineConnectionsToCreate(actionsAllowlist)

	assert.Len(t, definitions, 2)

	defMap := make(map[string]ConnectionDefinition)
	for _, def := range definitions {
		defMap[def.FQNPrefix] = def
	}

	k8sDef, ok := defMap["com.datadoghq.kubernetes"]
	assert.True(t, ok)
	assert.Equal(t, "com.datadoghq.kubernetes", k8sDef.FQNPrefix)
	assert.Equal(t, "Kubernetes", k8sDef.IntegrationType)
	assert.Equal(t, "KubernetesServiceAccount", k8sDef.Credentials.Type)

	scriptDef, ok := defMap["com.datadoghq.script"]
	assert.True(t, ok)
	assert.Equal(t, "com.datadoghq.script", scriptDef.FQNPrefix)
	assert.Equal(t, "Script", scriptDef.IntegrationType)
	assert.Equal(t, "Script", scriptDef.Credentials.Type)
	assert.NotNil(t, scriptDef.Credentials.AdditionalFields)
	// Verify configFileLocation uses static path
	assert.Equal(t, getScriptConfigPath(),
		scriptDef.Credentials.AdditionalFields["configFileLocation"])
}

func TestDetermineConnectionsToCreate_SingleBundle(t *testing.T) {
	tests := []struct {
		name             string
		actionsAllowlist []string
		expectedBundle   string
	}{
		{
			name:             "kubernetes",
			actionsAllowlist: []string{"com.datadoghq.kubernetes.core.getPods"},
			expectedBundle:   "com.datadoghq.kubernetes",
		},
		{
			name:             "script",
			actionsAllowlist: []string{"com.datadoghq.script.runPredefinedScipt"},
			expectedBundle:   "com.datadoghq.script",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definitions := DetermineConnectionsToCreate(tt.actionsAllowlist)

			assert.Len(t, definitions, 1)
			assert.Equal(t, tt.expectedBundle, definitions[0].FQNPrefix)
		})
	}
}

func TestDetermineConnectionsToCreate_NoRelevantBundles(t *testing.T) {
	actionsAllowlist := []string{"com.datadoghq.gitlab.issues.getIssues"}

	definitions := DetermineConnectionsToCreate(actionsAllowlist)

	assert.Len(t, definitions, 0)
}

func TestDetermineConnectionsToCreate_EmptyAndNilAllowlist(t *testing.T) {
	tests := []struct {
		name             string
		actionsAllowlist []string
	}{
		{
			name:             "nil actionsAllowlist",
			actionsAllowlist: nil,
		},
		{
			name:             "empty slice",
			actionsAllowlist: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definitions := DetermineConnectionsToCreate(tt.actionsAllowlist)

			assert.Len(t, definitions, 0)
			assert.NotNil(t, definitions)
		})
	}
}

func TestGenerateConnectionName(t *testing.T) {
	definition := supportedConnections["kubernetes"]

	runnerID := "runner-abc123"

	name := GenerateConnectionName(definition, runnerID)

	assert.Equal(t, "Kubernetes (runner-abc123)", name)
}
