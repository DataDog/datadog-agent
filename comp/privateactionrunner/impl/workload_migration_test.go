// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedAPIKeyRunner(t *testing.T, cfg coreconfig.Component) *enrollment.Result {
	t.Helper()
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	identity := &enrollment.Result{
		PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 42, "runner-existing"),
		Hostname: "test-host", APIKeyHash: enrollment.HashAPIKey("original-key"),
	}
	require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, identity))
	return identity
}

func TestWIFMigrationPreservesSavedRunner(t *testing.T) {
	for _, scenario := range []string{"API-key identity", "legacy identity without hash", "retry after ambiguous response"} {
		t.Run(scenario, func(t *testing.T) {
			flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
			t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
			type request struct {
				header   http.Header
				runnerID string
			}
			requests := make(chan request, 2)
			var posts atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/unstable/on_prem_runners/workload_token", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Empty(t, body, "migration sends no new runner configuration")
				requests <- request{r.Header.Clone(), r.URL.Query().Get("existing_runner_id")}
				if posts.Add(1) == 1 && scenario == "retry after ambiguous response" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner-existing","attributes":{"org_id":42,"runner_id":"runner-existing"}}}`)
			}))
			defer server.Close()
			overrides := map[string]interface{}{
				"delegated_auth.org_uuid": "test-org",
				"dd_url":                  server.URL, "skip_ssl_validation": true, "api_key": "rotated-intake-key",
				par.WIFEnrollment: true, par.SelfEnroll: true, par.IdentityFilePath: filepath.Join(t.TempDir(), "identity.json"),
			}
			cfg := coreconfig.NewMockWithOverrides(t, overrides)
			identity := seedAPIKeyRunner(t, cfg)
			if scenario == "legacy identity without hash" {
				identity.APIKeyHash = ""
				require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, identity))
			}
			before, err := os.ReadFile(enrollment.GetIdentityFilePath(cfg))
			require.NoError(t, err)
			publicJWK, err := util.EcdsaToJWK(&identity.PrivateKey.PublicKey)
			require.NoError(t, err)
			tokenCalls := 0
			auth := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(_ context.Context, configKey string, req common.WorkloadTokenRequest) (string, error) {
				tokenCalls++
				require.Equal(t, "api_key", configKey)
				require.Equal(t, []string{"par_enrollment"}, req.Grants)
				require.Equal(t, publicJWK.KeyID, req.JWKThumbprint)
				return fmt.Sprintf("workload-token-%d", tokenCalls), nil
			}}
			hostname, _ := hostnamemock.NewMock("test-host")
			runner := &PrivateActionRunner{coreConfig: cfg, delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t)}
			if scenario == "retry after ambiguous response" {
				_, err := runner.getRunnerConfig(context.Background())
				require.Error(t, err)
				after, err := os.ReadFile(enrollment.GetIdentityFilePath(cfg))
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
			migrated, err := runner.getRunnerConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, identity.URN, migrated.Urn)
			require.True(t, identity.PrivateKey.Equal(migrated.PrivateKey))
			for i := 1; i <= tokenCalls; i++ {
				r := <-requests
				require.Equal(t, "runner-existing", r.runnerID)
				require.Equal(t, fmt.Sprintf("Bearer workload-token-%d", i), r.header.Get("Authorization"))
				require.Empty(t, r.header.Get("DD-API-KEY"))
				require.Empty(t, r.header.Get("DD-APPLICATION-KEY"))
				proof, err := jwt.ParseSigned(r.header.Get("X-Datadog-PAR-Proof"), []jose.SignatureAlgorithm{jose.ES256})
				require.NoError(t, err)
				var claims struct {
					jwt.Claims
					OrgID     int64  `json:"orgId"`
					RunnerID  string `json:"runnerId"`
					TokenHash string `json:"workload_token_hash"`
				}
				require.NoError(t, proof.Claims(&identity.PrivateKey.PublicKey, &claims))
				require.NoError(t, claims.Validate(jwt.Expected{Time: time.Now(), AnyAudience: jwt.Audience{"private-action-runner-wif-migration"}}))
				require.NotNil(t, claims.IssuedAt)
				require.NotNil(t, claims.Expiry)
				require.Positive(t, claims.Expiry.Time().Sub(claims.IssuedAt.Time()))
				require.LessOrEqual(t, claims.Expiry.Time().Sub(claims.IssuedAt.Time()), 2*time.Minute)
				require.EqualValues(t, 42, claims.OrgID)
				require.Equal(t, "runner-existing", claims.RunnerID)
				hash := sha256.Sum256([]byte(strings.TrimPrefix(r.header.Get("Authorization"), "Bearer ")))
				require.Equal(t, base64.RawURLEncoding.EncodeToString(hash[:]), claims.TokenHash)
			}
			saved, err := enrollment.GetIdentityFromPreviousEnrollment(context.Background(), cfg)
			require.NoError(t, err)
			require.True(t, saved.WIFEnrolled)
			require.Empty(t, saved.APIKeyHash)
			calls := tokenCalls
			for _, enabled := range []bool{true, false} {
				overrides["api_key"] = "rotated-again"
				overrides[par.WIFEnrollment] = enabled
				restarted := &PrivateActionRunner{coreConfig: coreconfig.NewMockWithOverrides(t, overrides), delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t)}
				recovered, err := restarted.getRunnerConfig(context.Background())
				require.NoError(t, err)
				require.Equal(t, identity.URN, recovered.Urn)
				require.True(t, identity.PrivateKey.Equal(recovered.PrivateKey))
			}
			require.Equal(t, calls, tokenCalls, "restart does not migrate again")
		})
	}
}

