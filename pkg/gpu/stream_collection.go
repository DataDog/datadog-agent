// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"iter"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// streamKey is a unique identifier for a CUDA stream that is not global.
// The streams are created with a specific GPU UUID, which means that pid+stream uniquely
// identify the GPU.
type streamKey struct {
	pid    uint32
	stream uint64
}

// globalStreamKey is a unique identifier for a CUDA stream that is global.
// Global streams depend on which GPU is active when they are used, so the GPU UUID
// needs to be part of the key
type globalStreamKey struct {
	pid     uint32
	gpuUUID string
}

type streamCollection struct {
	streams       map[streamKey]*StreamHandler
	globalStreams map[globalStreamKey]*StreamHandler
	sysCtx        *systemContext
	telemetry     *streamCollectionTelemetry
}

type streamCollectionTelemetry struct {
	missingContainers  telemetry.Counter
	missingDevices     telemetry.Counter
	finalizedProcesses telemetry.Counter
	activeHandlers     telemetry.Gauge
	removedHandlers    telemetry.Counter
}

func newStreamCollection(sysCtx *systemContext, telemetry telemetry.Component) *streamCollection {
	return &streamCollection{
		streams:       make(map[streamKey]*StreamHandler),
		globalStreams: make(map[globalStreamKey]*StreamHandler),
		sysCtx:        sysCtx,
		telemetry:     newStreamCollectionTelemetry(telemetry),
	}
}

func newStreamCollectionTelemetry(tm telemetry.Component) *streamCollectionTelemetry {
	subsystem := gpuTelemetryModule + "__streams"

	return &streamCollectionTelemetry{
		missingContainers:  tm.NewCounter(subsystem, "missing_containers", []string{"reason"}, "Number of missing containers"),
		missingDevices:     tm.NewCounter(subsystem, "missing_devices", nil, "Number of failures to get GPU devices for a stream"),
		finalizedProcesses: tm.NewCounter(subsystem, "finalized_processes", nil, "Number of processes that have ended"),
		activeHandlers:     tm.NewGauge(subsystem, "active_handlers", nil, "Number of active stream handlers"),
		removedHandlers:    tm.NewCounter(subsystem, "removed_handlers", []string{"device"}, "Number of removed stream handlers"),
	}
}

// getStream returns a StreamHandler for a given CUDA stream.
func (sc *streamCollection) getStream(header *gpuebpf.CudaEventHeader) (*StreamHandler, error) {
	if header.Stream_id == 0 {
		return sc.getGlobalStream(header)
	}

	return sc.getNonGlobalStream(header)
}

func (sc *streamCollection) getGlobalStream(header *gpuebpf.CudaEventHeader) (*StreamHandler, error) {
	pid, tid := getPidTidFromHeader(header)
	memoizedContainerID := sc.memoizedContainerID(header)

	// Global streams depend on which GPU is active when they are used, so we need to get the current active GPU device
	// The expensive step here is the container ID parsing, but we don't always need to do it so we pass a function
	// that can be called to retrieve it only when needed
	device, err := sc.sysCtx.getCurrentActiveGpuDevice(int(pid), int(tid), memoizedContainerID)
	if err != nil {
		sc.telemetry.missingDevices.Inc()
		return nil, err
	}

	key := globalStreamKey{
		pid:     pid,
		gpuUUID: device.UUID,
	}

	stream, ok := sc.globalStreams[key]
	if !ok {
		stream = sc.createStreamHandler(header, device, memoizedContainerID)
		sc.globalStreams[key] = stream
		sc.telemetry.activeHandlers.Set(float64(sc.streamCount()))
	}

	return stream, nil
}

func (sc *streamCollection) getNonGlobalStream(header *gpuebpf.CudaEventHeader) (*StreamHandler, error) {
	pid, _ := getPidTidFromHeader(header)

	key := streamKey{
		pid:    pid,
		stream: header.Stream_id,
	}

	stream, ok := sc.streams[key]
	if !ok {
		stream = sc.createStreamHandler(header, nil, sc.memoizedContainerID(header))
		sc.streams[key] = stream
		sc.telemetry.activeHandlers.Set(float64(sc.streamCount()))
	}

	return stream, nil
}

// createStreamHandler creates a new StreamHandler for a given CUDA stream.
// If the device not provided (it's nil), it will be retrieved from the system context.
func (sc *streamCollection) createStreamHandler(header *gpuebpf.CudaEventHeader, device *ddnvml.Device, containerIDFunc func() string) *StreamHandler {
	pid, tid := getPidTidFromHeader(header)
	metadata := streamMetadata{
		pid:         pid,
		streamID:    header.Stream_id,
		containerID: containerIDFunc(),
	}

	if device == nil {
		var err error
		device, err = sc.sysCtx.getCurrentActiveGpuDevice(int(pid), int(tid), containerIDFunc)
		if err != nil {
			log.Warnf("error getting GPU device for process %d: %s", pid, err)
			sc.telemetry.missingDevices.Inc()
			return nil
		}
	}

	metadata.gpuUUID = device.UUID
	metadata.smVersion = device.SMVersion

	return newStreamHandler(metadata, sc.sysCtx)
}

// memoizedContainerID returns a function that memoizes the container ID for a given CUDA stream.
// It's memoized because it's an expensive operation that we don't always need to do.
func (sc *streamCollection) memoizedContainerID(header *gpuebpf.CudaEventHeader) func() string {
	return funcs.MemoizeNoErrorUnsafe(func() string {
		cgroup := unix.ByteSliceToString(header.Cgroup[:])
		containerID, err := cgroups.ContainerFilter("", cgroup)
		if err != nil {
			log.Warnf("error getting container ID for cgroup %s: %s", cgroup, err)

			sc.telemetry.missingContainers.Inc("error")
			return ""
		}

		if containerID == "" {
			sc.telemetry.missingContainers.Inc("missing")
		}

		return containerID
	})
}

func (sc *streamCollection) allStreams() iter.Seq[*StreamHandler] {
	return func(yield func(*StreamHandler) bool) {
		for _, stream := range sc.streams {
			if !yield(stream) {
				return
			}
		}

		for _, stream := range sc.globalStreams {
			if !yield(stream) {
				return
			}
		}
	}
}

func (sc *streamCollection) streamCount() int {
	return len(sc.streams) + len(sc.globalStreams)
}

func (sc *streamCollection) markProcessStreamsAsEnded(pid uint32) {
	for handler := range sc.allStreams() {
		if handler.metadata.pid == pid {
			log.Debugf("Process %d ended, marking stream %d as ended", pid, handler.metadata.streamID)
			_ = handler.markEnd()
			sc.telemetry.finalizedProcesses.Inc()
		}
	}
}

func (sc *streamCollection) clean() {
	for key, handler := range sc.streams {
		if handler.processEnded {
			delete(sc.streams, key)
			sc.telemetry.removedHandlers.Inc(handler.metadata.gpuUUID)
		}
	}

	for key, handler := range sc.globalStreams {
		if handler.processEnded {
			delete(sc.globalStreams, key)
			sc.telemetry.removedHandlers.Inc(handler.metadata.gpuUUID)
		}
	}

	sc.telemetry.activeHandlers.Set(float64(sc.streamCount()))
}
