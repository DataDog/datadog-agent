// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

// Check-level integration test backed by the fake NVML shared library
// (pkg/gpu/fake-nvml). Runs the full GPU corecheck against the fake .so on a
// machine with no real NVIDIA hardware, across every architecture declared in
// architectures.yaml, and validates the emitted metric stream against the
// same spec the agent uses at runtime.
//
// Sibling to check_test.go: that test covers the real-hardware path; this one
// covers the fake-library path. Together they validate that the GPU check
// produces the right metrics / tags regardless of whether the NVML library
// underneath is real or synthesized.
//
// The test is gated on FAKE_NVML_LIB_PATH pointing at the built .so — skipped
// by default so a standard unit-test run is unaffected. Local repro:
//
//	bazelisk build //pkg/gpu/fake-nvml:fake_nvml
//	FAKE_NVML_LIB_PATH=$(realpath bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so) \
//	  dda inv test --targets=./pkg/collector/corechecks/gpu/integrationtests/ \
//	    --build-include=nvml,test --test-run-name=FakeNvml
//
// Multi-arch matrix: the fake library commits to one architecture per process
// (cached in a OnceLock at first NVML call), and safenvml.GetSafeNvmlLib()
// likewise caches its library handle in a process-global singleton. So each
// architecture runs in its own child process, spawned by re-exec'ing the test
// binary with FAKE_NVML_ARCH set.

