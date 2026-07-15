// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// setProfilingConfig populates every security_agent.internal_profiling.* key consumed by
// buildProfilingSettings so the test asserts a fully-specified mapping rather than defaults.
func setProfilingConfig(cfg configcomp.Component) {
	cfg.SetInTest("security_agent.internal_profiling.env", "ci")
	cfg.SetInTest("security_agent.internal_profiling.period", 90*time.Second)
	cfg.SetInTest("security_agent.internal_profiling.cpu_duration", 20*time.Second)
	cfg.SetInTest("security_agent.internal_profiling.mutex_profile_fraction", 5)
	cfg.SetInTest("security_agent.internal_profiling.block_profile_rate", 7)
	cfg.SetInTest("security_agent.internal_profiling.enable_goroutine_stacktraces", true)
	cfg.SetInTest("security_agent.internal_profiling.enable_block_profiling", true)
	cfg.SetInTest("security_agent.internal_profiling.enable_mutex_profiling", true)
	cfg.SetInTest("security_agent.internal_profiling.delta_profiles", true)
	cfg.SetInTest("security_agent.internal_profiling.unix_socket", "/tmp/profiling.sock")
	cfg.SetInTest("security_agent.internal_profiling.extra_tags", []string{"team:sec"})
}

// TestBuildProfilingSettings locks in the field-by-field mapping that is the documented single
// source of truth shared by setupInternalProfiling (boot) and the internal_profiling runtime
// setting. A transposed or dropped key would otherwise compile and ship silently.
func TestBuildProfilingSettings(t *testing.T) {
	cfg := configcomp.NewMock(t)
	setProfilingConfig(cfg)
	// Ensure the default site branch is taken regardless of the host environment.
	t.Setenv("TRACE_AGENT_URL", "")
	cfg.SetInTest("security_agent.internal_profiling.site", "datadoghq.com")

	s := buildProfilingSettings(cfg)

	assert.Equal(t, "security-agent", s.Service)
	assert.Equal(t, "ci", s.Env)
	assert.Equal(t, 90*time.Second, s.Period)
	assert.Equal(t, 20*time.Second, s.CPUDuration)
	assert.Equal(t, 5, s.MutexProfileFraction)
	assert.Equal(t, 7, s.BlockProfileRate)
	assert.True(t, s.WithGoroutineProfile)
	assert.True(t, s.WithBlockProfile)
	assert.True(t, s.WithMutexProfile)
	assert.True(t, s.WithDeltaProfiles)
	assert.Equal(t, "/tmp/profiling.sock", s.Socket)
	assert.Contains(t, s.Tags, "team:sec")
	assert.Contains(t, s.Tags, fmt.Sprintf("version:%v", version.AgentVersion))
	assert.Contains(t, s.Tags, "__dd_internal_profiling:datadog-agent")
}

// TestBuildProfilingSettingsURL covers the three mutually-exclusive ways the profiling URL is
// resolved, including the security-agent-specific TRACE_AGENT_URL forwarding that the generic
// ProfilingRuntimeSetting does not implement.
func TestBuildProfilingSettingsURL(t *testing.T) {
	t.Run("trace_agent_url takes precedence", func(t *testing.T) {
		cfg := configcomp.NewMock(t)
		setProfilingConfig(cfg)
		cfg.SetInTest("security_agent.internal_profiling.site", "datadoghq.com")
		cfg.SetInTest("security_agent.internal_profiling.profile_dd_url", "https://override.example")
		t.Setenv("TRACE_AGENT_URL", "127.0.0.1:8126")

		s := buildProfilingSettings(cfg)
		assert.Equal(t, fmt.Sprintf(profiling.ProfilingLocalURLTemplate, "127.0.0.1:8126"), s.ProfilingURL)
	})

	t.Run("profile_dd_url override when no trace agent url", func(t *testing.T) {
		cfg := configcomp.NewMock(t)
		setProfilingConfig(cfg)
		t.Setenv("TRACE_AGENT_URL", "")
		cfg.SetInTest("security_agent.internal_profiling.site", "datadoghq.com")
		cfg.SetInTest("security_agent.internal_profiling.profile_dd_url", "https://override.example")

		s := buildProfilingSettings(cfg)
		assert.Equal(t, "https://override.example", s.ProfilingURL)
	})

	t.Run("site template default", func(t *testing.T) {
		cfg := configcomp.NewMock(t)
		setProfilingConfig(cfg)
		t.Setenv("TRACE_AGENT_URL", "")
		cfg.SetInTest("security_agent.internal_profiling.site", "datadoghq.eu")
		cfg.SetInTest("security_agent.internal_profiling.profile_dd_url", "")

		s := buildProfilingSettings(cfg)
		assert.Equal(t, fmt.Sprintf(profiling.ProfilingURLTemplate, "datadoghq.eu"), s.ProfilingURL)
	})
}

func TestProfilingRuntimeSettingGet(t *testing.T) {
	cfg := configcomp.NewMock(t)
	s := profilingRuntimeSetting{}

	cfg.SetInTest("security_agent.internal_profiling.enabled", true)
	v, err := s.Get(cfg)
	require.NoError(t, err)
	assert.Equal(t, true, v)

	cfg.SetInTest("security_agent.internal_profiling.enabled", false)
	v, err = s.Get(cfg)
	require.NoError(t, err)
	assert.Equal(t, false, v)
}

// TestProfilingRuntimeSettingSetDisable verifies the disable path: it stops the profiler (a no-op
// when not running) and writes enabled=false. Both a bool and the "false" string are accepted.
func TestProfilingRuntimeSettingSetDisable(t *testing.T) {
	for _, v := range []interface{}{false, "false"} {
		cfg := configcomp.NewMock(t)
		cfg.SetInTest("security_agent.internal_profiling.enabled", true)
		s := profilingRuntimeSetting{}

		err := s.Set(cfg, v, model.SourceCLI)
		require.NoError(t, err)
		assert.False(t, cfg.GetBool("security_agent.internal_profiling.enabled"),
			"enabled should be false after Set(%v)", v)
	}
}

// TestProfilingRuntimeSettingSetUnsupported ensures an unsupported value is rejected before any
// profiler side effect and leaves the enabled flag untouched.
func TestProfilingRuntimeSettingSetUnsupported(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("security_agent.internal_profiling.enabled", false)
	s := profilingRuntimeSetting{}

	err := s.Set(cfg, 42, model.SourceCLI)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal_profiling")
	assert.False(t, cfg.GetBool("security_agent.internal_profiling.enabled"))
}

// TestRuntimeSettings guards the shared registration consumed by both the start subcommand and the
// Windows service entrypoint: the exact key set and the security-agent ConfigPrefix on the borrowed
// period/goroutines settings (a dropped prefix would make them read the wrong, prefix-less keys).
func TestRuntimeSettings(t *testing.T) {
	m := RuntimeSettings()

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{
		"log_level",
		"internal_profiling",
		"internal_profiling_goroutines",
		"internal_profiling_period",
	}, keys)

	period, ok := m["internal_profiling_period"].(*commonsettings.ProfilingPeriod)
	require.True(t, ok, "internal_profiling_period should be a *commonsettings.ProfilingPeriod")
	assert.Equal(t, secAgentConfigPrefix, period.ConfigPrefix)

	goroutines, ok := m["internal_profiling_goroutines"].(*commonsettings.ProfilingGoroutines)
	require.True(t, ok, "internal_profiling_goroutines should be a *commonsettings.ProfilingGoroutines")
	assert.Equal(t, secAgentConfigPrefix, goroutines.ConfigPrefix)
}
