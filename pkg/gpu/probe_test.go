// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"encoding/json"
	"os"
	"reflect"
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

const expectedCudaSampleMaxMemory = uint64(110)

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

	// Ensure we refresh the cache quickly, but still allow some throttling
	cfg.DeviceCacheRefreshInterval = 500 * time.Millisecond

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

type cudaSampleStreams struct {
	stream *StreamHandler
	global *StreamHandler
}

func (s *probeTestSuite) waitForExpectedCudasampleEvents(probe *Probe, pid int) cudaSampleStreams {
	t := s.T()

	var handlers cudaSampleStreams
	require.Eventually(t, func() bool {
		handlers = cudaSampleStreams{stream: nil, global: nil} // Ensure we see both handlers in the same iteration
		for _, h := range probe.streamHandlers.allStreams() {
			if h.metadata.pid == uint32(pid) {
				if h.metadata.streamID == 0 {
					handlers.global = h
				} else {
					handlers.stream = h
				}
			}
		}

		hasGlobalStreams := probe.streamHandlers.globalStreamsCount() == 1 && handlers.global != nil && len(handlers.global.pendingMemorySpans) > 0
		hasNonGlobalStreams := probe.streamHandlers.streamsCount() == 1 && handlers.stream != nil && len(handlers.stream.pendingKernelSpans) > 0
		return hasGlobalStreams && hasNonGlobalStreams
	}, 3*time.Second, 100*time.Millisecond, "stream and global handlers not found: existing is %v", probe.consumer.deps.streamHandlers)

	// Check that we're receiving the events we expect
	telemetryMock, ok := probe.deps.Telemetry.(telemetry.Mock)
	require.True(t, ok)

	expectedEvents := map[string]int{
		ebpf.CudaEventTypeKernelLaunch.String():      4, // cudaLaunchKernel x2, cuLaunchKernel, cuLaunchKernelEx
		ebpf.CudaEventTypeSetDevice.String():         1,
		ebpf.CudaEventTypeMemory.String():            2,
		ebpf.CudaEventTypeSync.String():              4, // cudaStreamSynchronize, cudaEventQuery, cudaEventSynchronize, cuStreamSynchronize
		ebpf.CudaEventTypeVisibleDevicesSet.String(): 1,
		ebpf.CudaEventTypeSyncDevice.String():        2, // cudaDeviceSynchronize, cudaMemcpy
	}

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		eventMetrics, err := telemetryMock.GetCountMetric("gpu__consumer", "events")
		assert.NoError(c, err)

		actualEvents := make(map[string]int)
		for _, m := range eventMetrics {
			if evType, ok := m.Tags()["event_type"]; ok {
				actualEvents[evType] = int(m.Value())
			}
		}

		for evName, value := range expectedEvents {
			assert.Equal(c, value, actualEvents[evName], "event %s count mismatch", evName)
			delete(actualEvents, evName)
		}

		assert.Empty(c, actualEvents, "unexpected events: %v", actualEvents)
	}, 3*time.Second, 100*time.Millisecond, "events not found")

	return handlers
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
	out := testutil.RunSample(t, testutil.CudaSample)
	cmd := out.Command
	require.NotNil(t, cmd)

	handlers := s.waitForExpectedCudasampleEvents(probe, cmd.Process.Pid)

	// Check device assignments
	require.Contains(t, probe.sysCtx.selectedDeviceByPIDAndTID, cmd.Process.Pid)
	tidMap := probe.sysCtx.selectedDeviceByPIDAndTID[cmd.Process.Pid]
	require.Len(t, tidMap, 1)
	require.ElementsMatch(t, []int{cmd.Process.Pid}, maps.Keys(tidMap))

	streamPastData := handlers.stream.getPastData()
	require.NotNil(t, streamPastData)
	require.Equal(t, 2, len(streamPastData.kernels))
	for i := range 2 {
		span := streamPastData.kernels[i]
		require.Equal(t, uint64(3-i*2), span.numKernels) // first kernel launch has 3 kernels, second has 1
		require.Equal(t, uint64(1*2*3*4*5*6), span.avgThreadCount)
		require.Greater(t, span.endKtime, span.startKtime)
	}

	globalPastData := handlers.global.getPastData()
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

	out := testutil.RunSample(t, testutil.CudaSample)
	cmd := out.Command
	require.NotNil(t, cmd)

	_ = s.waitForExpectedCudasampleEvents(probe, cmd.Process.Pid)

	stats, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotEmpty(t, stats.ProcessMetrics)

	// Ensure the metrics we get are correct
	metricKey := model.ProcessStatsKey{PID: uint32(cmd.Process.Pid), DeviceUUID: testutil.DefaultGpuUUID}
	metrics := getMetricsEntry(metricKey, stats)
	require.NotNil(t, metrics)
	require.Greater(t, metrics.UsedCores, 0.0) // core usage depends on the time this took to run, so it's not deterministic
	require.Equal(t, expectedCudaSampleMaxMemory, metrics.Memory.MaxBytes)
	require.Greater(t, metrics.ActiveTimePct, 0.0) // active time percentage should be > 0 since kernels ran
	require.LessOrEqual(t, metrics.ActiveTimePct, 100.0)

	// Check device-level metrics
	require.NotEmpty(t, stats.DeviceMetrics)
	var foundDevice bool
	for _, deviceMetric := range stats.DeviceMetrics {
		if deviceMetric.DeviceUUID == testutil.DefaultGpuUUID {
			foundDevice = true
			require.Greater(t, deviceMetric.Metrics.ActiveTimePct, 0.0)
			require.LessOrEqual(t, deviceMetric.Metrics.ActiveTimePct, 100.0)
			break
		}
	}
	require.True(t, foundDevice, "device metrics not found for default GPU")

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

	out := testutil.RunSampleWithArgs(t, testutil.CudaSample, sampleArgs)
	cmd := out.Command
	require.NotNil(t, cmd)

	_ = s.waitForExpectedCudasampleEvents(probe, cmd.Process.Pid)

	stats, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, stats)
	metricKey := model.ProcessStatsKey{PID: uint32(cmd.Process.Pid), DeviceUUID: selectedGPU}
	metrics := getMetricsEntry(metricKey, stats)
	require.NotNil(t, metrics)

	require.Greater(t, metrics.UsedCores, 0.0) // average core usage depends on the time this took to run, so it's not deterministic
	require.Equal(t, expectedCudaSampleMaxMemory, metrics.Memory.MaxBytes)
	require.Greater(t, metrics.ActiveTimePct, 0.0)
	require.LessOrEqual(t, metrics.ActiveTimePct, 100.0)

	// Check device-level metrics for the selected GPU
	var foundDevice bool
	for _, deviceMetric := range stats.DeviceMetrics {
		if deviceMetric.DeviceUUID == selectedGPU {
			foundDevice = true
			require.Greater(t, deviceMetric.Metrics.ActiveTimePct, 0.0)
			require.LessOrEqual(t, deviceMetric.Metrics.ActiveTimePct, 100.0)
			break
		}
	}
	require.True(t, foundDevice, "device metrics not found for selected GPU")
}

