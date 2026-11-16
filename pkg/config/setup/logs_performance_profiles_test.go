// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	schemaimpl "github.com/DataDog/datadog-agent/comp/core/schema/impl"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func mustFindProfile(t *testing.T, name string) logsPerformanceProfile {
	t.Helper()
	p, ok := findLogsPerformanceProfile(name)
	require.Truef(t, ok, "profile %q not found", name)
	return p
}

// throughputLadder lists profiles that must each contain the previous one's settings.
var throughputLadder = []string{"high-concurrency", "high-throughput", "max-throughput"}

func TestThroughputLadderIsMonotonicSuperset(t *testing.T) {
	for i := 1; i < len(throughputLadder); i++ {
		lower := mustFindProfile(t, throughputLadder[i-1])
		higher := mustFindProfile(t, throughputLadder[i])
		require.NotEmpty(t, lower.settings)
		require.NotEmpty(t, higher.settings)
		for _, s := range lower.settings {
			key, lowerVal := s.key, s.value
			higherVal, ok := higher.value(key)
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

	profile := mustFindProfile(t, "high-throughput")
	require.NotEmpty(t, profile.settings, "high-throughput must define settings")
	for _, s := range profile.settings {
		assert.EqualValues(t, resolveProfileValue(s.value), cfg.Get(s.key),
			"profile must set %s to its profile value", s.key)
	}
}

func TestLogsPerformanceProfileNameIsExact(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: High-Throughput
`)
	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, 100, cfg.GetInt("logs_config.message_channel_size"))
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

	for _, profile := range logsPerformanceProfiles {
		require.NotEmpty(t, profile.settings, "profile %q must define settings", profile.name)
		for _, s := range profile.settings {
			assert.Truef(t, cfg.IsKnown(s.key), "profile %q references unknown config key %q", profile.name, s.key)
		}
	}
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
	for _, s := range mustFindProfile(t, "high-throughput").settings {
		assert.EqualValues(t, resolveProfileValue(s.value), cfg.Get(s.key),
			"env-var-selected profile must apply %s", s.key)
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

func TestLogsPerformanceProfileFleetSwitchClearsPreviousProfile(t *testing.T) {
	cfg := confFromYAML(t, `
logs_config:
  profile: high-throughput
`)
	ApplyLogsPerformanceProfile(cfg)
	require.Equal(t, 200, cfg.GetInt("logs_config.message_channel_size"))

	cfg.Set("logs_config.profile", "low-latency", pkgconfigmodel.SourceFleetPolicies)
	ApplyLogsPerformanceProfile(cfg)

	assert.Equal(t, 100, cfg.GetInt("logs_config.message_channel_size"))
	assert.Equal(t, 4, cfg.GetInt("logs_config.pipelines"))
	assert.Equal(t, 1.0, cfg.GetFloat64("logs_config.batch_wait"))
}

func TestLogsPerformanceProfileOffWritesNothing(t *testing.T) {
	cfg := confFromYAML(t, ``)

	ApplyLogsPerformanceProfile(cfg)

	for _, key := range logsProfileKeys() {
		for _, vs := range cfg.GetAllSources(key) {
			if vs.Source == pkgconfigmodel.SourceConfigPostInit {
				assert.Nilf(t, vs.Value, "%s must have no post-init value", key)
			}
		}
	}
}

func TestLogsPerformanceProfileSchemaEnumMatchesCatalog(t *testing.T) {
	validator := schemaimpl.NewComponent().Comp
	validate := func(profile string) []*pkgconfigmodel.Violation {
		violations, err := validator.ValidateYAML([]byte("logs_config:\n  profile: '"+profile+"'\n"), pkgconfigmodel.CoreSchema)
		require.NoError(t, err)
		return violations
	}

	assert.Empty(t, validate(""))
	for _, p := range logsPerformanceProfiles {
		assert.Emptyf(t, validate(p.name), "schema enum must accept profile %q", p.name)
	}
	assert.NotEmpty(t, validate("High-Throughput"))
}

type batchWaitViolationValidator struct{}

func (batchWaitViolationValidator) ValidateConfig(pkgconfigmodel.Reader) ([]*pkgconfigmodel.Violation, error) {
	return []*pkgconfigmodel.Violation{{Message: "invalid batch_wait", Path: "/logs_config/batch_wait", SettingKey: "logs_config.batch_wait"}}, nil
}

func TestLogsPerformanceProfileKeepsSchemaEnforcementRevert(t *testing.T) {
	for _, profile := range []string{"", "low-latency"} {
		t.Run("profile="+profile, func(t *testing.T) {
			cfg := newTestConf(t)
			configPath := filepath.Join(t.TempDir(), "datadog.yaml")
			yaml := "schema:\n  enforce: true\nlogs_config:\n  batch_wait: 999\n  profile: '" + profile + "'\n"
			require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0o600))
			cfg.SetConfigFile(configPath)
			require.NoError(t, loadCustom(cfg, batchWaitViolationValidator{}, cfg))
			require.Equal(t, 5, cfg.GetInt("logs_config.batch_wait"))

			ApplyLogsPerformanceProfile(cfg)
			ApplyLogsPerformanceProfile(cfg)

			assert.Equal(t, 5, cfg.GetInt("logs_config.batch_wait"))
		})
	}
}
