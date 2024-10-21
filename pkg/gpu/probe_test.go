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

	probe, err := NewProbe(NewConfig(), nil)
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

	cfg := NewConfig()
	cfg.InitialProcessSync = false
	cfg.BPFDebug = true
	probe, err := NewProbe(cfg, nil)
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
