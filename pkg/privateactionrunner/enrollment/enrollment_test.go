// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"context"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

type recordingPublicClient struct {
	apiKey string
}

func (c *recordingPublicClient) EnrollWithApiKey(
	context.Context, string, string, string, []modes.Mode, *jose.JSONWebKey, string, string, string,
) (*par.CreateRunnerResponse, error) {
	return nil, assert.AnError
}

func (c *recordingPublicClient) EnrollWithApiKeyOnly(
	_ context.Context,
	apiKey string,
	_ string,
	_ []modes.Mode,
	_ *jose.JSONWebKey,
	_ string,
	_ string,
	_ string,
) (*par.CreateRunnerResponse, error) {
	c.apiKey = apiKey
	return &par.CreateRunnerResponse{OrgID: 42, RunnerID: "runner"}, nil
}

func TestEnrollmentBaseURL(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("dd_url", "http://fakeintake.test:8080/")

	assert.Equal(t, "https://api.datadoghq.com", enrollmentBaseURL(cfg, "datadoghq.com"))

	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	assert.Equal(t, "http://fakeintake.test:8080", enrollmentBaseURL(cfg, "datadoghq.com"))
}

func TestEnrollUsesPrivateActionRunnerBackend(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.eu")
	cfg.SetInTest("api_key", "uk1-api-key")
	cfg.SetInTest(setup.PARSite, "datadoghq.com")
	cfg.SetInTest(setup.PARAPIKey, "us1-api-key")

	recorder := &recordingPublicClient{}
	var baseURL string
	originalNewPublicClient := newPublicClient
	newPublicClient = func(_ configmodel.Reader, url string, _ map[string]string) opms.PublicClient {
		baseURL = url
		return recorder
	}
	t.Cleanup(func() { newPublicClient = originalNewPublicClient })

	result, err := Enroll(context.Background(), cfg, &AgentIdentifier{Hostname: "host"})
	require.NoError(t, err)
	assert.Equal(t, "https://api.datadoghq.com", baseURL)
	assert.Equal(t, "us1-api-key", recorder.apiKey)
	assert.Equal(t, "urn:dd:apps:on-prem-runner:us1:42:runner", result.URN)
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
