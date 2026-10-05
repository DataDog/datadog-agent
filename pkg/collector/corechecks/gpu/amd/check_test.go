// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package amd

import (
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	amdgpu "github.com/DataDog/datadog-agent/pkg/gpu/amd"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

// newMockContainerProvider returns a container provider mapping PIDs to the
// given containers.
func newMockContainerProvider(t *testing.T, pidToContainerID map[int]string) *mock_containers.MockContainerProvider {
	t.Helper()

	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	if pidToContainerID != nil {
		mockContainerProvider.EXPECT().GetPidToCid(gomock.Any()).Return(pidToContainerID).AnyTimes()
	}
	return mockContainerProvider
}

const testAMDUUID = "amd-00c0ffee00c0ffee"

// fakeAMDHost returns a sysfs root with one MI300X-like GPU.
func fakeAMDHost(t *testing.T) string {
	fs := amdgpu.NewFakeSysfs(t)
	fs.SetDriverVersion("6.14.14")
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee"))
	fs.AddHwmon(devDir, "hwmon0", amdgpu.JunctionOnlyHwmon())
	fs.AddCard("card0", devDir)
	return fs.Root
}

// setupAMDCheck configures an AMD GPU check whose discovery reads sysRoot.
// The settings are applied after the component mocks are created, as creating
// them resets the global configuration.
func setupAMDCheck(t *testing.T, sysRoot string, settings map[string]any) (*Check, *mocksender.MockSender) {
	t.Helper()
	return setupAMDCheckWithTagger(t, taggerfxmock.SetupFakeTagger(t), sysRoot, settings, map[int]string{})
}

// setupAMDCheckWithTagger is setupAMDCheck with a caller-provided tagger and a
// process-to-container mapping for workload tags.
func setupAMDCheckWithTagger(t *testing.T, fakeTagger tagger.Component, sysRoot string, settings map[string]any, pidToContainerID map[int]string) (*Check, *mocksender.MockSender) {
	t.Helper()
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	checkGeneric := newCheck(fakeTagger, testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t))
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	applyAMDTestSettings(t, settings)
	check.containerProvider = newMockContainerProvider(t, pidToContainerID)
	check.sysRoot = sysRoot
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))
	t.Cleanup(func() { check.Cancel() })

	mockSender := mocksender.NewMockSenderWithSenderManager(check.ID(), senderManager)
	mockSender.SetupAcceptAll()
	return check, mockSender
}

// applyAMDTestSettings enables GPU monitoring and AMD collection, unless a
// test overrides it, and applies the settings for the duration of the test.
func applyAMDTestSettings(t *testing.T, settings map[string]any) {
	t.Helper()
	gpu.WithGPUConfigEnabled(t)
	if _, overridden := settings["gpu.amd.enabled"]; !overridden {
		settings = maps.Clone(settings)
		if settings == nil {
			settings = map[string]any{}
		}
		settings["gpu.amd.enabled"] = true
	}
	for key, value := range settings {
		previous := pkgconfigsetup.Datadog().Get(key)
		t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest(key, previous) })
		pkgconfigsetup.Datadog().SetInTest(key, value)
	}
}

// emittedGauges returns the gauges sent by the check, keyed by metric name.
func emittedGauges(mockSender *mocksender.MockSender) map[string][]mock.Call {
	gauges := make(map[string][]mock.Call)
	for _, call := range mockSender.Mock.Calls {
		if call.Method == "GaugeWithTimestamp" {
			name := call.Arguments.String(0)
			gauges[name] = append(gauges[name], call)
		}
	}
	return gauges
}

