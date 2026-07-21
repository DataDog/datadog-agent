// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestProfilingPeriodGet(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("internal_profiling.period", 5*time.Minute)

	s := NewProfilingPeriod()
	v, err := s.Get(cfg)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, v)
}

func TestProfilingPeriodGetWithConfigPrefix(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("system_probe_config.internal_profiling.period", 3*time.Minute)

	s := &ProfilingPeriod{ConfigKey: "internal_profiling_period", ConfigPrefix: "system_probe_config."}
	v, err := s.Get(cfg)
	require.NoError(t, err)
	assert.Equal(t, 3*time.Minute, v)
}

func TestProfilingPeriodSetDurationString(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("internal_profiling.period", 5*time.Minute)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "2m", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, 2*time.Minute, cfg.GetDuration("internal_profiling.period"))
}

func TestProfilingPeriodSetDoesNotModifyCPUDuration(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("internal_profiling.period", 5*time.Minute)
	cfg.SetInTest("internal_profiling.cpu_duration", 1*time.Minute)

	s := NewProfilingPeriod()
	// Set must only ever write the period. dd-trace-go caps the CPU profile at the
	// period internally, so cpu_duration in config must be left untouched even when
	// the new period (30s) is shorter than cpu_duration (1m).
	err := s.Set(cfg, "30s", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, 30*time.Second, cfg.GetDuration("internal_profiling.period"))
	assert.Equal(t, 1*time.Minute, cfg.GetDuration("internal_profiling.cpu_duration"))
}

func TestProfilingPeriodSetBareSeconds(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("internal_profiling.period", 5*time.Minute)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "120", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, 2*time.Minute, cfg.GetDuration("internal_profiling.period"))
}

func TestProfilingPeriodSetRejectsZero(t *testing.T) {
	cfg := configcomp.NewMock(t)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "0s", model.SourceCLI)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

func TestProfilingPeriodSetRejectsNegative(t *testing.T) {
	cfg := configcomp.NewMock(t)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "-30s", model.SourceCLI)
	assert.Error(t, err)
}

func TestProfilingPeriodSetRejectsBelowOneSecond(t *testing.T) {
	cfg := configcomp.NewMock(t)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "500ms", model.SourceCLI)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1s")
}

func TestProfilingPeriodSetAcceptsOneSecond(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("internal_profiling.period", 5*time.Minute)

	s := NewProfilingPeriod()
	// 1s is the inclusive lower bound (the clamp is strict: period < time.Second).
	err := s.Set(cfg, "1s", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, time.Second, cfg.GetDuration("internal_profiling.period"))
}

func TestProfilingPeriodSetRejectsInvalidString(t *testing.T) {
	cfg := configcomp.NewMock(t)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "notaduration", model.SourceCLI)
	assert.Error(t, err)
}

func TestProfilingPeriodSetWithConfigPrefix(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetInTest("system_probe_config.internal_profiling.period", 5*time.Minute)
	cfg.SetInTest("system_probe_config.internal_profiling.cpu_duration", 1*time.Minute)

	s := &ProfilingPeriod{ConfigKey: "internal_profiling_period", ConfigPrefix: "system_probe_config."}
	err := s.Set(cfg, "30s", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, 30*time.Second, cfg.GetDuration("system_probe_config.internal_profiling.period"))
	// cpu_duration must stay untouched under the configured prefix.
	assert.Equal(t, 1*time.Minute, cfg.GetDuration("system_probe_config.internal_profiling.cpu_duration"))
}
