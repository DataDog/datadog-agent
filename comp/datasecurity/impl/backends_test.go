// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datasecurityimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectBackend_Postgres(t *testing.T) {
	kind, err := detectBackend(rcPayload{
		Tasks: []rcTask{{
			ScanData: rcScanData{
				Postgres: &rcPostgresScanData{Query: "SELECT 1", Table: "users"},
			},
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, backendPostgres, kind)
}

func TestDetectBackend_NoScanData(t *testing.T) {
	_, err := detectBackend(rcPayload{
		Tasks: []rcTask{{ScanData: rcScanData{}}},
	})
	assert.ErrorIs(t, err, errNoScanData)
}

func TestExtractPostgresCredentials(t *testing.T) {
	cfg := extractPostgresCredentials(map[string]any{
		"host":     "pg.example.com",
		"port":     5433,
		"username": "reader",
		"password": "secret",
		"dbname":   "inventory",
	})
	assert.Equal(t, "pg.example.com", cfg.Host)
	assert.Equal(t, 5433, cfg.Port)
	assert.Equal(t, "reader", cfg.Username)
	assert.Equal(t, "secret", cfg.Password)
	assert.Equal(t, "inventory", cfg.Dbname)
}

func TestInstanceDataSecurityEnabled(t *testing.T) {
	assert.True(t, instanceDataSecurityEnabled(map[string]any{
		"data_security": map[string]any{"enabled": true},
	}))
	assert.False(t, instanceDataSecurityEnabled(map[string]any{
		"data_security": map[string]any{"enabled": false},
	}))
	assert.False(t, instanceDataSecurityEnabled(map[string]any{}))
}
