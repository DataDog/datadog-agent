// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"
	"path"
	"testing"

	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestKernelLaunchesHandled(t *testing.T) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot())
	require.NoError(t, err)
	stream := newStreamHandler(0, sysCtx)

	kernStartTime := uint64(1)
	launch := &gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeKernelLaunch,
			Pid_tgid:  1,
			Ktime_ns:  kernStartTime,
			Stream_id: 1,
		},
		Kernel_addr:     42,
		Grid_size:       gpuebpf.Dim3{X: 10, Y: 10, Z: 10},
		Block_size:      gpuebpf.Dim3{X: 2, Y: 2, Z: 1},
		Shared_mem_size: 100,
	}
	threadCount := 10 * 10 * 10 * 2 * 2

	numLaunches := 3
	for i := 0; i < numLaunches; i++ {
		stream.handleKernelLaunch(launch)
	}

	// No sync, so we should have data
	require.Nil(t, stream.getPastData(false))

	// We should have a current kernel span running
	currTime := uint64(100)
	currData := stream.getCurrentData(currTime)
	require.NotNil(t, currData)
	require.Len(t, currData.spans, 1)

	span := currData.spans[0]
	require.Equal(t, kernStartTime, span.startKtime)
	require.Equal(t, currTime, span.endKtime)
	require.Equal(t, uint64(numLaunches), span.numKernels)
	require.Equal(t, uint64(threadCount), span.avgThreadCount)

	// Now we mark a sync event
	syncTime := uint64(200)
	stream.markSynchronization(syncTime)

	// We should have a past kernel span
	pastData := stream.getPastData(true)
	require.NotNil(t, pastData)

	require.Len(t, pastData.spans, 1)
	span = pastData.spans[0]
	require.Equal(t, kernStartTime, span.startKtime)
	require.Equal(t, syncTime, span.endKtime)
	require.Equal(t, uint64(numLaunches), span.numKernels)
	require.Equal(t, uint64(threadCount), span.avgThreadCount)

	// We should have no current data
	require.Nil(t, stream.getCurrentData(currTime))
}

func TestMemoryAllocationsHandled(t *testing.T) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot())
	require.NoError(t, err)
	stream := newStreamHandler(0, sysCtx)

	memAllocTime := uint64(1)
	memFreeTime := uint64(2)
	memAddr := uint64(42)
	allocSize := uint64(1024)

	allocation := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memAllocTime,
			Stream_id: 1,
		},
		Addr: memAddr,
		Size: allocSize,
		Type: gpuebpf.CudaMemAlloc,
	}

	free := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memFreeTime,
			Stream_id: 1,
		},
		Addr: memAddr,
		Type: gpuebpf.CudaMemFree,
	}

	stream.handleMemEvent(allocation)

	// With just an allocation event, we should have no data
	require.Nil(t, stream.getPastData(false))

	// We should have a current memory allocation span running
	currTime := uint64(100)
	currData := stream.getCurrentData(currTime)
	require.NotNil(t, currData)
	require.Len(t, currData.allocations, 1)

	memAlloc := currData.allocations[0]
	require.Equal(t, memAllocTime, memAlloc.startKtime)
	require.Equal(t, uint64(0), memAlloc.endKtime) // Not deallocated yet
	require.Equal(t, false, memAlloc.isLeaked)     // Cannot say this is a leak yet
	require.Equal(t, allocSize, memAlloc.size)

	// Now we free the memory
	stream.handleMemEvent(free)

	// We should have a past memory allocation span
	pastData := stream.getPastData(true)
	require.NotNil(t, pastData)

	require.Len(t, pastData.allocations, 1)
	memAlloc = pastData.allocations[0]
	require.Equal(t, memAllocTime, memAlloc.startKtime)
	require.Equal(t, memFreeTime, memAlloc.endKtime) // Not deallocated yet
	require.Equal(t, false, memAlloc.isLeaked)       // Cannot say this is a leak yet
	require.Equal(t, allocSize, memAlloc.size)

	// We should have no current data
	require.Nil(t, stream.getCurrentData(currTime))

	// Also check we didn't leak
	require.Empty(t, stream.memAllocEvents)
}

