// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	consumerstestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
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

	// Enable fatbin parsing in tests so we can validate it runs
	cfg.EnableFatbinParsing = true

	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	deps := ProbeDependencies{
		ProcessMonitor: consumerstestutil.NewTestProcessConsumer(t),
		WorkloadMeta:   testutil.GetWorkloadMetaMock(t),
		Telemetry:      testutil.GetTelemetryMock(t),
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
	cmd, err := testutil.RunSample(t, testutil.CudaSample)
	require.NoError(t, err)

	var handlerStream, handlerGlobal *StreamHandler
	require.Eventually(t, func() bool {
		handlerStream, handlerGlobal = nil, nil // Ensure we see both handlers in the same iteration
		for h := range probe.streamHandlers.allStreams() {
			if h.metadata.pid == uint32(cmd.Process.Pid) {
				if h.metadata.streamID == 0 {
					handlerGlobal = h
				} else {
					handlerStream = h
				}
			}
		}

		return len(probe.streamHandlers.globalStreams) == 1 && len(probe.streamHandlers.streams) == 1 && handlerStream != nil && handlerGlobal != nil && len(handlerStream.kernelSpans) > 0 && len(handlerGlobal.allocations) > 0
	}, 3*time.Second, 100*time.Millisecond, "stream and global handlers not found: existing is %v", probe.consumer.streamHandlers)

	// Check that we're receiving the events we expect
	telemetryMock, ok := probe.deps.Telemetry.(telemetry.Mock)
	require.True(t, ok)

	eventMetrics, err := telemetryMock.GetCountMetric("gpu__consumer", "events")
	require.NoError(t, err)

	actualEvents := make(map[string]int)
	for _, m := range eventMetrics {
		if evType, ok := m.Tags()["event_type"]; ok {
			actualEvents[evType] = int(m.Value())
		}
	}

	expectedEvents := map[string]int{
		ebpf.CudaEventTypeKernelLaunch.String():      1,
		ebpf.CudaEventTypeSetDevice.String():         1,
		ebpf.CudaEventTypeMemory.String():            2,
		ebpf.CudaEventTypeSync.String():              4, // cudaStreamSynchronize, cudaEventQuery, cudaEventSynchronize and cudaMemcpy
		ebpf.CudaEventTypeVisibleDevicesSet.String(): 1,
	}

	for evName, value := range expectedEvents {
		require.Equal(t, value, actualEvents[evName], "event %s count mismatch", evName)
		delete(actualEvents, evName)
	}

	require.Empty(t, actualEvents, "unexpected events: %v", actualEvents)

	// Check device assignments
	require.Contains(t, probe.consumer.sysCtx.selectedDeviceByPIDAndTID, cmd.Process.Pid)
	tidMap := probe.consumer.sysCtx.selectedDeviceByPIDAndTID[cmd.Process.Pid]
	require.Len(t, tidMap, 1)
	require.ElementsMatch(t, []int{cmd.Process.Pid}, maps.Keys(tidMap))

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

	cmd, err := testutil.RunSample(t, testutil.CudaSample)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return probe.streamHandlers.streamCount() == 2
	}, 3*time.Second, 100*time.Millisecond, "stream handlers count mismatch: expected: 2, got: %d", probe.streamHandlers.streamCount())

	stats, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotEmpty(t, stats.Metrics)

	// Check expected events are received, using the telemetry counts
	telemetryMock, ok := probe.deps.Telemetry.(telemetry.Mock)
	require.True(t, ok)

	eventMetrics, err := telemetryMock.GetCountMetric("gpu__consumer", "events")
	require.NoError(t, err)

	expectedEvents := map[string]int{
		ebpf.CudaEventTypeKernelLaunch.String():      1,
		ebpf.CudaEventTypeSetDevice.String():         1,
		ebpf.CudaEventTypeMemory.String():            2,
		ebpf.CudaEventTypeSync.String():              4, // cudaStreamSynchronize, cudaEventQuery, cudaEventSynchronize and cudaMemcpy
		ebpf.CudaEventTypeVisibleDevicesSet.String(): 1,
	}

	actualEvents := make(map[string]int)
	for _, m := range eventMetrics {
		if evType, ok := m.Tags()["event_type"]; ok {
			actualEvents[evType] = int(m.Value())
		}
	}

	require.ElementsMatch(t, maps.Keys(expectedEvents), maps.Keys(actualEvents))
	for evName, value := range expectedEvents {
		require.Equal(t, value, actualEvents[evName], "event %s count mismatch", evName)
	}

	// Ensure the metrics we get are correct
	metricKey := model.StatsKey{PID: uint32(cmd.Process.Pid), DeviceUUID: testutil.DefaultGpuUUID}
	metrics := getMetricsEntry(metricKey, stats)
	require.NotNil(t, metrics)
	require.Greater(t, metrics.UsedCores, 0.0) // core usage depends on the time this took to run, so it's not deterministic
	require.Equal(t, metrics.Memory.MaxBytes, uint64(110))

	// Check that the context was updated with the events
	require.Equal(t, probe.sysCtx.cudaVisibleDevicesPerProcess[cmd.Process.Pid], "42")
}

