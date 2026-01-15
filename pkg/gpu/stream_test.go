// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestKernelLaunchesHandled(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), config.New().StreamConfig, streamTelemetry)
	require.NoError(t, err)

	kernStartTime := uint64(1)
	launch := &gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeKernelLaunch),
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
	require.Nil(t, stream.getPastData())

	// We should have a current kernel span running
	currTime := uint64(100)
	currData := stream.getCurrentData(currTime)
	require.NotNil(t, currData)
	require.Len(t, currData.kernels, 1)

	span := currData.kernels[0]
	require.Equal(t, kernStartTime, span.startKtime)
	require.Equal(t, currTime, span.endKtime)
	require.Equal(t, uint64(numLaunches), span.numKernels)
	require.Equal(t, uint64(threadCount), span.avgThreadCount)

	// Now we mark a sync event
	syncTime := uint64(200)
	stream.markSynchronization(syncTime)

	// We should have a past kernel span
	pastData := stream.getPastData()
	require.NotNil(t, pastData)

	require.Len(t, pastData.kernels, 1)
	span = pastData.kernels[0]
	require.Equal(t, kernStartTime, span.startKtime)
	require.Equal(t, syncTime, span.endKtime)
	require.Equal(t, uint64(numLaunches), span.numKernels)
	require.Equal(t, uint64(threadCount), span.avgThreadCount)

	// We should have no current data
	require.Nil(t, stream.getCurrentData(currTime))
}

func TestMemoryAllocationsHandled(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), config.New().StreamConfig, streamTelemetry)
	require.NoError(t, err)

	memAllocTime := uint64(1)
	memFreeTime := uint64(2)
	memAddr := uint64(42)
	allocSize := uint64(1024)

	allocation := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
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
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
			Pid_tgid:  1,
			Ktime_ns:  memFreeTime,
			Stream_id: 1,
		},
		Addr: memAddr,
		Type: gpuebpf.CudaMemFree,
	}

	stream.handleMemEvent(allocation)

	// With just an allocation event, we should have no data
	require.Nil(t, stream.getPastData())

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
	pastData := stream.getPastData()
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
	require.Empty(t, stream.memAllocEvents.Keys())
}

func TestMemoryAllocationsDetectLeaks(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), config.New().StreamConfig, streamTelemetry)
	require.NoError(t, err)

	memAllocTime := uint64(1)
	memAddr := uint64(42)
	allocSize := uint64(1024)

	allocation := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
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
	pastData := stream.getPastData()
	require.NotNil(t, pastData)

	require.Len(t, pastData.allocations, 1)
	memAlloc := pastData.allocations[0]
	require.Equal(t, memAllocTime, memAlloc.startKtime)
	require.Equal(t, true, memAlloc.isLeaked)
	require.Equal(t, allocSize, memAlloc.size)
}

func TestMemoryAllocationsNoCrashOnInvalidFree(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), config.New().StreamConfig, streamTelemetry)
	require.NoError(t, err)

	memAllocTime := uint64(1)
	memFreeTime := uint64(2)
	memAddrAlloc := uint64(42)
	memAddrFree := uint64(43)
	allocSize := uint64(1024)

	// Ensure the addresses are different
	require.NotEqual(t, memAddrAlloc, memAddrFree)

	allocation := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
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
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
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
	require.Nil(t, stream.getPastData())
}

func TestMemoryAllocationsMultipleAllocsHandled(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), config.New().StreamConfig, streamTelemetry)
	require.NoError(t, err)

	memAllocTime1, memAllocTime2 := uint64(1), uint64(10)
	memFreeTime1, memFreeTime2 := uint64(15), uint64(20)
	memAddr1, memAddr2 := uint64(42), uint64(4096)
	allocSize1, allocSize2 := uint64(1024), uint64(2048)

	allocation1 := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
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
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
			Pid_tgid:  1,
			Ktime_ns:  memFreeTime1,
			Stream_id: 1,
		},
		Addr: memAddr1,
		Type: gpuebpf.CudaMemFree,
	}

	allocation2 := &gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
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
			Type:      uint32(gpuebpf.CudaEventTypeMemory),
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
	pastData := stream.getPastData()
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
	require.Empty(t, stream.memAllocEvents.Keys())
}

