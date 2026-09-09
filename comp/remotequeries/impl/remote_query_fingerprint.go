// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// remoteQueryMatchFingerprintVersion is the version of the canonical fingerprint
// encoding. The version travels inside the hashed canonical JSON, so the encoding
// can evolve without ambiguity: fingerprints from different versions compare
// unequal by construction.
const remoteQueryMatchFingerprintVersion = 1

// Target selector modes named in the canonical fingerprint: the flat host/port/
// dbname tuple or the managed database_instance identifier. The selector type is
// hashed explicitly so a tuple target and an instance target that happen to share
// field values can never collide.
const (
	remoteQueryFingerprintSelectorTuple            = "tuple"
	remoteQueryFingerprintSelectorDatabaseInstance = "database_instance"
)

// remoteQueryMatchFingerprintJSON is the versioned canonical JSON hashed into the
// match fingerprint: the integration, the selector type with its normalized target
// fields, and the sanitized matched-check identity (loader, config provider, and
// the matched instance target). Struct field order fixes the JSON field order, so
// encoding is deterministic for identical inputs; nothing here can carry
// credentials or raw integration configuration.
type remoteQueryMatchFingerprintJSON struct {
	Version     int                              `json:"v"`
	Integration string                           `json:"integration"`
	Target      remoteQueryFingerprintTargetJSON `json:"target"`
	Match       remoteQueryFingerprintMatchJSON  `json:"match"`
}

// remoteQueryFingerprintTargetJSON is the normalized requested target. Empty
// fields are omitted per selector mode: a tuple target carries host, port, and
// dbname; a database_instance target carries only the identifier.
type remoteQueryFingerprintTargetJSON struct {
	Selector         string `json:"selector"`
	Host             string `json:"host,omitempty"`
	Port             int    `json:"port,omitempty"`
	DBName           string `json:"dbname,omitempty"`
	DatabaseInstance string `json:"database_instance,omitempty"`
}

// remoteQueryFingerprintMatchJSON is the sanitized identity of the matched check:
// its loader, config provider, and the target parsed from its instance config.
type remoteQueryFingerprintMatchJSON struct {
	Loader         string                                   `json:"loader"`
	ConfigProvider string                                   `json:"config_provider"`
	InstanceTarget remoteQueryFingerprintInstanceTargetJSON `json:"instance_target"`
}

// remoteQueryFingerprintInstanceTargetJSON is the matched check's effective
// instance target: the normalized host/port/dbname plus the rendered database
// identifier. A change to any of them changes the fingerprint, so a config reload
// between resolve and execute fails the execute revalidation.
type remoteQueryFingerprintInstanceTargetJSON struct {
	Host             string `json:"host,omitempty"`
	Port             int    `json:"port,omitempty"`
	DBName           string `json:"dbname,omitempty"`
	DatabaseInstance string `json:"database_instance,omitempty"`
}

func remoteQueryFingerprintTargetOf(target remoteQueryTarget) remoteQueryFingerprintTargetJSON {
	if target.DatabaseInstance != "" {
		return remoteQueryFingerprintTargetJSON{
			Selector:         remoteQueryFingerprintSelectorDatabaseInstance,
			DatabaseInstance: target.DatabaseInstance,
		}
	}
	return remoteQueryFingerprintTargetJSON{
		Selector: remoteQueryFingerprintSelectorTuple,
		Host:     target.Host,
		Port:     target.Port,
		DBName:   target.DBName,
	}
}

func remoteQueryFingerprintInstanceTargetOf(instanceTarget integrationInstanceTarget) remoteQueryFingerprintInstanceTargetJSON {
	return remoteQueryFingerprintInstanceTargetJSON{
		Host:             instanceTarget.host,
		Port:             instanceTarget.port,
		DBName:           instanceTarget.dbname,
		DatabaseInstance: instanceTarget.databaseInstance,
	}
}

// computeMatchFingerprint returns the opaque, versioned fingerprint identifying
// the selected check and the requested target: hex-encoded SHA-256 over the
// canonical JSON. It is deterministic for the same inputs and changes whenever
// the selected check identity (loader, config provider, or instance target) or
// the requested target changes. The fingerprint has no cross-Agent meaning and
// is only compared for equality within the resolve->execute gap.
func computeMatchFingerprint(integration string, target remoteQueryTarget, match integrationCheckMatch) (string, error) {
	canonical := remoteQueryMatchFingerprintJSON{
		Version:     remoteQueryMatchFingerprintVersion,
		Integration: integration,
		Target:      remoteQueryFingerprintTargetOf(target),
		Match: remoteQueryFingerprintMatchJSON{
			Loader:         match.sanitized.Loader,
			ConfigProvider: match.sanitized.ConfigProvider,
			InstanceTarget: remoteQueryFingerprintInstanceTargetOf(match.instanceTarget),
		},
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode match fingerprint canonical JSON: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// matchFingerprintEqual reports whether the expected resolve-time fingerprint
// still identifies the freshly matched check on the requested target. The
// comparison recomputes the fingerprint from live state, so it answers false for
// a different check identity, a changed target, and a changed check config
// alike; an error means matching could not be revalidated at all.
func matchFingerprintEqual(expected string, integration string, target remoteQueryTarget, match integrationCheckMatch) (bool, error) {
	actual, err := computeMatchFingerprint(integration, target, match)
	if err != nil {
		return false, err
	}
	return expected == actual, nil
}
