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
// reads: loader, config provider, and the parsed instance target. No live check is
// needed because the fingerprint only reads the sanitized identity.
func fingerprintTestMatch(loader, provider string, instanceTarget integrationInstanceTarget) integrationCheckMatch {
	return integrationCheckMatch{
		sanitized:      sanitizedMatch{Integration: "postgres", Loader: loader, ConfigProvider: provider},
		instanceTarget: instanceTarget,
	}
}

func fingerprintTupleTarget() remoteQueryTarget {
	return remoteQueryTarget{Host: "localhost", Port: 5432, DBName: "postgres"}
}

// TestComputeMatchFingerprintIsDeterministic proves the same inputs hash to the
// same opaque value, so a fingerprint issued by resolve still verifies at execute
// time when nothing changed.
func TestComputeMatchFingerprintIsDeterministic(t *testing.T) {
	match := fingerprintTestMatch("python", "file", integrationInstanceTarget{host: "localhost", port: 5432, dbname: "postgres", databaseInstance: "rq-proof-a1-db1"})

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

// TestComputeMatchFingerprintChangesWithIdentity proves the fingerprint changes
// whenever the requested target or the selected check identity changes: any of
// these changes between resolve and execute must fail the execute revalidation.
func TestComputeMatchFingerprintChangesWithIdentity(t *testing.T) {
	match := fingerprintTestMatch("python", "file", integrationInstanceTarget{host: "localhost", port: 5432, dbname: "postgres", databaseInstance: "rq-proof-a1-db1"})
	base, err := computeMatchFingerprint("postgres", fingerprintTupleTarget(), match)
	require.NoError(t, err)

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
			target:      remoteQueryTarget{Host: "otherhost", Port: 5432, DBName: "postgres"},
			match:       match,
		},
		{
			name:        "different target port",
			integration: "postgres",
			target:      remoteQueryTarget{Host: "localhost", Port: 5433, DBName: "postgres"},
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
			name:        "different database_instance value",
			integration: "postgres",
			target:      remoteQueryTarget{DatabaseInstance: "rq-proof-a2-db1"},
			match:       match,
		},
		{
			name:        "different matched instance host",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", integrationInstanceTarget{host: "otherhost", port: 5432, dbname: "postgres", databaseInstance: "rq-proof-a1-db1"}),
		},
		{
			name:        "different matched instance port",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", integrationInstanceTarget{host: "localhost", port: 5433, dbname: "postgres", databaseInstance: "rq-proof-a1-db1"}),
		},
		{
			name:        "different matched instance dbname",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", integrationInstanceTarget{host: "localhost", port: 5432, dbname: "other", databaseInstance: "rq-proof-a1-db1"}),
		},
		{
			name:        "different matched instance identifier",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "file", integrationInstanceTarget{host: "localhost", port: 5432, dbname: "postgres", databaseInstance: "rq-proof-a2-db1"}),
		},
		{
			name:        "different loader",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("gohai", "file", integrationInstanceTarget{host: "localhost", port: 5432, dbname: "postgres", databaseInstance: "rq-proof-a1-db1"}),
		},
		{
			name:        "different config provider",
			integration: "postgres",
			target:      fingerprintTupleTarget(),
			match:       fingerprintTestMatch("python", "kubelet", integrationInstanceTarget{host: "localhost", port: 5432, dbname: "postgres", databaseInstance: "rq-proof-a1-db1"}),
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
// credentials or raw integration configuration: it is hex-encoded SHA-256, so
// only [0-9a-f] characters can appear, regardless of the config it was derived
// from. The instance target itself only carries parsed host/port/dbname/identifier
// fields — the raw config (with its password) never enters the canonical JSON.
func TestComputeMatchFingerprintEncodesNoSecrets(t *testing.T) {
	const secretPassword = "secret-value"
	const secretConfig = "host: localhost\nport: 5432\ndbname: postgres\nusername: alice\npassword: " + secretPassword + "\n"

	instanceTarget, ok := parseIntegrationInstanceTarget(postgresIntegration, secretConfig)
	require.True(t, ok)

	fingerprint, err := computeMatchFingerprint("postgres", fingerprintTupleTarget(), fingerprintTestMatch("python", "file", instanceTarget))
	require.NoError(t, err)

	assert.Regexp(t, fingerprintHexPattern, fingerprint)
	assert.NotContains(t, fingerprint, secretPassword)
	assert.NotContains(t, fingerprint, "alice")
}
