// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestShouldReenroll_NodeAgent(t *testing.T) {
	flavor.SetFlavor(flavor.DefaultAgent)

	tests := []struct {
		name              string
		agentHostname     string
		persistedHostname string
		want              bool
	}{
		{
			name:              "same hostname - no reenroll",
			agentHostname:     "my-host",
			persistedHostname: "my-host",
			want:              false,
		},
		{
			name:              "different hostname - reenroll",
			agentHostname:     "new-host",
			persistedHostname: "old-host",
			want:              true,
		},
		{
			name:              "empty persisted hostname - no reenroll (backward compat)",
			agentHostname:     "my-host",
			persistedHostname: "",
			want:              false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agentID := &AgentIdentifier{Hostname: tc.agentHostname}
			identity := &PersistedIdentity{Hostname: tc.persistedHostname, APIKeyHash: HashAPIKey("some-api-key")}
			assert.Equal(t, tc.want, ShouldReenroll(agentID, identity, "some-api-key"))
		})
	}
}

func TestShouldReenroll_ClusterAgent_NeverReenrolls(t *testing.T) {
	flavor.SetFlavor(flavor.ClusterAgent)
	defer flavor.SetFlavor(flavor.DefaultAgent)

	// Cluster agent re-enrollment is disabled; even a hostname or api_key mismatch should return false.
	agentID := &AgentIdentifier{OrchClusterID: "cluster-new"}
	identity := &PersistedIdentity{OrchClusterID: "cluster-old", APIKeyHash: HashAPIKey("old-key")}
	assert.False(t, ShouldReenroll(agentID, identity, "new-key"))
}

func TestShouldReenroll_APIKeyChanged(t *testing.T) {
	flavor.SetFlavor(flavor.DefaultAgent)

	tests := []struct {
		name            string
		currentAPIKey   string
		persistedAPIKey string
		want            bool
	}{
		{
			name:            "same api key - no reenroll",
			currentAPIKey:   "key-a",
			persistedAPIKey: "key-a",
			want:            false,
		},
		{
			name:            "different api key - reenroll",
			currentAPIKey:   "key-b",
			persistedAPIKey: "key-a",
			want:            true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agentID := &AgentIdentifier{Hostname: "my-host"}
			identity := &PersistedIdentity{Hostname: "my-host", APIKeyHash: HashAPIKey(tc.persistedAPIKey)}
			assert.Equal(t, tc.want, ShouldReenroll(agentID, identity, tc.currentAPIKey))
		})
	}

	t.Run("empty persisted api key hash - reenroll (legacy identity)", func(t *testing.T) {
		agentID := &AgentIdentifier{Hostname: "my-host"}
		identity := &PersistedIdentity{Hostname: "my-host"}
		assert.True(t, ShouldReenroll(agentID, identity, "any-key"))
	})
}
