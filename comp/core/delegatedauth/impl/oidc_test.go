// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package delegatedauthimpl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

const oidcTestOrgUUID = "11111111-2222-3333-4444-555555555555"

func oidcTestJWT(t *testing.T, aud, jti string) string {
	t.Helper()
	encode := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	claims := map[string]any{"aud": aud, "exp": time.Now().Add(time.Hour).Unix(), "jti": jti}
	return encode(map[string]string{"alg": "RS256"}) + "." + encode(claims) + ".c2lnbmF0dXJl"
}

type fakeIntakeKeyServer struct {
	*httptest.Server
	mu     sync.Mutex
	proofs []string
	keys   map[string]string
}

func newFakeIntakeKeyServer(t *testing.T) *fakeIntakeKeyServer {
	f := &fakeIntakeKeyServer{keys: map[string]string{}}
	f.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/intake-key" {
			http.NotFound(w, r)
			return
		}
		proof := r.Header.Get("Authorization")
		f.mu.Lock()
		f.proofs = append(f.proofs, proof)
		key, ok := f.keys[proof]
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `{"data":{"attributes":{"api_key":%q}}}`, key)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeIntakeKeyServer) accept(token, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys["Delegated oidc-"+token] = key
}

func (f *fakeIntakeKeyServer) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.proofs...)
}

func writeOIDCToken(t *testing.T, path, token string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path+".tmp", []byte(token), 0o600))
	require.NoError(t, os.Rename(path+".tmp", path))
}

// Covers the real provider, the HTTPS intake-key exchange, and API key writeback.
func TestOIDCExchangeRotationAndRefusal(t *testing.T) {
	server := newFakeIntakeKeyServer(t)
	config := mock.New(t)
	config.SetInTest("dd_url", server.URL)
	config.SetInTest("skip_ssl_validation", true)

	path := filepath.Join(t.TempDir(), "token")
	first := oidcTestJWT(t, "datadog/"+oidcTestOrgUUID, "first")
	server.accept(first, "first-key")
	writeOIDCToken(t, path, first)

	comp := NewComponent().Comp.(*delegatedAuthComponent)
	t.Cleanup(func() { require.NoError(t, comp.replaceInstance(context.Background(), "api_key", nil)) })
	require.NoError(t, comp.AddInstance(context.Background(), delegatedauth.InstanceParams{
		Config: config, OrgUUID: oidcTestOrgUUID, APIKeyConfigKey: "api_key", RefreshInterval: 60,
		ProviderConfig: &cloudauthconfig.OIDCProviderConfig{TokenFile: path},
	}))
	require.Equal(t, []string{"Delegated oidc-" + first}, server.received())
	require.Equal(t, "first-key", config.GetString("api_key"))
	instance := comp.instances["api_key"]
	require.Equal(t, "oidc", instance.providerName)

	second := oidcTestJWT(t, "datadog/"+oidcTestOrgUUID, "second")
	server.accept(second, "second-key")
	writeOIDCToken(t, path, second)
	_, err := comp.refreshAndWriteAPIKey(context.Background(), instance)
	require.NoError(t, err)
	require.Equal(t, "Delegated oidc-"+second, server.received()[1])
	require.Equal(t, "second-key", config.GetString("api_key"))

	// A token scoped elsewhere is refused locally and never reaches the exchange.
	writeOIDCToken(t, path, oidcTestJWT(t, "https://other.example", "unscoped"))
	_, err = comp.refreshAndWriteAPIKey(context.Background(), instance)
	require.ErrorContains(t, err, "not scoped to Datadog")
	require.Len(t, server.received(), 2)
	require.Equal(t, "second-key", config.GetString("api_key"))

	// A denied exchange (mapping removed) keeps the last key and never falls back to a static one.
	denied := oidcTestJWT(t, "datadog/"+oidcTestOrgUUID, "denied")
	writeOIDCToken(t, path, denied)
	_, err = comp.refreshAndWriteAPIKey(context.Background(), instance)
	require.Error(t, err)
	require.Equal(t, "Delegated oidc-"+denied, server.received()[2])
	require.Equal(t, "second-key", config.GetString("api_key"))
}

func TestOIDCInvalidConfigFailsAddInstance(t *testing.T) {
	config := mock.New(t)
	comp := NewComponent().Comp.(*delegatedAuthComponent)
	err := comp.AddInstance(context.Background(), delegatedauth.InstanceParams{
		Config: config, OrgUUID: oidcTestOrgUUID, APIKeyConfigKey: "api_key", RefreshInterval: 60,
		ProviderConfig: &cloudauthconfig.OIDCProviderConfig{TokenFile: "relative/token"},
	})
	require.ErrorContains(t, err, "absolute path")
}
