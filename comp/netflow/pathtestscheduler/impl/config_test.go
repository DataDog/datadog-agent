// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package impl

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestNewSchedulerConfig_Defaults(t *testing.T) {
	// No overrides — all config keys are unset. Verify defaults are applied.
	mockConfig := config.NewMockWithOverrides(t, map[string]any{})

	cfg, err := newSchedulerConfig(mockConfig)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.False(t, cfg.enabled, "enabled should default to false")
	assert.Equal(t, defaultMaxDestinationsPerFlush, cfg.maxDestinationsPerFlush,
		"max_destinations_per_flush should default to %d", defaultMaxDestinationsPerFlush)
	assert.Empty(t, cfg.destExcludes, "dest_excludes should default to empty")
	assert.Empty(t, cfg.destExcludePrefixes, "dest_excludes prefixes should default to empty")
	assert.Equal(t, 0, cfg.minPackets, "min_packets should default to 0")
	assert.Equal(t, 0, cfg.minBytes, "min_bytes should default to 0")
}

func TestNewSchedulerConfig_CustomValues(t *testing.T) {
	mockConfig := config.NewMockWithOverrides(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": 100,
		"network_path.netflow_monitoring.dest_excludes":              []string{"10.0.0.0/8", "172.16.0.0/12"},
		"network_path.netflow_monitoring.min_packets":                5,
		"network_path.netflow_monitoring.min_bytes":                  1024,
	})

	cfg, err := newSchedulerConfig(mockConfig)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.True(t, cfg.enabled)
	assert.Equal(t, 100, cfg.maxDestinationsPerFlush)
	assert.Equal(t, []string{"10.0.0.0/8", "172.16.0.0/12"}, cfg.destExcludes)
	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
	}, cfg.destExcludePrefixes)
	assert.Equal(t, 5, cfg.minPackets)
	assert.Equal(t, 1024, cfg.minBytes)
}

func TestNewSchedulerConfig_BadCIDR(t *testing.T) {
	mockConfig := config.NewMockWithOverrides(t, map[string]any{
		"network_path.netflow_monitoring.dest_excludes": []string{"not-a-cidr"},
	})

	cfg, err := newSchedulerConfig(mockConfig)
	assert.Nil(t, cfg, "config should be nil on CIDR parse failure")
	require.Error(t, err, "bad CIDR should produce an error")
	assert.Contains(t, err.Error(), "not-a-cidr")
}

func TestNewSchedulerConfig_PartialBadCIDR(t *testing.T) {
	// Valid CIDR followed by an invalid one — ensure the error names the invalid one.
	mockConfig := config.NewMockWithOverrides(t, map[string]any{
		"network_path.netflow_monitoring.dest_excludes": []string{
			"10.0.0.0/8",
			"bad/cidr",
		},
	})

	_, err := newSchedulerConfig(mockConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad/cidr")
}

func TestNewSchedulerConfig_MaxDestinationsDefault_WhenZero(t *testing.T) {
	// Explicitly set to 0 — should fall back to the default.
	mockConfig := config.NewMockWithOverrides(t, map[string]any{
		"network_path.netflow_monitoring.max_destinations_per_flush": 0,
	})

	cfg, err := newSchedulerConfig(mockConfig)
	require.NoError(t, err)
	assert.Equal(t, defaultMaxDestinationsPerFlush, cfg.maxDestinationsPerFlush)
}
