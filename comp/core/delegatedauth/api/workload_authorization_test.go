// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

const testOrgUUID = "a1b2c3d4-e5f6-4890-abcd-ef1234567890"

func authorizationResponse(t *testing.T, now time.Time) map[string]any {
	t.Helper()
	return map[string]any{"data": map[string]any{
		"type": "workload_authorization",
		"attributes": map[string]any{
			"access_token": "sensitive-assertion", "expires_at": now.Add(time.Minute).Format(time.RFC3339),
			"org_id": 123, "org_uuid": testOrgUUID, "provider": "aws",
			"intake_mapping_id": "c3f78a55-2238-44d7-902f-befb8e74d640", "stable_principal": "role/*",
		},
	}}
}

func TestGetWorkloadAuthorization(t *testing.T) {
	thumbprint := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, workloadAuthorizationPath, r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Delegated provider-proof", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("DD-API-KEY"))
		var request struct {
			Data struct {
				Type       string
				Attributes map[string]string
			}
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "workload_authorization_request", request.Data.Type)
		require.Equal(t, common.PAREnrollmentPurpose, request.Data.Attributes["purpose"])
		require.Equal(t, thumbprint, request.Data.Attributes["jwk_thumbprint"])
		require.NoError(t, json.NewEncoder(w).Encode(authorizationResponse(t, time.Now())))
	}))
	defer server.Close()
	result, err := GetWorkloadAuthorization(context.Background(), mock.New(t), "provider-proof", server.URL, testOrgUUID, thumbprint)
	require.NoError(t, err)
	require.Equal(t, "sensitive-assertion", result.Token)
	require.Equal(t, testOrgUUID, result.OrgUUID)
	require.Equal(t, uint64(123), result.OrgID)
	require.NotContains(t, fmt.Sprintf("%+v %#v", result, result), result.Token)
	serialized, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), result.Token)
}

func TestWorkloadAuthorizationResponseValidation(t *testing.T) {
	now := time.Unix(1789660500, 0)
	for name, replacement := range map[string]any{
		"access_token": "", "org_uuid": "another-org", "org_id": 0,
		"provider": "unknown", "intake_mapping_id": "", "stable_principal": "",
		"expires_at": now.Add(-time.Minute).Format(time.RFC3339),
	} {
		t.Run(name, func(t *testing.T) {
			response := authorizationResponse(t, now)
			response["data"].(map[string]any)["attributes"].(map[string]any)[name] = replacement
			raw, err := json.Marshal(response)
			require.NoError(t, err)
			_, err = parseWorkloadAuthorization(raw, testOrgUUID, now)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "sensitive-assertion")
		})
	}
	_, err := parseWorkloadAuthorization([]byte(`{"sensitive-assertion":invalid}`), testOrgUUID, now)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "sensitive-assertion")
}

func TestWorkloadAuthorizationErrorsDoNotExposeCredentials(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"errors":[{"detail":"sensitive-assertion"}]}`))
			}))
			defer server.Close()
			_, err := GetWorkloadAuthorization(context.Background(), mock.New(t), "sensitive-proof", server.URL, testOrgUUID, base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
			require.ErrorContains(t, err, fmt.Sprint(status))
			require.NotContains(t, err.Error(), "sensitive")
		})
	}
}

func TestWorkloadAuthorizationRejectsRedirectsAndLargeResponses(t *testing.T) {
	var redirected atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Store(true) }))
	defer destination.Close()
	for _, redirect := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
				return
			}
			_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBodySize+1)))
		}))
		_, err := GetWorkloadAuthorization(context.Background(), mock.New(t), "proof", server.URL, testOrgUUID, base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
		server.Close()
		require.Error(t, err)
		require.False(t, redirected.Load())
	}
}

func TestWorkloadAuthorizationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetWorkloadAuthorization(ctx, mock.New(t), "proof", "http://127.0.0.1:1", testOrgUUID, base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	require.ErrorIs(t, err, context.Canceled)
}
