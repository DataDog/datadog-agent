// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

func TestEmitNvmlMetrics(t *testing.T) {
	// Create a mock sender
	mockSender := mocksender.NewMockSender("gpu")
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	wmetaMock := testutil.GetWorkloadMetaMockWithDefaultGPUs(t)
	// Create check instance using mocks
	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		wmetaMock,
	)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	// enable GPU check in configuration right before Configure
	WithGPUConfigEnabled(t)
	check.containerProvider = mock_containers.NewMockContainerProvider(gomock.NewController(t))
	require.NoError(t, check.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	// we need to cancel the check to make sure all resources and async workers are released
	// before deinitializing the mock library at test cleanup
	t.Cleanup(func() { checkGeneric.Cancel() })

	device1UUID := "gpu-uuid-1"
	device2UUID := "gpu-uuid-2"

	// create mock library returning just the 2 test devices
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	device1 := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions(), func(d *nvmlmock.Device) {
		d.GetUUIDFunc = func() (string, nvml.Return) { return device1UUID, nvml.SUCCESS }
	})
	device2 := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions(), func(d *nvmlmock.Device) {
		d.GetUUIDFunc = func() (string, nvml.Return) { return device2UUID, nvml.SUCCESS }
	})
	ddnvml.WithMockNVML(t, nvmlMock)
	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		switch index {
		case 0:
			return device1, nvml.SUCCESS
		case 1:
			return device2, nvml.SUCCESS
		default:
			return nil, nvml.ERROR_INVALID_ARGUMENT
		}
	}

	// Create mock collectors
	for i, deviceUUID := range []string{device1UUID, device2UUID} {
		metricValueBase := 10 * i

		check.collectors = append(check.collectors, &mockCollector{
			name:       "device",
			deviceUUID: deviceUUID,
			metrics: []nvidia.Metric{
				{Name: "metric1", Value: float64(metricValueBase + 1), Type: ddmetrics.GaugeType, Priority: 0},
				{Name: "metric2", Value: float64(metricValueBase + 2), Type: ddmetrics.GaugeType, Priority: 0},
			},
		})

		check.collectors = append(check.collectors, &mockCollector{
			name:       "fields",
			deviceUUID: deviceUUID,
			metrics: []nvidia.Metric{
				{Name: "metric2", Value: float64(metricValueBase + 2), Type: ddmetrics.GaugeType, Priority: 1},
				{Name: "metric3", Value: float64(metricValueBase + 3), Type: ddmetrics.GaugeType, Priority: 1},
			},
		})
	}

	// Set up GPU and container tags
	containerID1 := "container1"
	containerID2 := "container2"
	containerTags1 := []string{"container_id:" + containerID1}
	containerTags2 := []string{"container_id:" + containerID2}
	containerTags := append(containerTags1, containerTags2...)
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, containerID1), "foo", containerTags1, nil, nil, nil)
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, containerID2), "foo", containerTags2, nil, nil, nil)

	container1 := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID1,
			Kind: workloadmeta.KindContainer,
		},
	}
	container2 := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID2,
			Kind: workloadmeta.KindContainer,
		},
	}
	gpuToContainersMap := map[string][]*workloadmeta.Container{
		device1UUID: {container1, container2},
	}
	wmetaMock.Set(container1)
	wmetaMock.Set(container2)

	// Process the metrics
	metricTime := time.Now()
	metricTimestamp := float64(metricTime.UnixNano()) / float64(time.Second)
	require.NoError(t, check.deviceCache.Refresh())
	require.NoError(t, check.emitMetrics(mockSender, gpuToContainersMap, metricTime))

	// Verify metrics for each device
	for i, deviceUUID := range []string{device1UUID, device2UUID} {
		metricValueBase := 10 * i

		// Build expected tags
		var expectedTags []string
		if deviceUUID == device1UUID {
			// Device 1 has container tags
			expectedTags = append([]string{"gpu_uuid:" + deviceUUID}, containerTags...)
		} else {
			// Device 2 has no container tags
			expectedTags = []string{"gpu_uuid:" + deviceUUID}
		}
		slices.Sort(expectedTags)

		matchTagsFunc := func(tags []string) bool {
			slices.Sort(tags)
			return slices.Equal(tags, expectedTags)
		}

		// Verify metrics for this device
		// metric1: only from device collector (priority 0)
		mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.metric1", float64(metricValueBase+1), "", mock.MatchedBy(matchTagsFunc), metricTimestamp)

		// metric2: priority 1 wins (from fields collector)
		mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.metric2", float64(metricValueBase+2), "", mock.MatchedBy(matchTagsFunc), metricTimestamp)

		// metric3: only from fields collector (priority 1)
		mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.metric3", float64(metricValueBase+3), "", mock.MatchedBy(matchTagsFunc), metricTimestamp)
	}
}