func TestMemoryAllocationsDetectLeaks(t *testing.T) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot())
	require.NoError(t, err)
	stream := newStreamHandler(0, sysCtx)

	memAllocTime := uint64(1)
	memAddr := uint64(42)
	allocSize := uint64(1024)

	allocation := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memAllocTime,
			Stream_id: 1,
		},
		Addr: memAddr,
		Size: allocSize,
		Type: gpuebpf.CudaMemAlloc,
	}

	stream.handleMemEvent(allocation)
	stream.markEnd() // Mark the stream as ended. This should mark the allocation as leaked

	// We should have a past memory allocatio
	pastData := stream.getPastData(true)
	require.NotNil(t, pastData)

	require.Len(t, pastData.allocations, 1)
	memAlloc := pastData.allocations[0]
	require.Equal(t, memAllocTime, memAlloc.startKtime)
	require.Equal(t, true, memAlloc.isLeaked)
	require.Equal(t, allocSize, memAlloc.size)
}

func TestMemoryAllocationsNoCrashOnInvalidFree(t *testing.T) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot())
	require.NoError(t, err)
	stream := newStreamHandler(0, sysCtx)

	memAllocTime := uint64(1)
	memFreeTime := uint64(2)
	memAddrAlloc := uint64(42)
	memAddrFree := uint64(43)
	allocSize := uint64(1024)

	// Ensure the addresses are different
	require.NotEqual(t, memAddrAlloc, memAddrFree)

	allocation := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memAllocTime,
			Stream_id: 1,
		},
		Addr: memAddrAlloc,
		Size: allocSize,
		Type: gpuebpf.CudaMemAlloc,
	}

	free := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memFreeTime,
			Stream_id: 1,
		},
		Addr: memAddrFree,
		Type: gpuebpf.CudaMemFree,
	}

	stream.handleMemEvent(allocation)
	stream.handleMemEvent(free)

	// The free was for a different address, so we should have no data
	require.Nil(t, stream.getPastData(false))
}

func TestMemoryAllocationsMultipleAllocsHandled(t *testing.T) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot())
	require.NoError(t, err)
	stream := newStreamHandler(0, sysCtx)

	memAllocTime1, memAllocTime2 := uint64(1), uint64(10)
	memFreeTime1, memFreeTime2 := uint64(15), uint64(20)
	memAddr1, memAddr2 := uint64(42), uint64(4096)
	allocSize1, allocSize2 := uint64(1024), uint64(2048)

	allocation1 := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memAllocTime1,
			Stream_id: 1,
		},
		Addr: memAddr1,
		Size: allocSize1,
		Type: gpuebpf.CudaMemAlloc,
	}

	free1 := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memFreeTime1,
			Stream_id: 1,
		},
		Addr: memAddr1,
		Type: gpuebpf.CudaMemFree,
	}

	allocation2 := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memAllocTime2,
			Stream_id: 1,
		},
		Addr: memAddr2,
		Size: allocSize2,
		Type: gpuebpf.CudaMemAlloc,
	}

	free2 := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeMemory,
			Pid_tgid:  1,
			Ktime_ns:  memFreeTime2,
			Stream_id: 1,
		},
		Addr: memAddr2,
		Type: gpuebpf.CudaMemFree,
	}

	stream.handleMemEvent(allocation1)
	stream.handleMemEvent(allocation2)
	stream.handleMemEvent(free1)
	stream.handleMemEvent(free2)

	// We should have a past memory allocation span
	pastData := stream.getPastData(true)
	require.NotNil(t, pastData)

	require.Len(t, pastData.allocations, 2)
	foundAlloc1, foundAlloc2 := false, false

	for _, alloc := range pastData.allocations {
		if alloc.startKtime == memAllocTime1 {
			foundAlloc1 = true
			require.Equal(t, memFreeTime1, alloc.endKtime)
			require.Equal(t, false, alloc.isLeaked)
			require.Equal(t, allocSize1, alloc.size)
		} else if alloc.startKtime == memAllocTime2 {
			foundAlloc2 = true
			require.Equal(t, memFreeTime2, alloc.endKtime)
			require.Equal(t, false, alloc.isLeaked)
			require.Equal(t, allocSize2, alloc.size)
		}
	}

	require.True(t, foundAlloc1)
	require.True(t, foundAlloc2)

	// We should have no current data
	require.Nil(t, stream.getCurrentData(memFreeTime2+1))

	// Also check we didn't leak
	require.Empty(t, stream.memAllocEvents)
}