func TestAMDOnlyHostEmitsMetrics(t *testing.T) {
	check, mockSender := setupAMDCheck(t, fakeAMDHost(t), nil)

	require.NoError(t, check.Run())

	gauges := emittedGauges(mockSender)
	expected := map[string]float64{
		"gpu.gr_engine_active":       37,
		"gpu.sm_active":              37,
		"gpu.dram_active":            12,
		"gpu.memory.limit":           206141652992,
		"gpu.memory.free":            206141652992 - 294965248,
		"gpu.memory.utilization":     float64(294965248) / 206141652992,
		"gpu.temperature":            41,
		"gpu.memory.temperature":     35,
		"gpu.power.usage":            142000,
		"gpu.power.management_limit": 750000,
		"gpu.clock.speed.graphics":   1420,
		"gpu.clock.speed.memory":     1300,
		"gpu.pci.link.width.current": 16,
		"gpu.device.total":           1,
	}
	for name, value := range expected {
		require.Len(t, gauges[name], 1, name)
		assert.InDelta(t, value, gauges[name][0].Arguments.Get(1), 1e-6, name)
		assert.ElementsMatch(t, []string{
			"gpu_vendor:amd",
			"gpu_uuid:" + testAMDUUID,
			"gpu_device:amd_instinct_mi300x",
			"gpu_type:mi300x",
			"gpu_pci_bus_id:0000:c1:00.0",
			"gpu_slicing_mode:none",
			"gpu_mig_profile:none",
			"gpu_nvlink_capable:false",
			"gpu_parent_uuid:" + testAMDUUID,
			"gpu_driver_version:6.14.14",
		}, gauges[name][0].Arguments.Get(3), name)
	}
	// 32 GT/s x16 with 128b/130b encoding.
	require.Len(t, gauges["gpu.pci.link.speed.current"], 1)
	assert.InDelta(t, 32e9*128/130/8*16, gauges["gpu.pci.link.speed.current"][0].Arguments.Get(1), 1)
}

func TestAMDDeviceExcludedByConfig(t *testing.T) {
	check, mockSender := setupAMDCheck(t, fakeAMDHost(t), map[string]any{"gpu.excluded_devices": []string{"AMD-00C0FFEE00C0FFEE"}})

	require.NoError(t, check.Run(), "excluding every AMD GPU must not require an NVIDIA driver")
	assert.Empty(t, emittedGauges(mockSender))
}

func TestAMDSamplesOmitUnavailableValues(t *testing.T) {
	samples := amdSamples(amdgpu.Metrics{
		EdgeTemperatureC:     amdgpu.Reading{Value: 50, Valid: true},
		JunctionTemperatureC: amdgpu.Reading{Value: 70, Valid: true},
		VRAMTotalBytes:       amdgpu.Reading{Value: 100, Valid: true},
		VRAMUsedBytes:        amdgpu.Reading{Value: 150, Valid: true}, // inconsistent snapshot
		PCIeLinkSpeedGTs:     amdgpu.Reading{Value: 7, Valid: true},   // not a PCIe rate
		PCIeLinkWidth:        amdgpu.Reading{Value: 16, Valid: true},
	})

	values := map[string]float64{}
	for _, s := range samples {
		m := s.(*nvidia.Metric)
		values[m.Name] = m.Value
	}
	assert.Equal(t, map[string]float64{
		"device.total":           1,
		"temperature":            50, // edge sensor preferred over junction
		"memory.limit":           100,
		"pci.link.width.current": 16,
	}, values)
}

func TestAMDDeviceTagsMatchTagSpec(t *testing.T) {
	tagsSpec, err := gpuspec.LoadTagsSpec()
	require.NoError(t, err)

	fs := amdgpu.NewFakeSysfs(t)
	fs.SetDriverVersion("6.14.14")
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0001:0a:00.1", "amdgpu", amdgpu.MI300XAttributes(""))) // UUID from a nonzero PCI function
	devices, err := amdgpu.Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)

	for _, dev := range devices {
		for _, tag := range amdDeviceTags(dev) {
			name, value, found := strings.Cut(tag, ":")
			require.True(t, found, tag)
			spec, ok := tagsSpec.Tags[name]
			require.True(t, ok, "tag %s is not in the spec", name)
			if spec.Regex != nil {
				assert.Regexp(t, spec.Regex, value, "tag %s", name)
			}
		}
	}
}

func TestAMDRediscoveryDropsRemovedDevices(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:11:00.0", "amdgpu", amdgpu.MI300XAttributes("1111")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:21:00.0", "amdgpu", amdgpu.MI300XAttributes("2222")))
	check, mockSender := setupAMDCheck(t, fs.Root, map[string]any{"gpu.static_metrics_reporting_interval": 0})
	require.NoError(t, check.Run())

	require.NoError(t, os.Remove(filepath.Join(fs.Root, "class", "drm", "card0", "device")))
	mockSender.ResetCalls()
	require.NoError(t, check.Run())
	gauges := emittedGauges(mockSender)
	require.Len(t, gauges["gpu.device.total"], 1)
	assert.Contains(t, gauges["gpu.device.total"][0].Arguments.Get(3), "gpu_uuid:amd-2222")
	assert.NotContains(t, check.deviceTags, "amd-1111")

	require.NoError(t, os.Remove(filepath.Join(fs.Root, "class", "drm", "card1", "device")))
	mockSender.ResetCalls()
	require.NoError(t, check.Run(), "a host without AMD GPUs is not an AMD GPU check error")
	assert.Empty(t, emittedGauges(mockSender))
}