func TestRunDoesNotError(t *testing.T) {
	// Tests for the specific output are above, this only ensures that the run function does not error
	// even if things are not correctly setup
	senderManager := mocksender.CreateDefaultDemultiplexer()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ddnvml.WithMockNVML(t,
		testutil.GetBasicNvmlMockWithOptions(
			testutil.WithMockAllFunctions(),
			testutil.WithProcessInfoCallback(func(_ string) ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.SUCCESS // disable process info, we don't want to mock that part here
			}),
		),
	)
	wmetaMock := testutil.GetWorkloadMetaMockWithDefaultGPUs(t)

	// Create check instance using mocks
	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		wmetaMock,
	)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	// Add a container to the workload meta mock with GPU devices
	wmetaMock.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "container1",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "container1",
		},
		ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
			{
				Name: "nvidia.com/gpu",
				ID:   testutil.DefaultGpuUUID,
			},
		},
	})

	// Enable GPU check in configuration right before Configure
	WithGPUConfigEnabled(t)

	check.containerProvider = mock_containers.NewMockContainerProvider(gomock.NewController(t))
	err := checkGeneric.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test")
	require.NoError(t, err)
	// we need to cancel the check to make sure all resources and async workers are released
	// before deinitializing the mock library at test cleanup
	t.Cleanup(func() { checkGeneric.Cancel() })

	require.NoError(t, checkGeneric.Run())
}

func TestCollectorsOnDeviceChanges(t *testing.T) {
	// note: bump this when we'll add new collectors in nvidia.BuildCollectors
	const numSupportedCollectorTypes = 5

	// mock up device count so that we can check when check collectors are created/destroyed
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithMockAllFunctions(),
		testutil.WithProcessInfoCallback(func(_ string) ([]nvml.ProcessInfo, nvml.Return) {
			return nil, nvml.SUCCESS // disable process info, we don't want to mock that part here
		}),
		testutil.WithCapabilities(testutil.Capabilities{GPM: true}),
		testutil.WithMIGDisabled(),
	)
	ddnvml.WithMockNVML(t, nvmlMock)
	curDeviceCount := atomic.Int32{}
	curDeviceCount.Store(int32(len(testutil.GPUUUIDs)) - 2)
	nvmlMock.DeviceGetCountFunc = func() (int, nvml.Return) { return int(curDeviceCount.Load()), nvml.SUCCESS }

	// assert function to be used below, checking that the created collectors map to the current devices
	assertCollectors := func(collectors []nvidia.Collector) {
		visibleDevices := int(curDeviceCount.Load())
		assert.Len(t, collectors, visibleDevices*numSupportedCollectorTypes)

		expectedUUIDs := map[string]int{}
		for i := range visibleDevices { // check only on visible devices
			expectedUUIDs[testutil.GPUUUIDs[i]] = numSupportedCollectorTypes
		}

		actualUUIDs := map[string]int{}
		for _, c := range collectors {
			actualUUIDs[c.DeviceUUID()]++
		}

		assert.Equal(t, expectedUUIDs, actualUUIDs)
	}

	// create check instance using mocks
	iCheck := newCheck(taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMockWithDefaultGPUs(t))
	check, ok := iCheck.(*Check)
	require.True(t, ok)

	// enable GPU check in configuration right before Configure
	WithGPUConfigEnabled(t)

	// configure check
	check.containerProvider = mock_containers.NewMockContainerProvider(gomock.NewController(t))
	require.NoError(t, check.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	require.Empty(t, check.collectors)
	t.Cleanup(func() { check.Cancel() })

	// do a first run and check that collectors have been created
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// a second run should not trigger any new device being added
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// simulate device hot-plug
	curDeviceCount.Add(2)
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// simulate device falling off bus
	curDeviceCount.Add(-1)
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)
}

