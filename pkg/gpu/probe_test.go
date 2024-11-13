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
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	consumerstestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

type probeTestSuite struct {
	suite.Suite
}

func TestProbe(t *testing.T) {
	if err := config.CheckGPUSupported(); err != nil {
		t.Skipf("minimum kernel version not met, %v", err)
	}

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.CORE, ebpftest.RuntimeCompiled}, "", func(t *testing.T) {
		suite.Run(t, new(probeTestSuite))
	})
}

func (s *probeTestSuite) getProbe() *Probe {
	t := s.T()

	cfg := config.New()

	// Avoid waiting for the initial sync to finish in tests, we don't need it
	cfg.InitialProcessSync = false

	deps := ProbeDependencies{
		NvmlLib:        testutil.GetBasicNvmlMock(),
		ProcessMonitor: consumerstestutil.NewTestProcessConsumer(t),
	}
	probe, err := NewProbe(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, probe)
	t.Cleanup(probe.Close)

	return probe
}

func (s *probeTestSuite) TestCanLoad() {
	t := s.T()

	probe := s.getProbe()
	data, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, data)
}

func (s *probeTestSuite) TestCanReceiveEvents() {
	t := s.T()

	probe := s.getProbe()
	cmd := testutil.RunSample(t, testutil.CudaSample)

	utils.WaitForProgramsToBeTraced(t, "gpu", gpuAttacherName, cmd.Process.Pid, utils.ManualTracingFallbackDisabled)

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

func (s *probeTestSuite) TestCanGenerateStats() {
	t := s.T()

	probe := s.getProbe()

	cmd := testutil.RunSample(t, testutil.CudaSample)

	utils.WaitForProgramsToBeTraced(t, "gpu", gpuAttacherName, cmd.Process.Pid, utils.ManualTracingFallbackDisabled)

	// Wait until the process finishes and we can get the stats. Run this instead of waiting for the process to finish
	// so that we can time out correctly
	require.Eventually(t, func() bool {
		return !utils.IsProgramTraced(gpuModuleName, gpuAttacherName, cmd.Process.Pid)
	}, 20*time.Second, 500*time.Millisecond, "process not stopped")

	stats, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotEmpty(t, stats.ProcessStats)
	require.Contains(t, stats.ProcessStats, uint32(cmd.Process.Pid))

	pidStats := stats.ProcessStats[uint32(cmd.Process.Pid)]
	require.Greater(t, pidStats.UtilizationPercentage, 0.0) // percentage depends on the time this took to run, so it's not deterministic
	require.Equal(t, pidStats.Memory.MaxBytes, uint64(110))
}

func (s *probeTestSuite) TestDetectsContainer() {
	t := s.T()

	// Flaky test in CI, avoid failures on main for now.
	flake.Mark(t)

	probe := s.getProbe()

	args := testutil.GetDefaultArgs()
	args.EndWaitTimeSec = 1
	pid, cid := testutil.RunSampleInDockerWithArgs(t, testutil.CudaSample, testutil.MinimalDockerImage, args)

	utils.WaitForProgramsToBeTraced(t, "gpu", gpuAttacherName, pid, utils.ManualTracingFallbackDisabled)

	// Wait until the process finishes and we can get the stats. Run this instead of waiting for the process to finish
	// so that we can time out correctly
	require.Eventually(t, func() bool {
		return !utils.IsProgramTraced(gpuModuleName, gpuAttacherName, pid)
	}, 20*time.Second, 500*time.Millisecond, "process not stopped")

	// Check that the stream handlers have the correct container ID assigned
	for key, handler := range probe.consumer.streamHandlers {
		if key.pid == uint32(pid) {
			require.Equal(t, cid, handler.containerID)
		}
	}

	stats, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotEmpty(t, stats.ProcessStats)
	require.Contains(t, stats.ProcessStats, uint32(pid))

	pidStats := stats.ProcessStats[uint32(pid)]
	require.Greater(t, pidStats.UtilizationPercentage, 0.0) // percentage depends on the time this took to run, so it's not deterministic
	require.Equal(t, pidStats.Memory.MaxBytes, uint64(110))
}
