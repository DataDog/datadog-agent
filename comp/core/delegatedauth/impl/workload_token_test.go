// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package delegatedauthimpl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workloadProvider struct {
	common.Provider // Intake proof generation must not be called.
	proof           func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig, common.WorkloadTokenRequest) (string, error)
}

func (p workloadProvider) GenerateWorkloadAuthProof(ctx context.Context, cfg pkgconfigmodel.Reader, auth *common.AuthConfig, request common.WorkloadTokenRequest) (string, error) {
	return p.proof(ctx, cfg, auth, request)
}

func TestWorkloadTokenUsesSelectedInstance(t *testing.T) {
	cfg := mock.New(t)
	cfg.SetInTest("skip_ssl_validation", true)
	cfg.SetInTest("api_key", "primary-intake-key")
	cfg.SetInTest("logs_config.api_key", "logs-intake-key")
	d := NewComponent().Comp.(*delegatedAuthComponent)
	d.config = cfg
	request := common.WorkloadTokenRequest{Grants: []string{"par_enrollment"}, JWKThumbprint: strings.Repeat("A", 43)}
	for _, key := range []string{"api_key", "logs_config.api_key"} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/delegated-token?token_type=workload", r.URL.RequestURI())
			assert.Equal(t, "Delegated proof-for-"+key, r.Header.Get("Authorization"))
			assert.Empty(t, r.Header.Get("DD-API-KEY"))
			fmt.Fprintf(w, `{"data":{"type":"token","attributes":{"access_token":%q}}}`, "token-for-"+key)
		}))
		t.Cleanup(server.Close)
		instance := &authInstance{
			authConfig: &common.AuthConfig{OrgUUID: "org-for-" + key},
			targetSite: server.URL,
			provider: workloadProvider{proof: func(_ context.Context, gotCfg pkgconfigmodel.Reader, auth *common.AuthConfig, got common.WorkloadTokenRequest) (string, error) {
				require.Same(t, cfg, gotCfg)
				require.Equal(t, "org-for-"+key, auth.OrgUUID)
				require.Equal(t, request, got)
				// Status reads and refresh-state updates must remain possible during exchange.
				d.mu.Lock()
				d.mu.Unlock()
				return "proof-for-" + key, nil
			}},
		}
		if key == "api_key" {
			cfg.SetInTest("dd_url", server.URL)
			instance.targetSite = "" // Primary-site fallback uses the existing transport config.
		}
		d.instances[key] = instance
	}
	for _, key := range []string{"api_key", "logs_config.api_key"} {
		token, err := d.GetWorkloadToken(context.Background(), key, request)
		require.NoError(t, err)
		require.Equal(t, "token-for-"+key, token)
		require.Zero(t, d.instances[key].consecutiveFailures)
		require.True(t, d.instances[key].lastRefresh.IsZero())
	}
	require.Equal(t, "primary-intake-key", cfg.GetString("api_key"))
	require.Equal(t, "logs-intake-key", cfg.GetString("logs_config.api_key"))
}

func TestWorkloadTokenUnavailable(t *testing.T) {
	d := NewComponent().Comp.(*delegatedAuthComponent)
	request := common.WorkloadTokenRequest{Grants: []string{"par_enrollment"}}
	_, err := d.GetWorkloadToken(context.Background(), "api_key", request)
	require.ErrorContains(t, err, "unavailable")
	// A different configured instance must not be used as a fallback.
	d.instances["logs_config.api_key"] = &authInstance{provider: workloadProvider{}}
	_, err = d.GetWorkloadToken(context.Background(), "api_key", request)
	require.ErrorContains(t, err, "unavailable")
	d.instances["api_key"] = &authInstance{}
	_, err = d.GetWorkloadToken(context.Background(), "api_key", request)
	require.ErrorContains(t, err, "does not support workload tokens")
}

func TestWorkloadTokenHonorsCancellation(t *testing.T) {
	d := NewComponent().Comp.(*delegatedAuthComponent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	token, err := d.GetWorkloadToken(ctx, "api_key", common.WorkloadTokenRequest{})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, token)
}

func TestWorkloadTokenRejectsRemovedAdditionalEndpoint(t *testing.T) {
	cfg := mock.New(t)
	cfg.SetInTest("additional_endpoints", map[string][]string{})
	d := NewComponent().Comp.(*delegatedAuthComponent)
	d.config = cfg
	d.instances["additional-key"] = &authInstance{
		provider:                     workloadProvider{}, // Must reject before attempting to generate a proof.
		additionalEndpointDomain:     "datadoghq.eu",
		additionalEndpointsConfigKey: "additional_endpoints",
		originalDirective:            "DELA[org]",
	}
	token, err := d.GetWorkloadToken(context.Background(), "additional-key", common.WorkloadTokenRequest{Grants: []string{"par_enrollment"}})
	require.ErrorIs(t, err, errWritebackTargetChanged)
	require.Empty(t, token)
}