func TestCollectorsOnMIGDeviceChanges(t *testing.T) {
	// note: bump this when we'll add new collectors in nvidia.BuildCollectors
	const numSupportedCollectorTypes = 5

	// Use device index 5 which has MIG support in testutil
	deviceIdx := 5
	parentUUID := testutil.GPUUUIDs[deviceIdx]

	// Track the number of MIG children dynamically
	curMIGChildCount := atomic.Int32{}
	curMIGChildCount.Store(0) // Start with MIG disabled

	// Create the parent device mock
	parentDevice := testutil.GetDeviceMock(deviceIdx, testutil.WithMockAllDeviceFunctions(), func(d *nvmlmock.Device) {
		// Override MIG-related functions to be dynamic
		d.GetMigModeFunc = func() (int, int, nvml.Return) {
			if curMIGChildCount.Load() > 0 {
				return nvml.DEVICE_MIG_ENABLE, 0, nvml.SUCCESS
			}
			return nvml.DEVICE_MIG_DISABLE, 0, nvml.SUCCESS
		}
		d.GetMaxMigDeviceCountFunc = func() (int, nvml.Return) {
			return int(curMIGChildCount.Load()), nvml.SUCCESS
		}
		d.GetMigDeviceHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
			if index >= int(curMIGChildCount.Load()) {
				return nil, nvml.ERROR_NOT_FOUND
			}
			return testutil.GetMIGDeviceMock(deviceIdx, index, testutil.WithMockAllDeviceFunctions()), nvml.SUCCESS
		}
		d.GpmMigSampleGetFunc = func(_ int, _ nvml.GpmSample) nvml.Return {
			return nvml.SUCCESS
		}
		d.GpmSampleGetFunc = func(_ nvml.GpmSample) nvml.Return {
			return nvml.SUCCESS
		}
	})

	// Setup NVML mock with single parent device
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithMockAllFunctions(),
		testutil.WithProcessInfoCallback(func(_ string) ([]nvml.ProcessInfo, nvml.Return) {
			return nil, nvml.SUCCESS
		}),
		testutil.WithCapabilities(testutil.Capabilities{GPM: true}),
	)
	nvmlMock.DeviceGetCountFunc = func() (int, nvml.Return) { return 1, nvml.SUCCESS }
	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		if index == 0 {
			return parentDevice, nvml.SUCCESS
		}
		return nil, nvml.ERROR_INVALID_ARGUMENT
	}
	ddnvml.WithMockNVML(t, nvmlMock)

	// Assert function to check collectors match current device state
	assertCollectors := func(collectors []nvidia.Collector) {
		migCount := int(curMIGChildCount.Load())
		expectedCollectorCount := numSupportedCollectorTypes + (migCount * numSupportedCollectorTypes)
		assert.Len(t, collectors, expectedCollectorCount,
			"Expected %d collectors (1 parent*%d + %d mig*%d), got %d",
			expectedCollectorCount, numSupportedCollectorTypes, migCount, numSupportedCollectorTypes, len(collectors))

		// Count collectors by UUID
		actualUUIDs := map[string]int{}
		for _, c := range collectors {
			actualUUIDs[c.DeviceUUID()]++
		}

		// Build expected UUIDs
		expectedUUIDs := map[string]int{
			parentUUID: numSupportedCollectorTypes,
		}
		for i := 0; i < migCount; i++ {
			migUUID := testutil.MIGChildrenUUIDs[deviceIdx][i]
			expectedUUIDs[migUUID] = numSupportedCollectorTypes
		}

		assert.Equal(t, expectedUUIDs, actualUUIDs)
	}

	// Create check instance
	iCheck := newCheck(taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMockWithDefaultGPUs(t))
	check, ok := iCheck.(*Check)
	require.True(t, ok)

	// Enable GPU check and configure
	WithGPUConfigEnabled(t)
	mockCtrl := gomock.NewController(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(mockCtrl)
	// Expect GetPidToCid to be called and return an empty map (no processes)
	mockContainerProvider.EXPECT().GetPidToCid(gomock.Any()).Return(map[int]string{}).AnyTimes()
	check.containerProvider = mockContainerProvider
	require.NoError(t, check.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	require.Empty(t, check.collectors)
	t.Cleanup(func() { check.Cancel() })

	// First run: MIG disabled, should have collectors for parent device only
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// Second run: no change, collectors should remain the same
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// Enable MIG with 1 child
	curMIGChildCount.Store(1)
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// Increase MIG children count to 2 (max for device index 5)
	curMIGChildCount.Store(2)
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// Decrease MIG children count back to 1
	curMIGChildCount.Store(1)
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)

	// Disable MIG completely
	curMIGChildCount.Store(0)
	require.NoError(t, check.Run())
	assertCollectors(check.collectors)
}

// mockCollector implements the nvidia.Collector interface for testing
type mockCollector struct {
	name       nvidia.CollectorName
	deviceUUID string
	metrics    []nvidia.Metric
}

func (m *mockCollector) Collect() ([]nvidia.Metric, error) {
	return m.metrics, nil
}

func (m *mockCollector) Name() nvidia.CollectorName {
	return m.name
}

func (m *mockCollector) DeviceUUID() string {
	return m.deviceUUID
}

func mockMatchesTags(expectedTags []string) interface{} {
	slices.Sort(expectedTags)
	return mock.MatchedBy(func(tags []string) bool {
		slices.Sort(tags)
		return slices.Equal(tags, expectedTags)
	})
}

