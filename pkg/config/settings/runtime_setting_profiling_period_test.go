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
	cfg.SetWithoutSource("internal_profiling.period", 5*time.Minute)

	s := NewProfilingPeriod()
	v, err := s.Get(cfg)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, v)
}

func TestProfilingPeriodSetDurationString(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetWithoutSource("internal_profiling.period", 5*time.Minute)
	cfg.SetWithoutSource("internal_profiling.cpu_duration", 1*time.Minute)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "2m", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, 2*time.Minute, cfg.GetDuration("internal_profiling.period"))
	// cpu_duration (1m) <= period (2m), so it should be unchanged
	assert.Equal(t, 1*time.Minute, cfg.GetDuration("internal_profiling.cpu_duration"))
}

func TestProfilingPeriodSetClampsCPUDuration(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetWithoutSource("internal_profiling.period", 5*time.Minute)
	cfg.SetWithoutSource("internal_profiling.cpu_duration", 1*time.Minute)

	s := NewProfilingPeriod()
	// period=30s < cpu_duration=1m → cpu_duration should be clamped to 30s
	err := s.Set(cfg, "30s", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, 30*time.Second, cfg.GetDuration("internal_profiling.period"))
	assert.Equal(t, 30*time.Second, cfg.GetDuration("internal_profiling.cpu_duration"))
}

func TestProfilingPeriodSetBareSeconds(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetWithoutSource("internal_profiling.period", 5*time.Minute)
	cfg.SetWithoutSource("internal_profiling.cpu_duration", 1*time.Minute)

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

func TestProfilingPeriodSetRejectsInvalidString(t *testing.T) {
	cfg := configcomp.NewMock(t)

	s := NewProfilingPeriod()
	err := s.Set(cfg, "notaduration", model.SourceCLI)
	assert.Error(t, err)
}

func TestProfilingPeriodWithConfigPrefix(t *testing.T) {
	cfg := configcomp.NewMock(t)
	cfg.SetWithoutSource("system_probe_config.internal_profiling.period", 5*time.Minute)
	cfg.SetWithoutSource("system_probe_config.internal_profiling.cpu_duration", 1*time.Minute)

	s := &ProfilingPeriod{ConfigKey: "internal_profiling_period", ConfigPrefix: "system_probe_config."}
	err := s.Set(cfg, "30s", model.SourceCLI)
	require.NoError(t, err)

	assert.Equal(t, 30*time.Second, cfg.GetDuration("system_probe_config.internal_profiling.period"))
	assert.Equal(t, 30*time.Second, cfg.GetDuration("system_probe_config.internal_profiling.cpu_duration"))
}
