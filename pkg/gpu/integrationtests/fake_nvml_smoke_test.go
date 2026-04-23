// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

// Tier 1a smoke test for the fake NVML shared library
// (pkg/gpu/fake-nvml). Verifies that the built libfake_nvml.so can be dlopen'd
// through go-nvml and that every symbol the agent's safenvml wrapper calls
// behaves consistently with the per-architecture values in
// pkg/collector/corechecks/gpu/spec/architectures.yaml.
//
// The test is skipped unless `FAKE_NVML_LIB_PATH` points at the built .so.
// Build and run locally with:
//
//	bazelisk build //pkg/gpu/fake-nvml:fake_nvml
//	FAKE_NVML_LIB_PATH=$(realpath bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so) \
//	  dda inv test --targets=./pkg/gpu/integrationtests/... \
//	    --build-include=nvml,test --test-run-name=FakeNvml
//
// Note: the env var deliberately does NOT use a DD_ prefix because
// `dda inv test` strips every DD_-prefixed variable from the test environment
// (see sanitize_env_vars in tasks/gotest.py) to prevent real agent config
// from leaking into unit tests.
//
// Architecture selection in the fake library happens at the first NVML call
// and is cached in a OnceLock for the rest of the process. To cover every
// architecture declared in the spec, the top-level test re-executes itself as
// a child process once per architecture, passing the architecture in
// `FAKE_NVML_TEST_ARCH`. The child detects this and routes into
// `runOneArchitecture`. This is the standard Go stdlib pattern for test
// subprocesses (cf. `os/exec`, Go's own runtime tests).

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

const (
	fakeNvmlLibPathEnv     = "FAKE_NVML_LIB_PATH"
	fakeNvmlTestArchEnv    = "FAKE_NVML_TEST_ARCH"
	fakeNvmlArchEnv        = "FAKE_NVML_ARCH"
	fakeNvmlDeviceCountEnv = "FAKE_NVML_DEVICE_COUNT"

	// Use a device count that isn't the fake library's default (2) so a broken
	// env-var plumbing path fails loudly rather than coincidentally matching.
	fakeNvmlTestDeviceCount = 3
)

// TestFakeNvmlArchitectureMatrix is the top-level driver. It reads the
// architecture list from the spec YAML and spawns one child test process per
// architecture. Each child process exercises the full NVML surface against the
// built shared library.
func TestFakeNvmlArchitectureMatrix(t *testing.T) {
	libPath := os.Getenv(fakeNvmlLibPathEnv)
	if libPath == "" {
		t.Skipf("%s not set; build the fake-nvml .so and point this env var at it (see file comment)", fakeNvmlLibPathEnv)
	}
	if _, err := os.Stat(libPath); err != nil {
		t.Fatalf("%s=%q does not exist: %v", fakeNvmlLibPathEnv, libPath, err)
	}

	// Child process branch: run the actual checks against a single arch.
	if arch := os.Getenv(fakeNvmlTestArchEnv); arch != "" {
		runOneArchitecture(t, arch, libPath)
		return
	}

	// Parent branch: spawn one child per architecture.
	archSpec, err := gpuspec.LoadArchitecturesSpec()
	require.NoError(t, err, "load architectures spec")
	require.NotEmpty(t, archSpec.Architectures, "spec should declare at least one architecture")

	// Sort for deterministic test ordering.
	archNames := make([]string, 0, len(archSpec.Architectures))
	for name := range archSpec.Architectures {
		archNames = append(archNames, name)
	}
	sort.Strings(archNames)

	for _, arch := range archNames {
		t.Run(arch, func(t *testing.T) {
			// Re-exec the current test binary with only this one subtest.
			// `-test.run=^TestFakeNvmlArchitectureMatrix$` narrows execution;
			// the child detects the fakeNvmlTestArchEnv var and routes into
			// runOneArchitecture instead of spawning more children.
			cmd := exec.Command(os.Args[0],
				"-test.run=^TestFakeNvmlArchitectureMatrix$",
				"-test.v",
			)
			cmd.Env = append(os.Environ(),
				fakeNvmlTestArchEnv+"="+arch,
				fakeNvmlArchEnv+"="+arch,
				fmt.Sprintf("%s=%d", fakeNvmlDeviceCountEnv, fakeNvmlTestDeviceCount),
			)
			out, err := cmd.CombinedOutput()
			t.Logf("child output for %s:\n%s", arch, out)
			if err != nil {
				t.Fatalf("child process for arch %q failed: %v", arch, err)
			}
		})
	}
}