func TestTagsChangeBetweenRuns(t *testing.T) {
	// Create a mock sender
	mockSender := mocksender.NewMockSender("gpu")
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Create check instance using mocks
	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		testutil.GetWorkloadMetaMock(t),
	)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	// enable GPU check in configuration right before Configure
	WithGPUConfigEnabled(t)
	check.containerProvider = mock_containers.NewMockContainerProvider(gomock.NewController(t))
	require.NoError(t, check.Configure(mocksender.CreateDefaultDemultiplexer(), integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	// we need to cancel the check to make sure all resources and async workers are released
	// before deinitializing the mock library at test cleanup
	t.Cleanup(func() { checkGeneric.Cancel() })

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMockAllFunctions(), testutil.WithDeviceCount(1))
	ddnvml.WithMockNVML(t, nvmlMock)

	// Create mock collector
	deviceUUID := testutil.GPUUUIDs[0]
	check.collectors = append(check.collectors, &mockCollector{
		name:       "device",
		deviceUUID: deviceUUID,
		metrics: []nvidia.Metric{
			{Name: "test_metric", Value: 42.0, Type: ddmetrics.GaugeType, Priority: 0},
		},
	})

	require.NoError(t, check.deviceCache.Refresh())

	// First run: minimal GPU tags (just uuid fallback)
	metricTime1 := time.Now()
	metricTimestamp1 := float64(metricTime1.UnixNano()) / float64(time.Second)
	require.NoError(t, check.emitMetrics(mockSender, map[string][]*workloadmeta.Container{}, metricTime1))

	expectedTags1 := []string{"gpu_uuid:" + deviceUUID}
	mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.test_metric", 42.0, "", mockMatchesTags(expectedTags1), metricTimestamp1)

	// Reset mock to verify new calls
	mockSender.ResetCalls()

	// Second run: add GPU tags via tagger
	gpuTags1 := []string{"gpu_uuid:" + deviceUUID, "gpu_model:Tesla_T4", "pci_bus_id:0000:00:1e.0"}
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, deviceUUID), "foo", gpuTags1, nil, nil, nil)

	metricTime2 := time.Now()
	metricTimestamp2 := float64(metricTime2.UnixNano()) / float64(time.Second)
	require.NoError(t, check.emitMetrics(mockSender, map[string][]*workloadmeta.Container{}, metricTime2))

	mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.test_metric", 42.0, "", mockMatchesTags(gpuTags1), metricTimestamp2)

	// Reset mock for third run
	mockSender.ResetCalls()

	// Third run: change GPU tags to different values
	gpuTags2 := []string{"gpu_uuid:" + deviceUUID, "gpu_model:A100", "pci_bus_id:0000:00:1f.0", "datacenter:us-west-1"}
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, deviceUUID), "foo", gpuTags2, nil, nil, nil)

	metricTime3 := time.Now()
	metricTimestamp3 := float64(metricTime3.UnixNano()) / float64(time.Second)
	require.NoError(t, check.emitMetrics(mockSender, map[string][]*workloadmeta.Container{}, metricTime3))
	mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.test_metric", 42.0, "", mockMatchesTags(gpuTags2), metricTimestamp3)
}

func TestRunEmitsCorrectTags(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMockAllFunctions(), testutil.WithDeviceCount(2))
	ddnvml.WithMockNVML(t, nvmlMock)

	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		wmetaMock,
	)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	mockSender := mocksender.NewMockSenderWithSenderManager(check.ID(), senderManager)
	check.containerProvider = mock_containers.NewMockContainerProvider(gomock.NewController(t))
	WithGPUConfigEnabled(t)

	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	t.Cleanup(func() { check.Cancel() })

	// Reset the collectors, use the mock ones only
	check.collectors = nil

	// Configure the check with the desired gpu/container/process layout For
	// each GPU, we create one collector that sends a metric with no associated
	// workloads, and then another one that sends a metric with all the
	// processes associated with the GPU as associated workloads. For each
	// container associated with a GPU, we create a process that is associated
	// with the container, and a collector that sends a metric with the process
	// ID as associated workload.
	// We also configure the mock sender to expect the corresponding metrics to be emitted.
	var containers []*workloadmeta.Container
	var processes []*workloadmeta.Process
	desiredLayout := []struct {
		deviceUUID    string
		numContainers int
	}{{
		deviceUUID:    testutil.GPUUUIDs[0],
		numContainers: 1,
	},
		{
			deviceUUID:    testutil.GPUUUIDs[1],
			numContainers: 2,
		}}

	callCount := 0
	for _, layout := range desiredLayout {
		var allProcessEntityIDs []workloadmeta.EntityID
		var allContainerTags []string
		var allProcessTags []string

		device := &workloadmeta.GPU{
			EntityID: workloadmeta.EntityID{
				ID:   layout.deviceUUID,
				Kind: workloadmeta.KindGPU,
			},
			Device:             "mock_device",
			Vendor:             "datadog",
			DriverVersion:      "1.0.0",
			VirtualizationMode: "none",
		}
		deviceTags := []string{"gpu_uuid:" + layout.deviceUUID, "gpu_device:mock_device", "gpu_vendor:datadog", "gpu_driver_version:1.0.0", "gpu_virtualization_mode:none"}
		fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, layout.deviceUUID), "foo", deviceTags, nil, nil, nil)
		wmetaMock.Set(device)

		var metricsToSend []nvidia.Metric
		for i := 0; i < layout.numContainers; i++ {
			container := &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					ID:   fmt.Sprintf("container%d", len(containers)+i),
					Kind: workloadmeta.KindContainer,
				},
				ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
					{
						Name: "nvidia.com/gpu",
						ID:   layout.deviceUUID,
					},
				},
			}

			pid := int32(len(processes)+i) + 1000
			process := &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					ID:   strconv.Itoa(int(pid)),
					Kind: workloadmeta.KindProcess,
				},
				Owner:       &container.EntityID,
				ContainerID: container.EntityID.ID,
				Pid:         pid,
				NsPid:       pid,
			}

			processTags := []string{"pid:" + strconv.Itoa(int(pid)), "nspid:" + strconv.Itoa(int(pid))}
			containerTags := []string{"container_id:" + container.EntityID.ID}

			processes = append(processes, process)
			containers = append(containers, container)
			allProcessEntityIDs = append(allProcessEntityIDs, process.EntityID)
			allContainerTags = append(allContainerTags, containerTags...)
			allProcessTags = append(allProcessTags, processTags...)

			fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, container.EntityID.ID), "foo", containerTags, nil, nil, nil)
			wmetaMock.Set(process)
			wmetaMock.Set(container)

			callCount++
			metricsToSend = append(metricsToSend, nvidia.Metric{Name: "workload_metric", Value: float64(callCount), Type: ddmetrics.GaugeType, Priority: 0, AssociatedWorkloads: []workloadmeta.EntityID{process.EntityID}})

			expectedTags := append(deviceTags, processTags...)
			expectedTags = append(expectedTags, containerTags...)
			mockSender.On("GaugeWithTimestamp", "gpu.workload_metric", float64(callCount), "", mockMatchesTags(expectedTags), mock.Anything).Return()
		}

		callCount++
		metricsToSend = append(metricsToSend, nvidia.Metric{Name: "no_workload_metric", Value: float64(callCount), Type: ddmetrics.GaugeType, Priority: 0})
		noWorkloadTags := append(deviceTags, allContainerTags...)
		mockSender.On("GaugeWithTimestamp", "gpu.no_workload_metric", float64(callCount), "", mockMatchesTags(noWorkloadTags), mock.Anything).Return()

		callCount++
		// Use a Count metric just to make it easier to distinguish mock calls
		metricsToSend = append(metricsToSend, nvidia.Metric{Name: "all_workload_metric", Value: float64(callCount), Type: ddmetrics.CountType, Priority: 0, AssociatedWorkloads: allProcessEntityIDs})
		allWorkloadTags := append(deviceTags, allContainerTags...)
		allWorkloadTags = append(allWorkloadTags, allProcessTags...)
		mockSender.On("CountWithTimestamp", "gpu.all_workload_metric", float64(callCount), "", mockMatchesTags(allWorkloadTags), mock.Anything).Return()

		check.collectors = append(check.collectors, &mockCollector{
			name:       "mockCollector",
			deviceUUID: layout.deviceUUID,
			metrics:    metricsToSend,
		})
	}

	mockSender.On("Commit").Return()

	require.NoError(t, check.Run())

	mockSender.AssertExpectations(t)
}

