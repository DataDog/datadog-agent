// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amdgpu

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/amd"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newStore(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func newTestCollector(t *testing.T, store workloadmeta.Component, sysRoot string) *collector {
	cfg := config.NewMock(t)
	cfg.SetInTest("gpu.enabled", true)
	cfg.SetInTest("gpu.amd.enabled", true)
	cfg.SetInTest("gpu.integrate_with_workloadmeta_processes", true)
	c := newCollector(cfg, sysRoot)
	require.NoError(t, c.Start(context.Background(), store))
	return c
}

// fakeHost has one partitioned MI300X-like GPU (two KFD nodes) and one other
// AMD GPU, with process 100 on both and process 200 on the second.
func fakeHost(t *testing.T) *amd.FakeSysfs {
	fs := amd.NewFakeSysfs(t)
	fs.SetDriverVersion("6.14.14")
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", amd.MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:41:00.0", "amdgpu", amd.MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	fs.AddKFDNode(3, 5100, 0, 0x4100, 90402)
	fs.AddKFDProcess(100, 4102, 1<<30)
	fs.AddKFDProcess(100, 5100, 4096)
	fs.AddKFDProcess(200, 5100, 1<<20)
	return fs
}

func TestStartDisabled(t *testing.T) {
	for name, settings := range map[string]map[string]bool{
		"gpu disabled": {"gpu.enabled": false, "gpu.amd.enabled": true},
		"amd disabled": {"gpu.enabled": true, "gpu.amd.enabled": false},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := config.NewMock(t)
			for k, v := range settings {
				cfg.SetInTest(k, v)
			}
			err := newCollector(cfg, t.TempDir()).Start(context.Background(), nil)
			assert.True(t, dderrors.IsDisabled(err), "got %v", err)
		})
	}
}

func TestPullReportsGPUEntities(t *testing.T) {
	store := newStore(t)
	c := newTestCollector(t, store, fakeHost(t).Root)

	require.NoError(t, c.Pull(context.Background()))

	gpu, err := store.GetGPU("amd-00c0ffee00c0ffee")
	require.NoError(t, err)
	assert.Equal(t, &workloadmeta.GPU{
		EntityID:      workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: "amd-00c0ffee00c0ffee"},
		EntityMeta:    workloadmeta.EntityMeta{Name: "AMD Instinct MI300X"},
		Vendor:        "amd",
		Device:        "AMD Instinct MI300X",
		GPUType:       "mi300x",
		DriverVersion: "6.14.14",
		Index:         0,
		Architecture:  "gfx942",
		PCIBusID:      "0000:27:00.0",
		TotalMemory:   206141652992,
		DeviceType:    workloadmeta.GPUDeviceTypePhysical,
		ActivePIDs:    []int{100},
		Healthy:       true,
	}, gpu)
	assert.Equal(t, "none", gpu.SlicingMode())

	second, err := store.GetGPU("amd-0000-41-00-0")
	require.NoError(t, err)
	assert.Equal(t, 1, second.Index)
	assert.Equal(t, []int{100, 200}, second.ActivePIDs)
	assert.Len(t, store.ListGPUs(), 2)
}

func TestPullReportsClocksAndBusWidth(t *testing.T) {
	fs := amd.NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", amd.MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.SetKFDProperty(1, "max_engine_clk_fcompute", 2100)
	fs.AddKFDMemBank(1, 0, 1, 1300, 8192)
	store := newStore(t)
	c := newTestCollector(t, store, fs.Root)

	require.NoError(t, c.Pull(context.Background()))

	gpu, err := store.GetGPU("amd-00c0ffee00c0ffee")
	require.NoError(t, err)
	assert.Equal(t, uint32(2100), gpu.MaxClockRates[workloadmeta.GPUSM])
	assert.Equal(t, uint32(1300), gpu.MaxClockRates[workloadmeta.GPUMemory])
	assert.Equal(t, uint32(8192), gpu.MemoryBusWidth)
}