func TestKernelLaunchesIncludeEnrichedKernelData(t *testing.T) {
	proc := kernel.ProcFSRoot()
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), proc)
	require.NoError(t, err)

	// Set up the caches in system context so no actual queries are done
	pid, tid := uint64(1), uint64(1)
	kernAddress := uint64(42)
	binPath := "/path/to/binary"
	smVersion := uint32(75)
	kernName := "kernel"
	kernSize := uint64(1000)
	sharedMem := uint64(100)
	constantMem := uint64(200)

	sysCtx.pidMaps[int(pid)] = []*procfs.ProcMap{
		{StartAddr: 0, EndAddr: 1000, Offset: 0, Pathname: binPath},
	}

	procBinPath := path.Join(proc, fmt.Sprintf("%d/root/%s", pid, binPath))
	kernKey := cuda.CubinKernelKey{Name: kernName, SmVersion: smVersion}
	sysCtx.cudaSymbols[procBinPath] = &symbolsEntry{
		Symbols: &cuda.Symbols{
			SymbolTable: map[uint64]string{kernAddress: kernName},
			Fatbin: &cuda.Fatbin{
				Kernels: map[cuda.CubinKernelKey]*cuda.CubinKernel{
					kernKey: {
						Name:        kernName,
						KernelSize:  kernSize,
						SharedMem:   sharedMem,
						ConstantMem: constantMem,
					},
				},
			},
		},
	}

	sysCtx.deviceSmVersions = map[int]int{0: int(smVersion)}

	stream := newStreamHandler(uint32(pid), sysCtx)

	kernStartTime := uint64(1)
	launch := &gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:      gpuebpf.CudaEventTypeKernelLaunch,
			Pid_tgid:  uint64(pid<<32 + tid),
			Ktime_ns:  kernStartTime,
			Stream_id: 1,
		},
		Kernel_addr:     kernAddress,
		Grid_size:       gpuebpf.Dim3{X: 10, Y: 10, Z: 10},
		Block_size:      gpuebpf.Dim3{X: 2, Y: 2, Z: 1},
		Shared_mem_size: 0,
	}
	threadCount := 10 * 10 * 10 * 2 * 2

	numLaunches := 3
	for i := 0; i < numLaunches; i++ {
		stream.handleKernelLaunch(launch)
	}

	// No sync, so we should have data
	require.Nil(t, stream.getPastData(false))

	// We should have a current kernel span running
	currTime := uint64(100)
	currData := stream.getCurrentData(currTime)
	require.NotNil(t, currData)
	require.Len(t, currData.spans, 1)

	span := currData.spans[0]
	require.Equal(t, kernStartTime, span.startKtime)
	require.Equal(t, currTime, span.endKtime)
	require.Equal(t, uint64(numLaunches), span.numKernels)
	require.Equal(t, uint64(threadCount), span.avgThreadCount)
	require.Equal(t, sharedMem, span.avgMemoryUsage[sharedMemAlloc])
	require.Equal(t, constantMem, span.avgMemoryUsage[constantMemAlloc])
	require.Equal(t, kernSize, span.avgMemoryUsage[kernelMemAlloc])

	// Now we mark a sync event
	syncTime := uint64(200)
	stream.markSynchronization(syncTime)

	// We should have a past kernel span
	pastData := stream.getPastData(true)
	require.NotNil(t, pastData)

	require.Len(t, pastData.spans, 1)
	span = pastData.spans[0]
	require.Equal(t, kernStartTime, span.startKtime)
	require.Equal(t, syncTime, span.endKtime)
	require.Equal(t, uint64(numLaunches), span.numKernels)
	require.Equal(t, uint64(threadCount), span.avgThreadCount)
	require.Equal(t, sharedMem, span.avgMemoryUsage[sharedMemAlloc])
	require.Equal(t, constantMem, span.avgMemoryUsage[constantMemAlloc])
	require.Equal(t, kernSize, span.avgMemoryUsage[kernelMemAlloc])

	// We should have no current data
	require.Nil(t, stream.getCurrentData(currTime))
}
