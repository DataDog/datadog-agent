// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// throughputLadder is the monotonic superset chain: each profile must contain
// every setting of the one before it, at the same value, plus its own.
var throughputLadder = []string{"high-concurrency", "high-throughput", "max-throughput"}

func TestThroughputLadderIsMonotonicSuperset(t *testing.T) {
	for i := 1; i < len(throughputLadder); i++ {
		lower := logsPerformanceProfiles[throughputLadder[i-1]][1].settings
		higher := logsPerformanceProfiles[throughputLadder[i]][1].settings
		require.NotEmpty(t, lower)
		require.NotEmpty(t, higher)
		for key, lowerVal := range lower {
			higherVal, ok := higher[key]
			assert.Truef(t, ok, "%q must carry %q from %q (monotonic ladder)",
				throughputLadder[i], key, throughputLadder[i-1])
			assert.EqualValuesf(t, lowerVal, higherVal,
				"%q must not lower %q set by %q", throughputLadder[i], key, throughputLadder[i-1])
		}
	}
}

func TestMaxThroughputDisablesCompression(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: max-throughput
`)

	applyLogsPerformanceProfile(cfg)

	assert.False(t, cfg.GetBool("logs_config.use_compression"),
		"max-throughput must disable compression to remove the CPU bottleneck")
}

func TestHighThroughputUsesOnePipelinePerCore(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)

	applyLogsPerformanceProfile(cfg)

	assert.Equal(t, runtime.GOMAXPROCS(0), cfg.GetInt("logs_config.pipelines"),
		"high-throughput must scale pipelines to one per core, uncapped")
}

func TestLowResourceCapsPipelinesAtTwo(t *testing.T) {
	prev := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(prev)

	cfg := confFromYAML(t, `
logs_config:
  profile: low-resource
`)
	applyLogsPerformanceProfile(cfg)

	assert.Equal(t, 2, cfg.GetInt("logs_config.pipelines"),
		"low-resource should use few pipelines on a multi-core host")
}

func TestLowResourceNeverRaisesPipelinesOnSmallHost(t *testing.T) {
	// On a single-core host the default pipeline count is already 1; low-resource
	// must not raise it to 2.
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)

	cfg := confFromYAML(t, `
logs_config:
  profile: low-resource
`)
	applyLogsPerformanceProfile(cfg)

	assert.Equal(t, 1, cfg.GetInt("logs_config.pipelines"),
		"low-resource must never raise the pipeline count above the host's core count")
}

func TestLogsPerformanceProfileCovers(t *testing.T) {
	// Higher ladder tiers are supersets of lower ones.
	assert.True(t, LogsPerformanceProfileCovers("high-throughput", "high-concurrency"))
	assert.True(t, LogsPerformanceProfileCovers("max-throughput", "high-throughput"))
	assert.True(t, LogsPerformanceProfileCovers("max-throughput", "high-concurrency"))
	assert.True(t, LogsPerformanceProfileCovers("high-throughput", "high-throughput"), "a profile covers itself")

	// Lower tiers do not cover higher ones, and unrelated/unknown/off names never cover.
	assert.False(t, LogsPerformanceProfileCovers("high-concurrency", "high-throughput"))
	assert.False(t, LogsPerformanceProfileCovers("low-latency", "high-concurrency"))
	assert.False(t, LogsPerformanceProfileCovers("", "high-concurrency"))
	assert.False(t, LogsPerformanceProfileCovers("high-throughput", "does-not-exist"))
}

func TestLogsPerformanceProfileOffByDefault(t *testing.T) {
	cfg := confFromYAML(t, ``)

	applyLogsPerformanceProfile(cfg)

	// With no profile selected, the agent keeps its normal default settings.
	assert.Equal(t, 4, cfg.GetInt("logs_config.pipelines"),
		"pipelines must keep its default when no profile is selected")
	assert.Empty(t, cfg.GetString("logs_config.profile"))
}

func TestLogsPerformanceProfileApplied(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
  profile_version: 1
`)

	applyLogsPerformanceProfile(cfg)

	profile := logsPerformanceProfiles["high-throughput"][1]
	require.NotEmpty(t, profile.settings, "high-throughput v1 must define settings")
	for key, want := range profile.settings {
		assert.EqualValues(t, resolveProfileSettingValue(want), cfg.Get(key),
			"profile must set %s to its profile value", key)
	}
}

func TestLogsPerformanceProfileVersionDefaultsToV1(t *testing.T) {
	// profile_version omitted (0) must resolve to v1 for upgrade-safety.
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)

	applyLogsPerformanceProfile(cfg)

	profile := logsPerformanceProfiles["high-throughput"][1]
	for key, want := range profile.settings {
		assert.EqualValues(t, resolveProfileSettingValue(want), cfg.Get(key), "omitted version must apply v1 for %s", key)
	}
}

func TestLogsPerformanceProfileUnknownNameIsNoOp(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: does-not-exist
`)

	applyLogsPerformanceProfile(cfg)

	// Unknown profile must fail safe to defaults, never crash.
	assert.Equal(t, 4, cfg.GetInt("logs_config.pipelines"))
}

func TestLogsPerformanceProfileUnknownVersionIsNoOp(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
  profile_version: 9999
`)

	applyLogsPerformanceProfile(cfg)

	// Unknown version must fail safe to defaults, never crash.
	assert.Equal(t, 4, cfg.GetInt("logs_config.pipelines"))
}

