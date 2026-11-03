// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestEnrollmentUsesPAROverride(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	for _, apiKeyOnly := range []bool{false, true} {
		var path atomic.Value
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path.Store(r.URL.Path)
			assert.Equal(t, "api-key", r.Header.Get("DD-API-KEY"))
			w.WriteHeader(http.StatusForbidden) // Terminal response: no retry or real enrollment.
		}))
		t.Cleanup(server.Close)
		cfg := configmock.New(t)
		cfg.SetInTest("private_action_runner.par_dd_url", server.URL+"/prefix/")
		cfg.SetInTest("private_action_runner.enabled", true)
		cfg.SetInTest("remote_configuration.rc_dd_url", "https://rc-proxy.example.com")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		id := &AgentIdentifier{Hostname: "test-host"}
		if apiKeyOnly {
			_, err := SelfEnrollApiKeyOnly(ctx, cfg, "datadoghq.eu", "test", "api-key", id, nil)
			require.Error(t, err)
			assert.Equal(t, "/prefix/api/unstable/on_prem_runners/api_key_only", path.Load())
		} else {
			_, err := SelfEnroll(ctx, cfg, "datadoghq.eu", "test", "api-key", "app-key", id, nil)
			require.Error(t, err)
			assert.Equal(t, "/prefix/api/unstable/on_prem_runners", path.Load())
		}
		cancel()
		server.Close()
	}
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

	t.Run("empty persisted api key hash - no reenroll (backward compat)", func(t *testing.T) {
		agentID := &AgentIdentifier{Hostname: "my-host"}
		identity := &PersistedIdentity{Hostname: "my-host"}
		assert.False(t, ShouldReenroll(agentID, identity, "any-key"))
	})
}
