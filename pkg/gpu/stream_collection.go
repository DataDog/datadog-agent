// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
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
	streams       sync.Map // map[streamKey]*StreamHandler
	globalStreams sync.Map // map[globalStreamKey]*StreamHandler
	sysCtx        *systemContext
	telemetry     *streamTelemetry
	streamConfig  config.StreamConfig
}

type streamTelemetry struct {
	missingContainers  telemetry.Counter
	missingDevices     telemetry.Counter
	finalizedProcesses telemetry.Counter
	activeHandlers     telemetry.Gauge
	removedHandlers    telemetry.Counter
	rejectedStreams    telemetry.Counter

	// streamHandler-specific telemetry
	forcedSyncOnKernelLaunch telemetry.Counter
	allocEvicted             telemetry.Counter
	invalidFreeEvents        telemetry.Counter
	rejectedSpans            telemetry.Counter
}

func newStreamCollection(sysCtx *systemContext, telemetry telemetry.Component, config *config.Config) *streamCollection {
	return &streamCollection{
		sysCtx:       sysCtx,
		telemetry:    newStreamTelemetry(telemetry),
		streamConfig: config.StreamConfig,
	}
}

func newStreamTelemetry(tm telemetry.Component) *streamTelemetry {
	subsystem := consts.GpuTelemetryModule + "__streams"

	return &streamTelemetry{
		forcedSyncOnKernelLaunch: tm.NewCounter(subsystem, "forced_sync_on_kernel_launch", nil, "Number of forced syncs on kernel launch"),
		allocEvicted:             tm.NewCounter(subsystem, "alloc_evicted", nil, "Number of allocations evicted from the cache"),
		invalidFreeEvents:        tm.NewCounter(subsystem, "invalid_free_events", nil, "Number of invalid free events"),
		missingContainers:        tm.NewCounter(subsystem, "missing_containers", []string{"reason"}, "Number of missing containers"),
		missingDevices:           tm.NewCounter(subsystem, "missing_devices", nil, "Number of failures to get GPU devices for a stream"),
		finalizedProcesses:       tm.NewCounter(subsystem, "finalized_processes", nil, "Number of processes that have ended"),
		activeHandlers:           tm.NewGauge(subsystem, "active_handlers", nil, "Number of active stream handlers"),
		removedHandlers:          tm.NewCounter(subsystem, "removed_handlers", []string{"device", "reason"}, "Number of removed stream handlers and why"),
		rejectedStreams:          tm.NewCounter(subsystem, "rejected_streams_due_to_limit", nil, "Number of rejected streams due to the max stream limit"),
		rejectedSpans:            tm.NewCounter(subsystem, "rejected_spans_due_to_limit", nil, "Number of rejected spans due to the max span limit"),
	}
}

// getStream returns a StreamHandler for a given CUDA event header. This entry point should only
// get called by the consumer thread, as the mutex locking patterns only work under that assumption.
func (sc *streamCollection) getStream(header *gpuebpf.CudaEventHeader) (*StreamHandler, error) {
	if header.Stream_id == 0 {
		return sc.getGlobalStream(header)
	}

	return sc.getNonGlobalStream(header)
}

// getGlobalStream returns the global stream associated to the device that's currently active (influenced
// by calls to cudaSetDevice) for the given PID and TID (extraded from header).
// If non-existing, he stream gets created and added to collection.
func (sc *streamCollection) getGlobalStream(header *gpuebpf.CudaEventHeader) (*StreamHandler, error) {
	pid, tid := getPidTidFromHeader(header)
	cacher := sc.getHeaderContainerCache(header)

	// Global streams depend on which GPU is active when they are used, so we need to get the current active GPU device
	// The expensive step here is the container ID parsing, but we don't always need to do it so we pass a function
	// that can be called to retrieve it only when needed
	device, err := sc.sysCtx.getCurrentActiveGpuDevice(int(pid), int(tid), cacher.containerID)
	if err != nil {
		sc.telemetry.missingDevices.Inc()
		return nil, err
	}

	key := globalStreamKey{
		pid:     pid,
		gpuUUID: device.GetDeviceInfo().UUID,
	}

	// Try to get existing stream
	if streamValue, ok := sc.globalStreams.Load(key); ok {
		if stream, ok := streamValue.(*StreamHandler); ok {
			return stream, nil
		}
	}

	// There is no race condition here on the check + create, because there is only one thread (consumer thread)
	// that calls this code and can create streams.
	stream, err := sc.createStreamHandler(header, device, cacher.containerID)
	if err != nil {
		return nil, fmt.Errorf("error creating global stream: %w", err)
	}

	sc.globalStreams.Store(key, stream)
	sc.telemetry.activeHandlers.Set(float64(sc.allStreamsCount()))

	return stream, nil
}

