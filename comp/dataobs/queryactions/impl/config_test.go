// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDODatabaseConfig_ToInstanceMap(t *testing.T) {
	cfg := DODatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		DBName:   "testdb",
		Username: "do_reader",
		Password: "secret",
		SSLMode:  "require",
	}

	m := cfg.toInstanceMap()

	assert.Equal(t, "localhost", m["host"])
	assert.Equal(t, 5432, m["port"])
	assert.Equal(t, "testdb", m["dbname"])
	assert.Equal(t, "do_reader", m["username"])
	assert.Equal(t, "secret", m["password"])
	assert.Equal(t, "require", m["ssl_mode"])

	// Zero-value fields should be omitted
	_, hasSSL := m["ssl"]
	assert.False(t, hasSSL, "zero-value ssl field should not be in output")
	_, hasAWS := m["aws"]
	assert.False(t, hasAWS, "zero-value aws field should not be in output")
}

func TestDODatabaseConfig_ToInstanceMap_ExplicitFalse(t *testing.T) {
	cfg := DODatabaseConfig{
		Host:      "localhost",
		Port:      5432,
		TLSVerify: false, // explicit false must be preserved
		SSL:       false, // explicit false must be preserved
	}

	m := cfg.toInstanceMap()

	assert.Equal(t, false, m["tls_verify"], "explicit false for tls_verify must be preserved")
	assert.Equal(t, false, m["ssl"], "explicit false for ssl must be preserved")
}

func TestDODatabaseConfig_ToInstanceMap_WithNestedAWS(t *testing.T) {
	cfg := DODatabaseConfig{
		Host:     "mydb.rds.amazonaws.com",
		Port:     5432,
		Username: "datadog",
		AWS: map[string]any{
			"instance_endpoint": "my-rds-instance",
			"region":            "us-east-1",
		},
	}

	m := cfg.toInstanceMap()

	assert.Equal(t, "mydb.rds.amazonaws.com", m["host"])
	awsMap, ok := m["aws"].(map[string]any)
	require.True(t, ok, "aws should be a map[string]any")
	assert.Equal(t, "my-rds-instance", awsMap["instance_endpoint"])
	assert.Equal(t, "us-east-1", awsMap["region"])
}

func TestDODatabaseConfig_ToInstanceMap_OnlyAllowlistKeys(t *testing.T) {
	// All fields in the struct should map to dbCredentialAllowList keys.
	// If a new struct field is added without a corresponding allowlist entry,
	// toInstanceMap won't emit it (by design).
	cfg := DODatabaseConfig{
		Host:     "h",
		Port:     1,
		DBName:   "d",
		Username: "u",
		Password: "p",
	}
	m := cfg.toInstanceMap()
	for key := range m {
		found := false
		for _, allowed := range dbCredentialAllowList {
			if key == allowed {
				found = true
				break
			}
		}
		assert.True(t, found, "key %q in output is not in dbCredentialAllowList", key)
	}
}

func TestDBCredentialAllowList_AllHaveMatchingStructTag(t *testing.T) {
	// Every entry in dbCredentialAllowList must have a corresponding mapstructure tag
	// in DODatabaseConfig. This catches drift between the two.
	for _, key := range dbCredentialAllowList {
		_, ok := fieldByMapstructureTag[key]
		assert.True(t, ok, "dbCredentialAllowList entry %q has no matching mapstructure tag in DODatabaseConfig", key)
	}
}

func TestDODatabaseConfig_MatchesIdentifier(t *testing.T) {
	cfg := DODatabaseConfig{Host: "localhost", DBName: "mydb"}

	t.Run("exact match", func(t *testing.T) {
		assert.True(t, cfg.matchesIdentifier(&DBIdentifier{Host: "localhost", DBName: "mydb"}))
	})

	t.Run("host mismatch", func(t *testing.T) {
		assert.False(t, cfg.matchesIdentifier(&DBIdentifier{Host: "otherhost", DBName: "mydb"}))
	})

	t.Run("dbname mismatch", func(t *testing.T) {
		assert.False(t, cfg.matchesIdentifier(&DBIdentifier{Host: "localhost", DBName: "otherdb"}))
	})

	t.Run("empty dbname matches empty", func(t *testing.T) {
		emptyCfg := DODatabaseConfig{Host: "localhost"}
		assert.True(t, emptyCfg.matchesIdentifier(&DBIdentifier{Host: "localhost", DBName: ""}))
	})

	t.Run("empty dbname does not match non-empty", func(t *testing.T) {
		emptyCfg := DODatabaseConfig{Host: "localhost"}
		assert.False(t, emptyCfg.matchesIdentifier(&DBIdentifier{Host: "localhost", DBName: "mydb"}))
	})
}
