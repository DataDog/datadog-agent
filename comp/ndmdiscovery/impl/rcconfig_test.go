// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDefaults = rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536}

func TestConfigKind(t *testing.T) {
	kind, err := configKind([]byte(`{"kind":"autodiscovery","cidr":"10.0.0.0/24"}`))
	require.NoError(t, err)
	assert.Equal(t, kindAutodiscovery, kind)

	kind, err = configKind([]byte(`{"kind":"monitored_devices"}`))
	require.NoError(t, err)
	assert.Equal(t, "monitored_devices", kind)

	kind, err = configKind([]byte(`{"cidr":"10.0.0.0/24"}`))
	require.NoError(t, err)
	assert.Empty(t, kind, "a payload with no kind yields an empty kind, not an error")

	_, err = configKind([]byte(`not json`))
	require.Error(t, err)
}

func TestParseRangeConfigFull(t *testing.T) {
	raw := []byte(`{
		"kind": "autodiscovery",
		"autodiscovery_id": "ad-1",
		"namespace": "prod",
		"cidr": "10.0.0.0/24",
		"credential_ids": ["cred-a", "cred-b"],
		"interval_sec": 900,
		"ignored_ip_addresses": ["10.0.0.1"],
		"tags": ["site:paris"],
		"snmp_options": {"port": 1161, "timeout_ms": 2000, "retries": 1},
		"ping_options": {"count": 1, "interval_ms": 1000, "timeout_ms": 1000}
	}`)

	cfg, err := parseRangeConfig(raw, testDefaults)
	require.NoError(t, err)

	assert.Equal(t, "ad-1", cfg.AutodiscoveryID)
	assert.Equal(t, "prod", cfg.Namespace)
	assert.Equal(t, "10.0.0.0/24", cfg.CIDR)
	assert.Equal(t, []string{"cred-a", "cred-b"}, cfg.CredentialIDs)
	assert.Equal(t, 900, cfg.IntervalSec)
	assert.Equal(t, []string{"10.0.0.1"}, cfg.IgnoredIPAddresses)
	assert.Equal(t, []string{"site:paris"}, cfg.Tags)

	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, 1161, cfg.SNMPOptions.Port)
	assert.Equal(t, 2000, cfg.SNMPOptions.TimeoutMs)
	assert.Equal(t, 1, cfg.SNMPOptions.Retries)

	require.NotNil(t, cfg.PingOptions)
	assert.Equal(t, 1, cfg.PingOptions.Count)
	assert.Equal(t, 1000, cfg.PingOptions.IntervalMs)
	assert.Equal(t, 1000, cfg.PingOptions.TimeoutMs)
}

func TestParseRangeConfigDefaults(t *testing.T) {
	raw := []byte(`{
		"kind": "autodiscovery",
		"autodiscovery_id": "ad-1",
		"cidr": "10.0.0.0/24",
		"credential_ids": ["cred-a"]
	}`)

	cfg, err := parseRangeConfig(raw, testDefaults)
	require.NoError(t, err)

	assert.Equal(t, "default", cfg.Namespace)
	assert.Equal(t, 3600, cfg.IntervalSec)

	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, defaultSNMPPort, cfg.SNMPOptions.Port)
	assert.Equal(t, defaultSNMPTimeoutMs, cfg.SNMPOptions.TimeoutMs)
	assert.Equal(t, defaultSNMPRetries, cfg.SNMPOptions.Retries)

	assert.Nil(t, cfg.PingOptions, "no ping_options means ping is disabled for the range")
}

func TestParseRangeConfigPingDefaultsWhenSectionPresent(t *testing.T) {
	raw := []byte(`{
		"kind": "autodiscovery",
		"autodiscovery_id": "ad-1",
		"cidr": "10.0.0.0/24",
		"credential_ids": ["cred-a"],
		"ping_options": {}
	}`)

	cfg, err := parseRangeConfig(raw, testDefaults)
	require.NoError(t, err)

	require.NotNil(t, cfg.PingOptions)
	assert.Equal(t, defaultPingCount, cfg.PingOptions.Count)
	assert.Equal(t, defaultPingIntervalMs, cfg.PingOptions.IntervalMs)
	assert.Equal(t, defaultPingTimeoutMs, cfg.PingOptions.TimeoutMs)
}

func TestParseRangeConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		errPart string
	}{
		{"wrong kind", `{"kind":"monitored_devices","autodiscovery_id":"a","cidr":"10.0.0.0/24","credential_ids":["c"]}`, "kind"},
		{"missing autodiscovery_id", `{"kind":"autodiscovery","cidr":"10.0.0.0/24","credential_ids":["c"]}`, "autodiscovery_id"},
		{"missing cidr", `{"kind":"autodiscovery","autodiscovery_id":"a","credential_ids":["c"]}`, "cidr"},
		{"bad cidr", `{"kind":"autodiscovery","autodiscovery_id":"a","cidr":"nope","credential_ids":["c"]}`, "invalid CIDR"},
		{"range too large", `{"kind":"autodiscovery","autodiscovery_id":"a","cidr":"10.0.0.0/12","credential_ids":["c"]}`, "exceeds the maximum"},
		{"no credentials", `{"kind":"autodiscovery","autodiscovery_id":"a","cidr":"10.0.0.0/24","credential_ids":[]}`, "credential_ids"},
		{"bad port", `{"kind":"autodiscovery","autodiscovery_id":"a","cidr":"10.0.0.0/24","credential_ids":["c"],"snmp_options":{"port":70000}}`, "port"},
		{"malformed json", `{`, "unexpected end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRangeConfig([]byte(tt.raw), testDefaults)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}

func TestParseRangeConfigClampsInterval(t *testing.T) {
	raw := []byte(`{
		"kind": "autodiscovery",
		"autodiscovery_id": "ad-1",
		"cidr": "10.0.0.0/24",
		"credential_ids": ["cred-a"],
		"interval_sec": 5
	}`)

	cfg, err := parseRangeConfig(raw, testDefaults)
	require.NoError(t, err)
	assert.Equal(t, minIntervalSec, cfg.IntervalSec)
}
