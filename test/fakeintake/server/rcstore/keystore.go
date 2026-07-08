// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcstore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultKeyPath is the on-disk location of the persistent signing key seed
// when the caller does not specify one.
func DefaultKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".fakeintake", "signing.key"), nil
}

// KeyFromHexSeed derives an ed25519 private key from a 64-character hex-encoded
// 32-byte seed. Returns an error when hexSeed is malformed.
func KeyFromHexSeed(hexSeed string) (ed25519.PrivateKey, error) {
	seed, err := hex.DecodeString(hexSeed)
	if err != nil {
		return nil, fmt.Errorf("decode seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("expected %d-byte seed, got %d", ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// LoadOrCreateSigningKey reads a 32-byte ed25519 seed from path. If path is
// empty, falls back to DefaultKeyPath. If the file does not exist, a fresh
// key is generated and written. The returned bool reports whether a new key
// was generated this call (callers may want to log a hint about flushing
// remote-config.db).
func LoadOrCreateSigningKey(path string) (ed25519.PrivateKey, bool, error) {
	if path == "" {
		def, err := DefaultKeyPath()
		if err != nil {
			return nil, false, fmt.Errorf("resolve default key path: %w", err)
		}
		path = def
	}

	if seed, err := os.ReadFile(path); err == nil {
		if len(seed) != ed25519.SeedSize {
			return nil, false, fmt.Errorf("signing key %s: expected %d bytes, got %d", path, ed25519.SeedSize, len(seed))
		}
		return ed25519.NewKeyFromSeed(seed), false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("read signing key %s: %w", path, err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	_ = pub
	if err != nil {
		return nil, false, fmt.Errorf("generate signing key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, fmt.Errorf("create key dir: %w", err)
	}
	if err := os.WriteFile(path, priv.Seed(), 0o600); err != nil {
		return nil, false, fmt.Errorf("write signing key %s: %w", path, err)
	}
	return priv, true, nil
}