// getNonGlobalStream returns the non-global stream associated to the given PID and and stream ID (extracted from header).
// This does not depend on the active device on the given PID. If non-existing, he stream gets created and added to collection.
func (sc *streamCollection) getNonGlobalStream(header *gpuebpf.CudaEventHeader) (*StreamHandler, error) {
	pid, _ := getPidTidFromHeader(header)

	key := streamKey{
		pid:    pid,
		stream: header.Stream_id,
	}

	// Try to get existing stream
	if streamValue, ok := sc.streams.Load(key); ok {
		if stream, ok := streamValue.(*StreamHandler); ok {
			return stream, nil
		}
	}

	// There is no race condition here on the check + create, because there is
	// only one goroutine (the one from cudaEventConsumer) that calls this code
	// and can create streams.

	cacher := sc.getHeaderContainerCache(header)
	stream, err := sc.createStreamHandler(header, nil, cacher.containerID)
	if err != nil {
		return nil, fmt.Errorf("error creating non-global stream: %w", err)
	}

	sc.streams.Store(key, stream)
	sc.telemetry.activeHandlers.Set(float64(sc.allStreamsCount()))

	return stream, nil
}

// getActiveDeviceStreams returns all the streams associated to the device that's currently active (influenced
// by calls to cudaSetDevice) for the given PID and TID (extracted from header), including the global stream.
// Only already existing streams get collected, and none is added to the collection implicitly
func (sc *streamCollection) getActiveDeviceStreams(header *gpuebpf.CudaEventHeader) ([]*StreamHandler, error) {
	pid, tid := getPidTidFromHeader(header)
	cacher := sc.getHeaderContainerCache(header)

	device, err := sc.sysCtx.getCurrentActiveGpuDevice(int(pid), int(tid), cacher.containerID)
	if err != nil {
		sc.telemetry.missingDevices.Inc()
		return nil, err
	}

	res := []*StreamHandler{}
	globalStreamKey := globalStreamKey{
		pid:     pid,
		gpuUUID: device.GetDeviceInfo().UUID,
	}

	if streamValue, ok := sc.globalStreams.Load(globalStreamKey); ok {
		if stream, ok := streamValue.(*StreamHandler); ok {
			res = append(res, stream)
		}
	}

	sc.streams.Range(func(_ any, value any) bool {
		if stream, ok := value.(*StreamHandler); ok &&
			stream.metadata.pid == pid &&
			stream.metadata.gpuUUID == device.GetDeviceInfo().UUID {
			res = append(res, stream)
		}
		return true
	})

	return res, nil
}

// createStreamHandler creates a new StreamHandler for a given CUDA stream.
// If the device not provided (it's nil), it will be retrieved from the system context.
func (sc *streamCollection) createStreamHandler(header *gpuebpf.CudaEventHeader, device ddnvml.Device, containerIDFunc func() string) (*StreamHandler, error) {
	if sc.allStreamsCount() >= sc.streamConfig.MaxActiveStreams {
		sc.telemetry.rejectedStreams.Inc()
		return nil, fmt.Errorf("max streams (%d) reached", sc.streamConfig.MaxActiveStreams)
	}

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
			if logLimitProbe.ShouldLog() {
				log.Warnf("error getting GPU device for process %d: %s", pid, err)
			}
			sc.telemetry.missingDevices.Inc()
			return nil, err
		}
	}

	metadata.gpuUUID = device.GetDeviceInfo().UUID
	metadata.smVersion = device.GetDeviceInfo().SMVersion

	return newStreamHandler(metadata, sc.sysCtx, sc.streamConfig, sc.telemetry)
}

// headerContainerCache is a cache for the container ID of a CUDA event header.
// It's used to avoid retrieving the container ID for the same header multiple
// times. We use this specific implementation instead of MemoizeNoErrorUnsafe
// because by using it in this way we avoid allocations in the critical path of
// the consumer. A generic implementation is not an option here as it still
// generates an allocation.
type headerContainerCache struct {
	header            *gpuebpf.CudaEventHeader
	cachedContainerID string
	done              bool
	sc                *streamCollection
}

