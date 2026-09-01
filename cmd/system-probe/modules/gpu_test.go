// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf && nvml

package modules

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/prm"
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
	assert.NotEqual(t, http.StatusNotFound, w.Code)
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
