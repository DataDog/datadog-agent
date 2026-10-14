// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/gpuenvironment"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/containers"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysprobetestutil "github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

func TestSysprobeDataTracker(t *testing.T) {
	tracker := newSysprobeDataTracker()
	start := time.Now()
	errUnreachable := &net.OpError{Op: "dial", Net: "unix", Err: syscall.ENOENT}

	tracker.record(gpuenvironment.SystemProbeSourceEBPF, errUnreachable, start)
	tracker.record(gpuenvironment.SystemProbeSourcePRM, nil, start)
	assert.Empty(t, tracker.failures(start.Add(sysprobeDataUnavailableThreshold-time.Second)))

	// The first failure time is kept across consecutive failures.
	tracker.record(gpuenvironment.SystemProbeSourceEBPF, errUnreachable, start.Add(time.Minute))
	failures := tracker.failures(start.Add(sysprobeDataUnavailableThreshold))
	require.Len(t, failures, 1)
	assert.Equal(t, gpuenvironment.SystemProbeSourceEBPF, failures[0].Source)
	assert.Equal(t, gpuenvironment.SystemProbeErrorUnreachable, failures[0].ErrorClass)
	assert.Equal(t, errUnreachable.Error(), failures[0].Error)

	// A success resets the source.
	tracker.record(gpuenvironment.SystemProbeSourceEBPF, nil, start.Add(2*time.Minute))
	tracker.record(gpuenvironment.SystemProbeSourceEBPF, errUnreachable, start.Add(3*time.Minute))
	assert.Empty(t, tracker.failures(start.Add(sysprobeDataUnavailableThreshold+time.Minute)))
}

func TestClassifySysprobeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"module not loaded", fmt.Errorf("refresh: %w", &sysprobeclient.StatusCodeError{StatusCode: http.StatusNotFound}), gpuenvironment.SystemProbeErrorModuleNotLoaded},
		{"source disabled", &sysprobeclient.StatusCodeError{StatusCode: http.StatusNotImplemented}, gpuenvironment.SystemProbeErrorSourceDisabled},
		{"source unavailable", &sysprobeclient.StatusCodeError{StatusCode: http.StatusServiceUnavailable}, gpuenvironment.SystemProbeErrorSourceUnavailable},
		{"server error", &sysprobeclient.StatusCodeError{StatusCode: http.StatusInternalServerError}, gpuenvironment.SystemProbeErrorOther},
		{"deadline exceeded", fmt.Errorf("get: %w", context.DeadlineExceeded), gpuenvironment.SystemProbeErrorTimeout},
		{"socket missing", &net.OpError{Op: "dial", Net: "unix", Err: syscall.ENOENT}, gpuenvironment.SystemProbeErrorUnreachable},
		{"connection refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), gpuenvironment.SystemProbeErrorUnreachable},
		{"other", errors.New("invalid JSON"), gpuenvironment.SystemProbeErrorOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, classifySysprobeError(tt.err))
		})
	}
}

func TestSyncSysprobeDataHealthIssue(t *testing.T) {
	healthStore := healthplatformmock.New(t)
	check := &Check{sysprobeData: newSysprobeDataTracker()}
	check.SetIssueReporter(healthStore)
	start := time.Now()

	check.sysprobeData.record(gpuenvironment.SystemProbeSourceDriverEvents, &sysprobeclient.StatusCodeError{StatusCode: http.StatusNotFound}, start)
	check.syncSysprobeDataHealthIssue(start)
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.SystemProbeDataIssueID))

	check.syncSysprobeDataHealthIssue(start.Add(sysprobeDataUnavailableThreshold))
	issue := healthStore.GetIssue(gpuenvironment.SystemProbeDataIssueID)
	require.NotNil(t, issue)
	assert.Equal(t, gpuenvironment.SystemProbeDataIssueName, issue.IssueName)
	assert.Equal(t, gpuenvironment.SystemProbeDataIssueID, issue.Id)
	assert.Contains(t, issue.Title, gpuenvironment.SystemProbeSourceDriverEvents)

	check.sysprobeData.record(gpuenvironment.SystemProbeSourceDriverEvents, nil, start.Add(sysprobeDataUnavailableThreshold+time.Minute))
	check.syncSysprobeDataHealthIssue(start.Add(sysprobeDataUnavailableThreshold + time.Minute))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.SystemProbeDataIssueID))
}

func TestSyncDRACDIHealthIssue(t *testing.T) {
	original := containers.CDISpecDirs
	t.Cleanup(func() { containers.CDISpecDirs = original })
	specDir := t.TempDir()
	containers.CDISpecDirs = []string{specDir}

	healthStore := healthplatformmock.New(t)
	check := &Check{}
	check.SetIssueReporter(healthStore)
	start := time.Now()

	// No unresolved claims: a directory without specs is not an issue.
	check.syncDRACDIHealthIssue(0, start.Add(draCDISpecsMissingThreshold))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))

	// Below the threshold nothing is reported.
	check.syncDRACDIHealthIssue(2, start)
	check.syncDRACDIHealthIssue(2, start.Add(draCDISpecsMissingThreshold-time.Second))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))

	check.syncDRACDIHealthIssue(2, start.Add(draCDISpecsMissingThreshold))
	issue := healthStore.GetIssue(gpuenvironment.DRACDIIssueID)
	require.NotNil(t, issue)
	assert.Equal(t, gpuenvironment.DRACDIIssueName, issue.IssueName)
	assert.Equal(t, gpuenvironment.DRACDIIssueID, issue.Id)
	assert.Contains(t, issue.Title, specDir)
	assert.Equal(t, "2", issue.GetExtra().GetFields()[gpuenvironment.DRACDIContextKeyContainerCount].GetStringValue())

	// NVIDIA DRA claim specs appearing resolve the issue, even with unresolved claims.
	require.NoError(t, os.WriteFile(filepath.Join(specDir, containers.NvidiaDRAClaimSpecFileName("other")), []byte("{}"), 0o644))
	check.syncDRACDIHealthIssue(2, start.Add(2*draCDISpecsMissingThreshold))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))
}

