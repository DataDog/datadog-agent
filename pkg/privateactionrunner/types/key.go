// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package types

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

const (
	// KeyTypeX509RSA represents X509 RSA key type.
	KeyTypeX509RSA KeyType = "X509_RSA"
	// KeyTypeED25519 represents ED25519 key type.
	KeyTypeED25519 KeyType = "ED25519"
)

// KeyType represents the type of cryptographic key.
type KeyType string

// RawKey represents a raw key used only for initial JSON unmarshalling.
type RawKey struct {
	KeyType KeyType `json:"keyType"`
	Key     []byte  `json:"key"`
}

// X509RSAKey represents an X509 RSA key.
type X509RSAKey struct {
	KeyType KeyType
	Key     *rsa.PublicKey
}

// ED25519Key represents an ED25519 key.
type ED25519Key struct {
	KeyType KeyType
	Key     ed25519.PublicKey
}

// DecodedKey represents a decoded cryptographic key.
type DecodedKey interface {
	GetKeyType() KeyType
	Verify(data, signature []byte) error
}

// GetKeyType returns the key type for X509RSAKey.
func (key X509RSAKey) GetKeyType() KeyType { return key.KeyType }

// GetKeyType returns the key type for ED25519Key.
func (key ED25519Key) GetKeyType() KeyType { return key.KeyType }

// Verify verifies data using X509RSAKey.
func (key X509RSAKey) Verify(data, signature []byte) error {
	err := rsa.VerifyPSS(key.Key, crypto.SHA256, data, signature, nil)
	if err != nil {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// Verify verifies data using ED25519Key.
func (key ED25519Key) Verify(data, signature []byte) error {
	valid := ed25519.Verify(key.Key, data, signature)
	if !valid {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// ToPbKeyType converts KeyType to protobuf KeyType.
func (k KeyType) ToPbKeyType() privateactions.KeyType {
	if k == KeyTypeX509RSA {
		return privateactions.KeyType_X509_RSA
	}
	if k == KeyTypeED25519 {
		return privateactions.KeyType_ED25519
	}
	return privateactions.KeyType_KEY_TYPE_UNKNOWN
}