func (s *probeTestSuite) TestMultiGPUSupport() {
	t := s.T()

	probe := s.getProbe()

	sampleArgs := testutil.SampleArgs{
		StartWaitTimeSec:      6, // default wait time for WaitForProgramsToBeTraced is 5 seconds, give margin to attach manually to avoid flakes
		CudaVisibleDevicesEnv: "1,2",
		SelectedDevice:        1,
	}
	// Visible devices 1,2 -> selects 1 in that array -> global device index = 2
	selectedGPU := testutil.GPUUUIDs[2]

	cmd, err := testutil.RunSampleWithArgs(t, testutil.CudaSample, sampleArgs)
	require.NoError(t, err)

	//TODO: change this check to  count telemetry counter of the consumer (once added).
	// we are expecting 2 different streamhandlers because cudasample generates 3 events in total for 2 different streams (stream 0 and stream 30)
	require.Eventually(t, func() bool {
		return probe.streamHandlers.streamCount() == 2
	}, 3*time.Second, 100*time.Millisecond, "stream handlers count mismatch: expected: 2, got: %d", probe.streamHandlers.streamCount())

	stats, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, stats)
	metricKey := model.StatsKey{PID: uint32(cmd.Process.Pid), DeviceUUID: selectedGPU}
	metrics := getMetricsEntry(metricKey, stats)
	require.NotNil(t, metrics)

	require.Greater(t, metrics.UsedCores, 0.0) // average core usage depends on the time this took to run, so it's not deterministic
	require.Equal(t, metrics.Memory.MaxBytes, uint64(110))
}

func (s *probeTestSuite) TestDetectsContainer() {
	t := s.T()

	probe := s.getProbe()

	pid, cid := testutil.RunSampleInDocker(t, testutil.CudaSample, testutil.MinimalDockerImage)

	// Check that the stream handlers have the correct container ID assigned
	for handler := range probe.streamHandlers.allStreams() {
		if handler.metadata.pid == uint32(pid) {
			require.Equal(t, cid, handler.metadata.containerID)
		}
	}

	stats, err := probe.GetAndFlush()
	key := model.StatsKey{PID: uint32(pid), DeviceUUID: testutil.DefaultGpuUUID, ContainerID: cid}
	require.NoError(t, err)
	require.NotNil(t, stats)
	pidStats := getMetricsEntry(key, stats)
	require.NotNil(t, pidStats)

	require.Greater(t, pidStats.UsedCores, 0.0) // core usage depends on the time this took to run, so it's not deterministic
	require.Equal(t, pidStats.Memory.MaxBytes, uint64(110))
}
