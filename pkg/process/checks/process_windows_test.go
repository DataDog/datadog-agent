// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package checks

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestPerfCountersConfigSetting(t *testing.T) {
	resetOnce := func() {
		processProbeOnce = sync.Once{}
	}

	t.Run("use toolhelp API", func(t *testing.T) {
		resetOnce()
		defer resetOnce()

		cfg := config.Mock(t)
		cfg.Set("process_config.windows.use_perf_counters", false)
		probe := getProcessProbe()
		assert.IsType(t, procutil.NewWindowsToolhelpProbe(), probe)
	})

	t.Run("use PDH api", func(t *testing.T) {
		resetOnce()
		defer resetOnce()

		cfg := config.Mock(t)
		cfg.Set("process_config.windows.use_perf_counters", true)
		probe := getProcessProbe()
		assert.IsType(t, procutil.NewProcessProbe(), probe)
	})
}

func TestFormatCPUTimes(t *testing.T) {
	oldNumCPU := numCPU
	numCPU = func() int {
		return 4
	}
	defer func() {
		numCPU = oldNumCPU
	}()

	for name, test := range map[string]struct {
		statsNow   *procutil.Stats
		statsPrev  *procutil.CPUTimesStat
		timeNow    cpu.TimesStat
		timeBefore cpu.TimesStat
		expected   *model.CPUStat
	}{
		"times": {
			statsNow: &procutil.Stats{
				CPUTime: &procutil.CPUTimesStat{
					User:   101.01,
					System: 202.02,
				},
				NumThreads: 4,
				Nice:       5,
			},
			statsPrev: &procutil.CPUTimesStat{
				User:   11,
				System: 22,
			},
			timeNow:    cpu.TimesStat{User: 5000},
			timeBefore: cpu.TimesStat{User: 2500},
			expected: &model.CPUStat{
				LastCpu:    "cpu",
				TotalPct:   43.2048,
				UserPct:    14.4016,
				SystemPct:  28.8032,
				NumThreads: 4,
				Cpus:       []*model.SingleCPUStat{},
				Nice:       5,
				UserTime:   101,
				SystemTime: 202,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, formatCPUTimes(
				test.statsNow, test.statsNow.CPUTime, test.statsPrev, test.timeNow, test.timeBefore,
			))
		})
	}
}
