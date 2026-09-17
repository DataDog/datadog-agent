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

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
)

var testDefaults = rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536}

func testProbeSet(t *testing.T) *probeSet {
	t.Helper()
	return newProbeSet(logmock.New(t),
		&stubProbe{name: "ping", check: "ping"},
		&stubProbe{name: "snmp", check: "snmp"},
	)
}

func testProbes() map[string]json.RawMessage {
	return map[string]json.RawMessage{"snmp": json.RawMessage(`{"credential_ids":["cred-a"]}`)}
}

func testRange(id, cidr string) ndmdiscovery.Range {
	return ndmdiscovery.Range{ID: id, CIDR: cidr, Probes: testProbes()}
}

func TestParseRangeFull(t *testing.T) {
	cfg, err := parseRange(ndmdiscovery.Range{
		ID:                 "ad-1",
		Namespace:          "prod",
		CIDR:               "10.0.0.0/24",
		IntervalSec:        900,
		IgnoredIPAddresses: []string{"10.0.0.1"},
		Tags:               []string{"site:paris"},
		Probes: map[string]json.RawMessage{
			"ping": json.RawMessage(`{"count":2}`),
			"snmp": json.RawMessage(`{"credential_ids":["cred-a"]}`),
		},
	}, testDefaults, testProbeSet(t))
	require.NoError(t, err)

	assert.Equal(t, "ad-1", cfg.AutodiscoveryID)
	assert.Equal(t, "prod", cfg.Namespace)
	assert.Equal(t, "10.0.0.0/24", cfg.CIDR)
	assert.Equal(t, 900, cfg.IntervalSec)
	assert.Equal(t, []string{"10.0.0.1"}, cfg.IgnoredIPAddresses)
	assert.Equal(t, []string{"site:paris"}, cfg.Tags)
	assert.Equal(t, []string{"ping", "snmp"}, kinds(cfg.Probes), "probes come back in registry order")
}

func TestParseRangeDefaults(t *testing.T) {
	cfg, err := parseRange(testRange("ad-1", "10.0.0.0/24"), testDefaults, testProbeSet(t))
	require.NoError(t, err)

	assert.Equal(t, "default", cfg.Namespace)
	assert.Equal(t, 3600, cfg.IntervalSec)
}

func TestParseRangeKeepsARangeWhoseProbesAreAllUnusable(t *testing.T) {
	set := newProbeSet(logmock.New(t), &stubProbe{name: "snmp", unavailable: true})

	cfg, err := parseRange(testRange("ad-1", "10.0.0.0/24"), testDefaults, set)
	require.NoError(t, err, "a probe-level problem does not reject the range")
	assert.Empty(t, cfg.Probes)
}

func TestParseRangeValidation(t *testing.T) {
	tests := []struct {
		name    string
		r       ndmdiscovery.Range
		errPart string
	}{
		{"missing id", ndmdiscovery.Range{CIDR: "10.0.0.0/24", Probes: testProbes()}, "id"},
		{"missing cidr", ndmdiscovery.Range{ID: "a", Probes: testProbes()}, "cidr"},
		{"bad cidr", ndmdiscovery.Range{ID: "a", CIDR: "nope", Probes: testProbes()}, "invalid CIDR"},
		{"range too large", ndmdiscovery.Range{ID: "a", CIDR: "10.0.0.0/12", Probes: testProbes()}, "exceeds the maximum"},
		{"no probes", ndmdiscovery.Range{ID: "a", CIDR: "10.0.0.0/24"}, "probes"},
		{"empty probes", ndmdiscovery.Range{ID: "a", CIDR: "10.0.0.0/24", Probes: map[string]json.RawMessage{}}, "probes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRange(tt.r, testDefaults, testProbeSet(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}

func TestParseRangeClampsInterval(t *testing.T) {
	r := testRange("ad-1", "10.0.0.0/24")
	r.IntervalSec = 5

	cfg, err := parseRange(r, testDefaults, testProbeSet(t))
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
			cfg, err := parseRange(testRange(tt.id, "10.0.0.0/24"), testDefaults, testProbeSet(t))
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