func TestKernelLaunchEnrichment(t *testing.T) {
	for _, fatbinParsingEnabled := range []bool{true, false} {
		name := "fatbinParsingEnabled"
		if !fatbinParsingEnabled {
			name = "fatbinParsingDisabled"
		}

		t.Run(name, func(t *testing.T) {
			var proc string
			if fatbinParsingEnabled {
				proc = t.TempDir()
			} else {
				proc = kernel.ProcFSRoot()
			}

			ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
			sysCtx := getTestSystemContext(t, withFatbinParsingEnabled(fatbinParsingEnabled), withProcRoot(proc))

			if fatbinParsingEnabled {
				// Ensure the kernel cache is running so we can load the kernel data
				sysCtx.cudaKernelCache.Start()
				t.Cleanup(sysCtx.cudaKernelCache.Stop)
			} else {
				require.Nil(t, sysCtx.cudaKernelCache)
			}

			// Set up the caches in system context so no actual queries are done
			pid, tid := uint64(1), uint64(1)
			kernAddress := uint64(42)
			binPath := "binary"
			smVersion := uint32(75)
			kernName := "kernel"
			kernSize := uint64(1000)
			sharedMem := uint64(100)
			constantMem := uint64(200)

			kernel := &cuda.CubinKernel{
				Name:        kernName,
				KernelSize:  kernSize,
				SharedMem:   sharedMem,
				ConstantMem: constantMem,
			}

			if fatbinParsingEnabled {
				// Create all parent directories,
				// the path should match the procBinPath var value in cuda.AddKernelCacheEntry
				tmpFoldersPath := filepath.Join(proc, strconv.FormatUint(pid, 10), "root")
				err := os.MkdirAll(tmpFoldersPath, 0755)
				require.NoError(t, err)
				filePath := filepath.Join(tmpFoldersPath, binPath)
				data := []byte(kernName)
				//create a dummy file because AddKernelCacheEntry expects a file to exist to get the file stats for verification
				err = os.WriteFile(filePath, data, 0644)
				require.NoError(t, err)

				cuda.AddKernelCacheEntry(t, sysCtx.cudaKernelCache, int(pid), kernAddress, smVersion, binPath, kernel)
			}

			streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
			stream, err := newStreamHandler(streamMetadata{pid: uint32(pid), smVersion: smVersion}, sysCtx, config.New().StreamConfig, streamTelemetry)
			require.NoError(t, err)

			kernStartTime := uint64(1)
			launch := &gpuebpf.CudaKernelLaunch{
				Header: gpuebpf.CudaEventHeader{
					Type:      uint32(gpuebpf.CudaEventTypeKernelLaunch),
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

			if fatbinParsingEnabled {
				// We need to wait until the kernel cache loads the kernel data
				cuda.WaitForKernelCacheEntry(t, sysCtx.cudaKernelCache, int(pid), kernAddress, smVersion)
			}

			// No sync, so we should have data
			require.Nil(t, stream.getPastData())

			// We should have a current kernel span running
			currTime := uint64(100)
			currData := stream.getCurrentData(currTime)
			require.NotNil(t, currData)
			require.Len(t, currData.kernels, 1)

			span := currData.kernels[0]
			require.Equal(t, kernStartTime, span.startKtime)
			require.Equal(t, currTime, span.endKtime)
			require.Equal(t, uint64(numLaunches), span.numKernels)
			require.Equal(t, uint64(threadCount), span.avgThreadCount)

			if fatbinParsingEnabled {
				require.Equal(t, sharedMem, span.avgMemoryUsage[sharedMemAlloc])
				require.Equal(t, constantMem, span.avgMemoryUsage[constantMemAlloc])
				require.Equal(t, kernSize, span.avgMemoryUsage[kernelMemAlloc])
			} else {
				require.Equal(t, uint64(0), span.avgMemoryUsage[sharedMemAlloc])
				require.Equal(t, uint64(0), span.avgMemoryUsage[constantMemAlloc])
				require.Equal(t, uint64(0), span.avgMemoryUsage[kernelMemAlloc])
			}

			// Now we mark a sync event
			syncTime := uint64(200)
			stream.markSynchronization(syncTime)

			// We should have a past kernel span
			pastData := stream.getPastData()
			require.NotNil(t, pastData)

			require.Len(t, pastData.kernels, 1)
			span = pastData.kernels[0]
			require.Equal(t, kernStartTime, span.startKtime)
			require.Equal(t, syncTime, span.endKtime)
			require.Equal(t, uint64(numLaunches), span.numKernels)
			require.Equal(t, uint64(threadCount), span.avgThreadCount)

			if fatbinParsingEnabled {
				require.Equal(t, sharedMem, span.avgMemoryUsage[sharedMemAlloc])
				require.Equal(t, constantMem, span.avgMemoryUsage[constantMemAlloc])
				require.Equal(t, kernSize, span.avgMemoryUsage[kernelMemAlloc])
			} else {
				require.Equal(t, uint64(0), span.avgMemoryUsage[sharedMemAlloc])
				require.Equal(t, uint64(0), span.avgMemoryUsage[constantMemAlloc])
				require.Equal(t, uint64(0), span.avgMemoryUsage[kernelMemAlloc])
			}

			// We should have no current data
			require.Nil(t, stream.getCurrentData(currTime))
		})
	}
}

func TestKernelLaunchTriggersSyncIfLimitReached(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	limits := config.StreamConfig{
		MaxKernelLaunches:     5,
		MaxMemAllocEvents:     5,
		MaxPendingKernelSpans: 100,
		MaxPendingMemorySpans: 100,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, streamTelemetry)
	require.NoError(t, err)

	// Add a few kernel launches, not reaching the limit
	for i := 0; i < limits.MaxKernelLaunches-1; i++ {
		stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
			Header: gpuebpf.CudaEventHeader{
				Type:     uint32(gpuebpf.CudaEventTypeKernelLaunch),
				Ktime_ns: uint64(i),
			},
		})
	}

	require.Len(t, stream.kernelLaunches, limits.MaxKernelLaunches-1)

	// Add one more, should trigger a sync
	stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:     uint32(gpuebpf.CudaEventTypeKernelLaunch),
			Ktime_ns: uint64(limits.MaxKernelLaunches),
		},
	})
	require.Len(t, stream.kernelLaunches, 0)

	pastData := stream.getPastData()
	require.NotNil(t, pastData)
	require.Len(t, pastData.kernels, 1)
	span := pastData.kernels[0]
	require.Equal(t, uint64(limits.MaxKernelLaunches), span.numKernels)
}