func TestLogsPerformanceProfileYieldsToExplicitUserSetting(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
  pipelines: 1
`)

	applyLogsPerformanceProfile(cfg)

	// The explicitly-set key wins over the profile...
	assert.Equal(t, 1, cfg.GetInt("logs_config.pipelines"),
		"an explicitly-configured key must win over the profile")
	// ...but the profile still fills in the keys the user did not set.
	assert.Equal(t, 20, cfg.GetInt("logs_config.batch_max_concurrent_send"),
		"the profile must still apply keys the user did not set")
}

func TestLogsPerformanceProfileYieldsToEnvVarSetting(t *testing.T) {
	t.Setenv("DD_LOGS_CONFIG_BATCH_MAX_CONCURRENT_SEND", "3")
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)

	applyLogsPerformanceProfile(cfg)

	assert.Equal(t, 3, cfg.GetInt("logs_config.batch_max_concurrent_send"),
		"an env-var-configured key must win over the profile")
}

func TestLogsPerformanceProfileCatalogKeysAreKnown(t *testing.T) {
	cfg := confFromYAML(t, ``)

	for name, versions := range logsPerformanceProfiles {
		require.NotEmpty(t, versions, "profile %q must have at least one version", name)
		for version, profile := range versions {
			require.NotEmpty(t, profile.settings,
				"profile %q version %d must define settings", name, version)
			for key := range profile.settings {
				assert.Truef(t, cfg.IsKnown(key),
					"profile %q version %d references unknown config key %q", name, version, key)
			}
		}
	}
}

func TestResolvedLogsPerformanceProfile(t *testing.T) {
	t.Run("active profile returns its settings", func(t *testing.T) {
		cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)

		name, version, settings, ok := ResolvedLogsPerformanceProfile(cfg)

		require.True(t, ok)
		assert.Equal(t, "high-throughput", name)
		assert.Equal(t, 1, version)
		require.NotEmpty(t, settings)
		// Settings must be sorted by key for stable display.
		for i := 1; i < len(settings); i++ {
			assert.LessOrEqual(t, settings[i-1].Key, settings[i].Key)
		}
		byKey := map[string]interface{}{}
		for _, s := range settings {
			byKey[s.Key] = s.Value
		}
		assert.Contains(t, byKey, "logs_config.pipelines")
	})

	t.Run("no profile selected returns ok=false", func(t *testing.T) {
		cfg := confFromYAML(t, ``)
		_, _, _, ok := ResolvedLogsPerformanceProfile(cfg)
		assert.False(t, ok)
	})

	t.Run("unknown profile returns ok=false", func(t *testing.T) {
		cfg := confFromYAML(t, `
logs_config:
  profile: does-not-exist
`)
		_, _, _, ok := ResolvedLogsPerformanceProfile(cfg)
		assert.False(t, ok)
	})

	t.Run("unknown version returns ok=false", func(t *testing.T) {
		cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
  profile_version: 9999
`)
		_, _, _, ok := ResolvedLogsPerformanceProfile(cfg)
		assert.False(t, ok)
	})
}

func TestLogsPerformanceProfileExists(t *testing.T) {
	assert.True(t, LogsPerformanceProfileExists("high-throughput"))
	assert.False(t, LogsPerformanceProfileExists("does-not-exist"))
	assert.False(t, LogsPerformanceProfileExists(""))
}

func TestLogsPerformanceProfileV1AlwaysExists(t *testing.T) {
	// Bare `profile: <name>` resolves to v1, so every published profile must
	// define a version 1.
	for name, versions := range logsPerformanceProfiles {
		_, ok := versions[1]
		assert.Truef(t, ok, "profile %q must define version 1", name)
	}
}

// The profile config is intentionally undocumented in the datadog.yaml
// template (shipping as a hidden feature). These tests guard that hiding it
// from the template did not make the feature unreachable: the keys must stay
// bound to the schema and to their environment variables.
func TestLogsPerformanceProfileKeysRemainKnown(t *testing.T) {
	cfg := newTestConf(t)

	assert.True(t, cfg.IsKnown("logs_config.profile"),
		"logs_config.profile must stay bound even though it is hidden from the config template")
	assert.True(t, cfg.IsKnown("logs_config.profile_version"),
		"logs_config.profile_version must stay bound even though it is hidden from the config template")
}

func TestLogsPerformanceProfileEnvVarsRemainBound(t *testing.T) {
	t.Setenv("DD_LOGS_CONFIG_PROFILE", "high-throughput")
	t.Setenv("DD_LOGS_CONFIG_PROFILE_VERSION", "1")

	cfg := newTestConf(t)

	assert.Equal(t, "high-throughput", cfg.GetString("logs_config.profile"),
		"DD_LOGS_CONFIG_PROFILE must still feed logs_config.profile")
	assert.Equal(t, 1, cfg.GetInt("logs_config.profile_version"),
		"DD_LOGS_CONFIG_PROFILE_VERSION must still feed logs_config.profile_version")

	// The hidden feature must remain fully functional when driven by env vars.
	applyLogsPerformanceProfile(cfg)
	profile := logsPerformanceProfiles["high-throughput"][1]
	for key, want := range profile.settings {
		assert.EqualValues(t, resolveProfileSettingValue(want), cfg.Get(key),
			"env-var-selected profile must apply %s", key)
	}
}
