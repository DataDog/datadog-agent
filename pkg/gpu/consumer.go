// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const telemetryEventErrorMismatch = "size_mismatch"
const telemetryEventErrorUnknownType = "unknown_type"
const telemetryEventTypeUnknown = "unknown"
const telemetryEventHeader = "header"

type nonGlobalStreamKey struct {
	pid    uint32
	stream uint64
}

// cudaEventConsumer is responsible for consuming CUDA events from the eBPF probe, and delivering them
// to the appropriate stream handler.
type cudaEventConsumer struct {
	eventHandler        ddebpf.EventHandler
	once                sync.Once
	closed              chan struct{}
	streamHandlers      map[streamKey]*StreamHandler
	nonGlobalStreamKeys map[nonGlobalStreamKey]streamKey // TODO: Move this to a separate class that deals with stream assignment
	wg                  sync.WaitGroup
	running             atomic.Bool
	sysCtx              *systemContext
	cfg                 *config.Config
	telemetry           *cudaEventConsumerTelemetry
	debugCollector      *eventCollector
}

type cudaEventConsumerTelemetry struct {
	activeHandlers     telemetry.Gauge
	removedHandlers    telemetry.Counter
	events             telemetry.Counter
	eventErrors        telemetry.Counter
	finalizedProcesses telemetry.Counter
	missingContainers  telemetry.Counter
	missingDevices     telemetry.Counter
}

// newCudaEventConsumer creates a new CUDA event consumer.
func newCudaEventConsumer(sysCtx *systemContext, eventHandler ddebpf.EventHandler, cfg *config.Config, telemetry telemetry.Component) *cudaEventConsumer {
	return &cudaEventConsumer{
		eventHandler:        eventHandler,
		closed:              make(chan struct{}),
		streamHandlers:      make(map[streamKey]*StreamHandler),
		nonGlobalStreamKeys: make(map[nonGlobalStreamKey]streamKey),
		cfg:                 cfg,
		sysCtx:              sysCtx,
		telemetry:           newCudaEventConsumerTelemetry(telemetry),
		debugCollector:      newEventCollector(),
	}
}

func newCudaEventConsumerTelemetry(tm telemetry.Component) *cudaEventConsumerTelemetry {
	subsystem := gpuTelemetryModule + "__consumer"

	return &cudaEventConsumerTelemetry{
		activeHandlers:     tm.NewGauge(subsystem, "active_handlers", nil, "Number of active stream handlers"),
		removedHandlers:    tm.NewCounter(subsystem, "removed_handlers", nil, "Number of removed stream handlers"),
		events:             tm.NewCounter(subsystem, "events", []string{"event_type"}, "Number of processed CUDA events received by the consumer"),
		eventErrors:        tm.NewCounter(subsystem, "events__errors", []string{"event_type", "error"}, "Number of CUDA events that couldn't be processed due to an error"),
		finalizedProcesses: tm.NewCounter(subsystem, "finalized_processes", nil, "Number of finalized processes"),
		missingContainers:  tm.NewCounter(subsystem, "missing_containers", []string{"reason"}, "Number of missing containers"),
		missingDevices:     tm.NewCounter(subsystem, "missing_devices", nil, "Number of failures to get GPU devices for a stream"),
	}
}

// Stop stops the CUDA event consumer.
func (c *cudaEventConsumer) Stop() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		close(c.closed)
	})
	c.wg.Wait()
}

