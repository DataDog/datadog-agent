// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestEventSamplingEnabledFor(t *testing.T) {
	profiles := func(types ...model.EventType) RuntimeSecurityConfig {
		return RuntimeSecurityConfig{SecurityProfileV2Enabled: true, SecurityProfileV2EventTypes: types}
	}
	withOpen := func(c RuntimeSecurityConfig) RuntimeSecurityConfig {
		c.EventSamplingOpenEnabled = true
		return c
	}
	withUsage := func(c RuntimeSecurityConfig) RuntimeSecurityConfig {
		c.SBOMResolverUsageEnabled = true
		return c
	}

	for _, tc := range []struct {
		name      string
		cfg       RuntimeSecurityConfig
		eventType model.EventType
		want      bool
	}{
		{"open for profiles", withOpen(profiles(model.FileOpenEventType)), model.FileOpenEventType, true},
		{"open for profiles without opens", withOpen(profiles(model.ConnectEventType)), model.FileOpenEventType, false},
		{"open without profiles", withOpen(RuntimeSecurityConfig{}), model.FileOpenEventType, false},
		{"open for runtime usage", withUsage(withOpen(RuntimeSecurityConfig{})), model.FileOpenEventType, true},
		{"open for runtime usage without opens in profiles", withUsage(withOpen(profiles(model.ConnectEventType))), model.FileOpenEventType, true},
		{"open knob off for runtime usage", withUsage(RuntimeSecurityConfig{}), model.FileOpenEventType, false},
		{"connect for runtime usage", withUsage(RuntimeSecurityConfig{EventSamplingConnectEnabled: true}), model.ConnectEventType, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EventSamplingEnabledFor(tc.eventType); got != tc.want {
				t.Errorf("EventSamplingEnabledFor(%v) = %v, want %v", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestEventSamplingSanitize(t *testing.T) {
	valid := RuntimeSecurityConfig{
		EventSamplingOpenRate:          500,
		EventSamplingOpenThreshold:     80,
		EventSamplingConnectRate:       500,
		EventSamplingConnectThreshold:  40,
		EventSamplingBindRate:          500,
		EventSamplingBindThreshold:     60,
		EventSamplingSyscallsRate:      500,
		EventSamplingSyscallsThreshold: 60,
	}
	require.NoError(t, valid.sanitize())

	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeSecurityConfig)
	}{
		{"zero rates and thresholds", func(c *RuntimeSecurityConfig) {
			c.EventSamplingOpenRate = 0
			c.EventSamplingConnectRate = 0
			c.EventSamplingBindRate = 0
			c.EventSamplingSyscallsRate = 0
			c.EventSamplingOpenThreshold = 0
			c.EventSamplingConnectThreshold = 0
			c.EventSamplingBindThreshold = 0
			c.EventSamplingSyscallsThreshold = 0
		}},
		{"max valid rates and thresholds", func(c *RuntimeSecurityConfig) {
			c.EventSamplingOpenRate = 65534
			c.EventSamplingConnectRate = 65534
			c.EventSamplingBindRate = 65534
			c.EventSamplingSyscallsRate = 65534
			c.EventSamplingOpenThreshold = samplingPressureCritical - 1
			c.EventSamplingConnectThreshold = samplingPressureCritical - 1
			c.EventSamplingBindThreshold = samplingPressureCritical - 1
			c.EventSamplingSyscallsThreshold = samplingPressureCritical - 1
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			assert.NoError(t, cfg.sanitize())
		})
	}

	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeSecurityConfig)
	}{
		{"negative open rate", func(c *RuntimeSecurityConfig) { c.EventSamplingOpenRate = -1 }},
		{"overflow open rate", func(c *RuntimeSecurityConfig) { c.EventSamplingOpenRate = 65535 }},
		{"negative connect rate", func(c *RuntimeSecurityConfig) { c.EventSamplingConnectRate = -1 }},
		{"overflow connect rate", func(c *RuntimeSecurityConfig) { c.EventSamplingConnectRate = 65536 }},
		{"negative bind rate", func(c *RuntimeSecurityConfig) { c.EventSamplingBindRate = -1 }},
		{"overflow bind rate", func(c *RuntimeSecurityConfig) { c.EventSamplingBindRate = 65535 }},
		{"negative syscalls rate", func(c *RuntimeSecurityConfig) { c.EventSamplingSyscallsRate = -1 }},
		{"overflow syscalls rate", func(c *RuntimeSecurityConfig) { c.EventSamplingSyscallsRate = 70000 }},
		{"negative open threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingOpenThreshold = -1 }},
		{"critical open threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingOpenThreshold = samplingPressureCritical }},
		{"negative connect threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingConnectThreshold = -1 }},
		{"critical connect threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingConnectThreshold = samplingPressureCritical }},
		{"negative bind threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingBindThreshold = -1 }},
		{"critical bind threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingBindThreshold = samplingPressureCritical }},
		{"negative syscalls threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingSyscallsThreshold = -1 }},
		{"critical syscalls threshold", func(c *RuntimeSecurityConfig) { c.EventSamplingSyscallsThreshold = samplingPressureCritical }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			assert.Error(t, cfg.sanitize())
		})
	}
}
