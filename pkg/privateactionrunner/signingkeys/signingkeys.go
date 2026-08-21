// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package signingkeys decodes Private Action Runner signing-key snapshots.
package signingkeys

import (
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Key is the transport-safe form of a public verification key.
type Key struct {
	ID      string
	KeyType types.KeyType
	Key     []byte
}

// DecodeSnapshot validates and decodes a complete Remote Config snapshot.
func DecodeSnapshot(configs map[string]state.RawConfig) ([]Key, map[string]error) {
	keys := make([]Key, 0, len(configs))
	errs := make(map[string]error)
	for configID, rawConfig := range configs {
		key, err := Decode(rawConfig)
		if err != nil {
			errs[configID] = err
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
	return keys, errs
}

// Decode validates one Remote Config signing key.
func Decode(rawConfig state.RawConfig) (Key, error) {
	var rawKey types.RawKey
	if err := json.Unmarshal(rawConfig.Config, &rawKey); err != nil {
		return Key{}, fmt.Errorf("decoding JSON: %w", err)
	}
	if rawConfig.Metadata.ID == "" {
		return Key{}, errors.New("signing key ID is empty")
	}
	if err := Validate(rawKey.KeyType, rawKey.Key); err != nil {
		return Key{}, err
	}
	return Key{ID: rawConfig.Metadata.ID, KeyType: rawKey.KeyType, Key: append([]byte(nil), rawKey.Key...)}, nil
}

// Validate checks that key data has a supported type and valid public-key encoding.
func Validate(keyType types.KeyType, key []byte) error {
	block, _ := pem.Decode(key)
	if block == nil {
		return errors.New("decoding PEM block")
	}

	switch keyType {
	case types.KeyTypeX509RSA:
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("parsing certificate: %w", err)
		}
		if _, ok := cert.PublicKey.(*rsa.PublicKey); !ok {
			return errors.New("certificate does not contain an RSA public key")
		}
	case types.KeyTypeED25519:
		keyAny, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("parsing ED25519 public key: %w", err)
		}
		if _, ok := keyAny.(ed25519.PublicKey); !ok {
			return errors.New("public key is not ED25519")
		}
	default:
		return fmt.Errorf("unsupported key type: %s", keyType)
	}
	return nil
}

// Clone returns a deep copy of keys.
func Clone(keys []Key) []Key {
	cloned := make([]Key, len(keys))
	for i, key := range keys {
		cloned[i] = key
		cloned[i].Key = append([]byte(nil), key.Key...)
	}
	return cloned
}