func TestWIFMigrationFailurePreservesIdentity(t *testing.T) {
	for _, scenario := range []string{"token denied", "PAR denied", "different runner", "different org", "different hostname", "invalid key", "component unavailable", "WIF disabled", "self-enrollment disabled"} {
		t.Run(scenario, func(t *testing.T) {
			flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
			t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
			var posts atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				assert.Equal(t, "runner-existing", r.URL.Query().Get("existing_runner_id"), "never fall back to fresh enrollment")
				if scenario == "different runner" {
					fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner-other","attributes":{"org_id":42,"runner_id":"runner-other"}}}`)
				} else if scenario == "different org" {
					fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner-existing","attributes":{"org_id":43,"runner_id":"runner-existing"}}}`)
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			}))
			defer server.Close()
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"delegated_auth.org_uuid": "test-org",
				"dd_url":                  server.URL, "skip_ssl_validation": true, "api_key": "original-key",
				par.WIFEnrollment: scenario != "WIF disabled", par.SelfEnroll: scenario != "self-enrollment disabled",
				par.IdentityFilePath: filepath.Join(t.TempDir(), "identity.json"),
			})
			identity := seedAPIKeyRunner(t, cfg)
			if scenario == "invalid key" {
				require.NoError(t, os.WriteFile(enrollment.GetIdentityFilePath(cfg), []byte(fmt.Sprintf(`{"urn":%q,"private_key":"invalid"}`, identity.URN)), 0600))
			}
			before, err := os.ReadFile(enrollment.GetIdentityFilePath(cfg))
			require.NoError(t, err)
			tokenCalls := 0
			auth := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(context.Context, string, common.WorkloadTokenRequest) (string, error) {
				tokenCalls++
				if scenario == "token denied" {
					return "", errors.New("mapping does not permit enrollment")
				}
				return "workload-token", nil
			}}
			host := "test-host"
			if scenario == "different hostname" {
				host = "different-host"
			}
			hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname(host))
			runner := &PrivateActionRunner{coreConfig: cfg, delegatedAuth: auth, hostnameGetter: hostname, logger: logmock.New(t)}
			if scenario == "component unavailable" {
				runner.delegatedAuth = nil
			}
			_, err = runner.getRunnerConfig(context.Background())
			if scenario == "WIF disabled" || scenario == "self-enrollment disabled" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				if scenario == "different runner" || scenario == "different org" {
					require.ErrorContains(t, err, "different runner identity")
				}
			}
			after, err := os.ReadFile(enrollment.GetIdentityFilePath(cfg))
			require.NoError(t, err)
			require.Equal(t, before, after)
			wantTokens, wantPosts := 0, 0
			switch scenario {
			case "token denied":
				wantTokens = 1
			case "PAR denied", "different runner", "different org":
				wantTokens, wantPosts = 1, 1
			}
			require.Equal(t, wantTokens, tokenCalls)
			require.EqualValues(t, wantPosts, posts.Load())
		})
	}
}
