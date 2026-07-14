// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package encryptioncontext

import (
	"crypto/ecdh"
	"crypto/hpke"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sealForContext seals plaintext for a fresh key pair stored under encryptionContextID.
func sealForContext(t *testing.T, store *Store, encryptionContextID string, plaintext []byte) string {
	t.Helper()

	privateKey, err := hpke.DHKEM(ecdh.P256()).GenerateKey()
	require.NoError(t, err)
	store.Set(encryptionContextID, privateKey)

	sealed, err := hpke.Seal(privateKey.PublicKey(), hpke.HKDFSHA256(), hpke.AES256GCM(), hpkeInfo, plaintext)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(sealed)
}

func TestDecrypt(t *testing.T) {
	cases := []struct {
		name              string
		encryptionContext EncryptionContext
		plaintext         string
		skipSeal          bool
		wantErrContains   string
	}{
		{
			name:              "decrypts a sealed payload",
			encryptionContext: EncryptionContext{KeyType: KeyTypeHPKE, EncryptionContextID: "ctx-1"},
			plaintext:         "super-secret",
		},
		{
			name:              "rejects missing encryptionContextId",
			encryptionContext: EncryptionContext{KeyType: KeyTypeHPKE},
			plaintext:         "super-secret",
			wantErrContains:   "encryptionContextId is required",
		},
		{
			name:              "rejects unsupported keyType",
			encryptionContext: EncryptionContext{KeyType: "rsa", EncryptionContextID: "ctx-1"},
			plaintext:         "super-secret",
			wantErrContains:   "unsupported keyType",
		},
		{
			name:              "rejects unknown encryptionContextId",
			encryptionContext: EncryptionContext{KeyType: KeyTypeHPKE, EncryptionContextID: "ctx-unknown"},
			plaintext:         "super-secret",
			skipSeal:          true,
			wantErrContains:   "no private key found",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := NewStoreWithTTL(time.Minute)

			encryptedInput := "b25seSB1c2VkIGFzIGEgbm9uLWVtcHR5IHBsYWNlaG9sZGVy"
			if !testCase.skipSeal {
				encryptedInput = sealForContext(t, store, testCase.encryptionContext.EncryptionContextID, []byte(testCase.plaintext))
			}
			decrypted, err := Decrypt(store, testCase.encryptionContext, encryptedInput)
			if testCase.wantErrContains != "" {
				require.ErrorContains(t, err, testCase.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.plaintext, decrypted)

			// private key is single-use
			_, err = Decrypt(store, testCase.encryptionContext, encryptedInput)
			require.ErrorContains(t, err, "no private key found")
		})
	}

	t.Run("rejects empty encryptedInput", func(t *testing.T) {
		store := NewStoreWithTTL(time.Minute)
		_, err := Decrypt(store, EncryptionContext{KeyType: KeyTypeHPKE, EncryptionContextID: "ctx-1"}, "")
		require.ErrorContains(t, err, "encryptedInput is required")
	})

	t.Run("rejects non-base64 encryptedInput", func(t *testing.T) {
		store := NewStoreWithTTL(time.Minute)
		encryptionContext := EncryptionContext{KeyType: KeyTypeHPKE, EncryptionContextID: "ctx-1"}
		privateKey, err := hpke.DHKEM(ecdh.P256()).GenerateKey()
		require.NoError(t, err)
		store.Set(encryptionContext.EncryptionContextID, privateKey)

		_, err = Decrypt(store, encryptionContext, "not-base64!!!")
		require.ErrorContains(t, err, "not valid base64")
	})
}

type decryptedCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func TestDecryptInto(t *testing.T) {
	cases := []struct {
		name            string
		plaintext       string
		want            decryptedCredentials
		wantErrContains string
	}{
		{
			name:      "unmarshals decrypted JSON into the requested type",
			plaintext: `{"username":"admin","password":"hunter2"}`,
			want:      decryptedCredentials{Username: "admin", Password: "hunter2"},
		},
		{
			name:            "wraps invalid JSON as an error",
			plaintext:       `not-json`,
			wantErrContains: "failed to unmarshal decrypted input",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := NewStoreWithTTL(time.Minute)
			encryptionContext := EncryptionContext{KeyType: KeyTypeHPKE, EncryptionContextID: "ctx-1"}
			encryptedInput := sealForContext(t, store, encryptionContext.EncryptionContextID, []byte(testCase.plaintext))

			got, err := DecryptInto[decryptedCredentials](store, encryptionContext, encryptedInput)
			if testCase.wantErrContains != "" {
				require.ErrorContains(t, err, testCase.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.want, got)
		})
	}
}
