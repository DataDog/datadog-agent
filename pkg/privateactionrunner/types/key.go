// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"

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

func (k KeyType) ToPbKeyType() privateactionspb.KeyType {
	if k == KeyTypeX509RSA {
		return privateactionspb.KeyType_X509_RSA
	}
	if k == KeyTypeED25519 {
		return privateactionspb.KeyType_ED25519
	}
	return privateactionspb.KeyType_KEY_TYPE_UNKNOWN
}