func TestKernelLaunchWithManualSyncsAndLimitsReached(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	limits := config.StreamConfig{
		MaxKernelLaunches:     5,
		MaxMemAllocEvents:     5,
		MaxPendingKernelSpans: 100,
		MaxPendingMemorySpans: 100,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, streamTelemetry)
	require.NoError(t, err)

	// number of kernel launches between syncs
	sequence := []int{
		3,
		14,
		2,
		4,
		5,
	}

	expectedSpanLengths := []int{
		3, // manual sync from the first launch group
		5, // forced sync from the second launch group
		5, // forced sync from the second launch group
		4, // manual sync from the second launch group
		2, // manual sync from the third launch group
		4, // manual sync from the fourth launch group
		5, // forced sync, by the fifth launch group
	}

	launchedKernels := 0

	getTimeForKernel := func(kernelIndex int) uint64 {
		return uint64(kernelIndex * 3)
	}

	getTimeForSync := func(kernelIndex int) uint64 {
		return uint64((kernelIndex * 3) + 2)
	}

	for _, numLaunches := range sequence {
		for j := 0; j < numLaunches; j++ {
			stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
				Header: gpuebpf.CudaEventHeader{
					Type:     uint32(gpuebpf.CudaEventTypeKernelLaunch),
					Ktime_ns: getTimeForKernel(launchedKernels),
				},
			})
			launchedKernels++
		}

		stream.markSynchronization(getTimeForSync(launchedKernels - 1)) //sync corresponding to the last kernel sent
	}

	// No launch should remain
	require.Len(t, stream.kernelLaunches, 0)

	// Check that the spans are as expected
	pastData := stream.getPastData()
	require.NotNil(t, pastData)
	require.Len(t, pastData.kernels, len(expectedSpanLengths))
	kernelsSeen := 0
	for i, span := range pastData.kernels {
		spanLength := expectedSpanLengths[i]
		require.Equal(t, uint64(spanLength), span.numKernels, "numKernels for span %d is incorrect", i)
		require.Equal(t, getTimeForKernel(kernelsSeen), span.startKtime, "startKtime for span %d is incorrect", i)

		endKernelIndex := kernelsSeen + spanLength - 1

		if spanLength == 5 {
			// From a forced sync, so the end time is just one nanosecond after the last kernel launch
			require.Equal(t, getTimeForKernel(endKernelIndex)+1, span.endKtime, "endKtime for span %d (forced sync)is incorrect", i)
		} else {
			// From a regular sync event, so the end time is the one we send in the sync event
			require.Equal(t, getTimeForSync(endKernelIndex), span.endKtime, "endKtime for span %d (manual sync) is incorrect", i)
		}
		kernelsSeen += spanLength
	}
	require.Equal(t, kernelsSeen, launchedKernels)
}

