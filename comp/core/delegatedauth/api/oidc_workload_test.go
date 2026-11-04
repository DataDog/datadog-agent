// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/oidc"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

func TestOIDCFileToRCWorkloadExchange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	provider, err := oidc.FromConfig(&cloudauthconfig.OIDCProviderConfig{
		Source: "file", TokenFile: path, OrgUUIDSource: "claims",
	})
	require.NoError(t, err)
	current := "header.first.signature"
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v2/delegated-token?token_type=workload", r.URL.RequestURI())
		require.Equal(t, "Delegated oidc-"+current, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("DD-API-KEY"))
		var body struct {
			Data struct {
				Attributes common.WorkloadTokenRequest `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, common.WorkloadTokenRequest{Grants: []string{"rc_secrets"}}, body.Data.Attributes)
		fmt.Fprint(w, `{"data":{"type":"token","attributes":{"access_token":"opaque-rc-token"}}}`)
	}))
	defer server.Close()
	cfg := mock.New(t)
	cfg.SetInTest("dd_url", server.URL)
	cfg.SetInTest("skip_ssl_validation", true) // httptest uses a self-signed certificate.
	cfg.SetInTest("api_key", "existing-intake-key")
	for _, proof := range []string{"header.first.signature", "header.rotated.signature"} {
		current = proof
		require.NoError(t, os.WriteFile(path+".new", []byte(proof), 0600))
		require.NoError(t, os.Rename(path+".new", path))
		token, err := GetWorkloadToken(context.Background(), cfg, provider, &common.AuthConfig{OrgUUID: "org"}, common.WorkloadTokenRequest{Grants: []string{"rc_secrets"}}, "")
		require.NoError(t, err)
		require.Equal(t, "opaque-rc-token", token)
	}
	require.Equal(t, 2, calls)
	require.Equal(t, "existing-intake-key", cfg.GetString("api_key"))
}
