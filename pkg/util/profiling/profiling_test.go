// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestProfiling(t *testing.T) {
	settings := Settings{
		ProfilingURL:         "https://nowhere.testing.dev",
		Env:                  "testing",
		Service:              "test-agent",
		Period:               time.Minute,
		CPUDuration:          15 * time.Second,
		MutexProfileFraction: 0,
		BlockProfileRate:     0,
		WithGoroutineProfile: false,
		WithDeltaProfiles:    false,
		Tags:                 []string{"1.0.0"},
	}
	err := Start(settings)
	assert.NoError(t, err)

	Stop()
}

func TestIsRunning(t *testing.T) {
	assert.False(t, IsRunning(), "profiler should not be running initially")

	settings := Settings{
		ProfilingURL: "https://nowhere.testing.dev",
		Env:          "testing",
		Service:      "test-agent",
		Period:       time.Minute,
		CPUDuration:  15 * time.Second,
	}
	err := Start(settings)
	require.NoError(t, err)
	assert.True(t, IsRunning(), "profiler should be running after Start")

	Stop()
	assert.False(t, IsRunning(), "profiler should not be running after Stop")
}

func TestStartIdempotent(t *testing.T) {
	settings := Settings{
		ProfilingURL: "https://nowhere.testing.dev",
		Env:          "testing",
		Service:      "test-agent",
		Period:       time.Minute,
		CPUDuration:  15 * time.Second,
	}
	err := Start(settings)
	require.NoError(t, err)

	// Calling Start again while running should be a no-op
	err = Start(settings)
	assert.NoError(t, err)

	Stop()
}

func TestStopIdempotent(t *testing.T) {
	// Stop on an already-stopped profiler should not panic
	Stop()
	Stop()
}

func TestSettingsString(t *testing.T) {
	settings := Settings{
		Socket:               "/var/run/datadog/dsd.socket",
		ProfilingURL:         "https://intake.profile.datadoghq.com/v1/input",
		Env:                  "prod",
		Period:               30 * time.Second,
		CPUDuration:          10 * time.Second,
		MutexProfileFraction: 5,
		BlockProfileRate:     100,
		WithGoroutineProfile: true,
		WithBlockProfile:     true,
		WithMutexProfile:     false,
		WithDeltaProfiles:    true,
	}
	str := settings.String()
	assert.Contains(t, str, `/var/run/datadog/dsd.socket`)
	assert.Contains(t, str, `intake.profile.datadoghq.com`)
	assert.Contains(t, str, `prod`)
	assert.Contains(t, str, `Mutex:5`)
	assert.Contains(t, str, `Block:100`)
}

func TestSettingsApplyDefaults(t *testing.T) {
	t.Run("sets CPUDuration when zero", func(t *testing.T) {
		settings := Settings{}
		settings.applyDefaults()
		assert.NotZero(t, settings.CPUDuration, "CPUDuration should have a non-zero default")
	})

	t.Run("does not override CPUDuration when set", func(t *testing.T) {
		settings := Settings{CPUDuration: 5 * time.Second}
		settings.applyDefaults()
		assert.Equal(t, 5*time.Second, settings.CPUDuration)
	})

	t.Run("initializes nil Tags", func(t *testing.T) {
		settings := Settings{}
		settings.applyDefaults()
		assert.NotNil(t, settings.Tags)
		assert.Empty(t, settings.Tags)
	})

	t.Run("initializes nil CustomAttributes", func(t *testing.T) {
		settings := Settings{}
		settings.applyDefaults()
		assert.NotNil(t, settings.CustomAttributes)
		assert.Empty(t, settings.CustomAttributes)
	})
}

func TestGetBaseProfilingTags(t *testing.T) {
	extraTags := []string{"service:myapp", "region:us-east-1"}
	tags := GetBaseProfilingTags(extraTags)

	assert.Contains(t, tags, "service:myapp")
	assert.Contains(t, tags, "region:us-east-1")
	assert.Contains(t, tags, fmt.Sprintf("version:%v", version.AgentVersion))
	assert.Contains(t, tags, "__dd_internal_profiling:datadog-agent")
	assert.Len(t, tags, len(extraTags)+2)
}

func TestGetBaseProfilingTagsEmpty(t *testing.T) {
	tags := GetBaseProfilingTags(nil)
	assert.Len(t, tags, 2)

	hasVersion := false
	hasInternal := false
	for _, tag := range tags {
		if strings.HasPrefix(tag, "version:") {
			hasVersion = true
		}
		if tag == "__dd_internal_profiling:datadog-agent" {
			hasInternal = true
		}
	}
	assert.True(t, hasVersion)
	assert.True(t, hasInternal)
}

func TestBlockProfileRate(t *testing.T) {
	// Save and restore
	original := GetBlockProfileRate()
	defer SetBlockProfileRate(original)

	SetBlockProfileRate(1000)
	assert.Equal(t, 1000, GetBlockProfileRate())

	SetBlockProfileRate(0)
	assert.Equal(t, 0, GetBlockProfileRate())
}

func TestMutexProfileFraction(t *testing.T) {
	// Save and restore
	original := GetMutexProfileFraction()
	defer SetMutexProfileFraction(original)

	SetMutexProfileFraction(10)
	assert.Equal(t, 10, GetMutexProfileFraction())

	SetMutexProfileFraction(0)
	assert.Equal(t, 0, GetMutexProfileFraction())
}

func TestStartWithAllProfileTypes(t *testing.T) {
	settings := Settings{
		ProfilingURL:         "https://nowhere.testing.dev",
		Env:                  "testing",
		Service:              "test-agent",
		Period:               time.Minute,
		CPUDuration:          15 * time.Second,
		MutexProfileFraction: 5,
		BlockProfileRate:     100,
		WithGoroutineProfile: true,
		WithBlockProfile:     true,
		WithMutexProfile:     true,
		WithDeltaProfiles:    true,
		Socket:               "",
		Tags:                 []string{"env:test"},
		CustomAttributes:     []string{"custom1", "custom2"},
	}
	err := Start(settings)
	require.NoError(t, err)
	assert.True(t, IsRunning())

	Stop()
	assert.False(t, IsRunning())
}