func TestMemoryAllocationEviction(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	limits := config.StreamConfig{
		MaxKernelLaunches:     5,
		MaxMemAllocEvents:     5,
		MaxPendingKernelSpans: 100,
		MaxPendingMemorySpans: 100,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, streamTelemetry)
	require.NoError(t, err)

	// Add a few memory allocations, go over the limit
	for i := 0; i < limits.MaxMemAllocEvents+1; i++ {
		stream.handleMemEvent(&gpuebpf.CudaMemEvent{
			Header: gpuebpf.CudaEventHeader{
				Type:     uint32(gpuebpf.CudaEventTypeMemory),
				Ktime_ns: uint64(i + 100),
			},
			Type: gpuebpf.CudaMemAlloc,
			Addr: uint64(i + 1),
			Size: uint64((i + 1) * 1024),
		})

		if i < limits.MaxMemAllocEvents {
			// No evictions yet
			require.Equal(t, i+1, stream.memAllocEvents.Len())
		}
	}

	// At this point we should have gotten one eviction
	require.Equal(t, limits.MaxMemAllocEvents, stream.memAllocEvents.Len())

	// Check that we got an allocation evicted
	evictionCounter, err := telemetryMock.GetCountMetric("gpu__streams", "alloc_evicted")
	require.NoError(t, err)
	require.Len(t, evictionCounter, 1)
	require.Equal(t, float64(1), evictionCounter[0].Value())
}

func TestMemoryAllocationEvictionAndFrees(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	limits := config.StreamConfig{
		MaxKernelLaunches:     5,
		MaxMemAllocEvents:     5,
		MaxPendingKernelSpans: 1000,
		MaxPendingMemorySpans: 1000,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, streamTelemetry)
	require.NoError(t, err)

	totalEvents := 200 * limits.MaxMemAllocEvents

	for i := 0; i < totalEvents; i++ {
		addr := uint64(i)
		size := uint64(i * 1024)
		ktime := uint64(i)

		// All iterations get an allocation event
		stream.handleMemEvent(&gpuebpf.CudaMemEvent{
			Header: gpuebpf.CudaEventHeader{
				Type:     uint32(gpuebpf.CudaEventTypeMemory),
				Ktime_ns: ktime,
			},
			Type: gpuebpf.CudaMemAlloc,
			Addr: addr,
			Size: size,
		})

		// Only even iterations get a free event
		if i%2 == 0 {
			stream.handleMemEvent(&gpuebpf.CudaMemEvent{
				Type: gpuebpf.CudaMemFree,
				Addr: addr,
				Header: gpuebpf.CudaEventHeader{
					Type:     uint32(gpuebpf.CudaEventTypeMemory),
					Ktime_ns: ktime + 1,
				},
			})
		}

		// We should have at most the max number of allocations
		require.LessOrEqual(t, stream.memAllocEvents.Len(), limits.MaxMemAllocEvents)
	}

	// Now validate all the allocations
	require.Equal(t, limits.MaxMemAllocEvents, stream.memAllocEvents.Len())

	// We should have
	// - 100 allocations from the corresponding frees on every even iteration
	expectedAllocations := totalEvents / 2
	pastData := stream.getPastData()
	require.NotNil(t, pastData)
	require.Len(t, pastData.allocations, expectedAllocations)

	seenIndexes := make(map[uint64]bool)

	// Check that the allocations are correct
	for _, alloc := range pastData.allocations {
		// Only even allocations should have a corresponding free event
		require.True(t, alloc.startKtime%2 == 0, "allocation startKtime should be even")
		// The timeis deterministic, the one that we set in the free event
		require.Equal(t, alloc.startKtime+1, alloc.endKtime)

		require.Equal(t, uint64(alloc.startKtime*1024), alloc.size)
		require.False(t, seenIndexes[alloc.startKtime], "already saw allocation with index %d", alloc.startKtime)
		seenIndexes[alloc.startKtime] = true
	}

	require.Len(t, seenIndexes, expectedAllocations)

	// Check that we got the expected number of evictions
	evictionCounter, err := telemetryMock.GetCountMetric("gpu__streams", "alloc_evicted")
	require.NoError(t, err)
	require.Len(t, evictionCounter, 1)
	require.Equal(t, float64(totalEvents/2-limits.MaxMemAllocEvents), evictionCounter[0].Value())
}

func TestStreamHandlerIsInactive(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	limits := config.StreamConfig{
		MaxKernelLaunches:     5,
		MaxMemAllocEvents:     5,
		MaxPendingKernelSpans: 100,
		MaxPendingMemorySpans: 100,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, newStreamTelemetry(testutil.GetTelemetryMock(t)))
	require.NoError(t, err)

	inactivityThreshold := 1 * time.Second

	// Test case 1: Stream with no events should be considered active
	require.False(t, stream.isInactive(1000, inactivityThreshold))

	// Test case 2: Stream with recent events should be considered active
	launch := &gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeKernelLaunch),
			Pid_tgid:  1,
			Ktime_ns:  1000,
			Stream_id: 1,
		},
		Kernel_addr:     42,
		Grid_size:       gpuebpf.Dim3{X: 10, Y: 10, Z: 10},
		Block_size:      gpuebpf.Dim3{X: 2, Y: 2, Z: 1},
		Shared_mem_size: 100,
	}
	stream.handleKernelLaunch(launch)
	require.False(t, stream.isInactive(2000, inactivityThreshold))

	// Test case 3: Stream with events older than inactivity threshold should be considered inactive
	require.True(t, stream.isInactive(3000000000, inactivityThreshold)) // 3 seconds later with 1 second threshold
}