// Start starts the CUDA event consumer.
func (c *cudaEventConsumer) Start() {
	if c == nil {
		return
	}
	health := health.RegisterLiveness("gpu-tracer-cuda-events")
	processMonitor := monitor.GetProcessMonitor()
	cleanupExit := processMonitor.SubscribeExit(c.handleProcessExit)

	c.wg.Add(1)
	go func() {
		c.running.Store(true)
		processSync := time.NewTicker(c.cfg.ScanProcessesInterval)

		defer func() {
			cleanupExit()
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			c.wg.Done()
			log.Trace("CUDA event consumer stopped")
			c.running.Store(false)
		}()

		dataChannel := c.eventHandler.DataChannel()
		lostChannel := c.eventHandler.LostChannel()
		for {
			select {
			case <-c.closed:
				return
			case <-health.C:
			case <-processSync.C:
				c.checkClosedProcesses()
				c.sysCtx.cleanupOldEntries()
			case batchData, ok := <-dataChannel:
				if !ok {
					return
				}

				dataLen := len(batchData.Data)
				if dataLen < gpuebpf.SizeofCudaEventHeader {
					log.Errorf("Not enough data to parse header, data size=%d, expecting at least %d", dataLen, gpuebpf.SizeofCudaEventHeader)
					c.telemetry.eventErrors.Inc(telemetryEventHeader, telemetryEventErrorMismatch)
					continue
				}

				header := (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&batchData.Data[0]))
				dataPtr := unsafe.Pointer(&batchData.Data[0])

				var err error
				eventType := gpuebpf.CudaEventType(header.Type)
				c.telemetry.events.Inc(eventType.String())
				if isStreamSpecificEvent(eventType) {
					err = c.handleStreamEvent(header, dataPtr, dataLen)
				} else {
					err = c.handleGlobalEvent(header, dataPtr, dataLen)
				}

				if err != nil {
					log.Errorf("Error processing CUDA event: %v", err)
				}

				batchData.Done()
			// lost events only occur when using perf buffers
			case _, ok := <-lostChannel:
				if !ok {
					return
				}
			}
		}
	}()
	log.Trace("CUDA event consumer started")
}

func isStreamSpecificEvent(eventType gpuebpf.CudaEventType) bool {
	return eventType != gpuebpf.CudaEventTypeSetDevice
}

func handleTypedEvent[K any](c *cudaEventConsumer, handler func(*K), eventType gpuebpf.CudaEventType, data unsafe.Pointer, dataLen int, expectedSize int) error {
	if dataLen != expectedSize {
		evStr := eventType.String()
		c.telemetry.eventErrors.Inc(evStr, telemetryEventErrorMismatch)
		return fmt.Errorf("Not enough data to parse %s event, data size=%d, expecting %d", evStr, dataLen, expectedSize)
	}

	typedEvent := (*K)(data)

	handler(typedEvent)
	c.debugCollector.tryRecordEvent(typedEvent)

	return nil
}

func (c *cudaEventConsumer) handleStreamEvent(header *gpuebpf.CudaEventHeader, data unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)
	streamHandler, err := c.getStreamHandler(header)

	if err != nil {
		return fmt.Errorf("error getting stream handler: %w", err)
	}

	switch eventType {
	case gpuebpf.CudaEventTypeKernelLaunch:
		return handleTypedEvent(c, streamHandler.handleKernelLaunch, eventType, data, dataLen, gpuebpf.SizeofCudaKernelLaunch)
	case gpuebpf.CudaEventTypeMemory:
		return handleTypedEvent(c, streamHandler.handleMemEvent, eventType, data, dataLen, gpuebpf.SizeofCudaMemEvent)
	case gpuebpf.CudaEventTypeSync:
		return handleTypedEvent(c, streamHandler.handleSync, eventType, data, dataLen, int(gpuebpf.SizeofCudaSync))
	default:
		c.telemetry.eventErrors.Inc(telemetryEventTypeUnknown, telemetryEventErrorUnknownType)
		return fmt.Errorf("Unknown event type: %d", header.Type)
	}
}

func getPidTidFromHeader(header *gpuebpf.CudaEventHeader) (uint32, uint32) {
	tid := uint32(header.Pid_tgid & 0xFFFFFFFF)
	pid := uint32(header.Pid_tgid >> 32)
	return pid, tid
}

func (c *cudaEventConsumer) handleSetDevice(csde *gpuebpf.CudaSetDeviceEvent) {
	pid, tid := getPidTidFromHeader(&csde.Header)
	c.sysCtx.setDeviceSelection(int(pid), int(tid), csde.Device)
}

func (c *cudaEventConsumer) handleGlobalEvent(header *gpuebpf.CudaEventHeader, data unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)
	switch eventType {
	case gpuebpf.CudaEventTypeSetDevice:
		return handleTypedEvent(c, c.handleSetDevice, eventType, data, dataLen, gpuebpf.SizeofCudaSetDeviceEvent)
	default:
		c.telemetry.eventErrors.Inc(telemetryEventTypeUnknown, telemetryEventErrorUnknownType)
		return fmt.Errorf("Unknown event type: %d", header.Type)
	}
}

