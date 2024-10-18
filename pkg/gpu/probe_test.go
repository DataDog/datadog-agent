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

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestProbeCanLoad(t *testing.T) {
	kver, err := kernel.HostVersion()
	require.NoError(t, err)
	if kver < minimumKernelVersion {
		t.Skipf("minimum kernel version %s not met, read %s", minimumKernelVersion, kver)
	}

	nvmlMock := testutil.GetBasicNvmlMock()
	probe, err := NewProbe(NewConfig(), ProbeDependencies{NvmlLib: nvmlMock})
	require.NoError(t, err)
	require.NotNil(t, probe)
	t.Cleanup(probe.Close)

	data, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, data)
}

func TestProbeCanReceiveEvents(t *testing.T) {
	kver, err := kernel.HostVersion()
	require.NoError(t, err)
	if kver < minimumKernelVersion {
		t.Skipf("minimum kernel version %s not met, read %s", minimumKernelVersion, kver)
	}

	procMon := monitor.GetProcessMonitor()
	require.NotNil(t, procMon)
	require.NoError(t, procMon.Initialize(false))
	t.Cleanup(procMon.Stop)

	cfg := NewConfig()
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
			if key.Pid == uint32(cmd.Process.Pid) {
				if key.Stream == 0 {
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
	require.Equal(t, uint64(1), span.NumKernels)
	require.Equal(t, uint64(1*2*3*4*5*6), span.AvgThreadCount)
	require.Greater(t, span.EndKtime, span.StartKtime)

	require.Equal(t, 1, len(handlerGlobal.allocations))
	alloc := handlerGlobal.allocations[0]
	require.Equal(t, uint64(100), alloc.Size)
	require.False(t, alloc.IsLeaked)
	require.Greater(t, alloc.EndKtime, alloc.StartKtime)
}

func TestProbeCanGenerateStats(t *testing.T) {
	kver, err := kernel.HostVersion()
	require.NoError(t, err)
	if kver < minimumKernelVersion {
		t.Skipf("minimum kernel version %s not met, read %s", minimumKernelVersion, kver)
	}

	procMon := monitor.GetProcessMonitor()
	require.NotNil(t, procMon)
	require.NoError(t, procMon.Initialize(false))
	t.Cleanup(procMon.Stop)

	cfg := NewConfig()
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
	require.NotEmpty(t, stats.PIDStats)
	require.Contains(t, stats.PIDStats, uint32(cmd.Process.Pid))

	pidStats := stats.PIDStats[uint32(cmd.Process.Pid)]
	require.Greater(t, pidStats.UtilizationPercentage, 0.0) // percentage depends on the time this took to run, so it's not deterministic
	require.Equal(t, pidStats.MaxMemoryBytes, uint64(100))

}