// TestMemoryLimitTagStabilityOnIdleSample reproduces a non-determinism bug:
// when the GPU is idle (no running processes), the stateless collector downgrades
// memory.limit to Low priority (because allWorkloadIDs is empty). The eBPF
// collector also emits memory.limit at Low priority but may still carry cached
// inactive PIDs as AssociatedWorkloads. Because RemoveDuplicateMetrics resolves
// same-priority ties by map iteration order, the winner—and therefore the tag
// set on gpu.memory.limit—flips between PID-scoped and device-wide tagging
// across runs, creating unstable timeseries cardinality.
func TestMemoryLimitTagStabilityOnIdleSample(t *testing.T) {
	cachedPid := uint32(5678)
	deviceUUID := testutil.GPUUUIDs[0]
	var procInfo []nvml.ProcessInfo

	// Mock NVML: single device, no running processes (idle GPU).
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithMIGDisabled(),
		testutil.WithDeviceCount(1),
		testutil.WithMockAllFunctions(),
		testutil.WithProcessInfoCallback(func(_ string) ([]nvml.ProcessInfo, nvml.Return) {
			return procInfo, nvml.SUCCESS
		}),
	)
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)

	// SP cache: starts with one active PID so the eBPF collector seeds its
	// internal activeMetrics map.
	spCache := &nvidia.SystemProbeCache{}

	deps := &nvidia.CollectorDependencies{
		SystemProbeCache: spCache,
		Workloadmeta:     testutil.GetWorkloadMetaMockWithDefaultGPUs(t),
	}

	// Only keep stateless + ebpf; disable everything else.
	disabled := []string{"sampling", "fields", "gpm", "device_events"}
	collectors, err := nvidia.BuildCollectors(devices, deps, disabled)
	require.NoError(t, err)

	processData := [][]struct {
		pid    uint32
		memory uint64
	}{
		// Round 1: active process
		{{
			pid:    cachedPid,
			memory: 1024,
		}},
		// Round 2: no active process
		{},
	}

	for _, procData := range processData {
		// Setup data sources for both collectors
		var spStats model.GPUStats
		for _, proc := range procData {
			spStats.ProcessMetrics = append(spStats.ProcessMetrics, model.ProcessStatsTuple{
				Key: model.ProcessStatsKey{PID: proc.pid, DeviceUUID: deviceUUID},
				UtilizationMetrics: model.UtilizationMetrics{
					Memory: model.MemoryMetrics{CurrentBytes: proc.memory},
				},
			})
		}
		spCache.SetStatsForTest(&spStats)

		procInfo = make([]nvml.ProcessInfo, len(procData))
		for i, proc := range procData {
			procInfo[i] = nvml.ProcessInfo{Pid: proc.pid, UsedGpuMemory: proc.memory}
		}

		// Collect from the real collectors and group by collector name.
		collectorMetrics := make(map[nvidia.CollectorName][]nvidia.Metric)
		for _, c := range collectors {
			m, _ := c.Collect() // errors expected from unsupported APIs, ignore
			collectorMetrics[c.Name()] = m
		}

		// Part 1 (deterministic): the two collectors must NOT emit memory.limit
		// at the same priority. Equal priorities let map-iteration order decide
		// the dedup winner, which is non-deterministic.
		memLimitMetrics := make(map[nvidia.CollectorName][]nvidia.Metric)
		for name, metrics := range collectorMetrics {
			for _, m := range metrics {
				if m.Name == "memory.limit" {
					memLimitMetrics[name] = append(memLimitMetrics[name], m)
				}
			}
		}

		// We expect memory.limit from at least two different collectors.
		statelessCollector := nvidia.CollectorName("stateless")
		ebpfCollector := nvidia.CollectorName("ebpf")

		require.Len(t, memLimitMetrics, 2)
		require.Contains(t, memLimitMetrics, statelessCollector)
		require.Contains(t, memLimitMetrics, ebpfCollector)
		require.Len(t, memLimitMetrics[statelessCollector], 2) // memory.limit comes from two APIs in stateless collector
		require.Len(t, memLimitMetrics[ebpfCollector], 1)      // memory.limit comes from one API in ebpf collector
		require.NotEmpty(t, memLimitMetrics[ebpfCollector][0].AssociatedWorkloads, "memory.limit must be emitted when deduplication happens")

		for _, m := range memLimitMetrics[statelessCollector] {
			require.Greater(t, m.Priority, memLimitMetrics[ebpfCollector][0].Priority, "memory.limit must always have higher priority in stateless collector than in ebpf collector")
		}
	}
}

