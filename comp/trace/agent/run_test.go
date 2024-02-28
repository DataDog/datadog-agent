// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strings"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func TestProfilingConfig(t *testing.T) {
	tconfig := tracecfg.New()
	cconfig := coreconfig.Mock(t)
	cconfig.SetWithoutSource("apm_config.internal_profiling.enabled", true)
	cconfig.SetWithoutSource("internal_profiling.extra_tags", "k1:v1 k2:v2")
	cconfig.SetWithoutSource("internal_profiling.period", 30*time.Second)
	cconfig.SetWithoutSource("internal_profiling.cpu_duration", 15*time.Second)
	cconfig.SetWithoutSource("internal_profiling.mutex_profile_fraction", 7)
	cconfig.SetWithoutSource("internal_profiling.block_profile_rate", 10)
	cconfig.SetWithoutSource("internal_profiling.enable_goroutine_stacktraces", true)
	settings := profilingConfig(tconfig)
	assert.NotNil(t, settings)
	assert.Equal(t, settings.ProfilingURL, "https://intake.profile.datadoghq.com/v1/input")
	assert.Equal(t, settings.Tags[0:2], []string{"k1:v1", "k2:v2"})
	assert.True(t, strings.HasPrefix(settings.Tags[2], "version:"))
	assert.Len(t, settings.Tags, 3)
	assert.Equal(t, 30*time.Second, settings.Period)
	assert.Equal(t, 15*time.Second, settings.CPUDuration)
	assert.Equal(t, 7, settings.MutexProfileFraction)
	assert.Equal(t, 10, settings.BlockProfileRate)
	assert.True(t, settings.WithGoroutineProfile)
}
