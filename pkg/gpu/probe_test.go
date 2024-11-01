// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
)

func TestProbeCanLoad(t *testing.T) {
	if err := config.CheckGPUSupported(); err != nil {
		t.Skipf("minimum kernel version not met, %v", err)
	}

	nvmlMock := testutil.GetBasicNvmlMock()
	probe, err := NewProbe(config.NewConfig(), ProbeDependencies{NvmlLib: nvmlMock})
	require.NoError(t, err)
	require.NotNil(t, probe)
	t.Cleanup(probe.Close)

	data, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, data)
}

func TestProbeCanReceiveEvents(t *testing.T) {
	if err := config.CheckGPUSupported(); err != nil {
		t.Skipf("minimum kernel version not met, %v", err)
	}

	procMon := monitor.GetProcessMonitor()
	require.NotNil(t, procMon)
	require.NoError(t, procMon.Initialize(false))
	t.Cleanup(procMon.Stop)

	cfg := config.NewConfig()
	cfg.InitialProcessSync = false
	cfg.BPFDebug = true

	nvmlMock := testutil.GetBasicNvmlMock()

	probe, err := NewProbe(cfg, ProbeDependencies{NvmlLib: nvmlMock})
	require.NoError(t, err)
	require.NotNil(t, probe)
	t.Cleanup(probe.Close)

	cmd, err := testutil.RunSample(t, testutil.CudaSample)
	require.NoError(t, err)

	utils.WaitForProgramsToBeTraced(t, gpuAttacherName, cmd.Process.Pid, utils.ManualTracingFallbackDisabled)

	var handlerStream, handlerGlobal *StreamHandler
	require.Eventually(t, func() bool {
		for key, h := range probe.consumer.streamHandlers {
			if key.pid == uint32(cmd.Process.Pid) {
				if key.stream == 0 {
					handlerGlobal = h
				} else {
					handlerStream = h
				}
			}
		}

		return handlerStream != nil && handlerGlobal != nil
	}, 10*time.Second, 500*time.Millisecond, "stream and global handlers not found: existing is %v", probe.consumer.streamHandlers)

	require.Equal(t, 1, len(handlerStream.kernelSpans))
	span := handlerStream.kernelSpans[0]
	require.Equal(t, uint64(1), span.numKernels)
	require.Equal(t, uint64(1*2*3*4*5*6), span.avgThreadCount)
	require.Greater(t, span.endKtime, span.startKtime)

	require.Equal(t, 1, len(handlerGlobal.allocations))
	alloc := handlerGlobal.allocations[0]
	require.Equal(t, uint64(100), alloc.size)
	require.False(t, alloc.isLeaked)
	require.Greater(t, alloc.endKtime, alloc.startKtime)
}

func TestProbeCanGenerateStats(t *testing.T) {
	if err := config.CheckGPUSupported(); err != nil {
		t.Skipf("minimum kernel version not met, %v", err)
	}

	procMon := monitor.GetProcessMonitor()
	require.NotNil(t, procMon)
	require.NoError(t, procMon.Initialize(false))
	t.Cleanup(procMon.Stop)

	cfg := config.NewConfig()
	cfg.InitialProcessSync = false
	cfg.BPFDebug = true

	nvmlMock := testutil.GetBasicNvmlMock()

	probe, err := NewProbe(cfg, ProbeDependencies{NvmlLib: nvmlMock})
	require.NoError(t, err)
	require.NotNil(t, probe)
	t.Cleanup(probe.Close)

	cmd, err := testutil.RunSample(t, testutil.CudaSample)
	require.NoError(t, err)

	utils.WaitForProgramsToBeTraced(t, gpuAttacherName, cmd.Process.Pid, utils.ManualTracingFallbackDisabled)

	// Wait until the process finishes and we can get the stats. Run this instead of waiting for the process to finish
	// so that we can time out correctly
	require.Eventually(t, func() bool {
		return !utils.IsProgramTraced(gpuAttacherName, cmd.Process.Pid)
	}, 20*time.Second, 500*time.Millisecond, "process not stopped")
	require.NoError(t, err)

	stats, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotEmpty(t, stats.ProcessStats)
	require.Contains(t, stats.ProcessStats, uint32(cmd.Process.Pid))

	pidStats := stats.ProcessStats[uint32(cmd.Process.Pid)]
	require.Greater(t, pidStats.UtilizationPercentage, 0.0) // percentage depends on the time this took to run, so it's not deterministic
	require.Equal(t, pidStats.MaxMemoryBytes, uint64(100))

}
