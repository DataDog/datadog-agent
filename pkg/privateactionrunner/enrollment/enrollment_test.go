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
			identity := &PersistedIdentity{Hostname: tc.persistedHostname}
			assert.Equal(t, tc.want, ShouldReenroll(agentID, identity))
		})
	}
}

func TestShouldReenroll_ClusterAgent(t *testing.T) {
	flavor.SetFlavor(flavor.ClusterAgent)
	defer flavor.SetFlavor(flavor.DefaultAgent)

	tests := []struct {
		name               string
		agentClusterID     string
		persistedClusterID string
		want               bool
	}{
		{
			name:               "same cluster ID - no reenroll",
			agentClusterID:     "cluster-abc",
			persistedClusterID: "cluster-abc",
			want:               false,
		},
		{
			name:               "different cluster ID - reenroll",
			agentClusterID:     "cluster-new",
			persistedClusterID: "cluster-old",
			want:               true,
		},
		{
			name:               "empty persisted cluster ID - no reenroll (backward compat)",
			agentClusterID:     "cluster-abc",
			persistedClusterID: "",
			want:               false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agentID := &AgentIdentifier{OrchClusterID: tc.agentClusterID}
			identity := &PersistedIdentity{OrchClusterID: tc.persistedClusterID}
			assert.Equal(t, tc.want, ShouldReenroll(agentID, identity))
		})
	}
}
