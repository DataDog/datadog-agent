// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// Run locally with `go test -fuzz=FuzzConsumerHandleEvent -run=FuzzConsumerHandleEvent`
func FuzzConsumerHandleEvent(f *testing.F) {
	ddnvml.WithMockNVML(f, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	cfg := config.New()
	ctx := getTestSystemContext(f, withFatbinParsingEnabled(false)) // Keep it simple, disable fatbin parsing
	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(f), cfg)
	consumer := newTestCudaEventConsumer(f, ctx, cfg, handlers)

	// Set up visible devices cache for a test PID
	testPID := 1234
	ctx.visibleDevicesCache[testPID] = nvmltestutil.GetDDNVMLMocksWithIndexes(f, 0, 1)

	// Add seed corpus with valid event types
	// Seed with a minimal kernel launch event
	kernelLaunchSeed := make([]byte, gpuebpf.SizeofCudaKernelLaunch)
	header := (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&kernelLaunchSeed[0]))
	header.Type = uint32(gpuebpf.CudaEventTypeKernelLaunch)
	header.Pid_tgid = uint64(testPID)<<32 + uint64(testPID)
	header.Stream_id = 1
	f.Add(kernelLaunchSeed)

	// Seed with a minimal memory event
	memEventSeed := make([]byte, gpuebpf.SizeofCudaMemEvent)
	header = (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&memEventSeed[0]))
	header.Type = uint32(gpuebpf.CudaEventTypeMemory)
	header.Pid_tgid = uint64(testPID)<<32 + uint64(testPID)
	header.Stream_id = 1
	f.Add(memEventSeed)

	// Seed with a minimal sync event
	syncEventSeed := make([]byte, gpuebpf.SizeofCudaSync)
	header = (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&syncEventSeed[0]))
	header.Type = uint32(gpuebpf.CudaEventTypeSync)
	header.Pid_tgid = uint64(testPID)<<32 + uint64(testPID)
	header.Stream_id = 1
	f.Add(syncEventSeed)

	// Seed with a minimal set device event
	setDeviceSeed := make([]byte, gpuebpf.SizeofCudaSetDeviceEvent)
	header = (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&setDeviceSeed[0]))
	header.Type = uint32(gpuebpf.CudaEventTypeSetDevice)
	header.Pid_tgid = uint64(testPID)<<32 + uint64(testPID)
	f.Add(setDeviceSeed)

	// Seed with a minimal visible devices set event
	visibleDevicesSeed := make([]byte, gpuebpf.SizeofCudaVisibleDevicesSetEvent)
	header = (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&visibleDevicesSeed[0]))
	header.Type = uint32(gpuebpf.CudaEventTypeVisibleDevicesSet)
	header.Pid_tgid = uint64(testPID)<<32 + uint64(testPID)
	f.Add(visibleDevicesSeed)

	f.Fuzz(func(_ *testing.T, rawEvent []byte) {
		// Need at least a header to process
		if len(rawEvent) < gpuebpf.SizeofCudaEventHeader {
			return
		}

		// Parse the header and call handleEvent
		header := (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&rawEvent[0]))
		dataPtr := unsafe.Pointer(&rawEvent[0])
		dataLen := len(rawEvent)

		// Call handleEvent - we expect it to handle all inputs gracefully
		// without panicking. Errors are acceptable.
		_ = consumer.handleEvent(header, dataPtr, dataLen)
	})
}
