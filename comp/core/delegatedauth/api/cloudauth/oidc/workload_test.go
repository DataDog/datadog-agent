// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package oidc

import (
	"context"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/stretchr/testify/require"
)

func TestWorkloadProofRejectsUnsupportedRequestsBeforeAcquiringToken(t *testing.T) {
	a, err := New(tokenFunc(func(context.Context, string) (string, error) {
		t.Fatal("invalid requests must not acquire a provider credential")
		return "", nil
	}), "", "test", "claims")
	require.NoError(t, err)
	for _, request := range []common.WorkloadTokenRequest{
		{},
		{Grants: []string{"rc_secrets", "rc_secrets"}},
		{Grants: []string{"rc_secrets"}, JWKThumbprint: strings.Repeat("A", 43)},
	} {
		proof, err := a.GenerateWorkloadAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: "org"}, request)
		require.Error(t, err)
		require.Empty(t, proof)
	}
}

func TestWorkloadProofRefreshAndRouting(t *testing.T) {
	for _, mode := range []string{"claims", "request"} {
		t.Run(mode, func(t *testing.T) {
			current := "header.first.signature"
			calls := 0
			a, err := New(tokenFunc(func(_ context.Context, audience string) (string, error) {
				calls++
				require.Equal(t, "customer-audience", audience)
				return current, nil
			}), "customer-audience", "test", mode)
			require.NoError(t, err)
			request := common.WorkloadTokenRequest{Grants: []string{"rc_secrets"}}
			for _, token := range []string{"header.first.signature", "header.rotated.signature"} {
				current = token
				proof, err := a.GenerateWorkloadAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: "org"}, request)
				require.NoError(t, err)
				want := "oidc-" + token
				if mode == "request" {
					want += ":org"
				}
				require.Equal(t, want, proof)
			}
			require.Equal(t, 2, calls)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			proof, err := a.GenerateWorkloadAuthProof(ctx, nil, &common.AuthConfig{OrgUUID: "org"}, request)
			require.ErrorIs(t, err, context.Canceled)
			require.Empty(t, proof)
			require.Equal(t, 2, calls)
		})
	}
}
