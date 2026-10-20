// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// throughputLadder lists profiles that must each contain the previous one's settings.
var throughputLadder = []string{"high-concurrency", "high-throughput", "max-throughput"}

func TestThroughputLadderIsMonotonicSuperset(t *testing.T) {
	for i := 1; i < len(throughputLadder); i++ {
		lower := logsPerformanceProfiles[throughputLadder[i-1]].settings
		higher := logsPerformanceProfiles[throughputLadder[i]].settings
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

	ApplyLogsPerformanceProfile(cfg)

	assert.False(t, cfg.GetBool("logs_config.use_compression"),
		"max-throughput must disable compression to remove the CPU bottleneck")
}

func highThroughputPipelines() int { return min(16, runtime.GOMAXPROCS(0)) }

func TestHighThroughputPipelines(t *testing.T) {
	for _, tc := range []struct{ cores, want int }{{8, 8}, {16, 16}, {64, 16}} {
		t.Run(strconv.Itoa(tc.cores), func(t *testing.T) {
			prev := runtime.GOMAXPROCS(tc.cores)
			defer runtime.GOMAXPROCS(prev)

			cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)
			ApplyLogsPerformanceProfile(cfg)

			assert.Equal(t, tc.want, cfg.GetInt("logs_config.pipelines"))
		})
	}
}

func TestHighCompressionKeepsAdditionalEndpointsGzipFallback(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-compression
`)

	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, 6, cfg.GetInt("logs_config.zstd_compression_level"))
	assert.False(t, cfg.IsConfigured("logs_config.compression_kind"),
		"a profile-written compression_kind would disable the gzip fallback for additional endpoints")
}

func TestZstdLevelIgnoredReason(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, want string
	}{
		{name: "zstd by default", yaml: "logs_config: {}"},
		{name: "explicit zstd with additional endpoints", yaml: "logs_config:\n  compression_kind: zstd\n  additional_endpoints:\n    - host: example.com"},
		{name: "additional endpoints", yaml: "logs_config:\n  additional_endpoints:\n    - host: example.com", want: "additional_endpoints fall back to gzip"},
		{name: "additional endpoints as JSON", yaml: "logs_config:\n  additional_endpoints: '[{\"host\": \"example.com\"}]'", want: "additional_endpoints fall back to gzip"},
		{name: "null additional endpoints", yaml: "logs_config:\n  additional_endpoints: 'null'"},
		{name: "malformed additional endpoints", yaml: "logs_config:\n  additional_endpoints: '[{'"},
		{name: "explicit gzip", yaml: "logs_config:\n  compression_kind: gzip", want: `compression_kind is "gzip"`},
		{name: "compression off", yaml: "logs_config:\n  use_compression: false", want: "use_compression is false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := zstdLevelIgnoredReason(confFromYAML(t, tc.yaml))
			if tc.want == "" {
				assert.Empty(t, reason)
			} else {
				assert.Contains(t, reason, tc.want)
			}
		})
	}
}

func TestLowResourceCapsPipelinesAtTwo(t *testing.T) {
	prev := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(prev)

	cfg := confFromYAML(t, `
logs_config:
  profile: low-resource
`)
	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, 2, cfg.GetInt("logs_config.pipelines"),
		"low-resource should use few pipelines on a multi-core host")
}

func TestLowResourceNeverRaisesPipelinesOnSmallHost(t *testing.T) {
	// low-resource must not raise a single-core host to 2 pipelines.
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)

	cfg := confFromYAML(t, `
logs_config:
  profile: low-resource
`)
	ApplyLogsPerformanceProfile(cfg)

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

	ApplyLogsPerformanceProfile(cfg)

	// With no profile selected, the agent keeps its normal default settings.
	assert.Equal(t, 4, cfg.GetInt("logs_config.pipelines"),
		"pipelines must keep its default when no profile is selected")
	assert.Empty(t, cfg.GetString("logs_config.profile"))
}

func TestLogsPerformanceProfileApplied(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)

	ApplyLogsPerformanceProfile(cfg)

	profile := logsPerformanceProfiles["high-throughput"]
	require.NotEmpty(t, profile.settings, "high-throughput must define settings")
	for key, want := range profile.settings {
		assert.EqualValues(t, resolveProfileSettingValue(want), cfg.Get(key),
			"profile must set %s to its profile value", key)
	}
}

func TestLogsPerformanceProfileNameIsNormalized(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: " High-Throughput "
`)
	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, 200, cfg.GetInt("logs_config.message_channel_size"))
	name, _, ok := ResolvedLogsPerformanceProfile(cfg)
	require.True(t, ok)
	assert.Equal(t, "high-throughput", name)
}

