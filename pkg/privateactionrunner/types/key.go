// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

const (
	KeyTypeX509RSA KeyType = "X509_RSA"
	KeyTypeED25519 KeyType = "ED25519"
)

type (
	KeyType string
)

// raw key type used only for initial json unmarshalling
type RawKey struct {
	KeyType KeyType `json:"keyType"`
	Key     []byte  `json:"key"`
}

// we support x509RSA and ED25519 keys
type X509RSAKey struct {
	KeyType KeyType
	Key     *rsa.PublicKey
}

type ED25519Key struct {
	KeyType KeyType
	Key     ed25519.PublicKey
}

type DecodedKey interface {
	GetKeyType() KeyType
	Verify(data, signature []byte) error
}

// TaskVerificationKey is the public key that successfully authenticated a
// signed task. It is attached to the in-memory task only after verification.
type TaskVerificationKey struct {
	ID      string
	KeyType KeyType
	PEM     string
}

func (key X509RSAKey) GetKeyType() KeyType { return key.KeyType }
func (key ED25519Key) GetKeyType() KeyType { return key.KeyType }

func (key X509RSAKey) Verify(data, signature []byte) error {
	err := rsa.VerifyPSS(key.Key, crypto.SHA256, data, signature, nil)
	if err != nil {
		return errors.New("invalid signature")
	}
	return nil
}

func (key ED25519Key) Verify(data, signature []byte) error {
	valid := ed25519.Verify(key.Key, data, signature)
	if !valid {
		return errors.New("invalid signature")
	}
	return nil
}

func NewTaskVerificationKey(id string, key DecodedKey) (*TaskVerificationKey, error) {
	if id == "" {
		return nil, errors.New("verification key id is required")
	}
	var publicKey any
	switch decoded := key.(type) {
	case *X509RSAKey:
		publicKey = decoded.Key
	case X509RSAKey:
		publicKey = decoded.Key
	case *ED25519Key:
		publicKey = decoded.Key
	case ED25519Key:
		publicKey = decoded.Key
	default:
		return nil, fmt.Errorf("unsupported verification key type %T", key)
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("encode verification public key: %w", err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	if block == nil {
		return nil, errors.New("encode verification public key PEM")
	}
	return &TaskVerificationKey{ID: id, KeyType: key.GetKeyType(), PEM: string(block)}, nil
}

func (k KeyType) ToPbKeyType() privateactionspb.KeyType {
	if k == KeyTypeX509RSA {
		return privateactionspb.KeyType_X509_RSA
	}
	if k == KeyTypeED25519 {
		return privateactionspb.KeyType_ED25519
	}
	return privateactionspb.KeyType_KEY_TYPE_UNKNOWN
}