func (hcc *headerContainerCache) containerID() string {
	if hcc.done {
		return hcc.cachedContainerID
	}

	hcc.cachedContainerID = hcc.sc.getContainerID(hcc.header)
	hcc.done = true
	return hcc.cachedContainerID
}

func (sc *streamCollection) getHeaderContainerCache(header *gpuebpf.CudaEventHeader) headerContainerCache {
	return headerContainerCache{
		header: header,
		sc:     sc,
	}
}

func (sc *streamCollection) getContainerID(header *gpuebpf.CudaEventHeader) string {
	cgroup := unix.ByteSliceToString(header.Cgroup[:])
	containerID, err := cgroups.ContainerFilter("", cgroup)
	if err != nil {
		if logLimitProbe.ShouldLog() {
			log.Warnf("error getting container ID for cgroup %s: %s", cgroup, err)
		}

		sc.telemetry.missingContainers.Inc("error")
		return ""
	}

	if containerID == "" {
		sc.telemetry.missingContainers.Inc("missing")
	}

	return containerID
}

func (sc *streamCollection) allStreams() []*StreamHandler {
	var streams []*StreamHandler

	sc.streams.Range(func(_, value interface{}) bool {
		if stream, ok := value.(*StreamHandler); ok {
			streams = append(streams, stream)
		}
		return true
	})

	sc.globalStreams.Range(func(_, value interface{}) bool {
		if stream, ok := value.(*StreamHandler); ok {
			streams = append(streams, stream)
		}
		return true
	})

	return streams
}

func (sc *streamCollection) streamsCount() int {
	count := 0
	sc.streams.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func (sc *streamCollection) globalStreamsCount() int {
	count := 0
	sc.globalStreams.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func (sc *streamCollection) allStreamsCount() int {
	return sc.streamsCount() + sc.globalStreamsCount()
}

func (sc *streamCollection) markProcessStreamsAsEnded(pid uint32) {
	for _, handler := range sc.allStreams() {
		if handler.metadata.pid == pid {
			log.Debugf("Process %d ended, marking stream %d as ended", pid, handler.metadata.streamID)
			_ = handler.markEnd()
			sc.telemetry.finalizedProcesses.Inc()
		}
	}
}

// cleanHandlerMap cleans the handler map for a given stream collection.
func (sc *streamCollection) cleanHandlerMap(handlerMap *sync.Map, nowKtime int64) {
	handlerMap.Range(func(key, value interface{}) bool {
		if handler, ok := value.(*StreamHandler); ok {
			deleteReason := ""

			if handler.ended {
				deleteReason = "ended"
			} else if handler.isInactive(nowKtime, sc.streamConfig.Timeout) {
				deleteReason = "inactive"
			}

			if deleteReason != "" {
				handlerMap.Delete(key)
				handler.releasePoolResources()
				sc.telemetry.removedHandlers.Inc(handler.metadata.gpuUUID, deleteReason)
			}
		}
		return true
	})
}

// clean cleans the stream collection, removing inactive streams and marking processes as ended.
// nowKtime is the current kernel time, used to check if streams are inactive.
func (sc *streamCollection) clean(nowKtime int64) {
	sc.cleanHandlerMap(&sc.streams, nowKtime)
	sc.cleanHandlerMap(&sc.globalStreams, nowKtime)

	sc.telemetry.activeHandlers.Set(float64(sc.allStreamsCount()))
}

// String returns a human-readable representation of the streamCollection. Used for better debugging in tests
func (sc *streamCollection) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("streamCollection{streams=%d, globalStreams=%d}\n", sc.streamsCount(), sc.globalStreamsCount()))

	sc.streams.Range(func(key, value interface{}) bool {
		if k, ok := key.(streamKey); ok {
			if handler, ok := value.(*StreamHandler); ok {
				sb.WriteString(fmt.Sprintf("  [pid=%d, stream=%d]: %s\n", k.pid, k.stream, handler.String()))
			}
		}
		return true
	})

	sc.globalStreams.Range(func(key, value interface{}) bool {
		if k, ok := key.(globalStreamKey); ok {
			if handler, ok := value.(*StreamHandler); ok {
				sb.WriteString(fmt.Sprintf("  [pid=%d, gpu=%s (global)]: %s\n", k.pid, k.gpuUUID, handler.String()))
			}
		}
		return true
	})

	return sb.String()
}
