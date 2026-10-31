// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/jsonapi"
	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

func TestEnrollWithWorkloadToken(t *testing.T) {
	publicKey, err := util.JWKToPEM(&jose.JSONWebKey{Key: &newTestKey(t).PublicKey})
	require.NoError(t, err)
	request := par.CreateRunnerRequest{RunnerName: "wif-agent", RunnerModes: []modes.Mode{modes.ModePull}, PublicKeyPEM: publicKey, AgentHostname: "host", AgentFlavor: "agent"}
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/unstable/on_prem_runners/workload_token", r.URL.Path)
		require.Equal(t, "Bearer workload-token", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("DD-API-KEY"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		body := new(par.CreateRunnerRequest)
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, jsonapi.Unmarshal(raw, body))
		require.Equal(t, request, *body)
		payload, err := jsonapi.Marshal(par.CreateRunnerResponse{ID: "runner-test", RunnerID: "runner-test", OrgID: 42, RunnerModes: []string{"pull"}})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, err = w.Write(payload)
		require.NoError(t, err)
	}))
	defer server.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("skip_ssl_validation", true)
	cfg.SetInTest("api_key", "existing-intake-key")
	client := NewPublicClient(cfg, server.URL, map[string]string{
		"DD-API-KEY": "unused-api-key", "DD-APPLICATION-KEY": "unused-app-key", "Authorization": "unused-auth",
	})
	response, err := client.EnrollWithWorkloadToken(context.Background(), "workload-token", request)
	require.NoError(t, err)
	require.Equal(t, "runner-test", response.RunnerID)
	require.EqualValues(t, 42, response.OrgID)
	require.Equal(t, 1, calls)
	require.Equal(t, "existing-intake-key", cfg.GetString("api_key"))
}

func TestWorkloadEnrollmentFailuresDoNotRetryOrExposeCredentials(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		status                 int
		body                   string
		rejected, unauthorized bool
	}{
		{"bad token", 401, "sensitive-body", true, true},
		{"disabled grant", 403, "sensitive-body", true, true},
		{"duplicate key", 409, "sensitive-body", true, false},
		{"rate limited", 429, "sensitive-body", false, false},
		{"unavailable", 503, "sensitive-body", false, false},
		{"redirect", 307, "sensitive-body", true, false},
		{"invalid response", 200, "sensitive-body", false, false},
		{"empty identity", 200, `{"data":{"type":"createRunnerResponse","id":"empty","attributes":{}}}`, false, false},
		{"oversized", 200, strings.Repeat("x", 1024*1024+1), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/elsewhere")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			cfg := configmock.New(t)
			cfg.SetInTest("skip_ssl_validation", true)
			response, err := NewPublicClient(cfg, server.URL, nil).EnrollWithWorkloadToken(context.Background(), "sensitive-token", par.CreateRunnerRequest{RunnerName: "test"})
			require.Error(t, err)
			require.Nil(t, response)
			require.NotContains(t, err.Error(), "sensitive")
			if tc.rejected {
				require.ErrorIs(t, err, ErrEnrollmentRejected)
			} else {
				require.NotErrorIs(t, err, ErrEnrollmentRejected)
			}
			if tc.unauthorized {
				require.ErrorIs(t, err, ErrEnrollmentUnauthorized)
			}
			require.Equal(t, 1, calls, "no automatic retry, redirect, or API-key fallback")
		})
	}
}

func TestWorkloadEnrollmentInvalidInputAndCancellation(t *testing.T) {
	cfg := configmock.New(t)
	_, err := NewPublicClient(cfg, "https://api.datadoghq.com", nil).EnrollWithWorkloadToken(context.Background(), "", par.CreateRunnerRequest{})
	require.ErrorContains(t, err, "token is required")
	_, err = NewPublicClient(cfg, "https://api.datadoghq.com", nil).EnrollWithWorkloadToken(context.Background(), "token", par.CreateRunnerRequest{ID: "existing-runner"})
	require.ErrorContains(t, err, "existing runner")
	_, err = NewPublicClient(cfg, "http://127.0.0.1:1", nil).EnrollWithWorkloadToken(context.Background(), "token", par.CreateRunnerRequest{})
	require.ErrorContains(t, err, "HTTPS")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewPublicClient(cfg, "https://127.0.0.1:1", nil).EnrollWithWorkloadToken(ctx, "token", par.CreateRunnerRequest{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestWorkloadEnrollmentUsesPublicClientTransport(t *testing.T) {
	calls := 0
	client := &publicClient{
		ddBaseURL: "https://api.datadoghq.com",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			require.Equal(t, "Bearer workload-token", req.Header.Get("Authorization"))
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"type":"createRunnerResponse","id":"runner-test","attributes":{"runner_id":"runner-test","org_id":42}}}`))}, nil
		})},
	}
	_, err := client.EnrollWithWorkloadToken(context.Background(), "workload-token", par.CreateRunnerRequest{RunnerName: "test"})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestWorkloadEnrollmentPreservesAPIKeyRedirectPolicy(t *testing.T) {
	redirected := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/result" {
			redirected++
			require.Equal(t, "api-key", r.Header.Get("DD-API-KEY"))
			fmt.Fprint(w, "{}")
			return
		}
		w.Header().Set("Location", "/result")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("skip_ssl_validation", true)
	client := NewPublicClient(cfg, server.URL, nil).(*publicClient)
	_, err := client.EnrollWithWorkloadToken(context.Background(), "workload-token", par.CreateRunnerRequest{RunnerName: "test"})
	require.ErrorIs(t, err, ErrEnrollmentRejected)
	require.Zero(t, redirected)
	_, _, err = client.doEnrollRequest(context.Background(), server.URL+createPARApiKeyOnlyPath, []byte("{}"), "api-key", "")
	require.NoError(t, err)
	require.Equal(t, 1, redirected)
}
