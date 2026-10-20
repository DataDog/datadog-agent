// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelper

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

type Credential struct {
	Version               int                 `json:"version"`
	OrgID                 int64               `json:"orgId"`
	RunnerID              string              `json:"runnerId"`
	Keys                  []CredentialKey     `json:"keys"`
	AllowedCommands       []string            `json:"allowedCommands"`
	AllowedPaths          []string            `json:"allowedPaths"`
	AllowedSystemServices map[string][]string `json:"allowedSystemServices"`
	ElevatableCommands    []string            `json:"elevatableCommands"`
	DirectorRoot          json.RawMessage     `json:"directorRoot,omitempty"`
	decodedKeys           map[string]verificationKey
	trustBackendPolicy    bool
}

type CredentialKey struct {
	ID   string  `json:"id"`
	Type KeyType `json:"type"`
	PEM  string  `json:"pem"`
}

type verificationKey interface {
	keyType() KeyType
	verify(digest, signature []byte) error
}

type rsaKey struct{ key *rsa.PublicKey }

func (rsaKey) keyType() KeyType { return KeyTypeX509RSA }
func (k rsaKey) verify(digest, signature []byte) error {
	return rsa.VerifyPSS(k.key, crypto.SHA256, digest, signature, nil)
}

type ed25519Key struct{ key ed25519.PublicKey }

func (ed25519Key) keyType() KeyType { return KeyTypeED25519 }
func (k ed25519Key) verify(digest, signature []byte) error {
	if !ed25519.Verify(k.key, digest, signature) {
		return errors.New("invalid signature")
	}
	return nil
}

func LoadCredential(path string) (*Credential, error) {
	data, err := readCredentialFile(path)
	if err != nil {
		return nil, fmt.Errorf("read verification credential: %w", err)
	}
	var credential Credential
	dec := json.NewDecoder(&byteReader{data: data})
	dec.DisallowUnknownFields()
	if err := dec.Decode(&credential); err != nil {
		return nil, fmt.Errorf("decode verification credential: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, fmt.Errorf("decode verification credential: %w", err)
	}
	if credential.Version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported credential version %d", credential.Version)
	}
	if credential.OrgID > 0 && credential.RunnerID == "" {
		return nil, errors.New("policy must set both orgId and runnerId or neither")
	}
	if credential.OrgID <= 0 && credential.RunnerID != "" {
		return nil, errors.New("policy must set both orgId and runnerId or neither")
	}
	if len(credential.DirectorRoot) > 0 {
		if err := validateDirectorRoot(credential.DirectorRoot); err != nil {
			return nil, err
		}
	}
	credential.decodedKeys = make(map[string]verificationKey, len(credential.Keys))
	for _, raw := range credential.Keys {
		if err := addCredentialKey(credential.decodedKeys, raw, false); err != nil {
			return nil, err
		}
	}
	return &credential, nil
}

func (c *Credential) withSocketVerificationKeys(keys []CredentialKey) (*Credential, error) {
	if len(c.DirectorRoot) == 0 {
		requestCredential, err := NewRequestCredential(keys)
		if err != nil {
			return nil, err
		}
		requestCredential.OrgID = c.OrgID
		requestCredential.RunnerID = c.RunnerID
		requestCredential.AllowedCommands = c.AllowedCommands
		requestCredential.AllowedPaths = c.AllowedPaths
		requestCredential.AllowedSystemServices = c.AllowedSystemServices
		requestCredential.ElevatableCommands = c.ElevatableCommands
		requestCredential.trustBackendPolicy = false
		return requestCredential, nil
	}
	var directorKeys []CredentialKey
	for _, key := range keys {
		if key.Type == KeyTypeTUFDirector {
			directorKeys = append(directorKeys, key)
		}
	}
	if len(directorKeys) != 1 {
		if len(keys) == 1 {
			return c.withDirectorProof(keys[0])
		}
		return nil, errors.New("exactly one Director proof is required")
	}
	return c.withDirectorProof(directorKeys[0])
}

// NewRequestCredential trusts one bare verification key supplied with the
// request and applies no local policy. The embedding application is
// responsible for independently gating this mode through administrator
// configuration.
func NewRequestCredential(keys []CredentialKey) (*Credential, error) {
	var bareKeys []CredentialKey
	for _, key := range keys {
		if key.Type != KeyTypeTUFDirector {
			bareKeys = append(bareKeys, key)
		}
	}
	if len(bareKeys) != 1 {
		return nil, errors.New("exactly one request verification key is required")
	}
	credential := &Credential{
		Version:            ProtocolVersion,
		decodedKeys:        make(map[string]verificationKey, 1),
		trustBackendPolicy: true,
	}
	if err := addCredentialKey(credential.decodedKeys, bareKeys[0], false); err != nil {
		return nil, err
	}
	return credential, nil
}

func addCredentialKey(keys map[string]verificationKey, raw CredentialKey, replace bool) error {
	if raw.ID == "" {
		return errors.New("credential key id is required")
	}
	if _, exists := keys[raw.ID]; exists && !replace {
		return fmt.Errorf("duplicate credential key %q", raw.ID)
	}
	key, err := decodeKey(raw)
	if err != nil {
		return fmt.Errorf("decode credential key %q: %w", raw.ID, err)
	}
	keys[raw.ID] = key
	return nil
}

func decodeKey(raw CredentialKey) (verificationKey, error) {
	block, _ := pem.Decode([]byte(raw.PEM))
	if block == nil {
		return nil, errors.New("invalid PEM")
	}
	switch raw.Type {
	case KeyTypeX509RSA:
		cert, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			key, ok := cert.PublicKey.(*rsa.PublicKey)
			if !ok {
				return nil, errors.New("certificate does not contain an RSA key")
			}
			return rsaKey{key: key}, nil
		}
		parsed, publicKeyErr := x509.ParsePKIXPublicKey(block.Bytes)
		if publicKeyErr != nil {
			return nil, fmt.Errorf("parse RSA certificate or public key: %w", err)
		}
		key, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("PEM does not contain an RSA key")
		}
		return rsaKey{key: key}, nil
	case KeyTypeED25519:
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		key, ok := parsed.(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("PEM does not contain an Ed25519 key")
		}
		return ed25519Key{key: key}, nil
	default:
		return nil, fmt.Errorf("unsupported key type %q", raw.Type)
	}
}

func (c *Credential) verifyEnvelope(envelope SignedEnvelope) error {
	if envelope.HashType != "SHA256" {
		return fmt.Errorf("unsupported hash type %q", envelope.HashType)
	}
	if len(envelope.Data) == 0 || len(envelope.Signatures) == 0 {
		return errors.New("signed envelope is incomplete")
	}
	digest := sha256.Sum256(envelope.Data)
	for _, signature := range envelope.Signatures {
		key := c.decodedKeys[signature.KeyID]
		if key == nil {
			continue
		}
		if key.keyType() != signature.KeyType {
			return fmt.Errorf("key type mismatch for %q", signature.KeyID)
		}
		if err := key.verify(digest[:], signature.Signature); err != nil {
			return fmt.Errorf("verify signature: %w", err)
		}
		return nil
	}
	return errors.New("no trusted signature found")
}
