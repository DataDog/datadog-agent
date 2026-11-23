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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/prm"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
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

func TestAgentPIDsNextToSystemProbe(t *testing.T) {
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

	// Only the agents of the system-probe pod, and those whose cgroup cannot be read (kept as before)
	require.Equal(t, []uint32{10, 12, 13}, agentPIDsNextToSystemProbe(sysprobe, []uint32{10, 11, 12, 13}, cgroupOf))
	// Host services
	require.Equal(t, []uint32{20}, agentPIDsNextToSystemProbe("/system.slice/datadog-agent-sysprobe.service", []uint32{20},
		func(uint32) (string, error) { return "/system.slice/datadog-agent.service", nil }))
	// Unknown system-probe cgroup: all the agents, as before
	require.Equal(t, []uint32{10, 11}, agentPIDsNextToSystemProbe("", []uint32{10, 11}, cgroupOf))
	// The agent of the system-probe pod is restarting: the other agents are not touched
	require.Empty(t, agentPIDsNextToSystemProbe(sysprobe, []uint32{11}, cgroupOf))
}
