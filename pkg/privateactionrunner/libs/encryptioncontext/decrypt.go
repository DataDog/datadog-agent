// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package encryptioncontext

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// KeyTypeCurve25519 is the only supported EncryptionContext.KeyType.
const KeyTypeCurve25519 = "curve25519"

// EncryptionContext identifies the key pair an input was sealed with.
type EncryptionContext struct {
	KeyType             string `json:"keyType"`
	EncryptionContextID string `json:"encryptionContextId"`
}

// Decrypt opens the base64-encoded, NaCl-sealed encryptedInput using the
// private key evicted from store for encryptionContext.
func Decrypt(store *Store, encryptionContext EncryptionContext, encryptedInput string) (string, error) {
	if encryptionContext.EncryptionContextID == "" {
		return "", errors.New("encryptionContext.encryptionContextId is required")
	}
	if encryptionContext.KeyType != KeyTypeCurve25519 {
		return "", fmt.Errorf("unsupported keyType %q (expected %q)", encryptionContext.KeyType, KeyTypeCurve25519)
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

	publicKeyBytes, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("failed to derive public key: %w", err)
	}
	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	plaintext, ok := box.OpenAnonymous(nil, ciphertext, &publicKey, privateKey)
	if !ok {
		return "", errors.New("failed to open sealed input")
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
