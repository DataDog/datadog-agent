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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
)

// Must match the HPKE info used by encryptioncontext and wf-actions-server.
var testHPKEInfo = []byte("dd-par-per-task-secret-input")

func sealSecretInputs(t *testing.T, store *encryptioncontext.Store, encryptionContextID, plaintext string) string {
	t.Helper()
	privateKey, err := hpke.DHKEM(ecdh.P256()).GenerateKey()
	require.NoError(t, err)
	store.Set(encryptionContextID, privateKey)
	sealed, err := hpke.Seal(privateKey.PublicKey(), hpke.HKDFSHA256(), hpke.AES256GCM(), testHPKEInfo, []byte(plaintext))
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(sealed)
}

func TestRequestEncryptedCredentials(t *testing.T) {
	for _, test := range []struct {
		name                string
		withStore           bool
		sealedContextID     string
		requestContextID    string
		requestHeaders      []interface{}
		wantAuthorization   string
		wantErrContains     string
		wantNoServerRequest bool
	}{
		{
			name:              "adds the decrypted Authorization header",
			withStore:         true,
			sealedContextID:   "ctx-1",
			requestContextID:  "ctx-1",
			wantAuthorization: "Bearer secret-token",
		},
		{
			name:              "overrides an Authorization header set in the step",
			withStore:         true,
			sealedContextID:   "ctx-1",
			requestContextID:  "ctx-1",
			requestHeaders:    []interface{}{map[string]interface{}{"key": "Authorization", "value": []interface{}{"Bearer from-step"}}},
			wantAuthorization: "Bearer secret-token",
		},
		{
			name:                "fails when the key is unknown",
			withStore:           true,
			sealedContextID:     "ctx-1",
			requestContextID:    "ctx-other",
			wantErrContains:     "no private key found",
			wantNoServerRequest: true,
		},
		{
			name:                "fails without an encryption store",
			withStore:           false,
			sealedContextID:     "ctx-1",
			requestContextID:    "ctx-1",
			wantErrContains:     "encrypted credentials are not supported",
			wantNoServerRequest: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var gotAuthorization string
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				gotAuthorization = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(server.Close)

			store := encryptioncontext.NewStore()
			encryptedCredentials := sealSecretInputs(t, store, test.sealedContextID, `{"headers":{"Authorization":"Bearer secret-token"}}`)
			inputs := map[string]interface{}{
				"url":                  server.URL + "/resource",
				"encryptedCredentials": encryptedCredentials,
				"encryptionContext": map[string]interface{}{
					"keyType":             encryptioncontext.KeyTypeHPKE,
					"encryptionContextId": test.requestContextID,
				},
			}
			if test.requestHeaders != nil {
				inputs["requestHeaders"] = test.requestHeaders
			}
			handler := NewHttpRequestAction(newHTTPTestConfig(t, server.URL))
			if test.withStore {
				handler = handler.WithEncryptionStore(store)
			}

			_, err := handler.Run(context.Background(), newRequestTask(inputs), nil)

			if test.wantErrContains != "" {
				require.ErrorContains(t, err, test.wantErrContains)
				require.Zero(t, requests)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantAuthorization, gotAuthorization)
		})
	}
}