func TestStreamHandlerMaxPendingSpans(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	limits := config.StreamConfig{
		MaxKernelLaunches:     1000,
		MaxMemAllocEvents:     1000,
		MaxPendingKernelSpans: 5,
		MaxPendingMemorySpans: 5,
	}
	telemetryMock := testutil.GetTelemetryMock(t)

	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, newStreamTelemetry(telemetryMock))
	require.NoError(t, err)

	spansToSend := limits.MaxPendingKernelSpans * 2
	prevRejectionCount := 0

	t.Run("KernelLaunches", func(t *testing.T) {
		for i := 0; i < spansToSend; i++ {
			stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
				Header: gpuebpf.CudaEventHeader{
					Ktime_ns: uint64(time.Now().UnixNano()),
				},
			})
			stream.markSynchronization(uint64(time.Now().UnixNano()))
		}

		data := stream.getPastData()
		require.NotNil(t, data)
		require.Len(t, data.kernels, limits.MaxPendingKernelSpans)

		rejectionCounter, err := telemetryMock.GetCountMetric("gpu__streams", "rejected_spans_due_to_limit")
		require.NoError(t, err)
		require.Len(t, rejectionCounter, 1)
		rejectionCount := int(rejectionCounter[0].Value())
		prevRejectionCount = rejectionCount
		require.Equal(t, spansToSend-limits.MaxPendingKernelSpans, rejectionCount)
	})

	t.Run("MemoryAllocations", func(t *testing.T) {
		for i := 0; i < spansToSend; i++ {
			stream.handleMemEvent(&gpuebpf.CudaMemEvent{
				Header: gpuebpf.CudaEventHeader{
					Ktime_ns: uint64(time.Now().UnixNano()),
				},
				Type: gpuebpf.CudaMemAlloc,
				Addr: uint64(i),
				Size: uint64(1024),
			})
			stream.handleMemEvent(&gpuebpf.CudaMemEvent{
				Header: gpuebpf.CudaEventHeader{
					Ktime_ns: uint64(time.Now().UnixNano()),
				},
				Type: gpuebpf.CudaMemFree,
				Addr: uint64(i),
				Size: uint64(1024),
			})
		}

		data := stream.getPastData()
		require.NotNil(t, data)
		require.Len(t, data.allocations, limits.MaxPendingKernelSpans)

		rejectionCounter, err := telemetryMock.GetCountMetric("gpu__streams", "rejected_spans_due_to_limit")
		require.NoError(t, err)
		require.Len(t, rejectionCounter, 1)
		rejectionCount := int(rejectionCounter[0].Value()) - prevRejectionCount
		require.Equal(t, spansToSend-limits.MaxPendingKernelSpans, rejectionCount)
	})
}
func TestGetPastDataConcurrency(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	eventsPerSync := 100
	limits := config.StreamConfig{
		MaxKernelLaunches:     eventsPerSync * 1000,
		MaxMemAllocEvents:     eventsPerSync * 1000,
		MaxPendingKernelSpans: eventsPerSync * 1000,
		MaxPendingMemorySpans: eventsPerSync * 1000,
	}

	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, newStreamTelemetry(testutil.GetTelemetryMock(t)))
	require.NoError(t, err)

	// Create a goroutine that will send kernel launches and syncs
	done := make(chan struct{})
	sentSyncs := atomic.Uint64{}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for {
			select {
			case <-done:
				wg.Done()
				return
			default:
				// Send 100 events of each type and then synchronize
				for i := 0; i < eventsPerSync; i++ {
					stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
						Header: gpuebpf.CudaEventHeader{
							Ktime_ns: uint64(time.Now().UnixNano()),
						},
						Grid_size:       gpuebpf.Dim3{X: 10, Y: 10, Z: 10},
						Block_size:      gpuebpf.Dim3{X: 2, Y: 2, Z: 1},
						Shared_mem_size: 100,
					})
					memType := gpuebpf.CudaMemAlloc
					if i%2 == 1 {
						memType = gpuebpf.CudaMemFree
					}
					stream.handleMemEvent(&gpuebpf.CudaMemEvent{
						Header: gpuebpf.CudaEventHeader{
							Ktime_ns: uint64(time.Now().UnixNano()),
						},
						Type: uint32(memType),
						Addr: uint64(i / 2),
						Size: uint64(1024),
					})
				}
				stream.markSynchronization(uint64(time.Now().UnixNano()))
				sentSyncs.Add(1)
			}
		}
	}()
	t.Cleanup(func() {
		// Ensure the goroutine is done before ending the test
		close(done)
		wg.Wait()
	})

	// Ensure some data is sent
	time.Sleep(200 * time.Millisecond)

	beforeGetDataSyncs := sentSyncs.Load()
	require.Greater(t, beforeGetDataSyncs, uint64(0))

	data := stream.getPastData()
	require.NotNil(t, data)

	// As the data is being sent concurrently, we don't know the exact amount of data
	// sent, but it must be greater or equal than the number of syncs sent before the data was requested
	require.GreaterOrEqual(t, len(data.kernels), int(beforeGetDataSyncs))
	require.GreaterOrEqual(t, len(data.allocations), int(beforeGetDataSyncs))
}