func TestDisabledCollectorsConfiguration(t *testing.T) {
	tests := []struct {
		name               string
		disabledCollectors []string
		expected           []string
	}{
		{
			name:               "disable gpm collector",
			disabledCollectors: []string{"gpm"},
			expected:           []string{"gpm"},
		},
		{
			name:               "disable multiple collectors",
			disabledCollectors: []string{"gpm", "fields", "sampling"},
			expected:           []string{"gpm", "fields", "sampling"},
		},
		{
			name:               "disable all collectors",
			disabledCollectors: []string{"stateless", "sampling", "fields", "gpm", "device_events"},
			expected:           []string{"stateless", "sampling", "fields", "gpm", "device_events"},
		},
		{
			name:               "no collectors disabled",
			disabledCollectors: []string{},
			expected:           []string{},
		},
		{
			name:               "nil disabled_collectors list",
			disabledCollectors: nil,
			expected:           []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			wmetaMock := testutil.GetWorkloadMetaMockWithDefaultGPUs(t)

			checkGeneric := newCheck(
				fakeTagger,
				testutil.GetTelemetryMock(t),
				wmetaMock,
			)
			check, ok := checkGeneric.(*Check)
			require.True(t, ok)

			WithGPUConfigEnabled(t)
			pkgconfigsetup.Datadog().SetWithoutSource("gpu.disabled_collectors", tt.disabledCollectors)
			t.Cleanup(func() {
				pkgconfigsetup.Datadog().SetWithoutSource("gpu.disabled_collectors", []string{})
			})

			check.containerProvider = mock_containers.NewMockContainerProvider(gomock.NewController(t))
			err := check.Configure(
				mocksender.CreateDefaultDemultiplexer(),
				integration.FakeConfigHash,
				[]byte{},
				[]byte{},
				"test",
			)
			require.NoError(t, err)

			// Verify the disabled collectors are correctly identified in the check struct
			assert.Equal(t, len(tt.expected), len(check.disabledCollectors),
				"expected %d disabled collectors, got %d", len(tt.expected), len(check.disabledCollectors))
			assert.ElementsMatch(t, tt.expected, check.disabledCollectors,
				"disabled collectors mismatch")
		})
	}
}

func TestMetricsFollowSpec(t *testing.T) {
	metricsSpec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)
	archFile, err := gpuspec.LoadArchitecturesSpec()
	require.NoError(t, err)

	// Build spec metric set for quick membership checks.
	specMetrics := make(map[string]struct{}, len(metricsSpec.Metrics))
	for name := range metricsSpec.Metrics {
		specMetrics[name] = struct{}{}
	}

	deviceModes := []gpuspec.DeviceMode{
		gpuspec.DeviceModePhysical,
		gpuspec.DeviceModeMIG,
		gpuspec.DeviceModeVGPU,
	}

	for archName, archSpec := range archFile.Architectures {
		t.Run("arch="+archName, func(t *testing.T) {
			for _, mode := range deviceModes {
				if !gpuspec.IsModeSupportedByArchitecture(archSpec, mode) {
					continue
				}

				t.Run("mode="+string(mode), func(t *testing.T) {
					emittedMetrics, knownTagValues := collectMetricSamples(t, archName, mode, archSpec)

					t.Run("_emits_only_expected_metrics", func(t *testing.T) {
						for metricName := range emittedMetrics {
							assert.Contains(t, specMetrics, metricName, "metric emitted by check is missing from spec: %s", metricName)

							metricSpec := metricsSpec.Metrics[metricName]
							assert.True(t, metricSpec.SupportsArchitecture(archName), "metric %s emitted on unsupported architecture %s", metricName, archName)
							assert.False(t, metricSpec.IsDeviceModeExplicitlyUnsupported(mode), "metric %s emitted on unsupported device mode %s", metricName, mode)
						}
					})

					for name, m := range metricsSpec.Metrics {
						if !m.SupportsArchitecture(archName) || !m.SupportsDeviceMode(mode) {
							continue
						}

						t.Run(name, func(t *testing.T) {
							metrics, found := emittedMetrics[name]
							require.True(t, found, "spec metric is not emitted by check run: %s", name)
							validateMetricTagsAgainstSpec(t, metricsSpec, name, m, metrics, knownTagValues)
						})
					}
				})
			}
		})
	}
}

