// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
)

var testDefaults = rangeDefaults{Namespace: "default", IntervalSec: 3600, MaxAddresses: 65536}

func testProbes() map[string]json.RawMessage {
	return map[string]json.RawMessage{"snmp": json.RawMessage(`{"cred_ids":["cred-a"]}`)}
}

func testRange(id, cidr string) ndmdiscovery.Range {
	return ndmdiscovery.Range{ID: id, NetworkAddress: cidr, Probes: testProbes()}
}

func TestParseRangeFull(t *testing.T) {
	cfg, err := parseRange(ndmdiscovery.Range{
		ID:                 "ad-1",
		Namespace:          "prod",
		NetworkAddress:     "10.0.0.0/24",
		IntervalSec:        900,
		IgnoredIPAddresses: []string{"10.0.0.1"},
		Tags:               []string{"site:paris"},
		Probes: map[string]json.RawMessage{
			"ping": json.RawMessage(`{"count":2}`),
			"snmp": json.RawMessage(`{"cred_ids":["cred-a"]}`),
		},
	}, testDefaults, logmock.New(t))
	require.NoError(t, err)

	assert.Equal(t, "ad-1", cfg.AutodiscoveryID)
	assert.Equal(t, "prod", cfg.Namespace)
	assert.Equal(t, "10.0.0.0/24", cfg.NetworkAddress)
	assert.Equal(t, 900, cfg.IntervalSec)
	assert.Equal(t, []string{"10.0.0.1"}, cfg.IgnoredIPAddresses)
	assert.Equal(t, []string{"site:paris"}, cfg.Tags)
	require.NotNil(t, cfg.Probes.Ping)
	assert.Equal(t, 2, cfg.Probes.Ping.Count)
	require.NotNil(t, cfg.Probes.SNMP)
	assert.Equal(t, []string{"cred-a"}, cfg.Probes.SNMP.CredIDs)
}

func TestParseRangeDefaults(t *testing.T) {
	cfg, err := parseRange(testRange("ad-1", "10.0.0.0/24"), testDefaults, logmock.New(t))
	require.NoError(t, err)

	assert.Equal(t, "default", cfg.Namespace)
	assert.Equal(t, 3600, cfg.IntervalSec)
}

func TestParseRangeKeepsARangeWhoseProbesAreAllUnusable(t *testing.T) {
	r := ndmdiscovery.Range{ID: "ad-1", NetworkAddress: "10.0.0.0/24", Probes: map[string]json.RawMessage{
		"ping": json.RawMessage(`[]`),
	}}

	cfg, err := parseRange(r, testDefaults, logmock.New(t))
	require.NoError(t, err, "a probe-level problem does not reject the range")
	assert.Nil(t, cfg.Probes.Ping)
	assert.Nil(t, cfg.Probes.SNMP)
}

func TestParseRangeValidation(t *testing.T) {
	tests := []struct {
		name    string
		r       ndmdiscovery.Range
		errPart string
	}{
		{"missing id", ndmdiscovery.Range{NetworkAddress: "10.0.0.0/24", Probes: testProbes()}, "id"},
		{"missing cidr", ndmdiscovery.Range{ID: "a", Probes: testProbes()}, "network_address"},
		{"bad cidr", ndmdiscovery.Range{ID: "a", NetworkAddress: "nope", Probes: testProbes()}, "invalid CIDR"},
		{"range too large", ndmdiscovery.Range{ID: "a", NetworkAddress: "10.0.0.0/12", Probes: testProbes()}, "exceeds the maximum"},
		{"no probes", ndmdiscovery.Range{ID: "a", NetworkAddress: "10.0.0.0/24"}, "probes"},
		{"empty probes", ndmdiscovery.Range{ID: "a", NetworkAddress: "10.0.0.0/24", Probes: map[string]json.RawMessage{}}, "probes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRange(tt.r, testDefaults, logmock.New(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
		})
	}
}

func TestParseRangeClampsInterval(t *testing.T) {
	r := testRange("ad-1", "10.0.0.0/24")
	r.IntervalSec = 5

	cfg, err := parseRange(r, testDefaults, logmock.New(t))
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
			cfg, err := parseRange(testRange(tt.id, "10.0.0.0/24"), testDefaults, logmock.New(t))
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

func TestRangeConfigEqual(t *testing.T) {
	base := func() rangeConfig {
		cfg := testRangeConfig("ad-1", "10.0.0.0/26")
		cfg.Tags = []string{"site:paris"}
		cfg.Probes.Ping = &pingprobe.Options{Count: 1, Interval: time.Second, Timeout: time.Second}
		return cfg
	}

	tests := map[string]struct {
		mutate func(*rangeConfig)
		want   bool
	}{
		"identical":                      {mutate: func(*rangeConfig) {}, want: true},
		"nil and empty ignored list":     {mutate: func(c *rangeConfig) { c.IgnoredIPAddresses = []string{} }, want: true},
		"another network address":        {mutate: func(c *rangeConfig) { c.NetworkAddress = "10.0.1.0/26" }, want: false},
		"another interval":               {mutate: func(c *rangeConfig) { c.IntervalSec = 60 }, want: false},
		"another tag":                    {mutate: func(c *rangeConfig) { c.Tags = []string{"site:lyon"} }, want: false},
		"another credential":             {mutate: func(c *rangeConfig) { c.Probes.SNMP.CredIDs = []string{"cred-b"} }, want: false},
		"another ping timeout":           {mutate: func(c *rangeConfig) { c.Probes.Ping.Timeout = 2 * time.Second }, want: false},
		"ping dropped":                   {mutate: func(c *rangeConfig) { c.Probes.Ping = nil }, want: false},
		"snmp dropped":                   {mutate: func(c *rangeConfig) { c.Probes.SNMP = nil }, want: false},
		"another reason a probe dropped": {mutate: func(c *rangeConfig) { c.Probes.Dropped = []string{"ping unavailable"} }, want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			other := base()
			tt.mutate(&other)
			assert.Equal(t, tt.want, base().equal(other))
			assert.Equal(t, tt.want, other.equal(base()), "equal is symmetric")
		})
	}
}

func TestRangeConfigEqualCoversEveryField(t *testing.T) {
	assert.Equal(t, 7, reflect.TypeOf(rangeConfig{}).NumField(), "a rangeConfig field was added or removed: update rangeConfig.equal and TestRangeConfigEqual")
	assert.Equal(t, 3, reflect.TypeOf(probeParams{}).NumField(), "a probeParams field was added or removed: update probeParams.equal and TestRangeConfigEqual")
	assert.Equal(t, 4, reflect.TypeOf(snmpParams{}).NumField(), "an snmpParams field was added or removed: update snmpParams.equal and TestRangeConfigEqual")
}