func (c *cudaEventConsumer) handleProcessExit(pid uint32) {
	for key, handler := range c.streamHandlers {
		if key.pid == pid {
			log.Debugf("Process %d ended, marking stream %d as ended", pid, key.stream)
			// the probe is responsible for deleting the stream handler
			_ = handler.markEnd()
			c.telemetry.finalizedProcesses.Inc()
		}
	}
}

func (c *cudaEventConsumer) getStreamKey(header *gpuebpf.CudaEventHeader) (streamKey, error) {
	pid, tid := getPidTidFromHeader(header)

	var nonGlobalKey nonGlobalStreamKey
	if header.Stream_id != 0 {
		// Non-global stream, check if we have created it before
		nonGlobalKey.pid = pid
		nonGlobalKey.stream = header.Stream_id

		if key, ok := c.nonGlobalStreamKeys[nonGlobalKey]; ok {
			return key, nil
		}
	}

	cgroup := unix.ByteSliceToString(header.Cgroup[:])
	containerID, err := cgroups.ContainerFilter("", cgroup)
	if err != nil {
		// We don't want to return an error here, as we can still process the event without the container ID
		log.Warnf("error getting container ID for cgroup %s: %s", cgroup, err)
		c.telemetry.missingContainers.Inc("error")
	} else if containerID == "" {
		c.telemetry.missingContainers.Inc("missing")
	}

	key := streamKey{
		pid:         pid,
		stream:      header.Stream_id,
		gpuUUID:     "",
		containerID: containerID,
	}

	gpuDevice, err := c.sysCtx.getCurrentActiveGpuDevice(int(pid), int(tid), containerID)
	if err != nil {
		c.telemetry.missingDevices.Inc()
		return streamKey{}, fmt.Errorf("Error getting GPU device for process %d: %w", pid, err)
	}

	var ret nvml.Return
	key.gpuUUID, ret = gpuDevice.GetUUID()
	if ret != nvml.SUCCESS {
		return streamKey{}, fmt.Errorf("Error getting GPU UUID for process %d: %v", pid, nvml.ErrorString(ret))
	}

	if header.Stream_id != 0 {
		c.nonGlobalStreamKeys[nonGlobalKey] = key
	}

	return key, nil
}

func (c *cudaEventConsumer) getStreamHandler(header *gpuebpf.CudaEventHeader) (*StreamHandler, error) {
	key, err := c.getStreamKey(header)
	if err != nil {
		return nil, err
	}

	if _, ok := c.streamHandlers[key]; !ok {
		smVersion, ok := c.sysCtx.deviceSmVersions[key.gpuUUID]
		if !ok {
			if key.gpuUUID != "" {
				// Only warn when we have a device, otherwise it's expected to not find the SM version
				// if the device UUID is empty
				log.Warnf("SM version not found for device %s, using default", key.gpuUUID)
			}
			smVersion = noSmVersion
		}

		c.streamHandlers[key] = newStreamHandler(key.pid, key.containerID, smVersion, c.sysCtx)
		c.telemetry.activeHandlers.Set(float64(len(c.streamHandlers)))
	}

	return c.streamHandlers[key], nil
}

func (c *cudaEventConsumer) checkClosedProcesses() {
	seenPIDs := make(map[uint32]struct{})
	_ = kernel.WithAllProcs(c.cfg.ProcRoot, func(pid int) error {
		seenPIDs[uint32(pid)] = struct{}{}
		return nil
	})

	for key, handler := range c.streamHandlers {
		if _, ok := seenPIDs[key.pid]; !ok {
			log.Debugf("Process %d ended, marking stream %d as ended", key.pid, key.stream)
			_ = handler.markEnd()
		}
	}
}

func (c *cudaEventConsumer) cleanFinishedHandlers() {
	for key, handler := range c.streamHandlers {
		if handler.processEnded {
			delete(c.streamHandlers, key)

			if key.stream != 0 {
				delete(c.nonGlobalStreamKeys, nonGlobalStreamKey{pid: key.pid, stream: key.stream})
			}
		}
	}

	c.telemetry.activeHandlers.Set(float64(len(c.streamHandlers)))
}
