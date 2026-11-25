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
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
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

func TestShouldReenroll(t *testing.T) {
	tests := []struct {
		name          string
		flavor        string
		agentHostname string
		identity      *PersistedIdentity
		currentKey    string
		want          bool
	}{
		{"no identity", flavor.DefaultAgent, "my-host", nil, "key-a", false},
		{"node agent, same hostname", flavor.DefaultAgent, "my-host", &PersistedIdentity{Hostname: "my-host"}, "key-a", false},
		{"node agent, different hostname", flavor.DefaultAgent, "new-host", &PersistedIdentity{Hostname: "old-host"}, "key-a", true},
		{"node agent, no persisted hostname", flavor.DefaultAgent, "my-host", &PersistedIdentity{}, "key-a", false},
		{"node agent, same api_key", flavor.DefaultAgent, "my-host", &PersistedIdentity{Hostname: "my-host", APIKeyHash: HashAPIKey("key-a")}, "key-a", false},
		{"node agent, different api_key", flavor.DefaultAgent, "my-host", &PersistedIdentity{Hostname: "my-host", APIKeyHash: HashAPIKey("key-a")}, "key-b", true},
		{"node agent, no persisted api_key hash", flavor.DefaultAgent, "my-host", &PersistedIdentity{Hostname: "my-host"}, "key-b", false},
		{"cluster agent, same api_key", flavor.ClusterAgent, "pod-1", &PersistedIdentity{APIKeyHash: HashAPIKey("key-a")}, "key-a", false},
		{"cluster agent, different api_key", flavor.ClusterAgent, "pod-1", &PersistedIdentity{APIKeyHash: HashAPIKey("key-a")}, "key-b", true},
		{"cluster agent, no persisted api_key hash", flavor.ClusterAgent, "pod-1", &PersistedIdentity{}, "key-b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor.SetTestFlavor(t, tt.flavor)

			assert.Equal(t, tt.want, ShouldReenroll(&AgentIdentifier{Hostname: tt.agentHostname}, tt.identity, tt.currentKey))
		})
	}
}

func TestIdentityMatchesPAROverrides(t *testing.T) {
	for _, tc := range []struct {
		name, key, site, region, wifOrg string
		want                            bool
	}{
		{name: "legacy behavior", region: "eu1", want: true},
		{name: "own key", key: "par-key", region: "eu1", want: true},
		{name: "own key replaces WIF identity", key: "par-key", region: "eu1", wifOrg: "old-org"},
		{name: "own site matches", site: "datadoghq.com", region: "us1", want: true},
		{name: "own site replaces other region", site: "datadoghq.com", region: "eu1"},
		{name: "normalized site", site: " US3.DATADOGHQ.COM. ", region: "us3", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("site", "datadoghq.eu")
			cfg.SetInTest(par.APIKey, tc.key)
			cfg.SetInTest(par.RunnerSite, tc.site)
			identity := &PersistedIdentity{URN: "urn:dd:apps:on-prem-runner:" + tc.region + ":42:runner", WIFOrgUUID: tc.wifOrg}
			require.Equal(t, tc.want, identity.MatchesPAROverrides(cfg))
		})
	}
}