func BenchmarkHandleEvents(b *testing.B) {
	ddnvml.WithMockNVML(b, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	// Set limits high enough so that we don't hit them, as we have nothing consuming the channels
	// and we want to test just the non-blocking send
	limits := config.StreamConfig{
		MaxKernelLaunches:     1000000,
		MaxMemAllocEvents:     1000000,
		MaxPendingKernelSpans: 1000000,
		MaxPendingMemorySpans: 1000000,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(b), limits, newStreamTelemetry(testutil.GetTelemetryMock(b)))
	require.NoError(b, err)

	now := uint64(time.Now().UnixNano())
	i := 0

	for b.Loop() {
		stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
			Header: gpuebpf.CudaEventHeader{
				Ktime_ns:  now + uint64(i),
				Stream_id: 1,
			},
		})
		memType := gpuebpf.CudaMemAlloc
		if i%2 == 1 {
			memType = gpuebpf.CudaMemFree
		}
		stream.handleMemEvent(&gpuebpf.CudaMemEvent{
			Header: gpuebpf.CudaEventHeader{
				Ktime_ns:  now + uint64(i),
				Stream_id: 1,
			},
			Type: uint32(memType),
			Addr: uint64(i / 2),
			Size: uint64(1024),
		})

		i++
	}
}

type poolStats struct {
	active int
	get    int
	put    int
}

func getMetricWithTags(t *testing.T, metrics []telemetry.Metric, tags map[string]string) int {
	for _, metric := range metrics {
		matchesAll := true
		metricTags := metric.Tags()
		for tag, expectedValue := range tags {
			if metricTags[tag] != expectedValue {
				matchesAll = false
				break
			}
		}
		if matchesAll {
			return int(metric.Value())
		}
	}
	t.Fatalf("metric not found with tags %v", tags)
	return 0
}

func getPoolStats(t *testing.T, telemetryMock telemetry.Mock, pool string) poolStats {
	active, err := telemetryMock.GetGaugeMetric("sync__pool", "active")
	require.NoError(t, err)
	require.NotEmpty(t, active)
	activeCount := getMetricWithTags(t, active, map[string]string{"module": "gpu", "pool_name": pool})

	get, err := telemetryMock.GetCountMetric("sync__pool", "get")
	require.NoError(t, err)
	require.NotEmpty(t, get)
	getCount := getMetricWithTags(t, get, map[string]string{"module": "gpu", "pool_name": pool})

	put, err := telemetryMock.GetCountMetric("sync__pool", "put")
	require.NoError(t, err)
	require.NotEmpty(t, put)
	putCount := getMetricWithTags(t, put, map[string]string{"module": "gpu", "pool_name": pool})

	return poolStats{
		active: activeCount,
		get:    getCount,
		put:    putCount,
	}
}

