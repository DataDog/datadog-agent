// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestRunnerCredentialHTTPBasicAuthSmoke(t *testing.T) {
	for _, test := range []struct {
		name              string
		usernameReference bool
		legacyFile        bool
		passwordKey       string
		allowedActions    string
		allowedDomains    string
		wantError         string
	}{
		{name: "plaintext username", passwordKey: "http_password"},
		{name: "runner username", usernameReference: true, passwordKey: "http_password"},
		{name: "legacy file", legacyFile: true},
		{name: "missing password reference", passwordKey: "missing", wantError: "requested runner credential is not available"},
		{name: "action denied", passwordKey: "http_password", allowedActions: "[]", wantError: "is not allowed for action"},
		{name: "destination denied", passwordKey: "http_password", allowedDomains: "[other.example.com]", wantError: "request domain is not allowed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				username, password, ok := r.BasicAuth()
				if r.URL.Path != "/basic-auth" || !ok || username != "postman" || password != "password" || r.Header.Get("X-Smoke") != "basic-v2" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"authenticated":true}`))
			}))
			defer server.Close()
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			domains := test.allowedDomains
			if domains == "" {
				domains = fmt.Sprintf("[%s]", target.Hostname())
			}
			actions := test.allowedActions
			if actions == "" {
				actions = "[com.datadoghq.http.request]"
			}
			catalog := configmock.NewFromYAML(t, fmt.Sprintf(`
private_action_runner:
  credentials:
    values:
      http_username:
        value: postman
        allowed_actions: %s
        allowed_http_domains: %s
      http_password:
        value: password
        allowed_actions: %s
        allowed_http_domains: %s
`, actions, domains, actions, domains))
			credentialResolver, err := resolver.NewPrivateCredentialResolver(catalog)
			require.NoError(t, err)

			plainToken := func(segments []string, value string) *privateactionspb.ConnectionTokenV2 {
				return &privateactionspb.ConnectionTokenV2{
					NameSegments: segments,
					Source: &privateactionspb.ConnectionTokenV2_PlainText_{
						PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: value},
					},
				}
			}
			referenceToken := func(name, key string) *privateactionspb.ConnectionTokenV2 {
				return &privateactionspb.ConnectionTokenV2{
					NameSegments: []string{"root_tokens", name},
					Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
						RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: key},
					},
				}
			}
			conn := &privateactionspb.ConnectionInfo{CredentialsType: privateactionspb.CredentialsType_BASIC_AUTH}
			if test.legacyFile {
				credentialsPath := filepath.Join(t.TempDir(), "basic.json")
				require.NoError(t, os.WriteFile(credentialsPath, []byte(`{"auth_type":"Basic Auth","credentials":[{"username":"postman","password":"password"}]}`), 0600))
				conn.Tokens = []*privateactionspb.ConnectionToken{
					privateconnection.NewPlainTextToken([]string{"root_tokens", "username"}, "postman"),
					privateconnection.NewFileSecretToken([]string{"root_tokens", "password"}, credentialsPath),
					privateconnection.NewPlainTextToken([]string{"base_url"}, server.URL),
					privateconnection.NewPlainTextToken([]string{"headers", "X-Smoke"}, "basic-v2"),
				}
			} else {
				username := plainToken([]string{"root_tokens", "username"}, "postman")
				if test.usernameReference {
					username = referenceToken("username", "http_username")
				}
				conn.TokensV2 = []*privateactionspb.ConnectionTokenV2{
					username,
					referenceToken("password", test.passwordKey),
					plainToken([]string{"base_url"}, server.URL),
					plainToken([]string{"headers", "X-Smoke"}, "basic-v2"),
				}
			}
			task := &types.Task{}
			task.Data.Attributes = &types.Attributes{
				BundleID: "com.datadoghq.http", Name: "request",
				Inputs:         map[string]interface{}{"verb": "GET", "url": server.URL + "/basic-auth"},
				ConnectionInfo: conn,
			}
			credentials, err := credentialResolver.ResolveCredentials(context.Background(), task)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, credentials)
				require.Zero(t, requests.Load())
				return
			}
			require.NoError(t, err)
			require.Equal(t, privateconnection.BasicAuthType, credentials.Type)
			handler := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second, Allowlist: []string{target.Hostname()}})
			output, err := handler.Run(context.Background(), task, credentials)
			require.NoError(t, err)
			response, ok := output.(*RequestOutputs)
			require.True(t, ok)
			require.Equal(t, http.StatusOK, response.Status)
			require.Equal(t, map[string]interface{}{"authenticated": true}, response.Body)
			require.EqualValues(t, 1, requests.Load())
		})
	}
}