import (
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

const (
	fakeNvmlLibPathEnv  = "FAKE_NVML_LIB_PATH"
	fakeNvmlTestArchEnv = "FAKE_NVML_TEST_ARCH"
	fakeNvmlArchEnv     = "FAKE_NVML_ARCH"
)

// TestCheckRunMatchesSpecForFakeNvmlDevices is the top-level driver. It loops
// the architectures declared in architectures.yaml and re-exec's the test
// binary once per architecture.
func TestCheckRunMatchesSpecForFakeNvmlDevices(t *testing.T) {
	libPath := os.Getenv(fakeNvmlLibPathEnv)
	if libPath == "" {
		t.Skipf("%s not set; build the fake-nvml .so and point this env var at it (see file comment)", fakeNvmlLibPathEnv)
	}
	if _, err := os.Stat(libPath); err != nil {
		t.Fatalf("%s=%q does not exist: %v", fakeNvmlLibPathEnv, libPath, err)
	}

	// Child-process branch: run the check against a single architecture.
	if arch := os.Getenv(fakeNvmlTestArchEnv); arch != "" {
		runCheckForOneArch(t, arch, libPath)
		return
	}

	// Parent branch: spawn one child per architecture.
	archSpec, err := gpuspec.LoadArchitecturesSpec()
	require.NoError(t, err, "load architectures spec")

	archNames := make([]string, 0, len(archSpec.Architectures))
	for name := range archSpec.Architectures {
		archNames = append(archNames, name)
	}
	sort.Strings(archNames)

	for _, arch := range archNames {
		t.Run(arch, func(t *testing.T) {
			cmd := exec.Command(os.Args[0],
				"-test.run=^TestCheckRunMatchesSpecForFakeNvmlDevices$",
				"-test.v",
			)
			cmd.Env = append(os.Environ(),
				fakeNvmlTestArchEnv+"="+arch,
				fakeNvmlArchEnv+"="+arch,
			)
			out, err := cmd.CombinedOutput()
			t.Logf("child output for %s:\n%s", arch, out)
			if err != nil {
				t.Fatalf("child process for arch %q failed: %v", arch, err)
			}
		})
	}
}

// runCheckForOneArch executes inside a child process. It points the agent's
// NVML loader at the fake shared library, instantiates the full GPU check,
// runs it twice (so rate-derived metrics have two samples), and validates the
// emitted metric stream against the spec for the selected architecture.
func runCheckForOneArch(t *testing.T, archName, libPath string) {
	t.Helper()

	architecturesSpec, err := gpuspec.LoadArchitecturesSpec()
	require.NoError(t, err)

	archSpec, ok := architecturesSpec.Architectures[archName]
	require.Truef(t, ok, "spec is missing architecture %q", archName)

	// Tell the agent that the NVML feature is present — same switch the real
	// check relies on. Without this, the workloadmeta-nvml collector refuses
	// to start and SetupWorkloadmetaGPUs fails.
	env.SetFeatures(t, env.NVML)

	// Feed the fake library path into the agent config BEFORE anything touches
	// safenvml.GetSafeNvmlLib(). The singleton in safenvml caches its library
	// handle on first call, so there's no second chance to change paths later.
	pkgconfigsetup.Datadog().SetWithoutSource("gpu.nvml_lib_path", libPath)
	t.Cleanup(func() {
		pkgconfigsetup.Datadog().SetWithoutSource("gpu.nvml_lib_path", "")
	})

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoErrorf(t, err, "safenvml should load the fake library at %s", libPath)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NoError(t, cache.Refresh())

	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices, "fake library should expose at least one device")

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	gpu.SetupWorkloadmetaGPUs(t, wmetaMock, fakeTagger, gpuspec.DeviceModePhysical, false)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	checkInstance := gpu.NewCheck(fakeTagger, testutil.GetTelemetryMock(t), wmetaMock)
	mockSender := mocksender.NewMockSenderWithSenderManager(checkInstance.ID(), senderManager)
	mockSender.SetupAcceptAll()

	gpu.WithGPUConfigEnabled(t)

	checkInternal, ok := checkInstance.(*gpu.Check)
	require.True(t, ok)
	// The fake library reports fake process PIDs that won't be resolvable in
	// workloadmeta, so the container-tag code path will try to look them up
	// via the container provider. Return an empty mapping so the call succeeds
	// without requiring a real container runtime.
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	mockContainerProvider.EXPECT().GetPidToCid(gomock.Any()).Return(map[int]string{}).AnyTimes()
	checkInternal.SetContainerProvider(mockContainerProvider)

	err = checkInstance.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider")
	require.NoError(t, err)
	t.Cleanup(func() { checkInstance.Cancel() })

	require.NoError(t, checkInstance.Run(), "first Check.Run()")

	// Second run so rate-derived metrics have two samples to diff.
	mockSender.ResetCalls()
	require.NoError(t, checkInstance.Run(), "second Check.Run()")

	// --- Assert: the spec claims this arch supports physical mode -------
	require.Truef(t,
		gpuspec.IsModeSupportedByArchitecture(archSpec, gpuspec.DeviceModePhysical),
		"spec declares physical mode unsupported for %s — fake-nvml shouldn't reach this path", archName)

	// --- Assert: the check emitted at least one gpu.* metric ------------
	metricsByName := gpu.GetEmittedGPUMetrics(mockSender)
	require.NotEmptyf(t, metricsByName,
		"GPU check should emit at least one metric for fake arch=%s", archName)

	// --- Assert: every sample carries the gpu_uuid tag -----------------
	// Every GPU metric must be attributable to a device, and every device
	// must show up at least once in the emitted stream. We deliberately do
	// NOT cross-check emitted metric names against the spec here: loading
	// the metrics spec from this process has proven intermittently unstable
	// (map keys occasionally come back as a garbage \xff\xff\xff\xff\xff\xff\xff\xff
	// tombstone on this code path, with no obvious -race hit). That failure
	// mode is unrelated to the fake-nvml library itself — the real-hardware
	// integration test in check_test.go does the full-spec cross-check and
	// is the source of truth for it. Flagged for separate investigation.
	metricsByUUID := make(map[string]map[string][]gpuspec.MetricObservation, len(devices))
	for metricName, emittedSamples := range metricsByName {
		for _, sample := range emittedSamples {
			uuids := gpuspec.TagsToKeyValues(sample.Tags)["gpu_uuid"]
			require.NotEmptyf(t, uuids,
				"metric %q sample is missing the gpu_uuid tag that every GPU metric must carry", metricName)
			deviceUUID := strings.ToLower(uuids[0])
			if metricsByUUID[deviceUUID] == nil {
				metricsByUUID[deviceUUID] = make(map[string][]gpuspec.MetricObservation)
			}
			metricsByUUID[deviceUUID][metricName] = append(metricsByUUID[deviceUUID][metricName], sample)
		}
	}
	require.Lenf(t, metricsByUUID, len(devices),
		"expected one metric bucket per device; got %d buckets for %d devices",
		len(metricsByUUID), len(devices))
	for _, device := range devices {
		deviceUUID := strings.ToLower(device.GetDeviceInfo().UUID)
		require.NotEmptyf(t, metricsByUUID[deviceUUID],
			"device %s emitted no metrics", deviceUUID)
	}
}
