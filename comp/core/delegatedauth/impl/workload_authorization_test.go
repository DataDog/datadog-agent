// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package delegatedauthimpl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

type authorizationTestProvider func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig) (string, error)

func (f authorizationTestProvider) GenerateAuthProof(ctx context.Context, cfg pkgconfigmodel.Reader, auth *common.AuthConfig) (string, error) {
	return f(ctx, cfg, auth)
}

func TestWorkloadAuthorizationSelectsInstance(t *testing.T) {
	const orgUUID = "a1b2c3d4-e5f6-4890-abcd-ef1234567890"
	thumbprint := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, replaceInstance := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected instance", true: "concurrent replacement"}[replaceInstance], func(t *testing.T) {
			cfg := mock.New(t)
			cfg.Set("api_key", "unchanged-api-key", pkgconfigmodel.SourceAgentRuntime)
			comp := &delegatedAuthComponent{config: cfg, instances: make(map[string]*authInstance)}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Delegated selected-proof", r.Header.Get("Authorization"))
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"type": "workload_authorization", "attributes": map[string]any{
						"access_token": "assertion", "expires_at": time.Now().Add(time.Minute).Format(time.RFC3339),
						"org_id": 123, "org_uuid": orgUUID, "provider": "aws",
						"intake_mapping_id": "c3f78a55-2238-44d7-902f-befb8e74d640", "stable_principal": "role/*",
					},
				}}))
			}))
			defer server.Close()
			proofCalls := 0
			comp.instances["api_key"] = &authInstance{
				targetSite: server.URL, authConfig: &common.AuthConfig{OrgUUID: orgUUID},
				provider: authorizationTestProvider(func(_ context.Context, _ pkgconfigmodel.Reader, auth *common.AuthConfig) (string, error) {
					proofCalls++
					require.Equal(t, orgUUID, auth.OrgUUID)
					if replaceInstance {
						comp.mu.Lock()
						comp.instances["api_key"] = &authInstance{}
						comp.mu.Unlock()
					}
					return "selected-proof", nil
				}),
			}
			comp.instances["logs_config.api_key"] = &authInstance{
				authConfig: &common.AuthConfig{OrgUUID: "other-org"},
				provider: authorizationTestProvider(func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig) (string, error) {
					t.Fatal("wrong instance selected")
					return "", nil
				}),
			}
			result, err := comp.GetWorkloadAuthorization(context.Background(), "api_key", thumbprint)
			if replaceInstance {
				require.ErrorIs(t, err, common.ErrWorkloadAuthorizationUnavailable)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, "assertion", result.Token)
				_, err = comp.GetWorkloadAuthorization(context.Background(), "api_key", thumbprint)
				require.NoError(t, err)
				require.Equal(t, 2, proofCalls, "each call must generate a fresh proof")
			}
			require.Equal(t, "unchanged-api-key", cfg.GetString("api_key"))
		})
	}
}

func TestWorkloadAuthorizationUnavailableAndProofFailure(t *testing.T) {
	comp := &delegatedAuthComponent{config: mock.New(t), instances: make(map[string]*authInstance)}
	_, err := comp.GetWorkloadAuthorization(context.Background(), "api_key", "thumbprint")
	require.ErrorIs(t, err, common.ErrWorkloadAuthorizationUnavailable)
	comp.instances["api_key"] = &authInstance{
		authConfig: &common.AuthConfig{OrgUUID: "org"},
		provider: authorizationTestProvider(func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig) (string, error) {
			return "", errors.New("secret-provider-credential")
		}),
	}
	_, err = comp.GetWorkloadAuthorization(context.Background(), "api_key", "thumbprint")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-provider-credential")
}
