// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package checks

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/gopsutil/cpu"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestPerfCountersConfigSetting(t *testing.T) {
	t.Run("use toolhelp API", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.windows.use_perf_counters", false)
		probe := newProcessProbe(cfg, procutil.WithPermission(false))
		assert.IsType(t, procutil.NewWindowsToolhelpProbe(), probe)
	})

	t.Run("use PDH api", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.windows.use_perf_counters", true)
		probe := newProcessProbe(cfg, procutil.WithPermission(false))
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
		statsNow  *procutil.Stats
		statsPrev *procutil.CPUTimesStat
		expected  *model.CPUStat
	}{
		"times": {
			statsNow: &procutil.Stats{
				CPUTime: &procutil.CPUTimesStat{
					User:      1.0101,
					System:    2.0202,
					Timestamp: 5000,
				},
				NumThreads: 4,
				Nice:       5,
			},
			statsPrev: &procutil.CPUTimesStat{
				User:      0.11,
				System:    0.22,
				Timestamp: 2500,
			},
			expected: &model.CPUStat{
				LastCpu:    "cpu",
				TotalPct:   10.8012,
				UserPct:    3.6004,
				SystemPct:  7.2008,
				NumThreads: 4,
				Cpus:       []*model.SingleCPUStat{},
				Nice:       5,
				UserTime:   1,
				SystemTime: 2,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, formatCPUTimes(
				test.statsNow, test.statsNow.CPUTime, test.statsPrev, cpu.TimesStat{}, cpu.TimesStat{},
			))
		})
	}
}
