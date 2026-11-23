// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/stretchr/testify/require"
)

func TestStandaloneEnrollmentUsesExistingStartupPath(t *testing.T) {
	for _, tc := range []struct {
		name, logsOrg, mainOrg, parKey, parSite string
	}{
		{name: "static API key"},
		{name: "logs-only WIF", logsOrg: "logs-org"},
		{name: "independent US1 backend", parKey: "par-key", parSite: "datadoghq.com"},
		{name: "PAR key overrides primary WIF", mainOrg: "main-org", parKey: "par-key", parSite: "datadoghq.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
			var expectedKey atomic.Value
			expectedKey.Store("test-key")
			if tc.parKey != "" {
				expectedKey.Store(tc.parKey)
			}
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				require.Equal(t, expectedKey.Load(), r.Header.Get("DD-API-KEY"))
				require.Equal(t, "/api/unstable/on_prem_runners/api_key_only", r.URL.Path)
				var body struct {
					Data struct {
						Attributes struct {
							AgentFlavor string `json:"agent_flavor"`
						} `json:"attributes"`
					} `json:"data"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, flavor.PrivateActionRunner, body.Data.Attributes.AgentFlavor)
				fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":42,"runner_id":"runner"}}}`)
			}))
			defer srv.Close()
			overrides := map[string]interface{}{
				"delegated_auth.org_uuid":             tc.mainOrg,
				"site":                                "datadoghq.eu",
				"private_action_runner.api_key":       tc.parKey,
				"private_action_runner.site":          tc.parSite,
				"logs_config.delegated_auth.org_uuid": tc.logsOrg,
				"api_key":                             "test-key", "dd_url": srv.URL, "private_action_runner.enabled": true,
				"private_action_runner.self_enroll": true, "private_action_runner.api_key_only_enrollment": true,
				"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
			}
			cfg := coreconfig.NewMockWithOverrides(t, overrides)
			flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
			hostname, _ := hostnamemock.NewMock("test-host")
			storage := enrollment.NewIdentityStorage(cfg, nil)
			runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t), identityStorage: storage}
			_, err := runner.getRunnerConfig(context.Background())
			require.NoError(t, err)
			identity, err := storage.Load(context.Background())
			require.NoError(t, err)
			require.NotNil(t, identity)
			require.NotEmpty(t, identity.PrivateKey)
			region := "eu1"
			if tc.parSite == "datadoghq.com" {
				region = "us1"
			}
			require.Equal(t, "urn:dd:apps:on-prem-runner:"+region+":42:runner", identity.URN)
			require.Equal(t, enrollment.HashAPIKey(expectedKey.Load().(string)), identity.APIKeyHash)
			require.Equal(t, "test-key", cfg.GetString("api_key"))
			require.Equal(t, "datadoghq.eu", cfg.GetString("site"))
			_, err = runner.getRunnerConfig(context.Background())
			require.NoError(t, err)
			require.EqualValues(t, 1, posts.Load(), "a saved identity follows existing reuse behavior")
			// Only rotation of the effective enrollment key replaces the runner.
			overrides["api_key"] = "rotated-key"
			if tc.parKey != "" {
				cfg.SetInTest("api_key", "rotated-key")
				_, err = runner.getRunnerConfig(context.Background())
				require.NoError(t, err)
				require.EqualValues(t, 1, posts.Load(), "main Agent key rotation must not replace PAR's identity")
				overrides["private_action_runner.api_key"] = "rotated-key"
			}
			expectedKey.Store("rotated-key")
			cfg = coreconfig.NewMockWithOverrides(t, overrides)
			// Config mocks share a backing store; discard identity populated by the previous startup.
			cfg.UnsetForSource("private_action_runner.urn", model.SourceAgentRuntime)
			cfg.UnsetForSource("private_action_runner.private_key", model.SourceAgentRuntime)
			runner.coreConfig = cfg
			_, err = runner.getRunnerConfig(context.Background())
			require.NoError(t, err)
			require.EqualValues(t, 2, posts.Load())
			identity, err = storage.Load(context.Background())
			require.NoError(t, err)
			require.Equal(t, enrollment.HashAPIKey("rotated-key"), identity.APIKeyHash)
		})
	}
}