func TestLogsPerformanceProfileUnknownNameIsNoOp(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: does-not-exist
`)

	ApplyLogsPerformanceProfile(cfg)

	// Unknown profile must fail safe to defaults, never crash.
	assert.Equal(t, 4, cfg.GetInt("logs_config.pipelines"))
}

func TestLogsPerformanceProfileYieldsToExplicitUserSetting(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
  pipelines: 1
`)

	ApplyLogsPerformanceProfile(cfg)

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

	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, 3, cfg.GetInt("logs_config.batch_max_concurrent_send"),
		"an env-var-configured key must win over the profile")
}

func TestLogsPerformanceProfileCatalogKeysAreKnown(t *testing.T) {
	cfg := confFromYAML(t, ``)

	for name, profile := range logsPerformanceProfiles {
		require.NotEmpty(t, profile.settings, "profile %q must define settings", name)
		for key := range profile.settings {
			assert.Truef(t, cfg.IsKnown(key), "profile %q references unknown config key %q", name, key)
		}
	}
}

func TestResolvedLogsPerformanceProfile(t *testing.T) {
	t.Run("active profile returns its settings", func(t *testing.T) {
		cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)

		name, settings, ok := ResolvedLogsPerformanceProfile(cfg)

		require.True(t, ok)
		assert.Equal(t, "high-throughput", name)
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
		_, _, ok := ResolvedLogsPerformanceProfile(cfg)
		assert.False(t, ok)
	})

	t.Run("unknown profile returns ok=false", func(t *testing.T) {
		cfg := confFromYAML(t, `
logs_config:
  profile: does-not-exist
`)
		_, _, ok := ResolvedLogsPerformanceProfile(cfg)
		assert.False(t, ok)
	})
}

func TestLogsPerformanceProfileExists(t *testing.T) {
	assert.True(t, LogsPerformanceProfileExists("high-throughput"))
	assert.False(t, LogsPerformanceProfileExists("does-not-exist"))
	assert.False(t, LogsPerformanceProfileExists(""))
}

// The profile keys must stay bound to the schema and to their environment variables.
func TestLogsPerformanceProfileKeysRemainKnown(t *testing.T) {
	cfg := newTestConf(t)

	assert.True(t, cfg.IsKnown("logs_config.profile"))
}

func TestLogsPerformanceProfileEnvVarsRemainBound(t *testing.T) {
	t.Setenv("DD_LOGS_CONFIG_PROFILE", "high-throughput")

	cfg := newTestConf(t)

	assert.Equal(t, "high-throughput", cfg.GetString("logs_config.profile"),
		"DD_LOGS_CONFIG_PROFILE must still feed logs_config.profile")

	ApplyLogsPerformanceProfile(cfg)
	profile := logsPerformanceProfiles["high-throughput"]
	for key, want := range profile.settings {
		assert.EqualValues(t, resolveProfileSettingValue(want), cfg.Get(key),
			"env-var-selected profile must apply %s", key)
	}
}

func TestLogsPerformanceProfileReapplyIsIdempotent(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)

	// Re-applying after fleet policies merge, with no new sources, must not change the result.
	ApplyLogsPerformanceProfile(cfg)
	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, highThroughputPipelines(), cfg.GetInt("logs_config.pipelines"),
		"re-applying the same profile must be idempotent")
	assert.Equal(t, 20, cfg.GetInt("logs_config.batch_max_concurrent_send"))
}

func TestLogsPerformanceProfileSelectedByFleetIsExpanded(t *testing.T) {
	// No profile in the config file: the first override pass is a no-op.
	cfg := confFromYAML(t, ``)
	ApplyLogsPerformanceProfile(cfg)
	assert.Equal(t, 4, cfg.GetInt("logs_config.pipelines"),
		"no profile yet: defaults must stand")

	// Fleet policies merge after the first pass and select a profile.
	cfg.Set("logs_config.profile", "high-throughput", pkgconfigmodel.SourceFleetPolicies)
	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, highThroughputPipelines(), cfg.GetInt("logs_config.pipelines"),
		"a fleet-policy-selected profile must be expanded by the re-run")
}