func TestEnrichedKernelLaunchPool(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	withTelemetryEnabledPools(t, telemetryMock)
	cfg := config.StreamConfig{
		MaxKernelLaunches:     10,
		MaxMemAllocEvents:     10,
		MaxPendingKernelSpans: 10,
		MaxPendingMemorySpans: 10,
	}

	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), cfg, streamTelemetry)
	require.NoError(t, err)

	extraLaunches := 5
	numLaunches := cfg.MaxKernelLaunches + extraLaunches
	for i := 0; i < numLaunches; i++ {
		stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
			Header: gpuebpf.CudaEventHeader{
				Ktime_ns: uint64(time.Now().UnixNano()),
			},
		})
	}

	// after this loop, the first 10 (cfg.MaxKernelLaunches) should have been processed as
	// we reached the max, and once processed they should have been put back in the pool
	stats := getPoolStats(t, telemetryMock, "enrichedKernelLaunch")
	require.Equal(t, extraLaunches, stats.active)
	require.Equal(t, numLaunches, stats.get)
	require.Equal(t, cfg.MaxKernelLaunches, stats.put)

	// getting the current data should not release or get any items
	currData := stream.getCurrentData(uint64(time.Now().UnixNano()))
	require.NotNil(t, currData)
	stats = getPoolStats(t, telemetryMock, "enrichedKernelLaunch")
	require.Equal(t, extraLaunches, stats.active)
	require.Equal(t, numLaunches, stats.get)
	require.Equal(t, cfg.MaxKernelLaunches, stats.put)

	// forcing a synchronization should release the items
	stream.markSynchronization(uint64(time.Now().UnixNano()))
	stats = getPoolStats(t, telemetryMock, "enrichedKernelLaunch")
	require.Equal(t, 0, stats.active)
	require.Equal(t, numLaunches, stats.get)
	require.Equal(t, numLaunches, stats.put)
}

func testPool(t *testing.T, poolName string, genSpan func(stream *StreamHandler), maxSpans int) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	withTelemetryEnabledPools(t, telemetryMock)
	cfg := config.StreamConfig{
		MaxKernelLaunches:     10,
		MaxMemAllocEvents:     10,
		MaxPendingKernelSpans: maxSpans,
		MaxPendingMemorySpans: maxSpans,
	}

	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), cfg, streamTelemetry)
	require.NoError(t, err)

	extraSpans := 5
	numSpans := maxSpans + extraSpans
	for i := 0; i < numSpans; i++ {
		genSpan(stream)
	}

	// we have generated more kernel spans than the limit. The first 10 (maxSpans) will have been
	// added into the channel, waiting for consumption. The rest, being over the channel size, should have
	// been rejected by the channel and released back to the pool
	stats := getPoolStats(t, telemetryMock, poolName)
	require.Equal(t, maxSpans, stats.active)
	require.Equal(t, numSpans, stats.get)
	require.Equal(t, extraSpans, stats.put)

	// getting current data should not release or get any items, as there are no pending events
	currData := stream.getCurrentData(uint64(time.Now().UnixNano()))
	require.Nil(t, currData)
	stats = getPoolStats(t, telemetryMock, poolName)
	require.Equal(t, maxSpans, stats.active)
	require.Equal(t, numSpans, stats.get)
	require.Equal(t, extraSpans, stats.put)

	// now getting the past data will consume the items from the channel, but not release the
	// as the ownership is passed to streamSpans
	pastData := stream.getPastData()
	require.NotNil(t, pastData)

	switch poolName {
	case "kernelSpan":
		require.Len(t, pastData.kernels, maxSpans)
	case "memorySpan":
		require.Len(t, pastData.allocations, maxSpans)
	default:
		require.Fail(t, "invalid pool name", poolName)
	}

	stats = getPoolStats(t, telemetryMock, poolName)
	require.Equal(t, maxSpans, stats.active)
	require.Equal(t, numSpans, stats.get)
	require.Equal(t, extraSpans, stats.put)

	// once we release the spans, they should be released back to the pool
	pastData.releaseSpans()
	stats = getPoolStats(t, telemetryMock, poolName)
	require.Equal(t, 0, stats.active)
	require.Equal(t, numSpans, stats.get)
	require.Equal(t, numSpans, stats.put)
}

func TestKernelSpanPool(t *testing.T) {
	testPool(t, "kernelSpan", func(stream *StreamHandler) {
		stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
			Header: gpuebpf.CudaEventHeader{
				Ktime_ns: uint64(time.Now().UnixNano()),
			},
		})
		stream.markSynchronization(uint64(time.Now().UnixNano()))
	}, 10)
}

func TestKernelSpanPoolNoLeakWhenNoKernelsMatchTimeFilter(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	withTelemetryEnabledPools(t, telemetryMock)
	cfg := config.StreamConfig{
		MaxKernelLaunches:     10,
		MaxMemAllocEvents:     10,
		MaxPendingKernelSpans: 10,
		MaxPendingMemorySpans: 10,
	}

	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), cfg, streamTelemetry)
	require.NoError(t, err)

	kernelTime := uint64(1000)
	stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Ktime_ns: kernelTime,
		},
	})

	// Query for data before the kernel was launched, so no kernels match the filter.
	// This triggers getCurrentKernelSpan to allocate a span but return nil.
	// Before the fix, this would leak the span.
	currData := stream.getCurrentData(kernelTime - 1)
	require.NotNil(t, currData)
	require.Empty(t, currData.kernels)
	require.Empty(t, currData.allocations)

	stats := getPoolStats(t, telemetryMock, "kernelSpan")
	require.Equal(t, 0, stats.active, "kernelSpan pool should have no active items when getCurrentKernelSpan returns nil")
	require.Equal(t, stats.get, stats.put, "kernelSpan pool get/put should be balanced")
}