func (s *probeTestSuite) TestDetectsContainer() {
	t := s.T()

	probe := s.getProbe()

	// note: after starting, the program will wait ~5s before making any CUDA call
	out := testutil.RunSampleInDocker(t, testutil.CudaSample, testutil.MinimalDockerImage)
	pid := out.PID
	cid := out.ContainerID

	handlers := s.waitForExpectedCudasampleEvents(probe, pid)

	require.Equal(t, cid, handlers.global.metadata.containerID)
	require.Equal(t, cid, handlers.stream.metadata.containerID)

	// Check that stats are properly collected
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		stats, err := probe.GetAndFlush()
		require.NoError(c, err)
		require.NotNil(c, stats)

		key := model.ProcessStatsKey{PID: uint32(pid), DeviceUUID: testutil.DefaultGpuUUID, ContainerID: cid}
		pidStats := getMetricsEntry(key, stats)
		require.NotNil(c, pidStats)

		// core usage depends on the time this took to run, so it's not deterministic
		require.Greater(c, pidStats.UsedCores, 0.0)
		require.Equal(c, expectedCudaSampleMaxMemory, pidStats.Memory.MaxBytes)
		require.Greater(c, pidStats.ActiveTimePct, 0.0)
		require.LessOrEqual(c, pidStats.ActiveTimePct, 100.0)
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
		out := testutil.RunSampleWithArgs(b, testutil.RateSample, rateArgs)
		cmd := out.Command
		require.NotNil(b, cmd)

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

func assertEventEqual[T any](t *testing.T, expected any, actualMarshaled []byte) {
	expectedEvent, ok := expected.(*T)
	require.True(t, ok, "expected event should be pointer to type %T", expectedEvent)

	var actual T
	require.NoError(t, json.Unmarshal(actualMarshaled, &actual))

	// Get the header from both events using reflection
	expectedHeader := reflect.ValueOf(*expectedEvent).FieldByName("Header").Interface().(ebpf.CudaEventHeader)
	actualHeader := reflect.ValueOf(actual).FieldByName("Header").Interface().(ebpf.CudaEventHeader)

	// Only compare type and stream ID, as those are the only values we can
	// predict
	require.Equal(t, expectedHeader.Type, actualHeader.Type, "header type mismatch")
	require.Equal(t, expectedHeader.Stream_id, actualHeader.Stream_id, "header stream ID mismatch")

	// Now set the header in the expected event to the actual header, so that we can compare the rest of the event
	reflect.ValueOf(expectedEvent).Elem().FieldByName("Header").Set(reflect.ValueOf(actualHeader))

	require.Equal(t, expectedEvent, &actual)
}

func (s *probeTestSuite) TestDebugCollectorEvents() {
	t := s.T()

	probe := s.getProbe()

	// Build expected events array based on cudasample.c in order:
	// Line 33: cudaSetDevice(device) -> CudaEventTypeSetDevice
	// Line 34: cudaLaunchKernel(...) -> CudaEventTypeKernelLaunch
	// Line 35: cuLaunchKernel(...) -> CudaEventTypeKernelLaunch
	// Line 36-37: cuLaunchKernelEx(...) -> CudaEventTypeKernelLaunch
	// Line 38: cudaMalloc(&ptr, 100) -> CudaEventTypeMemory (alloc)
	// Line 39: cudaFree(ptr) -> CudaEventTypeMemory (free)
	// Line 40: cudaStreamSynchronize(stream) -> CudaEventTypeSync
	// Line 41: cuStreamSynchronize(stream) -> CudaEventTypeSync
	// Line 47: cudaMemcpy(...) -> CudaEventTypeSyncDevice
	// Line 49: cudaEventQuery(event) -> CudaEventTypeSync
	// Line 50: cudaEventSynchronize(event) -> CudaEventTypeSync
	// Line 54: cudaLaunchKernel(...) -> CudaEventTypeKernelLaunch
	// Line 56: cudaDeviceSynchronize() -> CudaEventTypeSyncDevice
	// Line 58: setenv("CUDA_VISIBLE_DEVICES", "42", 1) -> CudaEventTypeVisibleDevicesSet
	deviceIdx := int32(0) // default device index used by RunSample
	kernelAddr := uint64(0x1234)
	allocSize := uint64(100)
	streamID := uint64(30)

	expectedEvents := []interface{}{
		&ebpf.CudaSetDeviceEvent{Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeSetDevice), Stream_id: 0}, Device: deviceIdx},
		&ebpf.CudaKernelLaunch{
			Header:          ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeKernelLaunch), Stream_id: streamID},
			Kernel_addr:     kernelAddr,
			Grid_size:       ebpf.Dim3{X: 1, Y: 2, Z: 3},
			Block_size:      ebpf.Dim3{X: 4, Y: 5, Z: 6},
			Shared_mem_size: 10,
		},
		&ebpf.CudaKernelLaunch{
			Header:          ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeKernelLaunch), Stream_id: streamID},
			Kernel_addr:     kernelAddr,
			Grid_size:       ebpf.Dim3{X: 1, Y: 2, Z: 3},
			Block_size:      ebpf.Dim3{X: 4, Y: 5, Z: 6},
			Shared_mem_size: 10,
		},
		&ebpf.CudaKernelLaunch{ // cuLaunchKernelEx
			Header:          ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeKernelLaunch), Stream_id: streamID},
			Kernel_addr:     kernelAddr,
			Grid_size:       ebpf.Dim3{X: 1, Y: 2, Z: 3},
			Block_size:      ebpf.Dim3{X: 4, Y: 5, Z: 6},
			Shared_mem_size: 10,
		},
		&ebpf.CudaMemEvent{
			Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeMemory), Stream_id: 0},
			Type:   uint32(ebpf.CudaMemAlloc),
			Size:   allocSize,
			Addr:   uint64(0xdeadbeef),
		},
		&ebpf.CudaMemEvent{
			Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeMemory), Stream_id: 0},
			Type:   uint32(ebpf.CudaMemFree),
			Addr:   uint64(0xdeadbeef),
		},
		&ebpf.CudaSync{Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeSync), Stream_id: streamID}},
		&ebpf.CudaSync{Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeSync), Stream_id: streamID}},
		&ebpf.CudaSyncDeviceEvent{Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeSyncDevice), Stream_id: 0}},
		&ebpf.CudaSync{Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeSync), Stream_id: streamID}},
		&ebpf.CudaSync{Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeSync), Stream_id: streamID}},
		&ebpf.CudaKernelLaunch{
			Header:          ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeKernelLaunch), Stream_id: streamID},
			Kernel_addr:     kernelAddr,
			Grid_size:       ebpf.Dim3{X: 1, Y: 2, Z: 3},
			Block_size:      ebpf.Dim3{X: 4, Y: 5, Z: 6},
			Shared_mem_size: 10,
		},
		&ebpf.CudaSyncDeviceEvent{Header: ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeSyncDevice), Stream_id: 0}},
		&ebpf.CudaVisibleDevicesSetEvent{
			Header:  ebpf.CudaEventHeader{Type: uint32(ebpf.CudaEventTypeVisibleDevicesSet), Stream_id: 0},
			Devices: [256]byte{0x34, 0x32, 0x00}, // "42" as bytes
		},
	}

	totalExpectedEvents := len(expectedEvents)

	probe.consumer.debugCollector.enable(totalExpectedEvents * 2) // some margin in case we get extra events

	// Run the CUDA sample
	out := testutil.RunSample(t, testutil.CudaSample)
	cmd := out.Command
	require.NotNil(t, cmd)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	require.Eventually(t, func() bool {
		return len(probe.consumer.debugCollector.eventStore) >= totalExpectedEvents
	}, 10*time.Second, 100*time.Millisecond, "should collect at least %d events", totalExpectedEvents)

	collectedEvents := probe.consumer.debugCollector.eventStore
	require.Equal(t, len(collectedEvents), totalExpectedEvents, "should collect exactly %d events", totalExpectedEvents)

	// Parse collected events
	for i, eventBytes := range collectedEvents {
		expectedEvent := expectedEvents[i]

		// Parse event-specific fields
		switch expectedEvent.(type) {
		case *ebpf.CudaKernelLaunch:
			assertEventEqual[ebpf.CudaKernelLaunch](t, expectedEvent, eventBytes)
		case *ebpf.CudaMemEvent:
			assertEventEqual[ebpf.CudaMemEvent](t, expectedEvent, eventBytes)
		case *ebpf.CudaSetDeviceEvent:
			assertEventEqual[ebpf.CudaSetDeviceEvent](t, expectedEvent, eventBytes)
		case *ebpf.CudaSync:
			assertEventEqual[ebpf.CudaSync](t, expectedEvent, eventBytes)
		case *ebpf.CudaSyncDeviceEvent:
			assertEventEqual[ebpf.CudaSyncDeviceEvent](t, expectedEvent, eventBytes)
		case *ebpf.CudaVisibleDevicesSetEvent:
			assertEventEqual[ebpf.CudaVisibleDevicesSetEvent](t, expectedEvent, eventBytes)
		default:
			require.Fail(t, "unexpected event type: %T", expectedEvent)
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
