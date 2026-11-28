// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"crypto/ecdh"
	"crypto/hpke"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func runRawRequest(t *testing.T, servers []*httptest.Server, inputs map[string]interface{}, credential *privateconnection.PrivateCredentials) (*RequestOutputs, error) {
	t.Helper()
	return runRawRequestWithStore(t, servers, inputs, credential, encryptioncontext.NewStore())
}

// runRawRequestWithStore runs request in raw mode; inputs use raw-mode names.
func runRawRequestWithStore(t *testing.T, servers []*httptest.Server, inputs map[string]interface{}, credential *privateconnection.PrivateCredentials, store *encryptioncontext.Store) (*RequestOutputs, error) {
	t.Helper()
	var hosts []string
	for _, server := range servers {
		target, err := url.Parse(server.URL)
		require.NoError(t, err)
		hosts = append(hosts, target.Hostname())
	}
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.http", Name: "request",
		Inputs:                withRawMode(inputs),
		SecDatadogHeaderValue: "sec-value",
	}
	handler := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second, Allowlist: hosts}).WithEncryptionStore(store)
	output, err := handler.Run(context.Background(), task, credential)
	if err != nil {
		return nil, err
	}
	return output.(*RequestOutputs), nil
}

// withRawMode sets responseMode and maps a headers map to requestHeaders.
func withRawMode(inputs map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{"responseMode": ResponseModeRaw}
	for key, value := range inputs {
		switch key {
		case "method":
			out["verb"] = value
		case "headers":
			var headers []interface{}
			for name, values := range value.(map[string]interface{}) {
				headers = append(headers, map[string]interface{}{"key": name, "value": values})
			}
			out["requestHeaders"] = headers
		default:
			out[key] = value
		}
	}
	return out
}

func decodeRawBody(t *testing.T, output *RequestOutputs) string {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString(output.BodyBase64)
	require.NoError(t, err)
	return string(body)
}

func tokenConnection(baseURL string, headers ...privateconnection.PrivateCredentialsToken) *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Type:        privateconnection.TokenAuthType,
		Tokens:      []privateconnection.PrivateCredentialsToken{{Name: "token", Value: "runner-secret"}},
		HttpDetails: privateconnection.HttpDetails{BaseURL: baseURL, Headers: headers},
	}
}

// An MCP server answers an unauthenticated request with 401 and the challenge
// that starts OAuth discovery (RFC 9728), so it must come back as an output.
func TestRawRequestReturnsUnauthorizedChallenge(t *testing.T) {
	challenge := `Bearer resource_metadata="https://mcp.internal/.well-known/oauth-protected-resource", scope="tools"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", challenge)
		w.Header().Set("Set-Cookie", "session=private")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer server.Close()

	output, err := runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{"method": "POST", "url": server.URL + "/mcp"}, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, output.Status)
	assert.Equal(t, challenge, output.Headers.Get("WWW-Authenticate"))
	assert.Empty(t, output.Headers.Get("Set-Cookie"))
	assert.JSONEq(t, `{"error":"invalid_token"}`, decodeRawBody(t, output))
}

// Token exchange and refresh need an application/x-www-form-urlencoded body
// sent byte for byte, which the default mode re-encodes as JSON.
func TestRawRequestSendsBodyVerbatim(t *testing.T) {
	form := "grant_type=authorization_code&code=abc&code_verifier=xyz&redirect_uri=https%3A%2F%2Fapp.datadoghq.com%2Fcb"
	var gotBody, gotContentType, gotSecHeader, gotCookie string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody, gotContentType = string(body), r.Header.Get("Content-Type")
		gotSecHeader, gotCookie = r.Header.Get(DdSecurityHeader), r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer"}`))
	}))
	defer server.Close()

	output, err := runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{
		"method": "POST",
		"url":    server.URL + "/token",
		"headers": map[string]interface{}{
			"Content-Type": []string{"application/x-www-form-urlencoded"},
			"Cookie":       []string{"leak=1"},
			"Sec-Datadog":  []string{"forged"},
		},
		"bodyBase64": base64.StdEncoding.EncodeToString([]byte(form)),
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, output.Status)
	assert.Equal(t, form, gotBody)
	assert.Equal(t, "application/x-www-form-urlencoded", gotContentType)
	assert.Equal(t, "sec-value", gotSecHeader)
	assert.Empty(t, gotCookie)
	assert.JSONEq(t, `{"access_token":"at","token_type":"Bearer"}`, decodeRawBody(t, output))
}

