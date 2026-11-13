// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWIFReenrollment(t *testing.T) {
	for _, tc := range []struct {
		name, savedOrg, savedSite, failure string
		reuse                              bool
	}{
		{name: "saved API-key runner"},
		{name: "legacy identity without hash"},
		{name: "hostname changed"},
		{name: "same WIF configuration", savedOrg: "test-org", savedSite: "datadoghq.com", reuse: true},
		{name: "equivalent WIF configuration", savedOrg: "TEST-ORG", savedSite: "datadoghq.com", reuse: true},
		{name: "WIF org changed", savedOrg: "previous-org", savedSite: "datadoghq.com"},
		{name: "WIF site changed", savedOrg: "test-org", savedSite: "datadoghq.eu"},
		{name: "missing WIF org", savedSite: "datadoghq.com"},
		{name: "missing WIF site", savedOrg: "test-org"},
		{name: "token denied", failure: "token"},
		{name: "PAR denied", failure: "PAR"},
		{name: "PAR unavailable", failure: "unavailable"},
		{name: "WIF change token denied", savedOrg: "previous-org", savedSite: "datadoghq.com", failure: "token"},
		{name: "WIF change PAR denied", savedOrg: "previous-org", savedSite: "datadoghq.com", failure: "PAR"},
		{name: "WIF change PAR unavailable", savedOrg: "previous-org", savedSite: "datadoghq.com", failure: "unavailable"},
		{name: "WIF disabled", reuse: true},
		{name: "self-enrollment disabled", reuse: true},
		{name: "WIF change with self-enrollment disabled", savedOrg: "previous-org", savedSite: "datadoghq.com", reuse: true},
	} {
		scenario := tc.name
		t.Run(scenario, func(t *testing.T) {
			flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
			t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
			identityPath := filepath.Join(t.TempDir(), "identity.json")
			var before []byte
			var posts atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				assert.Equal(t, "/api/unstable/on_prem_runners/workload_token", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Empty(t, r.URL.RawQuery)
				assert.Empty(t, r.Header.Get("X-Datadog-PAR-Proof"))
				assert.Empty(t, r.Header.Get("DD-API-KEY"))
				assert.Equal(t, "Bearer workload-token", r.Header.Get("Authorization"))
				current, err := os.ReadFile(identityPath)
				assert.NoError(t, err)
				assert.Equal(t, before, current, "keep the old identity until enrollment succeeds")
				if tc.failure == "PAR" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if tc.failure == "unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner-new","attributes":{"org_id":42,"runner_id":"runner-new"}}}`)
			}))
			defer server.Close()
			overrides := map[string]interface{}{
				"delegated_auth.org_uuid": "test-org", "dd_url": server.URL, "skip_ssl_validation": true,
				"api_key":      "old-key",
				par.SelfEnroll: scenario != "self-enrollment disabled" && scenario != "WIF change with self-enrollment disabled", par.IdentityFilePath: identityPath,
			}
			if scenario == "WIF disabled" {
				overrides["delegated_auth.org_uuid"] = ""
			}
			if scenario == "equivalent WIF configuration" {
				overrides["site"] = " DatadogHQ.COM. "
			}
			cfg := coreconfig.NewMockWithOverrides(t, overrides)
			storage := enrollment.NewIdentityStorage(cfg, nil)
			key, _, err := util.GenerateKeys()
			require.NoError(t, err)
			old := &enrollment.Result{PrivateKey: key.Key.(*ecdsa.PrivateKey),
				URN: util.MakeRunnerURN("us1", 42, "runner-old"), Hostname: "test-host", APIKeyHash: enrollment.HashAPIKey("old-key")}
			old.WIFOrgUUID, old.WIFSite = tc.savedOrg, tc.savedSite
			if scenario == "legacy identity without hash" || tc.savedOrg != "" || tc.savedSite != "" {
				old.APIKeyHash = ""
			}
			require.NoError(t, storage.Persist(context.Background(), old))
			before, err = os.ReadFile(identityPath)
			require.NoError(t, err)
			tokenCalls := 0
			auth := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(_ context.Context, _ string, req common.WorkloadTokenRequest) (string, error) {
				tokenCalls++
				require.Equal(t, []string{"par_enrollment"}, req.Grants)
				require.NotEqual(t, key.KeyID, req.JWKThumbprint, "replacement has its own key")
				if tc.failure == "token" {
					return "", errors.New("mapping does not permit enrollment")
				}
				return "workload-token", nil
			}}
			host := "test-host"
			if scenario == "hostname changed" {
				host = "new-host"
			}
			hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname(host))
			runner := &PrivateActionRunner{coreConfig: cfg, delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t), identityStorage: storage}
			result, err := runner.getRunnerConfig(context.Background())
			after, readErr := os.ReadFile(identityPath)
			require.NoError(t, readErr)
			switch {
			case tc.failure != "":
				require.Error(t, err)
				require.Equal(t, before, after)
				require.Equal(t, 1, tokenCalls)
				wantPosts := int32(1)
				if tc.failure == "token" {
					wantPosts = 0
				}
				require.Equal(t, wantPosts, posts.Load(), "no API-key fallback")
			case tc.reuse:
				require.NoError(t, err)
				require.Equal(t, old.URN, result.Urn)
				require.True(t, old.PrivateKey.Equal(result.PrivateKey))
				require.Equal(t, before, after)
				require.Zero(t, tokenCalls)
				require.Zero(t, posts.Load())
			default:
				require.NoError(t, err)
				require.Equal(t, util.MakeRunnerURN("us1", 42, "runner-new"), result.Urn)
				require.False(t, old.PrivateKey.Equal(result.PrivateKey))
				saved, err := storage.Load(context.Background())
				require.NoError(t, err)
				require.Equal(t, "test-org", saved.WIFOrgUUID)
				require.Equal(t, "datadoghq.com", saved.WIFSite)
				require.Empty(t, saved.APIKeyHash)
				for _, primaryOrg := range []string{"test-org", ""} {
					overrides["api_key"] = "rotated-intake-key"
					overrides["delegated_auth.org_uuid"] = primaryOrg
					restartConfig := coreconfig.NewMockWithOverrides(t, overrides)
					restartStorage := enrollment.NewIdentityStorage(restartConfig, nil)
					restart := &PrivateActionRunner{coreConfig: restartConfig, delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t), identityStorage: restartStorage}
					reused, err := restart.getRunnerConfig(context.Background())
					require.NoError(t, err)
					require.Equal(t, result.Urn, reused.Urn)
					require.True(t, result.PrivateKey.Equal(reused.PrivateKey))
				}
				require.Equal(t, 1, tokenCalls)
				require.EqualValues(t, 1, posts.Load())
			}
		})
	}
}
