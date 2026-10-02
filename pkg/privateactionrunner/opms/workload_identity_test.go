// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/stretchr/testify/require"
)

func TestWorkloadExchangeWithoutVersionDoesNotLeakCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer assertion", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("DD-API-KEY"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		require.Equal(t, "proof", r.Header.Get("X-Datadog-PAR-Proof"))
		var body struct {
			Data struct {
				Attributes struct {
					PublicKey string `json:"public_key_pem"`
				}
			}
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "key", body.Data.Attributes.PublicKey)
		_, _ = w.Write([]byte(`{"data":{"type":"createRunnerResponse","attributes":{"runner_id":"runner","org_id":123}}}`))
	}))
	defer server.Close()
	result, err := ExchangeWorkloadIdentity(context.Background(), mock.New(t), server.URL, "assertion", "runner", "proof", &par.CreateRunnerRequest{PublicKeyPEM: "key"}, map[string]string{"DD-API-KEY": "secret", "DD-APPLICATION-KEY": "secret", "Authorization": "overridden"})
	require.NoError(t, err)
	require.Equal(t, "runner", result.RunnerID)
}
func TestWorkloadExchangeRejectsRedirectsAndInvalidResponses(t *testing.T) {
	redirected := false
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer destination.Close()
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "redirect", status: 307},
		{name: "changed runner", status: 200, body: `{"data":{"type":"createRunnerResponse","attributes":{"runner_id":"another","org_id":123}}}`},
		{name: "missing org", status: 200, body: `{"data":{"type":"createRunnerResponse","attributes":{"runner_id":"runner"}}}`},
		{name: "oversized", status: 200, body: strings.Repeat("x", (1<<20)+1)},
		{name: "server error", status: 503, body: "private error secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", destination.URL)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := ExchangeWorkloadIdentity(context.Background(), mock.New(t), server.URL, "assertion", "runner", "proof", &par.CreateRunnerRequest{PublicKeyPEM: "key"}, nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "assertion")
			require.False(t, redirected)
		})
	}
}