type failingDeviceCache struct {
	ddnvml.DeviceCache
}

func (failingDeviceCache) AllPhysicalDevices() ([]ddnvml.Device, error) {
	return nil, errors.New("device cache unavailable")
}

func newDRAContainer(id string) *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: id},
		Runtime:  workloadmeta.ContainerRuntimeContainerd,
		State:    workloadmeta.ContainerState{Running: true},
		ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{{
			Name:       string(gpuutil.GpuNvidiaDRA),
			CdiDevices: []string{"k8s.gpu.nvidia.com/claim=" + id + "-gpu-0"},
		}},
	}
}

func TestGetGPUToContainersMapSyncsDRACDIHealthIssue(t *testing.T) {
	original := containers.CDISpecDirs
	t.Cleanup(func() { containers.CDISpecDirs = original })
	containers.CDISpecDirs = []string{t.TempDir()}

	nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions())
	wmetaMock := testutil.GetWorkloadMetaMockWithDefaultGPUs(t)
	check := newConfiguredGPUCheck(t, taggerfxmock.SetupFakeTagger(t), wmetaMock, mocksender.CreateDefaultDemultiplexer(t), nil)
	healthStore := healthplatformmock.New(t)
	check.SetIssueReporter(healthStore)

	workloadContainer := newDRAContainer("workload")
	agentContainer := newDRAContainer("agent")
	wmetaMock.Set(workloadContainer)
	wmetaMock.Set(agentContainer)
	wmetaMock.Set(&workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: strconv.Itoa(os.Getpid())},
		Pid:      int32(os.Getpid()),
		Owner:    &agentContainer.EntityID,
	})

	start := time.Now()
	check.getGPUToContainersMap(start)
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))

	check.getGPUToContainersMap(start.Add(draCDISpecsMissingThreshold))
	issue := healthStore.GetIssue(gpuenvironment.DRACDIIssueID)
	require.NotNil(t, issue)
	// The Agent's own container is not counted.
	assert.Equal(t, "1", issue.GetExtra().GetFields()[gpuenvironment.DRACDIContextKeyContainerCount].GetStringValue())

	// Without the device list the containers are not inspected, so the issue
	// must not be resolved even though no unresolved claims remain.
	wmetaMock.Unset(workloadContainer)
	check.deviceCache = failingDeviceCache{DeviceCache: check.deviceCache}
	check.getGPUToContainersMap(start.Add(2 * draCDISpecsMissingThreshold))
	assert.NotNil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))
}

func TestRunRecordsSysprobeDataSources(t *testing.T) {
	socketPath := sysprobetestutil.SystemProbeSocketPath(t, "gpu-check")
	server, err := sysprobetestutil.NewSystemProbeTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/driver-events":
			http.Error(w, "GPU driver event subscriber failed to start", http.StatusServiceUnavailable)
		case "/gpu/prm-metrics":
			http.Error(w, "GPU PRM endpoint is disabled", http.StatusNotImplemented)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}), socketPath)
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Close)

	spConfig := pkgconfigsetup.SystemProbe()
	for key, value := range map[string]interface{}{
		"system_probe_config.sysprobe_socket":  socketPath,
		"gpu_monitoring.enabled":               true,
		"gpu_monitoring.enable_ebpf_probes":    true,
		"gpu_monitoring.prm_endpoint_enabled":  true,
		"gpu_monitoring.driver_events_enabled": true,
	} {
		original := spConfig.Get(key)
		spConfig.SetInTest(key, value)
		t.Cleanup(func() { spConfig.SetInTest(key, original) })
	}

	nvmltestutil.SetupMockNVML(t, testutil.WithMockAllFunctions(), testutil.WithProcessData(nil, nvml.SUCCESS))
	check := newConfiguredGPUCheck(t, taggerfxmock.SetupFakeTagger(t), testutil.GetWorkloadMetaMockWithDefaultGPUs(t), mocksender.CreateDefaultDemultiplexer(t), nil)
	require.NotNil(t, check.spCache)
	require.NotNil(t, check.prmCache)
	require.True(t, check.driverEventsEnabled)
	// PRM is only requested from system-probe when a collector registered a request.
	check.prmCache.RegisterRequest(model.PRMRequest{DeviceUUID: testutil.DefaultGpuUUID, Port: 1, Group: 34})

	require.NoError(t, check.Run())

	errorClasses := make(map[string]string)
	for _, failure := range check.sysprobeData.failures(time.Now().Add(sysprobeDataUnavailableThreshold)) {
		errorClasses[failure.Source] = failure.ErrorClass
	}
	assert.Equal(t, map[string]string{
		gpuenvironment.SystemProbeSourceEBPF:         gpuenvironment.SystemProbeErrorModuleNotLoaded,
		gpuenvironment.SystemProbeSourcePRM:          gpuenvironment.SystemProbeErrorSourceDisabled,
		gpuenvironment.SystemProbeSourceDriverEvents: gpuenvironment.SystemProbeErrorSourceUnavailable,
	}, errorClasses)
}

func TestHealthIssueSyncWithNilReporter(t *testing.T) {
	check := &Check{sysprobeData: newSysprobeDataTracker()}

	require.NotPanics(t, func() {
		check.syncSysprobeDataHealthIssue(time.Now())
		check.syncDRACDIHealthIssue(1, time.Now())
	})
}