// runOneArchitecture executes inside a child process. The fake library will
// see `FAKE_NVML_ARCH` in its environment, commit to that architecture at the
// first NVML call, and keep it for the rest of this process. The test then
// verifies that every observable property matches what the spec said.
func runOneArchitecture(t *testing.T, archName, libPath string) {
	t.Helper()

	archSpec, err := gpuspec.LoadArchitecturesSpec()
	require.NoError(t, err, "load architectures spec")
	expected, ok := archSpec.Architectures[archName]
	require.Truef(t, ok, "spec is missing architecture %q (child env var was bogus)", archName)

	lib := nvml.New(nvml.WithLibraryPath(libPath))
	require.NotNil(t, lib, "nvml.New returned nil")

	// Init / Shutdown bracket every NVML session.
	require.Equal(t, nvml.SUCCESS, lib.Init(), "nvmlInit_v2 should succeed")
	t.Cleanup(func() {
		_ = lib.Shutdown()
	})

	// --- Device enumeration ---------------------------------------------

	count, ret := lib.DeviceGetCount()
	require.Equal(t, nvml.SUCCESS, ret, "DeviceGetCount")
	require.Equal(t, fakeNvmlTestDeviceCount, count,
		"FAKE_NVML_DEVICE_COUNT=%d should override default count of 2",
		fakeNvmlTestDeviceCount)

	// --- Per-device identity must match the spec's defaults block -------

	defaults := expected.Defaults
	expectedTotalBytes := defaults.TotalMemoryMib * 1024 * 1024
	seenUUIDs := make(map[string]struct{}, count)

	for i := 0; i < count; i++ {
		dev, ret := lib.DeviceGetHandleByIndex(i)
		require.Equal(t, nvml.SUCCESS, ret, "DeviceGetHandleByIndex(%d)", i)

		name, ret := dev.GetName()
		require.Equal(t, nvml.SUCCESS, ret, "GetName[%d]", i)
		assert.Equal(t, defaults.DeviceName, name, "device name should come from spec defaults")

		uuid, ret := dev.GetUUID()
		require.Equal(t, nvml.SUCCESS, ret, "GetUUID[%d]", i)
		assert.Contains(t, uuid, "GPU-", "UUID should use the NVML-style prefix")
		assert.NotContains(t, seenUUIDs, uuid, "UUIDs must be unique across devices")
		seenUUIDs[uuid] = struct{}{}

		cores, ret := dev.GetNumGpuCores()
		require.Equal(t, nvml.SUCCESS, ret, "GetNumGpuCores[%d]", i)
		assert.Equal(t, int(defaults.NumGpuCores), cores, "num_gpu_cores should come from spec defaults")

		major, minor, ret := dev.GetCudaComputeCapability()
		require.Equal(t, nvml.SUCCESS, ret, "GetCudaComputeCapability[%d]", i)
		assert.Equal(t, int(defaults.CudaComputeMajor), major, "cuda_compute_major")
		assert.Equal(t, int(defaults.CudaComputeMinor), minor, "cuda_compute_minor")

		archVal, ret := dev.GetArchitecture()
		require.Equal(t, nvml.SUCCESS, ret, "GetArchitecture[%d]", i)
		assert.Equal(t, nvml.DeviceArchitecture(defaults.NvmlArchitecture), archVal,
			"nvml_architecture should come from spec defaults")

		mem, ret := dev.GetMemoryInfo()
		require.Equal(t, nvml.SUCCESS, ret, "GetMemoryInfo[%d]", i)
		assert.Equal(t, expectedTotalBytes, mem.Total,
			"total memory should be total_memory_mib * 1 MiB")
		assert.Less(t, mem.Used, mem.Total, "used memory must be strictly less than total")
		assert.Less(t, mem.Free, mem.Total, "free memory must be strictly less than total")

		// Stateless fields that don't come from the spec but must still be
		// plausible — just smoke-check the return codes.
		_, ret = dev.GetTemperature(nvml.TEMPERATURE_GPU)
		assert.Equal(t, nvml.SUCCESS, ret, "GetTemperature[%d]", i)
		_, ret = dev.GetPowerUsage()
		assert.Equal(t, nvml.SUCCESS, ret, "GetPowerUsage[%d]", i)
		_, ret = dev.GetFanSpeed()
		assert.Equal(t, nvml.SUCCESS, ret, "GetFanSpeed[%d]", i)
	}

	// --- GPM behaviour must match capabilities.gpm ----------------------

	assertGpmBehaviour(t, lib, expected.Capabilities.GPM)
}

