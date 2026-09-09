// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package encryptioncontext

import (
	"crypto/hpke"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const KeyTypeHPKE = "hpke-p256-hkdf-sha256-aes256gcm"

// hpkeInfo is the HPKE `info` used for domain separation. It must match the
// value used by the sealing side (wf-actions-server) byte-for-byte.
var hpkeInfo = []byte("dd-par-per-task-secret-input")

// EncryptionContext identifies the key pair an input was sealed with.
type EncryptionContext struct {
	KeyType             string `json:"keyType"`
	EncryptionContextID string `json:"encryptionContextId"`
}

// Decrypt opens the base64-encoded, HPKE-sealed encryptedInput using the
// private key evicted from store for encryptionContext.
func Decrypt(store *Store, encryptionContext EncryptionContext, encryptedInput string) (string, error) {
	if encryptionContext.EncryptionContextID == "" {
		return "", errors.New("encryptionContext.encryptionContextId is required")
	}
	if encryptionContext.KeyType != KeyTypeHPKE {
		return "", fmt.Errorf("unsupported keyType %q (expected %q)", encryptionContext.KeyType, KeyTypeHPKE)
	}
	if encryptedInput == "" {
		return "", errors.New("encryptedInput is required")
	}

	privateKey, found := store.GetAndDelete(encryptionContext.EncryptionContextID)
	if !found {
		return "", fmt.Errorf("no private key found for encryptionContextId %q", encryptionContext.EncryptionContextID)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedInput)
	if err != nil {
		return "", fmt.Errorf("encryptedInput is not valid base64: %w", err)
	}

	plaintext, err := hpke.Open(privateKey, hpke.HKDFSHA256(), hpke.AES256GCM(), hpkeInfo, ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to open sealed input: %w", err)
	}
	return string(plaintext), nil
}

// DecryptInto decrypts encryptedInput like Decrypt, then unmarshals the
// JSON plaintext into T.
func DecryptInto[T any](store *Store, encryptionContext EncryptionContext, encryptedInput string) (T, error) {
	var out T

	plaintext, err := Decrypt(store, encryptionContext, encryptedInput)
	if err != nil {
		return out, err
	}

	if err := json.Unmarshal([]byte(plaintext), &out); err != nil {
		return out, fmt.Errorf("failed to unmarshal decrypted input: %w", err)
	}
	return out, nil
}