func TestAMDStaticMetricsFollowReportingInterval(t *testing.T) {
	check, mockSender := setupAMDCheck(t, fakeAMDHost(t), map[string]any{"gpu.static_metrics_reporting_interval": "15s"})
	check.refreshDevices()
	start := time.Unix(1000, 0)

	for _, elapsed := range []time.Duration{0, 5 * time.Second, 30 * time.Second} {
		require.NoError(t, check.emitMetrics(mockSender, start.Add(elapsed)))
	}
	gauges := emittedGauges(mockSender)
	for _, name := range []string{"gpu.device.total", "gpu.memory.limit"} {
		var timestamps []float64
		for _, call := range gauges[name] {
			timestamps = append(timestamps, call.Arguments.Get(4).(float64))
		}
		assert.Equal(t, []float64{1000, 1015, 1030}, timestamps, name)
	}
	var timestamps []float64
	for _, call := range gauges["gpu.gr_engine_active"] {
		timestamps = append(timestamps, call.Arguments.Get(4).(float64))
	}
	assert.Equal(t, []float64{1000, 1005, 1030}, timestamps)
}

func TestAMDDeviceTagsComeFromTagger(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	entityTags := []string{"gpu_vendor:amd", "gpu_uuid:" + testAMDUUID, "gpu_architecture:gfx942", "team:ml"}
	check, mockSender := setupAMDCheckWithTagger(t, fakeTagger, fakeAMDHost(t), nil, map[int]string{})

	// Before workloadmeta reaches the tagger, discovery provides device tags.
	require.NoError(t, check.Run())
	gauges := emittedGauges(mockSender)["gpu.gr_engine_active"]
	require.Len(t, gauges, 1)
	assert.Contains(t, gauges[0].Arguments.Get(3), "gpu_device:amd_instinct_mi300x")
	assert.NotContains(t, gauges[0].Arguments.Get(3), "team:ml")

	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testAMDUUID), "amdgpu", entityTags, nil, nil, nil)
	mockSender.ResetCalls()

	require.NoError(t, check.Run())

	gauges = emittedGauges(mockSender)["gpu.gr_engine_active"]
	require.Len(t, gauges, 1)
	assert.ElementsMatch(t, entityTags, gauges[0].Arguments.Get(3))
}

func TestAMDProcessMemoryCarriesWorkloadTags(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee"))
	fs.AddCard("card0", devDir)
	fs.AddKFDNode(1, 4101, 0, 0xc100, 90402)
	pid := os.Getpid() // a live process, so that its PID namespace can be resolved
	fs.AddKFDProcess(pid, 4101, 1<<30)

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, "ctr-amd"), "fake", []string{"container_id:ctr-amd"}, nil, nil, nil)
	check, mockSender := setupAMDCheckWithTagger(t, fakeTagger, fs.Root, nil, map[int]string{pid: "ctr-amd"})
	wmetaMock, ok := check.wmeta.(workloadmetamock.Mock)
	require.True(t, ok)
	wmetaMock.Set(&workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "ctr-amd"}})

	require.NoError(t, check.Run())
	gauges := emittedGauges(mockSender)

	usage := gauges["gpu.process.memory.usage"]
	require.Len(t, usage, 1)
	assert.InDelta(t, float64(1<<30), usage[0].Arguments.Get(1), 0)
	usageTags := usage[0].Arguments.Get(3).([]string)
	assert.Contains(t, usageTags, "pid:"+strconv.Itoa(pid))
	assert.Contains(t, usageTags, "container_id:ctr-amd")
	assert.Contains(t, usageTags, "gpu_uuid:"+testAMDUUID)

	// The device limit carries the same workload tags, and is sent once.
	limit := gauges["gpu.memory.limit"]
	require.Len(t, limit, 1)
	assert.Contains(t, limit[0].Arguments.Get(3).([]string), "container_id:ctr-amd")

	// Device metrics that are not per process are not attributed.
	assert.NotContains(t, gauges["gpu.gr_engine_active"][0].Arguments.Get(3).([]string), "container_id:ctr-amd")
}