// assertGpmBehaviour verifies the fake library's GPM path:
//   - on GPM archs (hopper, blackwell): Alloc/Free/SampleGet/MetricsGet all
//     succeed and the 8 metric IDs the agent asks for come back with a value
//     in [0, 1].
//   - on non-GPM archs: nvmlGpmSampleAlloc returns NOT_SUPPORTED, which is
//     exactly the error the agent's GPM collector keys off to exit at
//     instantiation. Other GPM entry points either never get called or return
//     NOT_SUPPORTED; we don't assert on them to keep the test focused on the
//     agent-relevant contract.
func assertGpmBehaviour(t *testing.T, lib nvml.Interface, gpmEnabled bool) {
	t.Helper()

	sample, ret := lib.GpmSampleAlloc()
	if !gpmEnabled {
		assert.Equal(t, nvml.ERROR_NOT_SUPPORTED, ret,
			"nvmlGpmSampleAlloc must return NOT_SUPPORTED on non-GPM archs so the agent collector degrades cleanly")
		return
	}
	require.Equal(t, nvml.SUCCESS, ret, "nvmlGpmSampleAlloc on a GPM arch")
	t.Cleanup(func() { _ = lib.GpmSampleFree(sample) })

	dev, ret := lib.DeviceGetHandleByIndex(0)
	require.Equal(t, nvml.SUCCESS, ret, "DeviceGetHandleByIndex(0) for GPM sample")

	require.Equal(t, nvml.SUCCESS, dev.GpmSampleGet(sample), "nvmlGpmSampleGet")

	// Metric IDs the agent actually asks for, from
	// pkg/collector/corechecks/gpu/nvidia/gpm.go. We don't import that package
	// here to keep this integration test self-contained and free of the
	// test-only dependency fan-out.
	metricIDs := []nvml.GpmMetricId{
		nvml.GPM_METRIC_GRAPHICS_UTIL,
		nvml.GPM_METRIC_SM_UTIL,
		nvml.GPM_METRIC_SM_OCCUPANCY,
		nvml.GPM_METRIC_INTEGER_UTIL,
		nvml.GPM_METRIC_ANY_TENSOR_UTIL,
		nvml.GPM_METRIC_FP64_UTIL,
		nvml.GPM_METRIC_FP32_UTIL,
		nvml.GPM_METRIC_FP16_UTIL,
	}

	mg := &nvml.GpmMetricsGetType{
		Version:    nvml.GPM_METRICS_GET_VERSION,
		NumMetrics: uint32(len(metricIDs)),
		Sample1:    sample,
		Sample2:    sample,
	}
	for i, id := range metricIDs {
		mg.Metrics[i].MetricId = uint32(id)
	}

	ret = lib.GpmMetricsGet(mg)
	require.Equal(t, nvml.SUCCESS, ret, "nvmlGpmMetricsGet on a GPM arch")

	for i, id := range metricIDs {
		m := mg.Metrics[i]
		assert.Equalf(t, uint32(nvml.SUCCESS), m.NvmlReturn,
			"agent-queried metric id %d (%s) must be supported", id, gpmMetricName(id))
		assert.GreaterOrEqualf(t, m.Value, 0.0, "metric id %d should be non-negative", id)
		assert.LessOrEqualf(t, m.Value, 1.0, "metric id %d should be at most 1.0 (normalized util)", id)
	}

	// Unknown metric IDs must come back as NOT_SUPPORTED at the per-metric
	// level, not as a top-level failure — that's how real NVML behaves and
	// what the agent's removeUnsupportedMetrics() path keys off.
	mg2 := &nvml.GpmMetricsGetType{
		Version:    nvml.GPM_METRICS_GET_VERSION,
		NumMetrics: 1,
		Sample1:    sample,
		Sample2:    sample,
	}
	mg2.Metrics[0].MetricId = 0xffffffff
	ret = lib.GpmMetricsGet(mg2)
	require.Equal(t, nvml.SUCCESS, ret, "nvmlGpmMetricsGet should succeed overall even with unknown ids")
	assert.Equal(t, uint32(nvml.ERROR_NOT_SUPPORTED), mg2.Metrics[0].NvmlReturn,
		"unknown metric IDs must be per-metric NOT_SUPPORTED")
}

// gpmMetricName is a best-effort label for test output. If go-nvml ever renames
// or reshuffles the metric ID enum, we'd just print the numeric ID.
func gpmMetricName(id nvml.GpmMetricId) string {
	name := strings.TrimPrefix(fmt.Sprintf("%v", id), "GPM_METRIC_")
	if name == "" {
		return fmt.Sprintf("id=%d", id)
	}
	return name
}