func TestLogsPerformanceProfileFleetKnobOverridesProfile(t *testing.T) {
	// A profile is active from the config file; the first pass expands it.
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)
	ApplyLogsPerformanceProfile(cfg)
	require.Equal(t, highThroughputPipelines(), cfg.GetInt("logs_config.pipelines"))

	// A fleet-policy knob must win over the first pass's post-init write.
	cfg.Set("logs_config.pipelines", 1, pkgconfigmodel.SourceFleetPolicies)
	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, 1, cfg.GetInt("logs_config.pipelines"),
		"a fleet-policy knob override must take precedence over the profile")
	assert.Equal(t, 20, cfg.GetInt("logs_config.batch_max_concurrent_send"),
		"the profile must still fill in keys fleet policy did not override")
}

func TestPlanLogsPerformanceProfile(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		candidate   string
		wantOK      bool
		wantChanges []string
		wantBlocked map[string]pkgconfigmodel.Source
	}{
		{
			name:      "default config changes every key",
			candidate: "high-concurrency",
			wantOK:    true,
			wantChanges: []string{
				"logs_config.batch_max_concurrent_send",
				"logs_config.payload_channel_size",
			},
		},
		{
			name: "key set via file is blocked",
			yaml: `
logs_config:
  pipelines: 1
`,
			candidate: "high-throughput",
			wantOK:    true,
			wantChanges: []string{
				"logs_config.batch_max_concurrent_send",
				"logs_config.message_channel_size",
				"logs_config.payload_channel_size",
			},
			wantBlocked: map[string]pkgconfigmodel.Source{"logs_config.pipelines": pkgconfigmodel.SourceFile},
		},
		{
			name: "key already at target is blocked and not a change",
			yaml: `
logs_config:
  batch_max_concurrent_send: 20
`,
			candidate:   "high-concurrency",
			wantOK:      true,
			wantChanges: []string{"logs_config.payload_channel_size"},
			wantBlocked: map[string]pkgconfigmodel.Source{"logs_config.batch_max_concurrent_send": pkgconfigmodel.SourceFile},
		},
		{
			name:      "unknown profile",
			candidate: "does-not-exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := confFromYAML(t, tt.yaml)

			plan, ok := PlanLogsPerformanceProfile(cfg, tt.candidate)

			require.Equal(t, tt.wantOK, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.candidate, plan.Name)
			assert.NotEmpty(t, plan.Description)
			assert.Len(t, plan.Current, len(logsPerformanceProfiles[tt.candidate].settings))
			for i := 1; i < len(plan.Current); i++ {
				assert.Less(t, plan.Current[i-1].Key, plan.Current[i].Key)
			}

			var changeKeys []string
			for _, c := range plan.Changes {
				changeKeys = append(changeKeys, c.Key)
				want := resolveProfileSettingValue(logsPerformanceProfiles[tt.candidate].settings[c.Key])
				assert.Equal(t, want, c.To, c.Key)
				assert.False(t, profileValuesEqual(c.From, c.To), c.Key)
			}
			assert.Equal(t, tt.wantChanges, changeKeys)

			blocked := map[string]pkgconfigmodel.Source{}
			for _, b := range plan.Blocked {
				blocked[b.Key] = pkgconfigmodel.Source(b.Source)
			}
			assert.Equal(t, len(tt.wantBlocked), len(blocked))
			for key, src := range tt.wantBlocked {
				assert.Equal(t, src, blocked[key], key)
			}
		})
	}
}

func TestPlanLogsPerformanceProfileResolvesPipelineSentinel(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  pipelines: 8
`)
	plan, ok := PlanLogsPerformanceProfile(cfg, "low-resource")
	require.True(t, ok)
	for _, c := range plan.Changes {
		assert.NotEqual(t, "logs_config.pipelines", c.Key)
	}
	require.Len(t, plan.Blocked, 1)
	assert.Equal(t, "logs_config.pipelines", plan.Blocked[0].Key)

	plan, ok = PlanLogsPerformanceProfile(confFromYAML(t, ``), "low-resource")
	require.True(t, ok)
	for _, c := range plan.Changes {
		if c.Key == "logs_config.pipelines" {
			assert.Equal(t, min(2, runtime.GOMAXPROCS(0)), c.To)
		}
	}
}

func TestProfileValuesEqual(t *testing.T) {
	assert.True(t, profileValuesEqual(20, 20.0))
	assert.True(t, profileValuesEqual(int64(20), 20))
	assert.False(t, profileValuesEqual(20, 21))
	assert.True(t, profileValuesEqual(false, false))
	assert.False(t, profileValuesEqual(false, 0))
	assert.False(t, profileValuesEqual(nil, 0))
}
