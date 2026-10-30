// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
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
	partypes "github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/jsonapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWIFEnrollmentPersistsIdentityAcrossIntakeRotation(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	identityPath := filepath.Join(t.TempDir(), "identity.json")
	type receivedRequest struct {
		path, authorization, apiKey, appKey string
		body                                []byte
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		_, err = os.Stat(identityPath)
		assert.True(t, os.IsNotExist(err), "identity is saved only after successful enrollment")
		received <- receivedRequest{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("DD-API-KEY"), r.Header.Get("DD-APPLICATION-KEY"), body}
		fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner-wif","attributes":{"org_id":42,"runner_id":"runner-wif"}}}`)
	}))
	defer server.Close()
	overrides := map[string]interface{}{
		"dd_url": server.URL, "skip_ssl_validation": true,
		// WIF must not validate or send these credentials, even with API-key-only mode off.
		"api_key": "ENC[unused-intake-key]", par.APIKeyOnlyEnrollment: false,
		"delegated_auth.org_uuid": "test-org",
		par.WIFEnrollment:         true, par.SelfEnroll: true, par.IdentityFilePath: identityPath,
	}
	cfg := coreconfig.NewMockWithOverrides(t, overrides)
	hostname, _ := hostnamemock.NewMock("test-host")
	var signedRequest common.WorkloadTokenRequest
	tokenCalls := 0
	auth := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(_ context.Context, key string, request common.WorkloadTokenRequest) (string, error) {
		tokenCalls++
		require.Equal(t, "api_key", key)
		require.Equal(t, []string{"par_enrollment"}, request.Grants)
		signedRequest = request
		return "workload-token", nil
	}}
	runner := &PrivateActionRunner{coreConfig: cfg, delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t)}
	enrolled, err := runner.getRunnerConfig(context.Background())
	require.NoError(t, err)
	got := <-received
	require.Equal(t, "/api/unstable/on_prem_runners/workload_token", got.path)
	require.Equal(t, "Bearer workload-token", got.authorization)
	require.Empty(t, got.apiKey)
	require.Empty(t, got.appKey)
	var request partypes.CreateRunnerRequest
	require.NoError(t, jsonapi.Unmarshal(got.body, &request))
	require.Equal(t, "test-host", request.AgentHostname)
	require.Equal(t, flavor.PrivateActionRunner, request.AgentFlavor)
	require.Empty(t, request.OrchClusterID)
	block, _ := pem.Decode([]byte(request.PublicKeyPEM))
	require.NotNil(t, block)
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	jwk, err := util.EcdsaToJWK(publicKey)
	require.NoError(t, err)
	require.Equal(t, jwk.KeyID, signedRequest.JWKThumbprint)
	require.True(t, enrolled.PrivateKey.PublicKey.Equal(publicKey))
	saved, err := enrollment.GetIdentityFromPreviousEnrollment(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, enrolled.Urn, saved.URN)
	require.Empty(t, saved.APIKeyHash)
	require.Equal(t, "ENC[unused-intake-key]", cfg.GetString("api_key"))

	// A new component/config simulates restart: reuse the file, not runtime config writes.
	for _, enabled := range []bool{true, false} {
		overrides["api_key"] = "rotated-intake-key"
		overrides[par.WIFEnrollment] = enabled
		restartConfig := coreconfig.NewMockWithOverrides(t, overrides)
		restarted := &PrivateActionRunner{coreConfig: restartConfig, delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t)}
		recovered, err := restarted.getRunnerConfig(context.Background())
		require.NoError(t, err)
		require.Equal(t, enrolled.Urn, recovered.Urn)
		require.True(t, enrolled.PrivateKey.Equal(recovered.PrivateKey))
	}
	require.Equal(t, 1, tokenCalls)
	require.Empty(t, received, "restart does not enroll again")
}

func TestWIFEnrollmentFailuresDoNotFallBackOrPersist(t *testing.T) {
	for _, scenario := range []string{"token denied", "PAR denied", "PAR unavailable", "component unavailable", "cluster agent", "self-enrollment disabled", "partial identity"} {
		t.Run(scenario, func(t *testing.T) {
			flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
			t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
			var posts atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				assert.Equal(t, "/api/unstable/on_prem_runners/workload_token", r.URL.Path)
				if scenario == "PAR unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			}))
			defer server.Close()
			identityPath := filepath.Join(t.TempDir(), "identity.json")
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"dd_url": server.URL, "skip_ssl_validation": true, "api_key": "intake-key",
				"delegated_auth.org_uuid": "test-org",
				par.WIFEnrollment:         true, par.SelfEnroll: true, par.IdentityFilePath: identityPath,
			})
			tokenCalls := 0
			auth := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(context.Context, string, common.WorkloadTokenRequest) (string, error) {
				tokenCalls++
				if scenario == "token denied" {
					return "", errors.New("mapping does not permit PAR enrollment")
				}
				return "workload-token", nil
			}}
			hostname, _ := hostnamemock.NewMock("test-host")
			runner := &PrivateActionRunner{coreConfig: cfg, delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t)}
			switch scenario {
			case "component unavailable":
				runner.delegatedAuth = nil
			case "cluster agent":
				flavor.SetTestFlavor(t, flavor.ClusterAgent)
			case "self-enrollment disabled":
				cfg.SetInTest(par.SelfEnroll, false)
			case "partial identity":
				cfg.SetInTest(par.URN, util.MakeRunnerURN("us1", 42, "existing-runner"))
			}
			if scenario == "cluster agent" {
				// Exercise the enrollment guard without needing a Kubernetes cluster identifier.
				_, err := enrollment.EnrollWithWorkloadToken(context.Background(), cfg, auth, &enrollment.AgentIdentifier{})
				require.ErrorContains(t, err, "only supported for node Agent")
			} else {
				_, err := runner.getRunnerConfig(context.Background())
				require.Error(t, err)
			}
			expectedPosts, expectedTokens := 0, 0
			if scenario == "PAR denied" || scenario == "PAR unavailable" {
				expectedPosts, expectedTokens = 1, 1
			}
			if scenario == "token denied" {
				expectedTokens = 1
			}
			require.EqualValues(t, expectedPosts, posts.Load())
			require.Equal(t, expectedTokens, tokenCalls)
			_, err := os.Stat(identityPath)
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestWIFOptInDoesNotReplaceExistingAPIKeyRunner(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"delegated_auth.org_uuid": "test-org",
		"api_key":                 "original-key", par.WIFEnrollment: true, par.SelfEnroll: true,
		par.IdentityFilePath: filepath.Join(t.TempDir(), "identity.json"),
	})
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	// Seed the identity using the same storage used by API-key enrollment.
	identity := &enrollment.Result{URN: util.MakeRunnerURN("us1", 42, "existing-runner"), Hostname: "test-host", APIKeyHash: enrollment.HashAPIKey("original-key")}
	identity.PrivateKey = key.Key.(*ecdsa.PrivateKey)
	require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, identity))
	before, err := os.ReadFile(enrollment.GetIdentityFilePath(cfg))
	require.NoError(t, err)
	hostname, _ := hostnamemock.NewMock("test-host")
	runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t)}
	reused, err := runner.getRunnerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, identity.URN, reused.Urn)
	cfg.SetInTest("api_key", "rotated-key")
	_, err = runner.getRunnerConfig(context.Background())
	require.ErrorContains(t, err, "cannot replace an existing runner")
	after, err := os.ReadFile(enrollment.GetIdentityFilePath(cfg))
	require.NoError(t, err)
	require.Equal(t, before, after)
}