// OAuth discovery must not follow redirects, and a followed redirect would
// also skip the runner's URL allowlist, so a 3xx comes back as is.
func TestRawRequestDoesNotFollowRedirects(t *testing.T) {
	var followed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			followed = true
			return
		}
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer server.Close()

	output, err := runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{"method": "GET", "url": server.URL + "/.well-known/oauth-protected-resource"}, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, output.Status)
	assert.Equal(t, "/elsewhere", output.Headers.Get("Location"))
	assert.False(t, followed)
}

// A connection header (a runner-held static token) reaches the connection's host
// only. Calls to another allowed host, such as the identity provider, go out
// without it. Templates in the caller's headers are never rendered.
func TestRawRequestScopesConnectionHeadersToTheirHost(t *testing.T) {
	var mcpAuthorization, mcpProbe, idpAuthorization string
	mcp := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mcpAuthorization, mcpProbe = r.Header.Get("Authorization"), r.Header.Get("X-Probe")
	}))
	defer mcp.Close()
	idp := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		idpAuthorization = r.Header.Get("Authorization")
	}))
	defer idp.Close()
	servers := []*httptest.Server{mcp, idp}
	credential := tokenConnection(mcp.URL, privateconnection.PrivateCredentialsToken{Name: "Authorization", Value: "Bearer runner-secret"})

	_, err := runRawRequest(t, servers, map[string]interface{}{
		"method":  "POST",
		"url":     mcp.URL + "/mcp",
		"headers": map[string]interface{}{"X-Probe": []string{"{{ root_tokens.token }}"}},
	}, credential)
	require.NoError(t, err)
	assert.Equal(t, "Bearer runner-secret", mcpAuthorization)
	assert.Equal(t, "{{ root_tokens.token }}", mcpProbe)

	// httptest servers share 127.0.0.1, so the port is what tells the hosts apart.
	_, err = runRawRequest(t, servers, map[string]interface{}{"method": "POST", "url": idp.URL + "/token"}, credential)
	require.NoError(t, err)
	assert.Empty(t, idpAuthorization)
}

func TestRawRequestRejectsAuthorizationCollision(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	credential := tokenConnection(server.URL, privateconnection.PrivateCredentialsToken{Name: "Authorization", Value: "Bearer runner-secret"})

	_, err := runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{
		"method":  "POST",
		"url":     server.URL + "/mcp",
		"headers": map[string]interface{}{"authorization": []string{"Bearer oauth-access-token"}},
	}, credential)
	require.ErrorContains(t, err, "both the request and the connection set Authorization")
	assert.Zero(t, requests)
}

func TestRawRequestRequiresBaseURLForConnectionHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request expected")
	}))
	defer server.Close()
	credential := tokenConnection("", privateconnection.PrivateCredentialsToken{Name: "X-Api-Key", Value: "runner-secret"})

	_, err := runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{"method": "GET", "url": server.URL}, credential)
	require.ErrorContains(t, err, "require a base URL")
}

