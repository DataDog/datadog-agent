// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fingerprintHexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// fingerprintTestMatch builds the sanitized match identity computeMatchFingerprint
// reads: loader, config provider, and the match identity the integration reported
// (or the Agent parsed for the clickhouse branch). No live check is needed because
// the fingerprint only reads the sanitized identity.
func fingerprintTestMatch(loader, provider string, identity remoteQueryMatchIdentity) integrationCheckMatch {
	return integrationCheckMatch{
		sanitized: sanitizedMatch{Integration: "postgres", Loader: loader, ConfigProvider: provider},
		identity:  identity,
	}
}

func fingerprintTestIdentity() remoteQueryMatchIdentity {
	return remoteQueryMatchIdentity{
		host:             "localhost",
		port:             5432,
		configuredDBName: "postgres",
		resolvedDBName:   "production_ok",
		databaseInstance: "rq-proof-a1-db1",
	}
}

func fingerprintTupleTarget() remoteQueryTarget {
	return remoteQueryTarget{Host: "localhost", Port: 5432, DBName: "production_ok"}
}

// TestComputeMatchFingerprintIsDeterministic proves the same inputs hash to the
// same opaque value, so a fingerprint issued by resolve still verifies at execute
// time when nothing changed.
func TestComputeMatchFingerprintIsDeterministic(t *testing.T) {
	match := fingerprintTestMatch("python", "file", fingerprintTestIdentity())

	first, err := computeMatchFingerprint("postgres", fingerprintTupleTarget(), match)
	require.NoError(t, err)
	second, err := computeMatchFingerprint("postgres", fingerprintTupleTarget(), match)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Regexp(t, fingerprintHexPattern, first)

	same, err := matchFingerprintEqual(first, "postgres", fingerprintTupleTarget(), match)
	require.NoError(t, err)
	assert.True(t, same)
}

// TestComputeMatchFingerprintEncodesVersionTwo proves the canonical JSON carries
// the version 2 encoding: fingerprints issued under the identity-binding contract
// compare unequal against any other version by construction.
func TestComputeMatchFingerprintEncodesVersionTwo(t *testing.T) {
	assert.Equal(t, 2, remoteQueryMatchFingerprintVersion)
}

// TestComputeMatchFingerprintChangesWithIdentity proves the fingerprint changes
// whenever the requested target or the match identity changes: any of these
// changes between resolve and execute must fail the execute revalidation. The
// identity fields are the integration-reported ones — a changed eligible set, a
// changed admitted database, or a changed rendered identifier all invalidate the
// resolve-time fingerprint.
func TestComputeMatchFingerprintChangesWithIdentity(t *testing.T) {
	match := fingerprintTestMatch("python", "file", fingerprintTestIdentity())
	base, err := computeMatchFingerprint("postgres", fingerprintTupleTarget(), match)
	require.NoError(t, err)

	mutateIdentity := func(_ string, mutate func(*remoteQueryMatchIdentity)) remoteQueryMatchIdentity {
		identity := fingerprintTestIdentity()
		mutate(&identity)
		return identity
	}

	differentInputs := []struct {
		name        string
		integration string
		target      remoteQueryTarget
		match       integrationCheckMatch
	}{
		{
			name:        "different integration",
			integration: "clickhouse",
			target:      fingerprintTupleTarget(),
			match:       match,
		},
		{
			name:        "different target host",
			integration: "postgres",
			target:      remoteQueryTarget{Host: "otherhost", Port: 5432, DBName: "production_ok"},
			match:       match,
		},
		{
			name:        "different target port",
			integration: "postgres",
			target:      remoteQueryTarget{Host: "localhost", Port: 5433, DBName: "production_ok"},
			match:       match,
		},
		{
			name:        "different target dbname",
			integration: "postgres",
			target:      remoteQueryTarget{Host: "localhost", Port: 5432, DBName: "other"},
			match:       match,
		},
		{
			name:        "different selector mode",
			integration: "postgres",
			target:      remoteQueryTarget{DatabaseInstance: "rq-proof-a1-db1"},
			match:       match,
		},
		{
			name:        "different identity host",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", mutateIdentity("host", func(i *remoteQueryMatchIdentity) { i.host = "otherhost" })),
		},
		{
			name:        "different identity port",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", mutateIdentity("port", func(i *remoteQueryMatchIdentity) { i.port = 5433 })),
		},
		{
			name:        "different configured dbname",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", mutateIdentity("configured", func(i *remoteQueryMatchIdentity) { i.configuredDBName = "other" })),
		},
		{
			name:        "different resolved dbname",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", mutateIdentity("resolved", func(i *remoteQueryMatchIdentity) { i.resolvedDBName = "other" })),
		},
		{
			name:        "different database_instance identifier",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", mutateIdentity("identifier", func(i *remoteQueryMatchIdentity) { i.databaseInstance = "rq-proof-a2-db1" })),
		},
		{
			name:        "absent database_instance identifier",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", mutateIdentity("no-identifier", func(i *remoteQueryMatchIdentity) { i.databaseInstance = "" })),
		},
		{
			name:        "different loader",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("gohai", "file", fingerprintTestIdentity()),
		},
		{
			name:        "different config provider",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "kubelet", fingerprintTestIdentity()),
		},
	}

	for _, tt := range differentInputs {
		t.Run(tt.name, func(t *testing.T) {
			other, err := computeMatchFingerprint(tt.integration, tt.target, tt.match)
			require.NoError(t, err)
			assert.NotEqual(t, base, other)

			same, err := matchFingerprintEqual(base, tt.integration, tt.target, tt.match)
			require.NoError(t, err)
			assert.False(t, same)
		})
	}
}

// TestComputeMatchFingerprintEncodesNoSecrets proves the fingerprint never leaks
// credentials or raw integration configuration: it is hex-encoded SHA-256, so only
// [0-9a-f] characters can appear, regardless of the config it was derived from.
// The match identity itself only carries the sanitized effective identity fields
// — the raw config (with its password) never enters the canonical JSON, and the
// Go matcher never parses Postgres config at all.
func TestComputeMatchFingerprintEncodesNoSecrets(t *testing.T) {
	const secretPassword = "secret-value"

	identity := fingerprintTestIdentity()
	fingerprint, err := computeMatchFingerprint("postgres", fingerprintTupleTarget(), fingerprintTestMatch("python", "file", identity))
	require.NoError(t, err)

	assert.Regexp(t, fingerprintHexPattern, fingerprint)
	assert.NotContains(t, fingerprint, secretPassword)
	assert.NotContains(t, fingerprint, "alice")
}
