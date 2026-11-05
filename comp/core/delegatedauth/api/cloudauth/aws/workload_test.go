// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

func TestWorkloadProofSignsGrantsAndKey(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_SESSION_TOKEN", "test-session")
	cfg := configmock.New(t)
	auth := NewAWSAuth(nil)
	config := &common.AuthConfig{OrgUUID: "test-org"}
	request := common.WorkloadTokenRequest{Grants: []string{"par_enrollment", "another_grant"}, JWKThumbprint: strings.Repeat("A", 43)}
	for _, workload := range []bool{true, false} {
		var proof string
		var err error
		if workload {
			proof, err = auth.GenerateWorkloadAuthProof(context.Background(), cfg, config, request)
		} else {
			proof, err = auth.GenerateAuthProof(context.Background(), cfg, config)
		}
		require.NoError(t, err)
		parts := strings.Split(proof, "|")
		require.Len(t, parts, 4)
		raw, err := base64.StdEncoding.DecodeString(parts[1])
		require.NoError(t, err)
		var headers http.Header
		require.NoError(t, json.Unmarshal(raw, &headers))
		require.Equal(t, "test-org", headers.Get("X-Ddog-Org-Id"))
		signed := headers.Get("Authorization")
		require.Contains(t, signed, "x-ddog-org-id")
		if workload {
			require.Equal(t, "par_enrollment another_grant", headers.Get("X-Ddog-Workload-Grants"))
			require.Equal(t, request.JWKThumbprint, headers.Get("X-Ddog-Workload-Jwk-Thumbprint"))
			require.Contains(t, signed, "x-ddog-workload-grants")
			require.Contains(t, signed, "x-ddog-workload-jwk-thumbprint")
		} else {
			require.Empty(t, headers.Get("X-Ddog-Workload-Grants"))
			require.Empty(t, headers.Get("X-Ddog-Workload-Jwk-Thumbprint"))
			require.NotContains(t, signed, "x-ddog-workload")
		}
	}
	request.JWKThumbprint = ""
	proof, err := auth.GenerateWorkloadAuthProof(context.Background(), cfg, config, request)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(strings.Split(proof, "|")[1])
	require.NoError(t, err)
	var headers http.Header
	require.NoError(t, json.Unmarshal(raw, &headers))
	require.Empty(t, headers.Get("X-Ddog-Workload-Jwk-Thumbprint"))
	require.NotContains(t, headers.Get("Authorization"), "x-ddog-workload-jwk-thumbprint")
}
