// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package azureauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tokenJSON(accessToken string, expiresOn int64) string {
	return fmt.Sprintf(`{"access_token":"%s","expires_on":"%d"}`, accessToken, expiresOn)
}

func TestAppServiceToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "test-header", r.Header.Get("X-IDENTITY-HEADER"))
		assert.Contains(t, r.URL.RawQuery, "api-version=2019-08-01")
		assert.Contains(t, r.URL.RawQuery, "resource=https%3A%2F%2Fvault.azure.net")

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, tokenJSON("app-service-token", time.Now().Add(time.Hour).Unix()))
	}))
	defer server.Close()

	t.Setenv("IDENTITY_ENDPOINT", server.URL)
	t.Setenv("IDENTITY_HEADER", "test-header")
	t.Setenv("MSI_ENDPOINT", "")
	t.Setenv("MSI_SECRET", "")

	p := NewManagedIdentityTokenProvider("")
	tok, err := p.GetToken(context.Background(), "https://vault.azure.net")
	require.NoError(t, err)
	assert.Equal(t, "app-service-token", tok.AccessToken)
}

func TestAppServiceTokenWithClientID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "client_id=my-client-id")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, tokenJSON("app-service-token-client", time.Now().Add(time.Hour).Unix()))
	}))
	defer server.Close()

	t.Setenv("IDENTITY_ENDPOINT", server.URL)
	t.Setenv("IDENTITY_HEADER", "test-header")
	t.Setenv("MSI_ENDPOINT", "")
	t.Setenv("MSI_SECRET", "")

	p := NewManagedIdentityTokenProvider("my-client-id")
	tok, err := p.GetToken(context.Background(), "https://vault.azure.net")
	require.NoError(t, err)
	assert.Equal(t, "app-service-token-client", tok.AccessToken)
}

func TestAzureMLToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "ml-secret", r.Header.Get("Secret"))
		assert.Contains(t, r.URL.RawQuery, "api-version=2017-09-01")

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, tokenJSON("ml-token", time.Now().Add(time.Hour).Unix()))
	}))
	defer server.Close()

	t.Setenv("IDENTITY_ENDPOINT", "")
	t.Setenv("IDENTITY_HEADER", "")
	t.Setenv("MSI_ENDPOINT", server.URL)
	t.Setenv("MSI_SECRET", "ml-secret")

	p := NewManagedIdentityTokenProvider("")
	tok, err := p.GetToken(context.Background(), "https://vault.azure.net")
	require.NoError(t, err)
	assert.Equal(t, "ml-token", tok.AccessToken)
}

func TestCloudShellToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "true", r.Header.Get("Metadata"))
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, tokenJSON("cloud-shell-token", time.Now().Add(time.Hour).Unix()))
	}))
	defer server.Close()

	t.Setenv("IDENTITY_ENDPOINT", "")
	t.Setenv("IDENTITY_HEADER", "")
	t.Setenv("MSI_ENDPOINT", server.URL)
	t.Setenv("MSI_SECRET", "")

	p := NewManagedIdentityTokenProvider("")
	tok, err := p.GetToken(context.Background(), "https://vault.azure.net")
	require.NoError(t, err)
	assert.Equal(t, "cloud-shell-token", tok.AccessToken)
}

func TestArcToken(t *testing.T) {
	secretFile, err := os.CreateTemp("", "arc-secret")
	require.NoError(t, err)
	defer os.Remove(secretFile.Name())
	_, err = secretFile.WriteString("arc-secret-content")
	require.NoError(t, err)
	secretFile.Close()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: return 401 with secret file path.
			w.Header().Set("WWW-Authenticate", "Basic realm="+secretFile.Name())
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Second call: verify Authorization header.
		assert.Equal(t, "Basic arc-secret-content", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, tokenJSON("arc-token", time.Now().Add(time.Hour).Unix()))
	}))
	defer server.Close()

	t.Setenv("IDENTITY_ENDPOINT", server.URL)
	t.Setenv("IDENTITY_HEADER", "")
	t.Setenv("MSI_ENDPOINT", "")
	t.Setenv("MSI_SECRET", "")

	p := NewManagedIdentityTokenProvider("")
	tok, err := p.GetToken(context.Background(), "https://vault.azure.net")
	require.NoError(t, err)
	assert.Equal(t, "arc-token", tok.AccessToken)
	assert.Equal(t, 2, callCount)
}

func TestIMDSToken(t *testing.T) {
	// We can't actually call 169.254.169.254 in tests. Instead we test the
	// doTokenRequest helper with a mock server that behaves like IMDS.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, tokenJSON("imds-token", time.Now().Add(time.Hour).Unix()))
	}))
	defer server.Close()

	p := &managedIdentityTokenProvider{httpClient: http.DefaultClient}
	req, err := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Metadata", "true")

	tok, err := p.doTokenRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "imds-token", tok.AccessToken)
}

func TestTokenCaching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, tokenJSON("cached-token", time.Now().Add(time.Hour).Unix()))
	}))
	defer server.Close()

	t.Setenv("IDENTITY_ENDPOINT", server.URL)
	t.Setenv("IDENTITY_HEADER", "test-header")
	t.Setenv("MSI_ENDPOINT", "")
	t.Setenv("MSI_SECRET", "")

	p := NewManagedIdentityTokenProvider("")

	// First call fetches.
	tok1, err := p.GetToken(context.Background(), "https://vault.azure.net")
	require.NoError(t, err)
	assert.Equal(t, "cached-token", tok1.AccessToken)

	// Second call returns cached.
	tok2, err := p.GetToken(context.Background(), "https://vault.azure.net")
	require.NoError(t, err)
	assert.Equal(t, tok1.AccessToken, tok2.AccessToken)

	assert.Equal(t, 1, callCount, "token should have been fetched only once")
}

func TestParseTokenResponse(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		body := []byte(`{"access_token":"tok","expires_on":"1700000000"}`)
		tok, err := parseTokenResponse(body)
		require.NoError(t, err)
		assert.Equal(t, "tok", tok.AccessToken)
		assert.Equal(t, int64(1700000000), tok.ExpiresOn.Unix())
	})

	t.Run("empty access_token", func(t *testing.T) {
		body := []byte(`{"access_token":"","expires_on":"1700000000"}`)
		_, err := parseTokenResponse(body)
		assert.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseTokenResponse([]byte(`not json`))
		assert.Error(t, err)
	})
}
