// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

// TestMain defined to run initialization before any test is run
func TestMain(m *testing.M) {
	memPools.ensureInitNoTelemetry()
	os.Exit(m.Run())
}

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

	// Ensure we flush quickly, so that we don't have to wait as much for the pending events to be processed.
	cfg.RingBufferFlushInterval = 500 * time.Millisecond

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
		for _, h := range probe.streamHandlers.allStreams() {
			if h.metadata.pid == uint32(cmd.Process.Pid) {
				if h.metadata.streamID == 0 {
					handlerGlobal = h
				} else {
					handlerStream = h
				}
			}
		}

		return probe.streamHandlers.globalStreamsCount() == 1 && probe.streamHandlers.streamsCount() == 1 && handlerStream != nil && handlerGlobal != nil && len(handlerStream.pendingKernelSpans) > 0 && len(handlerGlobal.pendingMemorySpans) > 0
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
		ebpf.CudaEventTypeKernelLaunch.String():      2,
		ebpf.CudaEventTypeSetDevice.String():         1,
		ebpf.CudaEventTypeMemory.String():            2,
		ebpf.CudaEventTypeSync.String():              4, // cudaStreamSynchronize, cudaEventQuery, cudaEventSynchronize and cudaMemcpy
		ebpf.CudaEventTypeVisibleDevicesSet.String(): 1,
		ebpf.CudaEventTypeSyncDevice.String():        1,
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

	streamPastData := handlerStream.getPastData()
	require.NotNil(t, streamPastData)
	require.Equal(t, 2, len(streamPastData.kernels))
	for i := range 2 {
		span := streamPastData.kernels[i]
		require.Equal(t, uint64(1), span.numKernels)
		require.Equal(t, uint64(1*2*3*4*5*6), span.avgThreadCount)
		require.Greater(t, span.endKtime, span.startKtime)
	}

	globalPastData := handlerGlobal.getPastData()
	require.NotNil(t, globalPastData)
	require.Equal(t, 1, len(globalPastData.allocations))
	alloc := globalPastData.allocations[0]
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
		return probe.streamHandlers.allStreamsCount() == 2
	}, 3*time.Second, 100*time.Millisecond, "stream handlers count mismatch: expected: 2, got: %d", probe.streamHandlers.allStreamsCount())

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
		ebpf.CudaEventTypeKernelLaunch.String():      2,
		ebpf.CudaEventTypeSetDevice.String():         1,
		ebpf.CudaEventTypeMemory.String():            2,
		ebpf.CudaEventTypeSync.String():              4, // cudaStreamSynchronize, cudaEventQuery, cudaEventSynchronize and cudaMemcpy
		ebpf.CudaEventTypeVisibleDevicesSet.String(): 1,
		ebpf.CudaEventTypeSyncDevice.String():        1,
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

	sampleArgs := &testutil.CudaSampleArgs{
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
		return probe.streamHandlers.allStreamsCount() == 2
	}, 3*time.Second, 100*time.Millisecond, "stream handlers count mismatch: expected: 2, got: %d", probe.streamHandlers.allStreamsCount())

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

	// note: after starting, the program will wait ~5s before making any CUDA call
	pid, cid := testutil.RunSampleInDocker(t, testutil.CudaSample, testutil.MinimalDockerImage)

	require.Eventually(t, func() bool {
		return probe.streamHandlers.allStreamsCount() > 0
	}, 3*time.Second, 100*time.Millisecond, "stream handlers count mismatch: expected: 1, got: %d", probe.streamHandlers.allStreamsCount())

	// Check that the stream handlers have the correct container ID assigned
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		nStreamsFound := 0
		for _, handler := range probe.streamHandlers.allStreams() {
			if handler.metadata.pid == uint32(pid) {
				nStreamsFound++
				require.Equal(c, cid, handler.metadata.containerID)
			}
		}
		require.NotZero(c, nStreamsFound)
	}, 6*time.Second, 100*time.Millisecond)

	// Check that stats are properly collected
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		stats, err := probe.GetAndFlush()
		require.NoError(c, err)
		require.NotNil(c, stats)

		key := model.StatsKey{PID: uint32(pid), DeviceUUID: testutil.DefaultGpuUUID, ContainerID: cid}
		pidStats := getMetricsEntry(key, stats)
		require.NotNil(c, pidStats)

		// core usage depends on the time this took to run, so it's not deterministic
		require.Greater(c, pidStats.UsedCores, 0.0)
		require.Equal(c, pidStats.Memory.MaxBytes, uint64(110))
	}, 3*time.Second, 100*time.Millisecond)
}

// cpuUsage represents CPU usage metrics
type cpuUsage struct {
	userTime   time.Duration
	systemTime time.Duration
}

