// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type workloadAuthorizerFunc func(context.Context, string, string) (*common.WorkloadAuthorization, error)

func (f workloadAuthorizerFunc) GetWorkloadAuthorization(c context.Context, k, t string) (*common.WorkloadAuthorization, error) {
	return f(c, k, t)
}

func TestWorkloadEnrollmentPersistsBeforeRequestAndReusesKeyAfterRestart(t *testing.T) {
	oldFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.DefaultAgent)
	t.Cleanup(func() { flavor.SetFlavor(oldFlavor) })
	cfg := mock.New(t)
	path := filepath.Join(t.TempDir(), "identity.json")
	cfg.Set(setup.PARIdentityFilePath, path, configModel.SourceAgentRuntime)
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	var pendingKey string
	attempts := 0
	fail := true
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		require.Empty(t, r.Header.Get("DD-API-KEY"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		identity, err := getIdentityFromFile(cfg)
		require.NoError(t, err)
		require.True(t, identity.Pending)
		if pendingKey == "" {
			pendingKey = identity.PrivateKey
		} else {
			require.Equal(t, pendingKey, identity.PrivateKey)
		}
		if fail {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"type":"createRunnerResponse","id":"runner-1","attributes":{"runner_id":"runner-1","org_id":123,"authorization_version":1,"runner_modes":["pull"]}}}`))
	}))
	defer server.Close()
	cfg.Set("dd_url", server.URL, configModel.SourceAgentRuntime)
	proofCalls := 0
	authorizer := workloadAuthorizerFunc(func(_ context.Context, key, jkt string) (*common.WorkloadAuthorization, error) {
		proofCalls++
		require.Equal(t, "api_key", key)
		identity, err := getIdentityFromFile(cfg)
		require.NoError(t, err)
		require.NotNil(t, identity)
		jwk, err := util.Base64ToJWK(identity.PrivateKey)
		require.NoError(t, err)
		thumbprint, err := jwk.Thumbprint(crypto.SHA256)
		require.NoError(t, err)
		require.Equal(t, base64.RawURLEncoding.EncodeToString(thumbprint), jkt)
		return &common.WorkloadAuthorization{Token: "short-lived-assertion", OrgID: 123, IntakeMappingID: "mapping-1", Provider: "aws"}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, err := EnrollWorkloadIdentity(ctx, cfg, &AgentIdentifier{Hostname: "host-1"}, authorizer)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.GreaterOrEqual(t, proofCalls, 2)
	mu.Lock()
	fail = false
	mu.Unlock()
	result, err := EnrollWorkloadIdentity(context.Background(), cfg, &AgentIdentifier{Hostname: "host-1"}, authorizer)
	require.NoError(t, err)
	require.Equal(t, WorkloadIdentityAuthorization, result.AuthorizationType)
	identity, err := getIdentityFromFile(cfg)
	require.NoError(t, err)
	require.Equal(t, pendingKey, identity.PrivateKey)
	require.False(t, identity.Pending)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "short-lived-assertion")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	require.False(t, ShouldReenroll(&AgentIdentifier{Hostname: "host-1"}, identity, "rotated-api-key"))
	require.GreaterOrEqual(t, attempts, 3)
}

func TestWorkloadReauthorizationPreservesIdentityAndBindsProof(t *testing.T) {
	cfg := mock.New(t)
	cfg.Set(setup.PARIdentityFilePath, filepath.Join(t.TempDir(), "identity"), configModel.SourceAgentRuntime)
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	private, _, err := util.GenerateKeys()
	require.NoError(t, err)
	result := &Result{PrivateKey: private.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 123, "runner-1"), Hostname: "host-1", APIKeyHash: HashAPIKey("old-key")}
	require.NoError(t, PersistIdentity(context.Background(), cfg, result))
	before, err := getIdentityFromFile(cfg)
	require.NoError(t, err)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/api/unstable/on_prem_runners/runner-1/reauthorize", r.URL.Path)
		require.Equal(t, "Bearer assertion", r.Header.Get("Authorization"))
		proof, err := jwt.Parse(r.Header.Get("X-Datadog-PAR-Proof"), func(*jwt.Token) (any, error) { return &result.PrivateKey.PublicKey, nil }, jwt.WithValidMethods([]string{"ES256"}), jwt.WithExpirationRequired())
		require.NoError(t, err)
		claims := proof.Claims.(jwt.MapClaims)
		require.Equal(t, "runner-1", claims["runnerId"])
		require.Equal(t, float64(0), claims["expected_authorization_version"])
		hash := sha256.Sum256([]byte("assertion"))
		require.Equal(t, base64.RawURLEncoding.EncodeToString(hash[:]), claims["assertion_hash"])
		_, _ = w.Write([]byte(`{"data":{"type":"createRunnerResponse","id":"runner-1","attributes":{"runner_id":"runner-1","org_id":123,"authorization_version":1,"runner_modes":["pull"]}}}`))
	}))
	defer server.Close()
	cfg.Set("dd_url", server.URL, configModel.SourceAgentRuntime)
	authorizer := workloadAuthorizerFunc(func(context.Context, string, string) (*common.WorkloadAuthorization, error) {
		return &common.WorkloadAuthorization{Token: "assertion", OrgID: 123, IntakeMappingID: "mapping-2", Provider: "aws"}, nil
	})
	require.NoError(t, RefreshWorkloadIdentity(context.Background(), cfg, authorizer))
	after, err := getIdentityFromFile(cfg)
	require.NoError(t, err)
	require.Equal(t, before.PrivateKey, after.PrivateKey)
	require.Equal(t, before.URN, after.URN)
	require.Empty(t, after.APIKeyHash)
	require.Equal(t, "mapping-2", after.IntakeMappingID)
	require.NoError(t, RefreshWorkloadIdentity(context.Background(), cfg, authorizer))
	require.Equal(t, 1, calls)
	failure := workloadAuthorizerFunc(func(context.Context, string, string) (*common.WorkloadAuthorization, error) {
		return nil, errors.New("ETS unavailable")
	})
	require.Error(t, RefreshWorkloadIdentity(context.Background(), cfg, failure))
	saved, err := getIdentityFromFile(cfg)
	require.NoError(t, err)
	require.Equal(t, after, saved)
}

func TestPendingIdentityClaimDoesNotOverwriteAnotherKey(t *testing.T) {
	cfg := mock.New(t)
	cfg.Set(setup.PARIdentityFilePath, filepath.Join(t.TempDir(), "identity"), configModel.SourceAgentRuntime)
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	first, err := claimPendingIdentity(context.Background(), cfg, &Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), AuthorizationType: WorkloadIdentityAuthorization, Pending: true})
	require.NoError(t, err)
	other, _, err := util.GenerateKeys()
	require.NoError(t, err)
	second, err := claimPendingIdentity(context.Background(), cfg, &Result{PrivateKey: other.Key.(*ecdsa.PrivateKey), AuthorizationType: WorkloadIdentityAuthorization, Pending: true})
	require.NoError(t, err)
	require.Equal(t, first.PrivateKey, second.PrivateKey)
	raw, err := json.Marshal(first)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "access_token")
}
