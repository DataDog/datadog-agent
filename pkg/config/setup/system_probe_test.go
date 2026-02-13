// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package setup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSystemProbeDefaultConfig tests that InitSystemProbeConfig sets system probe settings correctly
func TestSystemProbeDefaultConfig(t *testing.T) {
	cfg := newEmptyMockConf(t)
	InitSystemProbeConfig(cfg)

	for _, tc := range []struct {
		key          string
		defaultValue interface{}
	}{
		{
			key:          "dynamic_instrumentation.circuit_breaker.interval",
			defaultValue: 1 * time.Second,
		},
		{
			key:          "dynamic_instrumentation.circuit_breaker.per_probe_cpu_limit",
			defaultValue: 0.1,
		},
		{
			key:          "dynamic_instrumentation.circuit_breaker.all_probes_cpu_limit",
			defaultValue: 0.5,
		},
		{
			key:          "dynamic_instrumentation.circuit_breaker.interrupt_overhead",
			defaultValue: 2 * time.Microsecond,
		},
	} {
		t.Run(tc.key, func(t *testing.T) {
			switch expected := tc.defaultValue.(type) {
			case time.Duration:
				actual := cfg.GetDuration(tc.key)
				require.NotZero(t, actual, "config key %s must not be zero - may indicate malformed key", tc.key)
				assert.Equal(t, expected, actual)
			case float64:
				assert.Equal(t, expected, cfg.GetFloat64(tc.key))
			default:
				t.Fatalf("unsupported type %T for key %s", tc.defaultValue, tc.key)
			}
		})
	}
}

func TestDiscoveryUseSdAgent(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		cfg := newEmptyMockConf(t)
		InitSystemProbeConfig(cfg)
		assert.False(t, cfg.GetBool("discovery.use_sd_agent"))
	})

	t.Run("enabled from env var", func(t *testing.T) {
		t.Setenv("DD_DISCOVERY_USE_SD_AGENT", "true")
		cfg := newEmptyMockConf(t)
		InitSystemProbeConfig(cfg)
		assert.True(t, cfg.GetBool("discovery.use_sd_agent"))
	})

	t.Run("disabled from env var", func(t *testing.T) {
		t.Setenv("DD_DISCOVERY_USE_SD_AGENT", "false")
		cfg := newEmptyMockConf(t)
		InitSystemProbeConfig(cfg)
		assert.False(t, cfg.GetBool("discovery.use_sd_agent"))
	})

	t.Run("enabled from config", func(t *testing.T) {
		cfg := newEmptyMockConf(t)
		InitSystemProbeConfig(cfg)
		cfg.SetWithoutSource("discovery.use_sd_agent", true)
		assert.True(t, cfg.GetBool("discovery.use_sd_agent"))
	})
}