// collectMetricSamples runs the GPU check with a capability-driven mock
// for the given architecture and device mode, then returns emitted metrics (without "gpu." prefix)
// and their tags.
func collectMetricSamples(t *testing.T, archName string, mode gpuspec.DeviceMode, archSpec gpuspec.ArchitectureSpec) (map[string][]metric, map[string]string) {
	t.Helper()

	collectionSetup := setupMockCheckForMetricCollection(t, archName, mode, archSpec)
	collectionSetup.runCollection()

	return getEmittedGPUMetricsWithTags(collectionSetup.mockSender), collectionSetup.knownTagValues
}

type metricCollectionSetup struct {
	mockSender     *mocksender.MockSender
	runCollection  func()
	knownTagValues map[string]string
}

func setupMockCheckForMetricCollection(t *testing.T, archName string, mode gpuspec.DeviceMode, archSpec gpuspec.ArchitectureSpec) metricCollectionSetup {
	t.Helper()
	opts := gpuspec.BuildMockOptionsForArchAndMode(t, archName, mode, archSpec)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager("gpu", senderManager)
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(opts...))

	wmeta := testutil.GetWorkloadMetaMock(t)
	for idx, uuid := range testutil.GPUUUIDs {
		gpu := newMockWorkloadMetaGPU(uuid, idx, workloadmeta.GPUDeviceTypePhysical, "")
		wmeta.Set(gpu)
		fakeTagger.SetTags(
			taggertypes.NewEntityID(taggertypes.GPU, uuid),
			"spec-test",
			gpuTagsFromWorkloadMetaGPU(gpu),
			nil,
			nil,
			nil,
		)
	}
	if mode == gpuspec.DeviceModeMIG {
		for parentIdx, uuids := range testutil.MIGChildrenUUIDs {
			parentUUID := testutil.GPUUUIDs[parentIdx]
			for migIdx, uuid := range uuids {
				gpu := newMockWorkloadMetaGPU(uuid, migIdx, workloadmeta.GPUDeviceTypeMIG, parentUUID)
				wmeta.Set(gpu)
				fakeTagger.SetTags(
					taggertypes.NewEntityID(taggertypes.GPU, uuid),
					"spec-test",
					gpuTagsFromWorkloadMetaGPU(gpu),
					nil,
					nil,
					nil,
				)
			}
		}
	}

	pidToContainerID := map[int]string{
		1:    "container-1",
		1234: "container-1234",
		5678: "container-5678",
	}
	for _, containerID := range pidToContainerID {
		wmeta.Set(&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   containerID,
			},
			Runtime: workloadmeta.ContainerRuntimeDocker,
		})
		fakeTagger.SetTags(
			taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
			"spec-test",
			nil,
			nil,
			[]string{
				"kube_container_name:name-" + containerID,
			},
			nil,
		)
	}

	checkGeneric := newCheck(fakeTagger, testutil.GetTelemetryMock(t), wmeta)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	WithGPUConfigEnabled(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	mockContainerProvider.EXPECT().GetPidToCid(gomock.Any()).Return(pidToContainerID).AnyTimes()
	check.containerProvider = mockContainerProvider
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	t.Cleanup(func() { checkGeneric.Cancel() })

	// process.core.usage/core.limit come from system-probe/eBPF collector. Provide deterministic
	// cache data for every device shape used by the mode.
	cacheDeviceUUIDs := []string{testutil.DefaultGpuUUID}
	if mode == gpuspec.DeviceModeMIG {
		parentIdx := testutil.DevicesWithMIGChildren[0]
		cacheDeviceUUIDs = append([]string{testutil.GPUUUIDs[parentIdx]}, testutil.MIGChildrenUUIDs[parentIdx]...)
	}
	processMetrics := make([]model.ProcessStatsTuple, 0, len(cacheDeviceUUIDs))
	deviceMetrics := make([]model.DeviceStatsTuple, 0, len(cacheDeviceUUIDs))
	for _, uuid := range cacheDeviceUUIDs {
		processMetrics = append(processMetrics, model.ProcessStatsTuple{
			Key: model.ProcessStatsKey{
				PID:        1234,
				DeviceUUID: uuid,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 42,
				Memory: model.MemoryMetrics{
					CurrentBytes: 100,
					MaxBytes:     200,
				},
				ActiveTimePct: 50,
			},
		})
		deviceMetrics = append(deviceMetrics, model.DeviceStatsTuple{
			DeviceUUID: uuid,
			Metrics: model.DeviceUtilizationMetrics{
				ActiveTimePct: 50,
			},
		})
	}
	spCache := &nvidia.SystemProbeCache{}
	spCache.SetStatsForTest(&model.GPUStats{
		ProcessMetrics: processMetrics,
		DeviceMetrics:  deviceMetrics,
	})
	check.spCache = spCache

	runCollection := func() {
		// Some metrics require a second run to be collected, so we run it twice.
		// XID events are injected between runs, after collectors have registered devices.
		require.NoError(t, checkGeneric.Run())
		if mode == gpuspec.DeviceModePhysical {
			require.NoError(t, check.deviceEvtGatherer.InjectEventsForTest(testutil.DefaultGpuUUID, []ddnvml.DeviceEventData{{
				DeviceUUID: testutil.DefaultGpuUUID,
				EventType:  nvml.EventTypeXidCriticalError,
				EventData:  31,
			}}))
		}
		mockSender.ResetCalls()
		require.NoError(t, checkGeneric.Run())
	}

	// Known values are defined at the same place where the mock behavior/data is configured.
	knownTagValues := map[string]string{
		"gpu_device":         strings.ToLower(strings.ReplaceAll(testutil.DefaultGPUName, " ", "_")),
		"gpu_vendor":         "nvidia",
		"gpu_driver_version": testutil.DefaultNvidiaDriverVersion,
		"type":               "31",
		"origin":             "hardware",
	}

	return metricCollectionSetup{
		mockSender:     mockSender,
		runCollection:  runCollection,
		knownTagValues: knownTagValues,
	}
}

