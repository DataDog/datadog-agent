// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), getStreamLimits(config.New()), streamTelemetry)
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
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), getStreamLimits(config.New()), streamTelemetry)
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
	require.Empty(t, stream.memAllocEvents.Keys())
}

func TestMemoryAllocationsDetectLeaks(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), getStreamLimits(config.New()), streamTelemetry)
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
	pastData := stream.getPastData(true)
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
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), getStreamLimits(config.New()), streamTelemetry)
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
	require.Nil(t, stream.getPastData(false))
}

func TestMemoryAllocationsMultipleAllocsHandled(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	streamTelemetry := newStreamTelemetry(testutil.GetTelemetryMock(t))
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), getStreamLimits(config.New()), streamTelemetry)
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
				tmpFoldersPath := filepath.Join(proc, fmt.Sprintf("%d", pid), "root")
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
			stream, err := newStreamHandler(streamMetadata{pid: uint32(pid), smVersion: smVersion}, sysCtx, getStreamLimits(config.New()), streamTelemetry)
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
			pastData := stream.getPastData(true)
			require.NotNil(t, pastData)

			require.Len(t, pastData.spans, 1)
			span = pastData.spans[0]
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
	limits := streamLimits{
		maxKernelLaunches: 5,
		maxAllocEvents:    5,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, streamTelemetry)
	require.NoError(t, err)

	// Add a few kernel launches, not reaching the limit
	for i := 0; i < limits.maxKernelLaunches-1; i++ {
		stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
			Header: gpuebpf.CudaEventHeader{
				Type:     uint32(gpuebpf.CudaEventTypeKernelLaunch),
				Ktime_ns: uint64(i),
			},
		})
	}

	require.Len(t, stream.kernelLaunches, limits.maxKernelLaunches-1)

	// Add one more, should trigger a sync
	stream.handleKernelLaunch(&gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:     uint32(gpuebpf.CudaEventTypeKernelLaunch),
			Ktime_ns: uint64(limits.maxKernelLaunches),
		},
	})
	require.Len(t, stream.kernelLaunches, 0)

	require.Len(t, stream.kernelSpans, 1)
	span := stream.kernelSpans[0]
	require.Equal(t, uint64(limits.maxKernelLaunches), span.numKernels)
}

func TestKernelLaunchWithManualSyncsAndLimitsReached(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	telemetryMock := testutil.GetTelemetryMock(t)
	streamTelemetry := newStreamTelemetry(telemetryMock)
	limits := streamLimits{
		maxKernelLaunches: 5,
		maxAllocEvents:    5,
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
	require.Len(t, stream.kernelSpans, len(expectedSpanLengths))
	kernelsSeen := 0
	for i, span := range stream.kernelSpans {
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
	limits := streamLimits{
		maxKernelLaunches: 5,
		maxAllocEvents:    5,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, streamTelemetry)
	require.NoError(t, err)

	// Add a few memory allocations, go over the limit
	for i := 0; i < limits.maxAllocEvents+1; i++ {
		stream.handleMemEvent(&gpuebpf.CudaMemEvent{
			Header: gpuebpf.CudaEventHeader{
				Type:     uint32(gpuebpf.CudaEventTypeMemory),
				Ktime_ns: uint64(i + 100),
			},
			Type: gpuebpf.CudaMemAlloc,
			Addr: uint64(i + 1),
			Size: uint64((i + 1) * 1024),
		})

		if i < limits.maxAllocEvents {
			// No evictions yet
			require.Equal(t, i+1, stream.memAllocEvents.Len())
		}
	}

	// At this point we should have gotten one eviction
	require.Equal(t, limits.maxAllocEvents, stream.memAllocEvents.Len())

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
	limits := streamLimits{
		maxKernelLaunches: 5,
		maxAllocEvents:    5,
	}
	stream, err := newStreamHandler(streamMetadata{}, getTestSystemContext(t), limits, streamTelemetry)
	require.NoError(t, err)

	totalEvents := 200 * limits.maxAllocEvents

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
		require.LessOrEqual(t, stream.memAllocEvents.Len(), limits.maxAllocEvents)
	}

	// Now validate all the allocations
	require.Equal(t, limits.maxAllocEvents, stream.memAllocEvents.Len())

	// We should have
	// - 100 allocations from the corresponding frees on every even iteration
	expectedAllocations := totalEvents / 2
	require.Len(t, stream.allocations, expectedAllocations)

	seenIndexes := make(map[uint64]bool)

	// Check that the allocations are correct
	for _, alloc := range stream.allocations {
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
	require.Equal(t, float64(totalEvents/2-limits.maxAllocEvents), evictionCounter[0].Value())
}

func TestStreamHandlerIsInactive(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	limits := streamLimits{
		maxKernelLaunches: 5,
		maxAllocEvents:    5,
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
