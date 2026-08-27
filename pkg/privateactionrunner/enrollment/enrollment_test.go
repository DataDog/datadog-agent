// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestEnrollmentBaseURL(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("dd_url", "http://fakeintake.test:8080/")

	assert.Equal(t, "https://api.datadoghq.com", enrollmentBaseURL(cfg, "datadoghq.com"))

	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	assert.Equal(t, "http://fakeintake.test:8080", enrollmentBaseURL(cfg, "datadoghq.com"))
}

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

func TestShouldReenroll_ClusterAgent_NeverReenrolls(t *testing.T) {
	flavor.SetFlavor(flavor.ClusterAgent)
	defer flavor.SetFlavor(flavor.DefaultAgent)

	// Cluster agent re-enrollment is disabled; even a mismatch should return false.
	agentID := &AgentIdentifier{OrchClusterID: "cluster-new"}
	identity := &PersistedIdentity{OrchClusterID: "cluster-old"}
	assert.False(t, ShouldReenroll(agentID, identity))
}
