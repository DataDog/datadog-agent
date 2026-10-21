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
	"os"
	"path/filepath"
	"testing"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

// Exercise the actual provider -> HTTPS ETS client -> config writeback path,
// including token rotation and a rejected exchange preserving the last key.
func TestOIDCExchangeAndRefresh(t *testing.T) {
	proofs := make(chan string, 3)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/intake-key" {
			http.NotFound(w, r)
			return
		}
		proof := r.Header.Get("Authorization")
		proofs <- proof
		switch proof {
		case "Delegated oidc-header.first.signature:org-123":
			fmt.Fprint(w, `{"data":{"attributes":{"api_key":"first-key"}}}`)
		case "Delegated oidc-header.second.signature:org-123":
			fmt.Fprint(w, `{"data":{"attributes":{"api_key":"second-key"}}}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()
	config := mock.New(t)
	config.SetInTest("dd_url", server.URL)
	config.SetInTest("skip_ssl_validation", true) // Trust only the test's self-signed server.
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("header.first.signature"), 0600))
	comp := NewComponent().Comp.(*delegatedAuthComponent)
	t.Cleanup(func() { require.NoError(t, comp.replaceInstance(context.Background(), "api_key", nil)) })
	require.NoError(t, comp.AddInstance(context.Background(), delegatedauth.InstanceParams{
		Config: config, OrgUUID: "org-123", APIKeyConfigKey: "api_key", RefreshInterval: 60,
		ProviderConfig: &cloudauthconfig.OIDCProviderConfig{Source: "file", TokenFile: path},
	}))
	require.Equal(t, "Delegated oidc-header.first.signature:org-123", <-proofs)
	require.Equal(t, "first-key", config.GetString("api_key"))
	instance := comp.instances["api_key"]
	require.Equal(t, "oidc", instance.providerName)
	require.NoError(t, os.WriteFile(path+".new", []byte("header.second.signature"), 0600))
	require.NoError(t, os.Rename(path+".new", path))
	_, err := comp.refreshAndWriteAPIKey(context.Background(), instance)
	require.NoError(t, err)
	require.Equal(t, "Delegated oidc-header.second.signature:org-123", <-proofs)
	require.Equal(t, "second-key", config.GetString("api_key"))
	require.NoError(t, os.WriteFile(path, []byte("header.denied.signature"), 0600))
	_, err = comp.refreshAndWriteAPIKey(context.Background(), instance)
	require.Error(t, err)
	require.Equal(t, "Delegated oidc-header.denied.signature:org-123", <-proofs)
	require.Equal(t, "second-key", config.GetString("api_key"))
}