// OAuth token and client registration responses carry refresh tokens and client
// secrets, so the caller can ask for the body sealed to a Datadog-held key.
func TestRawRequestEncryptsResponseBody(t *testing.T) {
	tokenResponse := `{"access_token":"at","refresh_token":"rt"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tokenResponse))
	}))
	defer server.Close()
	privateKey, err := hpke.DHKEM(ecdh.P256()).GenerateKey()
	require.NoError(t, err)

	output, err := runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{
		"method": "POST",
		"url":    server.URL + "/token",
		"encryptResponseTo": map[string]interface{}{
			"keyType":   RawResponseKeyType,
			"publicKey": base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
		},
	}, nil)
	require.NoError(t, err)
	assert.Empty(t, output.BodyBase64)
	sealed, err := base64.StdEncoding.DecodeString(output.EncryptedBody)
	require.NoError(t, err)
	assert.NotContains(t, string(sealed), "refresh_token")
	opened, err := hpke.Open(privateKey, hpke.HKDFSHA256(), hpke.AES256GCM(), rawResponseHPKEInfo, sealed)
	require.NoError(t, err)
	assert.Equal(t, tokenResponse, string(opened))
}

// An OAuth code exchange carries the code and client secret. They come through
// ExecuteOboWithSecretInputs, sealed to the one-time key that prepareEncryption
// left in the store, so they never appear in the task inputs.
func TestRawRequestDecryptsSecretInputs(t *testing.T) {
	form := "grant_type=authorization_code&code=secret-code&code_verifier=v"
	var gotBody, gotAuthorization, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody, gotAuthorization, gotAccept = string(body), r.Header.Get("Authorization"), r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"access_token":"at"}`))
	}))
	defer server.Close()

	store := encryptioncontext.NewStore()
	privateKey, err := hpke.DHKEM(ecdh.P256()).GenerateKey()
	require.NoError(t, err)
	store.Set("ctx-1", privateKey)
	secrets := `{"headers":{"Authorization":["Basic Y2xpZW50OnNlY3JldA=="]},"bodyBase64":"` + base64.StdEncoding.EncodeToString([]byte(form)) + `"}`
	// Same info bytes as libs/encryptioncontext, which wf-actions-server seals with.
	sealed, err := hpke.Seal(privateKey.PublicKey(), hpke.HKDFSHA256(), hpke.AES256GCM(), []byte("dd-par-per-task-secret-input"), []byte(secrets))
	require.NoError(t, err)

	inputs := map[string]interface{}{
		"method":               "POST",
		"url":                  server.URL + "/token",
		"headers":              map[string]interface{}{"Accept": []string{"application/json"}},
		"encryptionContext":    map[string]interface{}{"keyType": encryptioncontext.KeyTypeHPKE, "encryptionContextId": "ctx-1"},
		"encryptedCredentials": base64.StdEncoding.EncodeToString(sealed),
	}
	output, err := runRawRequestWithStore(t, []*httptest.Server{server}, inputs, nil, store)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, output.Status)
	assert.Equal(t, form, gotBody)
	assert.Equal(t, "Basic Y2xpZW50OnNlY3JldA==", gotAuthorization)
	assert.Equal(t, "application/json", gotAccept)

	// The key is single use, so replaying the same task fails before any request.
	_, err = runRawRequestWithStore(t, []*httptest.Server{server}, inputs, nil, store)
	require.ErrorContains(t, err, "failed to decrypt secret inputs")
}

func TestRawRequestTruncatesLargeResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", MaxRawResponseBodyBytes+10)))
	}))
	defer server.Close()

	output, err := runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{"method": "GET", "url": server.URL}, nil)
	require.NoError(t, err)
	assert.True(t, output.BodyTruncated)
	assert.Len(t, decodeRawBody(t, output), MaxRawResponseBodyBytes)
}

func TestRawRequestRejectsInvalidInputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request expected")
	}))
	defer server.Close()
	for name, inputs := range map[string]map[string]interface{}{
		"method":    {"method": "CONNECT", "url": server.URL},
		"mode":      {"method": "GET", "url": server.URL, "responseMode": "stream"},
		"json body": {"method": "POST", "url": server.URL, "body": map[string]interface{}{"a": 1}},
		"redirects": {"method": "GET", "url": server.URL, "followRedirect": true},
		"status":    {"method": "GET", "url": server.URL, "errorOnStatus": []string{"400-499"}},
		"params":    {"method": "GET", "url": server.URL, "urlParams": []interface{}{map[string]interface{}{"key": "a", "value": "b"}}},
		"scheme":    {"method": "GET", "url": "file:///etc/passwd"},
		"user info": {"method": "GET", "url": "https://user:pass@mcp.internal/"},
		"body":      {"method": "POST", "url": server.URL, "bodyBase64": "%%%"},
		"key type":  {"method": "GET", "url": server.URL, "encryptResponseTo": map[string]interface{}{"keyType": "rsa", "publicKey": "AA=="}},
		"key bytes": {"method": "GET", "url": server.URL, "encryptResponseTo": map[string]interface{}{"keyType": RawResponseKeyType, "publicKey": "AA=="}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runRawRequest(t, []*httptest.Server{server}, inputs, nil)
			require.Error(t, err)
		})
	}
}
