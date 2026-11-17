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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// sealedInputs seals secrets as wf-actions-server does for
// ExecuteOboWithSecretInputs, and stores the key as prepareEncryption would.
func sealedInputs(t *testing.T, store *encryptioncontext.Store, inputs map[string]interface{}, secrets interface{}) map[string]interface{} {
	t.Helper()
	privateKey, err := hpke.DHKEM(ecdh.P256()).GenerateKey()
	require.NoError(t, err)
	store.Set("ctx-1", privateKey)
	plaintext, err := json.Marshal(secrets)
	require.NoError(t, err)
	// Same info bytes as libs/encryptioncontext.
	sealed, err := hpke.Seal(privateKey.PublicKey(), hpke.HKDFSHA256(), hpke.AES256GCM(), []byte("dd-par-per-task-secret-input"), plaintext)
	require.NoError(t, err)
	inputs["encryptionContext"] = map[string]interface{}{"keyType": encryptioncontext.KeyTypeHPKE, "encryptionContextId": "ctx-1"}
	inputs["encryptedCredentials"] = base64.StdEncoding.EncodeToString(sealed)
	return inputs
}

func requestTask(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.http", Name: "request", Inputs: inputs, SecDatadogHeaderValue: "sec-value"}
	return task
}

func userTokenConnection(baseURL string) *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Type:        privateconnection.UserOAuth2TokenAuthType,
		HttpDetails: privateconnection.HttpDetails{BaseURL: baseURL},
	}
}

func bearerSecret(token string) SecretInputs {
	return SecretInputs{RequestHeaders: []Header{{Key: "Authorization", Value: []string{"Bearer " + token}}}}
}

func TestRequestSendsSecretHeaders(t *testing.T) {
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	for name, credential := range map[string]*privateconnection.PrivateCredentials{
		"no connection":          nil,
		"user OAuth2 connection": userTokenConnection(server.URL),
	} {
		t.Run(name, func(t *testing.T) {
			gotAuthorization = ""
			store := encryptioncontext.NewStore()
			handler := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second}).WithEncryptionStore(store)
			inputs := sealedInputs(t, store, map[string]interface{}{"verb": "GET", "url": server.URL}, bearerSecret("user-token"))

			output, err := handler.Run(context.Background(), requestTask(inputs), credential)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, output.(*RequestOutputs).Status)
			assert.Equal(t, "Bearer user-token", gotAuthorization)

			// The key is single use, so replaying the task fails before any request.
			gotAuthorization = ""
			_, err = handler.Run(context.Background(), requestTask(inputs), credential)
			require.ErrorContains(t, err, "failed to open secret inputs")
			assert.Empty(t, gotAuthorization)
		})
	}
}

func TestRequestRejectsSecretHeadersItCannotUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("no request expected") }))
	defer server.Close()

	for _, test := range []struct {
		name       string
		plain      []Header
		secrets    SecretInputs
		credential *privateconnection.PrivateCredentials
		want       string
	}{
		{name: "empty key", secrets: SecretInputs{RequestHeaders: []Header{{Key: "", Value: []string{"x"}}}}, want: `secret header "" is not allowed`},
		{name: "runtime worker key", secrets: SecretInputs{RequestHeaders: []Header{{Key: "Runtime-Worker-Key", Value: []string{"x"}}}}, want: "is not allowed"},
		{name: "unsupported header", secrets: SecretInputs{RequestHeaders: []Header{{Key: "Host", Value: []string{"x"}}}}, want: "unsupported header"},
		{name: "collision with the request", plain: []Header{{Key: "X-Token", Value: []string{"plain"}}}, secrets: SecretInputs{RequestHeaders: []Header{{Key: "x-token", Value: []string{"secret"}}}}, want: "is also set by the request"},
		{name: "collision with the runner", secrets: SecretInputs{RequestHeaders: []Header{{Key: "user-agent", Value: []string{"x"}}}}, want: "is also set by the request"},
		{name: "user OAuth2 without Authorization", secrets: SecretInputs{RequestHeaders: []Header{{Key: "X-Other", Value: []string{"x"}}}}, credential: userTokenConnection(server.URL), want: "secret Authorization header"},
		{name: "user OAuth2 with plain Authorization", plain: []Header{{Key: "authorization", Value: []string{"Bearer plain"}}}, secrets: bearerSecret("t"), credential: userTokenConnection(server.URL), want: "takes Authorization only from the secret inputs"},
		{name: "user OAuth2 to another host", secrets: bearerSecret("t"), credential: userTokenConnection("https://mcp.internal"), want: "does not match base URL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := encryptioncontext.NewStore()
			handler := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second}).WithEncryptionStore(store)
			inputs := map[string]interface{}{"verb": "GET", "url": server.URL}
			if test.plain != nil {
				inputs["requestHeaders"] = test.plain
			}
			_, err := handler.Run(context.Background(), requestTask(sealedInputs(t, store, inputs, test.secrets)), test.credential)
			assert.ErrorContains(t, err, test.want)
		})
	}
}

func TestRequestUserOAuth2NeedsTheToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("no request expected") }))
	defer server.Close()
	handler := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second}).WithEncryptionStore(encryptioncontext.NewStore())

	_, err := handler.Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": server.URL}), userTokenConnection(server.URL))
	require.ErrorContains(t, err, "needs the user's token as a secret input")
}

func TestRequestNeedsBothSecretInputs(t *testing.T) {
	handler := NewHttpRequestAction(&config.Config{}).WithEncryptionStore(encryptioncontext.NewStore())
	_, err := handler.Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": "https://mcp.internal", "encryptedCredentials": "AA=="}), nil)
	assert.ErrorContains(t, err, "need both encryptedCredentials and encryptionContext")
}

func TestRequestWithoutStoreRejectsSecretInputs(t *testing.T) {
	inputs := sealedInputs(t, encryptioncontext.NewStore(), map[string]interface{}{"verb": "GET", "url": "https://mcp.internal"}, bearerSecret("t"))
	_, err := NewHttpRequestAction(&config.Config{}).Run(context.Background(), requestTask(inputs), nil)
	assert.ErrorContains(t, err, "not supported by this runner")
}

// In raw mode, a user OAuth2 connection's token may only go to the connection's
// host, and only as a secret input.
func TestRawRequestUserOAuth2StaysOnItsHost(t *testing.T) {
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
	}))
	defer server.Close()
	rawSecrets := RawSecretInputs{Headers: map[string][]string{"Authorization": {"Bearer user-token"}}}

	store := encryptioncontext.NewStore()
	output, err := runRawRequestWithStore(t, []*httptest.Server{server}, sealedInputs(t, store, map[string]interface{}{"method": "GET", "url": server.URL}, rawSecrets), userTokenConnection(server.URL), store)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, output.Status)
	assert.Equal(t, "Bearer user-token", gotAuthorization)

	store = encryptioncontext.NewStore()
	_, err = runRawRequestWithStore(t, []*httptest.Server{server}, sealedInputs(t, store, map[string]interface{}{"method": "GET", "url": server.URL}, rawSecrets), userTokenConnection("https://mcp.internal"), store)
	require.ErrorContains(t, err, "only sends requests to its base URL")

	_, err = runRawRequest(t, []*httptest.Server{server}, map[string]interface{}{"method": "GET", "url": server.URL, "headers": map[string]interface{}{"Authorization": []string{"Bearer plain"}}}, userTokenConnection(server.URL))
	require.ErrorContains(t, err, "takes Authorization only from the secret inputs")
}
