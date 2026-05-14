// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package rcstore implements the TUF metadata signing used by fakeintake's
// Remote Config endpoints.
package rcstore

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// TUFExpires is hardcoded for fakeintake; long horizon so test setups don't
// hit expiry.
const TUFExpires = "2030-01-01T00:00:00Z"

// Config is a single Remote Config entry held by the in-memory store.
type Config struct {
	OrgID      string
	Product    string
	ConfigID   string
	ConfigName string
	Data       []byte
}

// Path returns the TUF target path for the config: datadog/<org>/<product>/<id>/<name>.
func (c Config) Path() string {
	return fmt.Sprintf("datadog/%s/%s/%s/%s", c.OrgID, c.Product, c.ConfigID, c.ConfigName)
}

// RepoMetas bundles the four TUF metadata documents that make up a repo.
type RepoMetas struct {
	Root      []byte
	Targets   []byte
	Snapshot  []byte
	Timestamp []byte
}

// CanonicalJSON encodes v in TUF canonical form: object keys sorted
// lexicographically, no whitespace, and no HTML escaping.
//
// Only used for hashing (the key ID and digest inputs). The signed payloads
// themselves are signed-then-embedded verbatim, so byte-identical re-serialisation
// is not required there.
func CanonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeCanonical(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case string:
		return writeCanonicalString(buf, val)
	default:
		// numbers, bools, nil — let json.Marshal handle them; not affected by
		// HTML escaping.
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		buf.Write(b)
		return nil
	}
}

func writeCanonicalString(buf *bytes.Buffer, s string) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	// json.Encoder.Encode appends a trailing newline; trim it.
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		buf.Truncate(buf.Len() - 1)
	}
	return nil
}

// signedEnvelope is the outer TUF wrapper. The signed field is embedded as
// json.RawMessage so the bytes the verifier reads are the bytes we signed.
type signedEnvelope struct {
	Signatures []tufSignature  `json:"signatures"`
	Signed     json.RawMessage `json:"signed"`
}

type tufSignature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

// SignEnvelope marshals signedValue once, signs those bytes with key, and
// returns a TUF envelope embedding the same bytes verbatim.
func SignEnvelope(key ed25519.PrivateKey, keyID string, signedValue any) ([]byte, error) {
	signedBytes, err := marshalNoEscape(signedValue)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(key, signedBytes)
	env := signedEnvelope{
		Signatures: []tufSignature{{KeyID: keyID, Sig: hex.EncodeToString(sig)}},
		Signed:     signedBytes,
	}
	return marshalNoEscape(env)
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// ComputeKeyID returns hex(sha256(canonical_json(key_object))) for an ed25519
// key, matching go-tuf's keyid computation when keyid_hash_algorithms is omitted.
func ComputeKeyID(publicKeyHex string) (string, error) {
	keyObj := map[string]any{
		"keytype": "ed25519",
		"keyval":  map[string]any{"public": publicKeyHex},
		"scheme":  "ed25519",
	}
	canonical, err := CanonicalJSON(keyObj)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// BuildRootJSON returns a signed root.json containing a single ed25519 key
// trusted for all four TUF roles.
func BuildRootJSON(key ed25519.PrivateKey, keyID, publicKeyHex string) ([]byte, error) {
	signed := map[string]any{
		"_type":               "root",
		"consistent_snapshot": true,
		"expires":             TUFExpires,
		"keys": map[string]any{
			keyID: map[string]any{
				"keytype": "ed25519",
				"keyval":  map[string]any{"public": publicKeyHex},
				"scheme":  "ed25519",
			},
		},
		"roles": map[string]any{
			"root":      map[string]any{"keyids": []any{keyID}, "threshold": 1},
			"snapshot":  map[string]any{"keyids": []any{keyID}, "threshold": 1},
			"targets":   map[string]any{"keyids": []any{keyID}, "threshold": 1},
			"timestamp": map[string]any{"keyids": []any{keyID}, "threshold": 1},
		},
		"spec_version": "1.0",
		"version":      1,
	}
	return SignEnvelope(key, keyID, signed)
}

// GenerateTUFMetas builds targets/snapshot/timestamp at the given version from
// the supplied configs, alongside the cached root bytes.
func GenerateTUFMetas(cfgs []Config, key ed25519.PrivateKey, keyID string, root []byte, version uint64) (RepoMetas, error) {
	targetsMap := make(map[string]any, len(cfgs))
	for _, c := range cfgs {
		targetsMap[c.Path()] = map[string]any{
			"custom": map[string]any{"v": version},
			"hashes": map[string]any{"sha256": sha256Hex(c.Data)},
			"length": uint64(len(c.Data)),
		}
	}

	targetsSigned := map[string]any{
		"_type":        "targets",
		"expires":      TUFExpires,
		"spec_version": "1.0",
		"targets":      targetsMap,
		"version":      version,
	}
	targetsJSON, err := SignEnvelope(key, keyID, targetsSigned)
	if err != nil {
		return RepoMetas{}, fmt.Errorf("sign targets: %w", err)
	}

	snapshotSigned := map[string]any{
		"_type":   "snapshot",
		"expires": TUFExpires,
		"meta": map[string]any{
			"targets.json": map[string]any{
				"hashes":  map[string]any{"sha256": sha256Hex(targetsJSON)},
				"length":  uint64(len(targetsJSON)),
				"version": version,
			},
		},
		"spec_version": "1.0",
		"version":      version,
	}
	snapshotJSON, err := SignEnvelope(key, keyID, snapshotSigned)
	if err != nil {
		return RepoMetas{}, fmt.Errorf("sign snapshot: %w", err)
	}

	timestampSigned := map[string]any{
		"_type":   "timestamp",
		"expires": TUFExpires,
		"meta": map[string]any{
			"snapshot.json": map[string]any{
				"hashes":  map[string]any{"sha256": sha256Hex(snapshotJSON)},
				"length":  uint64(len(snapshotJSON)),
				"version": version,
			},
		},
		"spec_version": "1.0",
		"version":      version,
	}
	timestampJSON, err := SignEnvelope(key, keyID, timestampSigned)
	if err != nil {
		return RepoMetas{}, fmt.Errorf("sign timestamp: %w", err)
	}

	return RepoMetas{
		Root:      append([]byte(nil), root...),
		Targets:   targetsJSON,
		Snapshot:  snapshotJSON,
		Timestamp: timestampJSON,
	}, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// VerifyEnvelope is a small helper used by tests to round-trip our own output:
// parse the envelope, verify the signature against the embedded raw bytes.
func VerifyEnvelope(pub ed25519.PublicKey, envelope []byte) error {
	var env signedEnvelope
	if err := json.Unmarshal(envelope, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	if len(env.Signatures) == 0 {
		return errors.New("envelope has no signatures")
	}
	sig, err := hex.DecodeString(env.Signatures[0].Sig)
	if err != nil {
		return fmt.Errorf("decode sig: %w", err)
	}
	signed := bytes.TrimRight(env.Signed, "\n")
	if !ed25519.Verify(pub, signed, sig) {
		return errors.New("signature verification failed")
	}
	return nil
}

// PublicKeyHex is a convenience accessor for tests/handlers that need to
// surface the verifying key.
func PublicKeyHex(key ed25519.PrivateKey) string {
	pub := key.Public().(ed25519.PublicKey)
	return strings.ToLower(hex.EncodeToString(pub))
}