func TestMemorySpanPoolNoLeakWhenNoKernelsMatchTimeFilter(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	withTelemetryEnabledPools(t, telemetryMock)
	cfg := config.StreamConfig{
		MaxKernelLaunches:     10,
		MaxMemAllocEvents:     10,
		MaxPendingKernelSpans: 10,
		MaxPendingMemorySpans: 10,
	}

	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), cfg, streamTelemetry)
	require.NoError(t, err)

	allocTime := uint64(1000)
	stream.handleMemEvent(&gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{
			Ktime_ns: allocTime,
		},
		Type: gpuebpf.CudaMemAlloc,
		Addr: uint64(42),
		Size: uint64(1024),
	})

	// Query for data before the allocation was made, so no allocations match the filter.
	currData := stream.getCurrentData(allocTime - 1)
	require.NotNil(t, currData)
	require.Empty(t, currData.kernels)
	require.Empty(t, currData.allocations)
}

func TestMemorySpanPool(t *testing.T) {
	testPool(t, "memorySpan", func(stream *StreamHandler) {
		stream.handleMemEvent(&gpuebpf.CudaMemEvent{
			Header: gpuebpf.CudaEventHeader{
				Ktime_ns: uint64(time.Now().UnixNano()),
			},
			Type: gpuebpf.CudaMemAlloc,
			Addr: uint64(10),
			Size: uint64(1024),
		})
		stream.handleMemEvent(&gpuebpf.CudaMemEvent{
			Header: gpuebpf.CudaEventHeader{
				Ktime_ns: uint64(time.Now().UnixNano()),
			},
			Type: gpuebpf.CudaMemFree,
			Addr: uint64(10),
		})
	}, 10)
}

func TestGetKernelDataReturnsUnwrappedErrors(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))

	t.Run("errFatbinParsingDisabled when cache is nil", func(t *testing.T) {
		sysCtx := getTestSystemContext(t, withFatbinParsingEnabled(false))
		require.Nil(t, sysCtx.cudaKernelCache)

		stream, err := newStreamHandler(streamMetadata{pid: 1, smVersion: 75}, sysCtx, config.New().StreamConfig, streamTelemetry)
		require.NoError(t, err)

		enrichedLaunch := &enrichedKernelLaunch{
			CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
				Kernel_addr: 42,
			},
			stream: stream,
		}

		_, err = enrichedLaunch.getKernelData()
		require.Error(t, err)
		// Test that the error is the exact sentinel error, not wrapped
		require.True(t, err == errFatbinParsingDisabled, "error should be the exact sentinel error errFatbinParsingDisabled")
	})

	t.Run("errFatbinParsingDisabled when smVersion is noSmVersion", func(t *testing.T) {
		sysCtx := getTestSystemContext(t, withFatbinParsingEnabled(true))
		sysCtx.cudaKernelCache.Start()
		t.Cleanup(sysCtx.cudaKernelCache.Stop)

		stream, err := newStreamHandler(streamMetadata{pid: 1, smVersion: noSmVersion}, sysCtx, config.New().StreamConfig, streamTelemetry)
		require.NoError(t, err)

		enrichedLaunch := &enrichedKernelLaunch{
			CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
				Kernel_addr: 42,
			},
			stream: stream,
		}

		_, err = enrichedLaunch.getKernelData()
		require.Error(t, err)
		// Test that the error is the exact sentinel error, not wrapped
		require.True(t, err == errFatbinParsingDisabled, "error should be the exact sentinel error errFatbinParsingDisabled")
	})

	t.Run("cuda.ErrKernelNotProcessedYet when kernel not in cache", func(t *testing.T) {
		proc := t.TempDir()
		sysCtx := getTestSystemContext(t, withFatbinParsingEnabled(true), withProcRoot(proc))
		sysCtx.cudaKernelCache.Start()
		t.Cleanup(sysCtx.cudaKernelCache.Stop)

		stream, err := newStreamHandler(streamMetadata{pid: 1, smVersion: 75}, sysCtx, config.New().StreamConfig, streamTelemetry)
		require.NoError(t, err)

		enrichedLaunch := &enrichedKernelLaunch{
			CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
				Kernel_addr: 42,
			},
			stream: stream,
		}

		_, err = enrichedLaunch.getKernelData()
		require.Error(t, err)
		// Test that the error is the exact sentinel error, not wrapped
		require.True(t, err == cuda.ErrKernelNotProcessedYet, "error should be the exact sentinel error cuda.ErrKernelNotProcessedYet")
	})
}