type metric struct {
	name string
	tags []string
}

func getEmittedGPUMetricsWithTags(mockSender *mocksender.MockSender) map[string][]metric {
	metricsByName := make(map[string][]metric)

	for _, call := range mockSender.Mock.Calls {
		if call.Method != "GaugeWithTimestamp" && call.Method != "CountWithTimestamp" {
			continue
		}

		if len(call.Arguments) == 0 {
			continue
		}

		metricName, ok := call.Arguments.Get(0).(string)
		if !ok || !strings.HasPrefix(metricName, "gpu.") {
			continue
		}

		specMetricName := strings.TrimPrefix(metricName, "gpu.")
		tags := []string{}
		if len(call.Arguments) > 3 {
			if callTags, ok := call.Arguments.Get(3).([]string); ok {
				tags = append([]string(nil), callTags...)
			}
		}

		metricsByName[specMetricName] = append(metricsByName[specMetricName], metric{
			name: specMetricName,
			tags: tags,
		})
	}

	return metricsByName
}

func newMockWorkloadMetaGPU(uuid string, index int, deviceType workloadmeta.GPUDeviceType, parentUUID string) *workloadmeta.GPU {
	gpu := &workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   uuid,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: testutil.DefaultGPUName,
		},
		Vendor:             "nvidia",
		Device:             testutil.DefaultGPUName,
		DriverVersion:      testutil.DefaultNvidiaDriverVersion,
		Index:              index,
		DeviceType:         deviceType,
		VirtualizationMode: "none",
	}

	if parentUUID != "" {
		gpu.ParentGPUUUID = parentUUID
	}

	return gpu
}

func gpuTagsFromWorkloadMetaGPU(gpu *workloadmeta.GPU) []string {
	return []string{
		"gpu_uuid:" + gpu.ID,
		"gpu_device:" + strings.ToLower(strings.ReplaceAll(gpu.Device, " ", "_")),
		"gpu_vendor:" + strings.ToLower(gpu.Vendor),
		"gpu_driver_version:" + gpu.DriverVersion,
	}
}

func validateMetricTagsAgainstSpec(t *testing.T, spec *gpuspec.MetricsSpec, metricName string, metricSpec gpuspec.MetricSpec, emittedMetrics []metric, knownTagValues map[string]string) {
	t.Helper()
	require.NotEmpty(t, emittedMetrics, "metric %s has no emitted samples to validate tags", metricName)

	requiredTags := make(map[string]struct{})
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec, ok := spec.Tagsets[tagsetName]
		require.True(t, ok, "metric %s references unknown tagset %s", metricName, tagsetName)
		for _, tag := range tagsetSpec.Tags {
			requiredTags[tag] = struct{}{}
		}
	}
	for _, tag := range metricSpec.CustomTags {
		requiredTags[tag] = struct{}{}
	}

	for _, emittedMetric := range emittedMetrics {
		tagsByKey := tagsToKeyValues(emittedMetric.tags)

		// check that all required tags are present
		for tag := range requiredTags {
			require.Contains(t, tagsByKey, tag, "metric %s missing required tag key %s", metricName, tag)
		}

		// check that no unknown tags are present, and that all known tags have non-empty values. If the tag should have
		// a specific value, check that the value is as expected.
		for key, values := range tagsByKey {
			_, allowed := requiredTags[key]
			require.True(t, allowed, "metric %s has unknown tag key %s", metricName, key)

			for _, value := range values {
				require.NotEmpty(t, value, "metric %s has empty value for tag %s", metricName, key)
				if expectedValue, ok := knownTagValues[key]; ok {
					require.Equal(t, expectedValue, value, "metric %s has unexpected value for tag %s", metricName, key)
				}
			}
		}
	}
}

func tagsToKeyValues(tags []string) map[string][]string {
	result := make(map[string][]string, len(tags))
	for _, tag := range tags {
		key, value, ok := strings.Cut(tag, ":")
		if !ok || key == "" || value == "" {
			continue
		}
		result[key] = append(result[key], value)
	}
	return result
}