func TestAMDPartialKFDTopologyDoesNotEmitUndercountedProcesses(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:d1:00.0", "amdgpu", amdgpu.MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0xc100, 90402)
	fs.AddKFDNode(2, 4102, 0, 0xc101, 90402)
	fs.AddKFDNode(3, 5100, 0, 0xd100, 90402)
	pid := os.Getpid()
	fs.AddKFDProcess(pid, 4101, 10)
	fs.AddKFDProcess(pid, 4102, 20)
	fs.AddKFDProcess(pid, 5100, 50)
	check, mockSender := setupAMDCheck(t, fs.Root, nil)
	assertDeviceMetrics := func() {
		t.Helper()
		activity := emittedGauges(mockSender)["gpu.gr_engine_active"]
		require.Len(t, activity, 2)
		for _, call := range activity {
			assert.Equal(t, float64(37), call.Arguments.Get(1))
			assert.Contains(t, call.Arguments.Get(3), "gpu_architecture:gfx942")
		}
	}
	assertProcessMetrics := func() {
		t.Helper()
		actual := make(map[string]float64)
		for _, call := range emittedGauges(mockSender)["gpu.process.memory.usage"] {
			for _, tag := range call.Arguments.Get(3).([]string) {
				if strings.HasPrefix(tag, "gpu_uuid:") {
					actual[tag] = call.Arguments.Get(1).(float64)
				}
			}
		}
		assert.Equal(t, map[string]float64{
			"gpu_uuid:" + testAMDUUID:   30,
			"gpu_uuid:amd-0000-d1-00-0": 50,
		}, actual)
	}
	require.NoError(t, check.Run())
	assertDeviceMetrics()
	assertProcessMetrics()

	fs.DenyKFDNode(2)
	mockSender.ResetCalls()
	require.NoError(t, check.Run())
	assertDeviceMetrics()
	assert.NotContains(t, emittedGauges(mockSender), "gpu.process.memory.usage")

	for _, name := range []string{"gpu_id", "properties"} {
		require.NoError(t, os.Chmod(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/2", name), 0o644))
	}
	mockSender.ResetCalls()
	require.NoError(t, check.Run())
	assertDeviceMetrics()
	assertProcessMetrics()
}

// Like NVIDIA's, the workload-attributed memory.limit follows the static
// reporting cadence: process memory is reported on every run, and the limit
// at the next static reporting point, carrying the workload tags.
func TestAMDProcessLimitsFollowStaticCadence(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddKFDNode(1, 4101, 0, 0xc100, 90402)
	check, mockSender := setupAMDCheck(t, fs.Root, map[string]any{"gpu.static_metrics_reporting_interval": "15s"})
	check.refreshDevices()
	start := time.Unix(1000, 0)
	require.NoError(t, check.emitMetrics(mockSender, start))

	pid := os.Getpid()
	fs.AddKFDProcess(pid, 4101, 42)
	timestamps := func(name string) []float64 {
		var ts []float64
		for _, call := range emittedGauges(mockSender)[name] {
			ts = append(ts, call.Arguments.Get(4).(float64))
			assert.Contains(t, call.Arguments.Get(3), "pid:"+strconv.Itoa(pid), name)
		}
		return ts
	}

	mockSender.ResetCalls()
	for _, elapsed := range []time.Duration{5 * time.Second, 10 * time.Second} {
		require.NoError(t, check.emitMetrics(mockSender, start.Add(elapsed)))
	}
	assert.Equal(t, []float64{1005, 1010}, timestamps("gpu.process.memory.usage"))
	assert.Empty(t, timestamps("gpu.memory.limit"), "early static samples are dropped")

	mockSender.ResetCalls()
	require.NoError(t, check.emitMetrics(mockSender, start.Add(15*time.Second)))
	assert.Equal(t, []float64{1015}, timestamps("gpu.memory.limit"))
}

func TestAMDProcessReadErrorsPreserveAvailableMetrics(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddKFDNode(1, 4101, 0, 0xc100, 90402)
	pid := os.Getpid()
	fs.AddKFDProcess(pid, 4101, 42)
	// The synthetic PID is never resolved because its VRAM cannot be read.
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc", strconv.Itoa(pid+1)), map[string]string{"vram_4101": "invalid\n"})
	check, mockSender := setupAMDCheck(t, fs.Root, nil)
	check.refreshDevices()

	require.ErrorContains(t, check.emitMetrics(mockSender, time.Unix(1000, 0)), "vram_4101")
	gauges := emittedGauges(mockSender)
	require.Len(t, gauges["gpu.process.memory.usage"], 1)
	assert.Equal(t, float64(42), gauges["gpu.process.memory.usage"][0].Arguments.Get(1))
	assert.Contains(t, gauges["gpu.process.memory.usage"][0].Arguments.Get(3), "pid:"+strconv.Itoa(pid))
	require.Len(t, gauges["gpu.memory.limit"], 1)
	assert.Equal(t, float64(206141652992), gauges["gpu.memory.limit"][0].Arguments.Get(1))
	require.Len(t, gauges["gpu.gr_engine_active"], 1)
	assert.Equal(t, float64(37), gauges["gpu.gr_engine_active"][0].Arguments.Get(1))
}

// A telemetry read blocked in the driver must not hold the check: the other
// devices and the blocked device's process memory are still reported, later
// runs skip the device instead of waiting again, and it recovers once the read
// returns.
func TestAMDHungTelemetryReadDoesNotBlockCheck(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	hungDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee"))
	fs.AddCard("card0", hungDir)
	fs.AddCard("card1", fs.AddPCIDevice("0000:d1:00.0", "amdgpu", amdgpu.MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0xc100, 90402)
	fs.AddKFDProcess(os.Getpid(), 4101, 42)
	// open() of a FIFO without a writer blocks in the kernel, like a sysfs read on a hung GPU.
	busy := filepath.Join(hungDir, "gpu_busy_percent")
	require.NoError(t, os.Remove(busy))
	require.NoError(t, syscall.Mkfifo(busy, 0o600))
	t.Cleanup(func() { // release the reader if the test fails before doing so
		if w, err := os.OpenFile(busy, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
	})

	check, mockSender := setupAMDCheck(t, fs.Root, nil)
	check.readTimeout = 500 * time.Millisecond
	check.refreshDevices()
	gaugesByUUID := func(name string) map[string]float64 {
		values := map[string]float64{}
		for _, call := range emittedGauges(mockSender)[name] {
			for _, tag := range call.Arguments.Get(3).([]string) {
				if uuid, ok := strings.CutPrefix(tag, "gpu_uuid:"); ok {
					values[uuid] = call.Arguments.Get(1).(float64)
				}
			}
		}
		return values
	}
	const healthyUUID = "amd-0000-d1-00-0"

	start := time.Now()
	require.ErrorContains(t, check.emitMetrics(mockSender, time.Unix(1000, 0)), "did not return within 500ms")
	assert.Less(t, time.Since(start), 3*time.Second)
	assert.Equal(t, map[string]float64{healthyUUID: 37}, gaugesByUUID("gpu.gr_engine_active"))
	assert.Equal(t, map[string]float64{testAMDUUID: 42}, gaugesByUUID("gpu.process.memory.usage"))
	// The limit of the blocked device's process memory comes from discovery.
	assert.Equal(t, float64(206141652992), gaugesByUUID("gpu.memory.limit")[testAMDUUID])

	mockSender.ResetCalls()
	start = time.Now()
	require.ErrorContains(t, check.emitMetrics(mockSender, time.Unix(1015, 0)), "still blocked from a previous run")
	assert.Less(t, time.Since(start), check.readTimeout, "the blocked device is skipped, not waited for again")
	assert.Equal(t, map[string]float64{healthyUUID: 37}, gaugesByUUID("gpu.gr_engine_active"))

	// Unblock the read: the pending reader gets 37, later reads the new file.
	pending := check.pendingReads[testAMDUUID]
	require.NotNil(t, pending)
	writer, err := os.OpenFile(busy, os.O_WRONLY, 0) // does not block: the reader is waiting
	require.NoError(t, err)
	require.NoError(t, os.Remove(busy))
	require.NoError(t, os.WriteFile(busy, []byte("55\n"), 0o644))
	_, err = writer.WriteString("37\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	select {
	case <-pending:
	case <-time.After(5 * time.Second):
		t.Fatal("the blocked read did not return")
	}

	mockSender.ResetCalls()
	require.NoError(t, check.emitMetrics(mockSender, time.Unix(1030, 0)))
	assert.Equal(t, map[string]float64{testAMDUUID: 55, healthyUUID: 37}, gaugesByUUID("gpu.gr_engine_active"))
	assert.Empty(t, check.pendingReads)
}

func TestAMDKubernetesAllocationsTagDeviceMetrics(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:d1:00.0", "amdgpu", amdgpu.MI300XAttributes("")))
	// 0000:d1:00.0 is split in two compute partitions (render nodes 129 and 130).
	fs.AddKFDNode(1, 4101, 0, 0xc100, 90402)
	fs.AddKFDNode(2, 4102, 0, 0xd100, 90402)
	fs.SetKFDRenderMinor(2, 129)
	fs.AddKFDNode(3, 4103, 0, 0xd101, 90402)
	fs.SetKFDRenderMinor(3, 130)
	fs.AddPartitionRenderNode("amdgpu_xcp_1", 129)
	fs.AddPartitionRenderNode("amdgpu_xcp_2", 130)

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	check, mockSender := setupAMDCheckWithTagger(t, fakeTagger, fs.Root, nil, map[int]string{})
	wmetaMock, ok := check.wmeta.(workloadmetamock.Mock)
	require.True(t, ok)

	addPod := func(containerID string, resources ...workloadmeta.ContainerAllocatedResource) {
		wmetaMock.Set(&workloadmeta.Container{
			EntityID:                   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
			EntityMeta:                 workloadmeta.EntityMeta{Name: containerID},
			ResolvedAllocatedResources: resources,
		})
		fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, containerID), "fake", []string{"container_id:" + containerID}, nil, nil, nil)
	}
	// Whole GPU with the default "single" resource naming strategy.
	addPod("whole", workloadmeta.ContainerAllocatedResource{Name: "amd.com/gpu", ID: "0000:c1:00.0"})
	// One partition each, with the "mixed" strategy resource name.
	addPod("part1", workloadmeta.ContainerAllocatedResource{Name: "amd.com/cpx_nps4", ID: "amdgpu_xcp_1"})
	addPod("part2", workloadmeta.ContainerAllocatedResource{Name: "amd.com/cpx_nps4", ID: "amdgpu_xcp_2"})
	// NVIDIA and unknown AMD allocations are not attributed to AMD GPUs.
	addPod("nvidia", workloadmeta.ContainerAllocatedResource{Name: "nvidia.com/gpu", ID: "GPU-00000000-1234-1234-1234-123456789012"})
	addPod("stale", workloadmeta.ContainerAllocatedResource{Name: "amd.com/gpu", ID: "0000:e1:00.0"})

	require.NoError(t, check.Run())

	containersByUUID := map[string][]string{}
	for _, call := range emittedGauges(mockSender)["gpu.gr_engine_active"] {
		var uuid string
		var ctrs []string
		for _, tag := range call.Arguments.Get(3).([]string) {
			if v, ok := strings.CutPrefix(tag, "gpu_uuid:"); ok {
				uuid = v
			}
			if v, ok := strings.CutPrefix(tag, "container_id:"); ok {
				ctrs = append(ctrs, v)
			}
		}
		containersByUUID[uuid] = ctrs
	}
	require.Len(t, containersByUUID, 2)
	assert.ElementsMatch(t, []string{"whole"}, containersByUUID[testAMDUUID])
	assert.ElementsMatch(t, []string{"part1", "part2"}, containersByUUID["amd-0000-d1-00-0"])
}

func TestAMDCheckDisabledByConfig(t *testing.T) {
	for name, settings := range map[string]map[string]any{
		"gpu disabled": {"gpu.enabled": false},
		"amd disabled": {"gpu.amd.enabled": false},
	} {
		t.Run(name, func(t *testing.T) {
			check, ok := newCheck(taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t)).(*Check)
			require.True(t, ok)
			applyAMDTestSettings(t, settings)
			require.Error(t, check.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))
		})
	}
}
