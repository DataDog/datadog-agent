// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
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

func TestParseRangeConfigRetriesExplicitZeroPreserved(t *testing.T) {
	raw := []byte(`{
		"kind": "autodiscovery",
		"autodiscovery_id": "ad-1",
		"cidr": "10.0.0.0/24",
		"credential_ids": ["cred-a"],
		"snmp_options": {"retries": 0}
	}`)

	cfg, err := parseRangeConfig(raw, testDefaults)
	require.NoError(t, err)

	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, 0, cfg.SNMPOptions.Retries, "an explicit retries:0 is a legitimate do-not-retry setting and must not be overwritten by the default")
}

func TestParseRangeConfigRetriesDefaultedWhenAbsent(t *testing.T) {
	raw := []byte(`{
		"kind": "autodiscovery",
		"autodiscovery_id": "ad-1",
		"cidr": "10.0.0.0/24",
		"credential_ids": ["cred-a"],
		"snmp_options": {"port": 1161}
	}`)

	cfg, err := parseRangeConfig(raw, testDefaults)
	require.NoError(t, err)

	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, defaultSNMPRetries, cfg.SNMPOptions.Retries, "retries omitted from the payload must fall back to the default")
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

func TestParseRangeConfigValidatesAutodiscoveryID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		ok   bool
	}{
		{"uuid", "6f1b2c3d-4e5f-4a6b-8c9d-0e1f2a3b4c5d", true},
		{"underscores and dashes", "range_a-1", true},
		{"dot", "range.a", false},
		{"slash", "range/a", false},
		{"sanitises to empty", "...", false},
		{"space", "range a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{
				"kind":             "autodiscovery",
				"autodiscovery_id": tt.id,
				"cidr":             "10.0.0.0/24",
				"credential_ids":   []string{"cred-a"},
			})
			require.NoError(t, err)

			cfg, err := parseRangeConfig(raw, testDefaults)
			if tt.ok {
				require.NoError(t, err)
				assert.Equal(t, tt.id, cfg.AutodiscoveryID)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "autodiscovery_id")
		})
	}
}

func TestParseRangeConfigValidatesNumericOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    string
		errPart string
	}{
		{"negative snmp timeout", `"snmp_options":{"timeout_ms":-1}`, "snmp_options.timeout_ms"},
		{"huge snmp timeout", `"snmp_options":{"timeout_ms":100000000}`, "snmp_options.timeout_ms"},
		{"negative retries", `"snmp_options":{"retries":-5}`, "snmp_options.retries"},
		{"huge retries", `"snmp_options":{"retries":500}`, "snmp_options.retries"},
		{"negative ping count", `"ping_options":{"count":-1}`, "ping_options.count"},
		{"huge ping count", `"ping_options":{"count":100000}`, "ping_options.count"},
		{"negative ping interval", `"ping_options":{"interval_ms":-1}`, "ping_options.interval_ms"},
		{"huge ping interval", `"ping_options":{"interval_ms":100000000}`, "ping_options.interval_ms"},
		{"negative ping timeout", `"ping_options":{"timeout_ms":-1}`, "ping_options.timeout_ms"},
		{"huge ping timeout", `"ping_options":{"timeout_ms":100000000}`, "ping_options.timeout_ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(`{"kind":"autodiscovery","autodiscovery_id":"ad-1","cidr":"10.0.0.0/24","credential_ids":["c"],` + tt.opts + `}`)
			_, err := parseRangeConfig(raw, testDefaults)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}

func TestParseRangeConfigRetriesExplicitZeroStillAccepted(t *testing.T) {
	raw := []byte(`{"kind":"autodiscovery","autodiscovery_id":"ad-1","cidr":"10.0.0.0/24","credential_ids":["c"],"snmp_options":{"retries":0}}`)
	cfg, err := parseRangeConfig(raw, testDefaults)
	require.NoError(t, err)
	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, 0, cfg.SNMPOptions.Retries, "retries:0 is a legitimate do-not-retry setting and must survive validation")
}
