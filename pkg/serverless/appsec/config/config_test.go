// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/appsec-internal-go/appsec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	defaultRules, err := appsec.RulesFromEnv()
	require.NoError(t, err)
	expectedDefaultConfig := &Config{
		Rules:          defaultRules,
		WafTimeout:     appsec.WAFTimeoutFromEnv(),
		TraceRateLimit: appsec.RateLimitFromEnv(),
		Obfuscator:     appsec.NewObfuscatorConfig(),
		APISec:         appsec.NewAPISecConfig(),
	}

	t.Run("default", func(t *testing.T) {
		cfg, err := NewConfig()
		require.NoError(t, err)
		require.Equal(t, expectedDefaultConfig, cfg)
	})

	t.Run("appsec", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			env     string
			enabled bool
		}{
			{
				name: "default",
			},
			{
				name: "enabled: 0",
				env:  "0",
			},
			{
				name: "enabled: false",
				env:  "false",
			},
			{
				name: "enabled: -1",
				env:  "-1",
			},
			{
				name: "enabled: junk data",
				env:  "junk data",
			},
			{
				name:    "enabled: 1",
				env:     "1",
				enabled: true,
			},
			{
				name:    "enabled: true",
				env:     "1",
				enabled: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv(enabledEnvVar, tc.env)
				enabled, _, err := IsEnabled()
				if tc.enabled {
					require.NoError(t, err)
				}
				require.Equal(t, tc.enabled, enabled)
			})
		}
	})

	t.Run("standalone", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			env        string
			standalone bool
		}{
			{
				name: "unset",
			},
			{
				name:       "non-bool env",
				env:        "A5M",
				standalone: true,
			},
			{
				name: "env=true",
				env:  "true",
			},
			{
				name: "env=1",
				env:  "1",
			},
			{
				name:       "env=false",
				env:        "false",
				standalone: true,
			},
			{
				name:       "env=0",
				env:        "0",
				standalone: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if tc.env != "" {
					t.Setenv(tracingEnabledEnvVar, tc.env)
				}
				require.Equal(t, tc.standalone, isStandalone())
			})
		}
	})
}