// The store logs every error a collector returns, on every pull. An
// unreadable KFD topology is a stable deployment condition (an unprivileged
// container), so it must neither be returned as an error nor hide the GPUs.
func TestPullKFDAccessDeniedIsNotAnError(t *testing.T) {
	fs := fakeHost(t)
	fs.DenyKFDNode(1)
	fs.DenyKFDNode(2)
	fs.DenyKFDNode(3)
	store := newStore(t)
	c := newTestCollector(t, store, fs.Root)

	for range 3 {
		require.NoError(t, c.Pull(context.Background()))
	}

	gpus := store.ListGPUs()
	require.Len(t, gpus, 2)
	for _, gpu := range gpus {
		assert.Empty(t, gpu.Architecture)
		assert.Empty(t, gpu.ActivePIDs, "processes cannot be attributed without the KFD GPU IDs")
	}
}
func TestPullPreservesProcessesWithPartialKFDTopology(t *testing.T) {
	store := newStore(t)
	fs := fakeHost(t)
	fs.AddKFDProcess(100, 4101, 10)
	fs.AddKFDProcess(100, 4102, 20)
	fs.AddKFDProcess(200, 4102, 40)
	fs.AddKFDProcess(200, 5100, 0)
	c := newTestCollector(t, store, fs.Root)
	firstGPU := workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: "amd-00c0ffee00c0ffee"}
	nvidiaGPU := workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: "GPU-nvidia"}
	processID := workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "200"}
	store.Notify([]workloadmeta.CollectorEvent{
		{Source: workloadmeta.SourceProcessCollector, Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{EntityID: processID, Pid: 200, ContainerID: "container-200"}},
		{Source: workloadmeta.SourceNVML, Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{EntityID: processID, Pid: 200, GPUs: []workloadmeta.EntityID{nvidiaGPU}}},
	})
	assertAssociations := func() {
		t.Helper()
		first, err := store.GetGPU(firstGPU.ID)
		require.NoError(t, err)
		assert.Equal(t, []int{100, 200}, first.ActivePIDs)
		assert.Equal(t, "gfx942", first.Architecture)
		second, err := store.GetGPU("amd-0000-41-00-0")
		require.NoError(t, err)
		assert.Equal(t, []int{100}, second.ActivePIDs)
		assert.Equal(t, uint64(206141652992), second.TotalMemory)
		process, err := store.GetProcess(200)
		require.NoError(t, err)
		assert.ElementsMatch(t, []workloadmeta.EntityID{firstGPU, nvidiaGPU}, process.GPUs)
		assert.Equal(t, "container-200", process.ContainerID)
	}
	require.NoError(t, c.Pull(context.Background()))
	assertAssociations()

	fs.DenyKFDNode(2)
	for range 2 {
		require.NoError(t, c.Pull(context.Background()), "permission denial must not become repetitive pull errors")
		assertAssociations()
	}
	for _, name := range []string{"gpu_id", "properties"} {
		require.NoError(t, os.Chmod(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/2", name), 0o644))
	}
	require.NoError(t, c.Pull(context.Background()))
	assertAssociations()

	// A complete snapshot can prove an exit, removing only AMD's contribution.
	require.NoError(t, os.RemoveAll(filepath.Join(fs.Root, "class/kfd/kfd/proc/200")))
	require.NoError(t, c.Pull(context.Background()))
	first, err := store.GetGPU(firstGPU.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{100}, first.ActivePIDs)
	process, err := store.GetProcess(200)
	require.NoError(t, err)
	assert.Equal(t, []workloadmeta.EntityID{nvidiaGPU}, process.GPUs)
	assert.Equal(t, "container-200", process.ContainerID)
}

func TestPullReportsGPUProcesses(t *testing.T) {
	store := newStore(t)
	fs := fakeHost(t)
	c := newTestCollector(t, store, fs.Root)
	require.NoError(t, c.Pull(context.Background()))

	proc, err := store.GetProcess(100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []workloadmeta.EntityID{
		{Kind: workloadmeta.KindGPU, ID: "amd-00c0ffee00c0ffee"},
		{Kind: workloadmeta.KindGPU, ID: "amd-0000-41-00-0"},
	}, proc.GPUs)

	// Process 200 exits: its GPU association is removed.
	require.NoError(t, os.RemoveAll(fs.Root+"/class/kfd/kfd/proc/200"))
	require.NoError(t, c.Pull(context.Background()))
	_, err = store.GetProcess(200)
	assert.Error(t, err)
	_, err = store.GetProcess(100)
	assert.NoError(t, err)
}

func TestPullWithoutProcessIntegration(t *testing.T) {
	store := newStore(t)
	cfg := config.NewMock(t)
	cfg.SetInTest("gpu.enabled", true)
	cfg.SetInTest("gpu.amd.enabled", true)
	cfg.SetInTest("gpu.integrate_with_workloadmeta_processes", false)
	c := newCollector(cfg, fakeHost(t).Root)
	require.NoError(t, c.Start(context.Background(), store))

	require.NoError(t, c.Pull(context.Background()))
	assert.Len(t, store.ListGPUs(), 2)
	_, err := store.GetProcess(100)
	assert.Error(t, err)
}

func TestPullRemovesVanishedGPUs(t *testing.T) {
	store := newStore(t)
	fs := fakeHost(t)
	c := newTestCollector(t, store, fs.Root)
	require.NoError(t, c.Pull(context.Background()))
	require.Len(t, store.ListGPUs(), 2)

	require.NoError(t, os.RemoveAll(fs.Root+"/class/drm/card1"))
	require.NoError(t, c.Pull(context.Background()))

	gpus := store.ListGPUs()
	require.Len(t, gpus, 1)
	assert.Equal(t, "amd-00c0ffee00c0ffee", gpus[0].ID)
}

func TestPullWithoutAMDGPUs(t *testing.T) {
	store := newStore(t)
	c := newTestCollector(t, store, t.TempDir())
	require.NoError(t, c.Pull(context.Background()))
	assert.Empty(t, store.ListGPUs())
}

func TestPullKeepsOtherProcessSources(t *testing.T) {
	store := newStore(t)
	fs := fakeHost(t)
	c := newTestCollector(t, store, fs.Root)
	processID := workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "100"}
	nvidiaID := workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: "GPU-nvidia"}
	store.Notify([]workloadmeta.CollectorEvent{
		{
			Source: workloadmeta.SourceProcessCollector,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{EntityID: processID, Pid: 100, ContainerID: "container-100"},
		},
		{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{EntityID: processID, Pid: 100, GPUs: []workloadmeta.EntityID{nvidiaID}},
		},
	})
	require.NoError(t, c.Pull(context.Background()))
	process, err := store.GetProcess(100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []workloadmeta.EntityID{
		nvidiaID,
		{Kind: workloadmeta.KindGPU, ID: "amd-00c0ffee00c0ffee"},
		{Kind: workloadmeta.KindGPU, ID: "amd-0000-41-00-0"},
	}, process.GPUs)
	assert.Equal(t, "container-100", process.ContainerID)

	// Changing AMD's GPU set must replace its own contribution, not append
	// duplicates or overwrite the independently reported NVIDIA association.
	fs.AddKFDProcess(100, 5100, 0)
	require.NoError(t, c.Pull(context.Background()))
	process, err = store.GetProcess(100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []workloadmeta.EntityID{
		nvidiaID, {Kind: workloadmeta.KindGPU, ID: "amd-00c0ffee00c0ffee"},
	}, process.GPUs)

	require.NoError(t, os.RemoveAll(filepath.Join(fs.Root, "class/kfd/kfd/proc/100")))
	require.NoError(t, c.Pull(context.Background()))
	process, err = store.GetProcess(100)
	require.NoError(t, err)
	assert.Equal(t, []workloadmeta.EntityID{nvidiaID}, process.GPUs)
	assert.Equal(t, "container-100", process.ContainerID)

	store.Notify([]workloadmeta.CollectorEvent{{
		Source: workloadmeta.SourceNVML,
		Type:   workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.Process{EntityID: processID},
	}})
	process, err = store.GetProcess(100)
	require.NoError(t, err)
	assert.Empty(t, process.GPUs)
	assert.Equal(t, "container-100", process.ContainerID)
}