// getCPUUsage reads CPU usage from /proc/self/stat
func getCPUUsage() (cpuUsage, error) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return cpuUsage{}, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return cpuUsage{}, syscall.EINVAL
	}

	// Fields 13 and 14 are utime and stime in clock ticks
	utime, err := strconv.ParseUint(fields[13], 10, 64)
	if err != nil {
		return cpuUsage{}, err
	}

	stime, err := strconv.ParseUint(fields[14], 10, 64)
	if err != nil {
		return cpuUsage{}, err
	}

	// Convert clock ticks to nanoseconds
	// Linux typically uses 100 Hz (100 clock ticks per second)
	clockTick := time.Second / 100

	return cpuUsage{
		userTime:   time.Duration(utime) * clockTick,
		systemTime: time.Duration(stime) * clockTick,
	}, nil
}

// getCPUPercent calculates CPU percentage usage between two measurements
func getCPUPercent(start, end cpuUsage, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}

	totalCPUTime := (end.userTime + end.systemTime) - (start.userTime + start.systemTime)
	maxPossibleCPU := elapsed * time.Duration(runtime.NumCPU())

	if maxPossibleCPU <= 0 {
		return 0
	}

	return float64(totalCPUTime) / float64(maxPossibleCPU) * 100.0
}

func BenchmarkProbeEventProcessing(b *testing.B) {
	if err := config.CheckGPUSupported(); err != nil {
		b.Skipf("minimum kernel version not met, %v", err)
	}

	// Use CO-RE mode for the benchmark
	for k, v := range ebpftest.CORE.Env() {
		b.Setenv(k, v)
	}

	cfg := config.New()
	cfg.InitialProcessSync = false
	cfg.EnableFatbinParsing = false
	cfg.AttacherDetailedLogs = false

	ddnvml.WithMockNVML(b, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	deps := ProbeDependencies{
		ProcessMonitor: consumerstestutil.NewTestProcessConsumer(b),
		WorkloadMeta:   testutil.GetWorkloadMetaMock(b),
		Telemetry:      testutil.GetTelemetryMock(b),
	}
	probe, err := NewProbe(cfg, deps)
	require.NoError(b, err)
	require.NotNil(b, probe)
	b.Cleanup(probe.Close)

	// Configure rate sample args for high throughput
	rateArgs := &testutil.RateSampleArgs{
		StartWaitTimeSec: 2,
		SelectedDevice:   0,
		CallsPerSecond:   50000,
		ExecutionTimeSec: 50,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Measure CPU usage for this iteration
		startCPU, err := getCPUUsage()
		require.NoError(b, err)
		startTime := time.Now()

		// Run the rate sample
		cmd, err := testutil.RunSampleWithArgs(b, testutil.RateSample, rateArgs)
		require.NoError(b, err)

		// Ensure that we have streams for the process
		require.Eventually(b, func() bool {
			for _, handler := range probe.streamHandlers.allStreams() {
				if handler.metadata.pid == uint32(cmd.Process.Pid) {
					return true
				}
			}
			return false
		}, 10*time.Second, 100*time.Millisecond, "stream handlers not created")

		// Get and flush stats to process events
		_, err = probe.GetAndFlush()
		require.NoError(b, err)

		// Measure CPU usage after processing
		endCPU, err := getCPUUsage()
		require.NoError(b, err)
		elapsed := time.Since(startTime)

		// Check telemetry for event counts
		telemetryMock, ok := probe.deps.Telemetry.(telemetry.Mock)
		require.True(b, ok)

		eventMetrics, err := telemetryMock.GetCountMetric("gpu__consumer", "events")
		require.NoError(b, err)

		totalEvents := 0
		for _, m := range eventMetrics {
			totalEvents += int(m.Value())
		}

		require.NotZero(b, totalEvents)

		// Calculate and report metrics
		cpuPercent := getCPUPercent(startCPU, endCPU, elapsed)
		if totalEvents > 0 {
			b.ReportMetric(float64(totalEvents), "events/op")
		}
		b.ReportMetric(cpuPercent, "cpu_percent")
		b.ReportMetric(float64(endCPU.userTime-startCPU.userTime)/float64(time.Millisecond), "user_cpu_ms/op")
		b.ReportMetric(float64(endCPU.systemTime-startCPU.systemTime)/float64(time.Millisecond), "system_cpu_ms/op")

		// Clean up process
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}
}

func TestCudaLibraryAttacherRule(t *testing.T) {
	rule := getCudaLibraryAttacherRule()

	expectedLibraries := []string{
		"libcudart.so",
		"libnd4jcuda.so",
	}

	for _, library := range expectedLibraries {
		t.Run(library, func(t *testing.T) {
			require.True(t, rule.MatchesLibrary(library))
			// Test with a suffix too
			require.True(t, rule.MatchesLibrary(library+".10-2"))

			// And test with a path before the library name
			require.True(t, rule.MatchesLibrary("/usr/lib/x86_64-linux-gnu/"+library))
			require.True(t, rule.MatchesLibrary("/usr/lib/x86_64-linux-gnu/"+library+".10-2"))
		})
	}
}
