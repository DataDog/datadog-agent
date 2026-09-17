// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
)

var testDefaults = rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536}

func testRange(id, cidr string) ndmdiscovery.Range {
	return ndmdiscovery.Range{ID: id, CIDR: cidr, CredentialIDs: []string{"cred-a"}}
}

func TestParseRangeFull(t *testing.T) {
	retries := 1
	cfg, err := parseRange(ndmdiscovery.Range{
		ID:                 "ad-1",
		Namespace:          "prod",
		CIDR:               "10.0.0.0/24",
		CredentialIDs:      []string{"cred-a", "cred-b"},
		IntervalSec:        900,
		IgnoredIPAddresses: []string{"10.0.0.1"},
		Tags:               []string{"site:paris"},
		SNMPOptions:        &ndmdiscovery.SNMPOptions{Port: 1161, TimeoutMs: 2000, Retries: &retries},
		PingOptions:        &ndmdiscovery.PingOptions{Count: 1, IntervalMs: 1000, TimeoutMs: 1000},
	}, testDefaults)
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

func TestParseRangeDefaults(t *testing.T) {
	cfg, err := parseRange(testRange("ad-1", "10.0.0.0/24"), testDefaults)
	require.NoError(t, err)

	assert.Equal(t, "default", cfg.Namespace)
	assert.Equal(t, 3600, cfg.IntervalSec)

	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, defaultSNMPPort, cfg.SNMPOptions.Port)
	assert.Equal(t, defaultSNMPTimeoutMs, cfg.SNMPOptions.TimeoutMs)
	assert.Equal(t, defaultSNMPRetries, cfg.SNMPOptions.Retries)

	assert.Nil(t, cfg.PingOptions, "no ping options means ping is disabled for the range")
}

func TestParseRangeRetriesExplicitZeroPreserved(t *testing.T) {
	zero := 0
	r := testRange("ad-1", "10.0.0.0/24")
	r.SNMPOptions = &ndmdiscovery.SNMPOptions{Retries: &zero}

	cfg, err := parseRange(r, testDefaults)
	require.NoError(t, err)
	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, 0, cfg.SNMPOptions.Retries)
}

func TestParseRangeRetriesDefaultedWhenAbsent(t *testing.T) {
	r := testRange("ad-1", "10.0.0.0/24")
	r.SNMPOptions = &ndmdiscovery.SNMPOptions{Port: 1161}

	cfg, err := parseRange(r, testDefaults)
	require.NoError(t, err)
	require.NotNil(t, cfg.SNMPOptions)
	assert.Equal(t, defaultSNMPRetries, cfg.SNMPOptions.Retries)
}

func TestParseRangePingDefaultsWhenSectionPresent(t *testing.T) {
	r := testRange("ad-1", "10.0.0.0/24")
	r.PingOptions = &ndmdiscovery.PingOptions{}

	cfg, err := parseRange(r, testDefaults)
	require.NoError(t, err)
	require.NotNil(t, cfg.PingOptions)
	assert.Equal(t, defaultPingCount, cfg.PingOptions.Count)
	assert.Equal(t, defaultPingIntervalMs, cfg.PingOptions.IntervalMs)
	assert.Equal(t, defaultPingTimeoutMs, cfg.PingOptions.TimeoutMs)
}

func TestParseRangeValidation(t *testing.T) {
	tests := []struct {
		name    string
		r       ndmdiscovery.Range
		errPart string
	}{
		{"missing id", ndmdiscovery.Range{CIDR: "10.0.0.0/24", CredentialIDs: []string{"c"}}, "id"},
		{"missing cidr", ndmdiscovery.Range{ID: "a", CredentialIDs: []string{"c"}}, "cidr"},
		{"bad cidr", ndmdiscovery.Range{ID: "a", CIDR: "nope", CredentialIDs: []string{"c"}}, "invalid CIDR"},
		{"range too large", ndmdiscovery.Range{ID: "a", CIDR: "10.0.0.0/12", CredentialIDs: []string{"c"}}, "exceeds the maximum"},
		{"no credentials", ndmdiscovery.Range{ID: "a", CIDR: "10.0.0.0/24"}, "credential_ids"},
		{
			"bad port",
			ndmdiscovery.Range{ID: "a", CIDR: "10.0.0.0/24", CredentialIDs: []string{"c"}, SNMPOptions: &ndmdiscovery.SNMPOptions{Port: 70000}},
			"port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRange(tt.r, testDefaults)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}

func TestParseRangeClampsInterval(t *testing.T) {
	r := testRange("ad-1", "10.0.0.0/24")
	r.IntervalSec = 5

	cfg, err := parseRange(r, testDefaults)
	require.NoError(t, err)
	assert.Equal(t, minIntervalSec, cfg.IntervalSec)
}

func TestParseRangeValidatesTheRangeID(t *testing.T) {
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
			cfg, err := parseRange(testRange(tt.id, "10.0.0.0/24"), testDefaults)
			if tt.ok {
				require.NoError(t, err)
				assert.Equal(t, tt.id, cfg.AutodiscoveryID)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "id")
		})
	}
}

func TestParseRangeValidatesNumericOptions(t *testing.T) {
	negative, huge := -5, 500
	tests := []struct {
		name    string
		mutate  func(*ndmdiscovery.Range)
		errPart string
	}{
		{"negative snmp timeout", func(r *ndmdiscovery.Range) {
			r.SNMPOptions = &ndmdiscovery.SNMPOptions{TimeoutMs: -1}
		}, "snmp_options.timeout_ms"},
		{"huge snmp timeout", func(r *ndmdiscovery.Range) {
			r.SNMPOptions = &ndmdiscovery.SNMPOptions{TimeoutMs: 100000000}
		}, "snmp_options.timeout_ms"},
		{"negative retries", func(r *ndmdiscovery.Range) {
			r.SNMPOptions = &ndmdiscovery.SNMPOptions{Retries: &negative}
		}, "snmp_options.retries"},
		{"huge retries", func(r *ndmdiscovery.Range) {
			r.SNMPOptions = &ndmdiscovery.SNMPOptions{Retries: &huge}
		}, "snmp_options.retries"},
		{"negative ping count", func(r *ndmdiscovery.Range) {
			r.PingOptions = &ndmdiscovery.PingOptions{Count: -1}
		}, "ping_options.count"},
		{"huge ping count", func(r *ndmdiscovery.Range) {
			r.PingOptions = &ndmdiscovery.PingOptions{Count: 100000}
		}, "ping_options.count"},
		{"negative ping interval", func(r *ndmdiscovery.Range) {
			r.PingOptions = &ndmdiscovery.PingOptions{IntervalMs: -1}
		}, "ping_options.interval_ms"},
		{"huge ping interval", func(r *ndmdiscovery.Range) {
			r.PingOptions = &ndmdiscovery.PingOptions{IntervalMs: 100000000}
		}, "ping_options.interval_ms"},
		{"negative ping timeout", func(r *ndmdiscovery.Range) {
			r.PingOptions = &ndmdiscovery.PingOptions{TimeoutMs: -1}
		}, "ping_options.timeout_ms"},
		{"huge ping timeout", func(r *ndmdiscovery.Range) {
			r.PingOptions = &ndmdiscovery.PingOptions{TimeoutMs: 100000000}
		}, "ping_options.timeout_ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testRange("ad-1", "10.0.0.0/24")
			tt.mutate(&r)
			_, err := parseRange(r, testDefaults)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}