func TestPullPreservesDevicesOnDiscoveryError(t *testing.T) {
	store := newStore(t)
	fs := fakeHost(t)
	c := newTestCollector(t, store, fs.Root)
	require.NoError(t, c.Pull(context.Background()))

	// A malformed attribute is deterministic even when tests run as root.
	devDir := filepath.Join(fs.Root, "devices/pci0000:00/0000:41:00.0")
	fs.WriteFiles(devDir, map[string]string{"device": "invalid\n"})
	require.ErrorContains(t, c.Pull(context.Background()), "PCI device ID")
	gpu, err := store.GetGPU("amd-0000-41-00-0")
	require.NoError(t, err)
	assert.Equal(t, []int{100, 200}, gpu.ActivePIDs)
	process, err := store.GetProcess(200)
	require.NoError(t, err)
	assert.Equal(t, []workloadmeta.EntityID{{Kind: workloadmeta.KindGPU, ID: gpu.ID}}, process.GPUs)

	// New devices can still be published while a different device fails.
	fs.AddCard("card2", fs.AddPCIDevice("0000:51:00.0", "amdgpu", amd.MI300XAttributes("")))
	require.Error(t, c.Pull(context.Background()))
	_, err = store.GetGPU("amd-0000-51-00-0")
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(fs.Root, "class/drm/card1")))
	require.NoError(t, c.Pull(context.Background()))
	_, err = store.GetGPU("amd-0000-41-00-0")
	assert.Error(t, err)
	_, err = store.GetProcess(200)
	assert.Error(t, err)
}

func TestPullPreservesProcessesOnReadError(t *testing.T) {
	store := newStore(t)
	fs := fakeHost(t)
	c := newTestCollector(t, store, fs.Root)
	require.NoError(t, c.Pull(context.Background()))
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/200"), map[string]string{"vram_5100": "invalid\n"})

	require.ErrorContains(t, c.Pull(context.Background()), "vram_5100")
	gpu, err := store.GetGPU("amd-0000-41-00-0")
	require.NoError(t, err)
	assert.Equal(t, []int{100, 200}, gpu.ActivePIDs)
	process, err := store.GetProcess(200)
	require.NoError(t, err)
	assert.Equal(t, []workloadmeta.EntityID{{Kind: workloadmeta.KindGPU, ID: gpu.ID}}, process.GPUs)

	fs.AddKFDProcess(200, 5100, 0)
	require.NoError(t, c.Pull(context.Background()))
	gpu, err = store.GetGPU("amd-0000-41-00-0")
	require.NoError(t, err)
	assert.Equal(t, []int{100}, gpu.ActivePIDs)
	_, err = store.GetProcess(200)
	assert.Error(t, err)
}
