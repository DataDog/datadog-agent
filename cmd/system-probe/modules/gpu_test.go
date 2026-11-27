// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf && nvml

package modules

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/prm"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	gputestutil "github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestGPUModuleOrder(t *testing.T) {
	allModules := All()
	assert.Less(t, slices.Index(allModules, EventMonitor), slices.Index(allModules, GPUMonitoring))
}

func TestGPUModuleRegistersPRMEndpointWhenEnabled(t *testing.T) {
	router := http.NewServeMux()
	moduleRouter := module.NewRouter("gpu", router)
	gpuModule := &GPUMonitoringModule{
		cfg:        &gpuconfig.Config{PRMEndpointEnabled: true},
		prmHandler: &prm.Handler{},
	}

	err := gpuModule.Register(moduleRouter)
	assert.NoError(t, err)

	req := httptest.NewRequest("POST", "/gpu/prm-metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.NotContains(t, []int{http.StatusNotFound, http.StatusNotImplemented}, w.Code)
}

func TestGPUModulePRMEndpointDisabledReturnsNotImplemented(t *testing.T) {
	router := http.NewServeMux()
	moduleRouter := module.NewRouter("gpu", router)
	gpuModule := &GPUMonitoringModule{cfg: &gpuconfig.Config{PRMEndpointEnabled: false}}
	require.NoError(t, gpuModule.Register(moduleRouter))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/gpu/prm-metrics", nil))
	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestGPUModuleDriverEventsDisabledDoesNotPanic(t *testing.T) {
	pkgconfigsetup.SystemProbe().SetInTest("gpu_monitoring.enable_ebpf_probes", false)
	pkgconfigsetup.SystemProbe().SetInTest("gpu_monitoring.driver_events_enabled", false)

	created, err := GPUMonitoring.Fn(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	t.Cleanup(created.Close)

	router := http.NewServeMux()
	moduleRouter := module.NewRouter("gpu", router)
	require.NoError(t, created.Register(moduleRouter))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gpu/driver-events", nil))
	require.Equal(t, http.StatusNotImplemented, w.Code)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gpu/check", nil))
	require.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestGPUModuleSurvivesDriverEventStartupError(t *testing.T) {
	ddnvml.WithMockNVML(t, gputestutil.NewMockNVML(gputestutil.WithDeviceCount(1)))
	pkgconfigsetup.SystemProbe().SetInTest("gpu_monitoring.enable_ebpf_probes", false)
	pkgconfigsetup.SystemProbe().SetInTest("gpu_monitoring.driver_events_enabled", true)

	created, err := GPUMonitoring.Fn(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	t.Cleanup(created.Close)

	router := http.NewServeMux()
	moduleRouter := module.NewRouter("gpu", router)
	require.NoError(t, created.Register(moduleRouter))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gpu/driver-events", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestGetAgentPIDs(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: 1, Exe: "/opt/datadog-agent/bin/agent/agent"},
		{Pid: 2, Exe: "/opt/datadog-agent/embedded/bin/trace-agent"},
		{Pid: 3, Exe: "/opt/datadog-agent/bin/agent/agent"},
	})

	pids, err := getAgentPIDs(procRoot)

	assert.NoError(t, err)
	assert.ElementsMatch(t, []uint32{1, 3}, pids)
}

func TestAgentsNextToSystemProbe(t *testing.T) {
	cgroups := map[uint32]string{
		10: "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podA.slice/cri-containerd-agent.scope",
		11: "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podB.slice/cri-containerd-other-agent.scope",
		12: "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podA.slice/cri-containerd-trace.scope",
	}
	cgroupOf := func(pid uint32) (string, error) {
		if cgroup, ok := cgroups[pid]; ok {
			return cgroup, nil
		}
		return "", errors.New("gone")
	}
	sysprobe := "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podA.slice/cri-containerd-sysprobe.scope"
	pids := func(agents []agentCgroup) []uint32 {
		var result []uint32
		for _, agent := range agents {
			result = append(result, agent.pid)
		}
		return result
	}

	// Only the agents of the system-probe pod; an agent whose cgroup cannot be read (13) is skipped, with a warning
	// under its own key, so that it does not hide a later failure to configure that agent
	t.Cleanup(func() {
		cgroupPermissionWarnings.Delete("agent-cgroup-13")
		cgroupPermissionWarnings.Delete("agent-cgroup-10")
	})
	agents := agentsNextToSystemProbe(sysprobe, []uint32{10, 11, 12, 13}, cgroupOf)
	_, warned := cgroupPermissionWarnings.Load("agent-cgroup-13")
	require.True(t, warned)
	_, warned = cgroupPermissionWarnings.Load("agent-13")
	require.False(t, warned)
	require.Equal(t, []uint32{10, 12}, pids(agents))
	require.Equal(t, cgroups[10], agents[0].cgroup)
	// Host services
	require.Equal(t, []uint32{20}, pids(agentsNextToSystemProbe("/system.slice/datadog-agent-sysprobe.service", []uint32{20},
		func(uint32) (string, error) { return "/system.slice/datadog-agent.service", nil })))
	// The agent of the system-probe pod is restarting: the other agents are not touched
	require.Empty(t, agentsNextToSystemProbe(sysprobe, []uint32{11}, cgroupOf))

	// A transient lookup failure is forgotten once the cgroup can be read again
	warnCgroupPermissionsOnce("agent-cgroup-10", "failed")
	agentsNextToSystemProbe(sysprobe, []uint32{10}, cgroupOf)
	_, warned = cgroupPermissionWarnings.Load("agent-cgroup-10")
	require.False(t, warned)
}

func TestForgetDepartedAgentWarnings(t *testing.T) {
	t.Cleanup(func() {
		cgroupPermissionWarnings.Range(func(key, _ any) bool { cgroupPermissionWarnings.Delete(key); return true })
	})
	warnCgroupPermissionsOnce("agent-1", "failed")
	warnCgroupPermissionsOnce("agent-cgroup-1", "failed")
	warnCgroupPermissionsOnce("agent-2", "failed")
	warnCgroupPermissionsOnce("agent-cgroup-2", "failed")
	warnCgroupPermissionsOnce("system-probe", "failed")

	forgetDepartedAgentWarnings([]uint32{2, 3})

	for _, key := range []string{"agent-2", "agent-cgroup-2"} {
		_, kept := cgroupPermissionWarnings.Load(key)
		require.True(t, kept, key)
	}
	for _, key := range []string{"agent-1", "agent-cgroup-1"} {
		_, kept := cgroupPermissionWarnings.Load(key)
		require.False(t, kept, "PID 1 is gone: %s", key)
	}
	_, kept := cgroupPermissionWarnings.Load("system-probe")
	require.True(t, kept)
}

func TestGPUModuleMigGrEndpoint(t *testing.T) {
	nvmltestutil.SetupMockNVML(t,
		gputestutil.WithDefaultMIGDevices(),
		gputestutil.WithGpmSupport(true),
		gputestutil.WithGpmMetricValues(map[nvml.GpmMetricId]gputestutil.MockGpmMetricValue{
			nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: 42, Return: nvml.SUCCESS},
		}),
	)
	cache := ddnvml.NewDeviceCache()
	migDevices, err := cache.AllMigDevices()
	require.NoError(t, err)
	require.NotEmpty(t, migDevices)
	uuid := migDevices[0].GetDeviceInfo().UUID

	newModule := func(enabled bool) (*GPUMonitoringModule, *http.ServeMux) {
		router := http.NewServeMux()
		gpuModule := &GPUMonitoringModule{
			cfg:          &gpuconfig.Config{MigGrEndpointEnabled: enabled},
			deviceCache:  cache,
			migGrSamples: map[string]migGrSample{},
		}
		require.NoError(t, gpuModule.Register(module.NewRouter("gpu", router)))
		return gpuModule, router
	}
	get := func(router *http.ServeMux, query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gpu/mig-gr"+query, nil))
		return w
	}
	age := func(m *GPUMonitoringModule, d time.Duration) {
		s := m.migGrSamples[uuid]
		s.at = s.at.Add(-d)
		m.migGrSamples[uuid] = s
	}

	_, disabled := newModule(false)
	assert.Equal(t, http.StatusNotImplemented, get(disabled, "?uuid="+uuid).Code)

	m, router := newModule(true)
	assert.Equal(t, http.StatusBadRequest, get(router, "").Code)
	assert.Equal(t, http.StatusNotFound, get(router, "?uuid=MIG-unknown").Code)

	// The first request stores a baseline, a close second one keeps it
	assert.Equal(t, migGrBaselineStoredStatus, get(router, "?uuid="+uuid).Code)
	baseline := m.migGrSamples[uuid]
	assert.Equal(t, migGrTooRecentStatus, get(router, "?uuid="+uuid).Code)
	assert.Equal(t, baseline, m.migGrSamples[uuid])

	// A request one check interval later returns the value
	age(m, 15*time.Second)
	w := get(router, "?uuid="+uuid)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "42", strings.TrimSpace(w.Body.String()))

	// A baseline older than migGrMaxAge is replaced instead of averaging over the gap
	age(m, migGrMaxAge+time.Minute)
	assert.Equal(t, migGrBaselineStoredStatus, get(router, "?uuid="+uuid).Code)

	// Samples of MIG devices nobody asks about anymore (e.g. removed by a reconfiguration) are freed
	lib, err := ddnvml.GetSafeNvmlLib()
	require.NoError(t, err)
	gone, err := lib.GpmSampleAlloc()
	require.NoError(t, err)
	m.migGrSamples["MIG-gone"] = migGrSample{sample: gone, at: time.Now().Add(-migGrMaxAge - time.Minute)}
	age(m, 15*time.Second)
	require.Equal(t, http.StatusOK, get(router, "?uuid="+uuid).Code)
	assert.NotContains(t, m.migGrSamples, "MIG-gone")

	// An NVML release drops the stored samples
	m.dropMigGrSamples(true)
	assert.Empty(t, m.migGrSamples)
}
