// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const telemetryEventErrorMismatch = "size_mismatch"
const telemetryEventErrorUnknownType = "unknown_type"
const telemetryEventTypeUnknown = "unknown"
const telemetryEventHeader = "header"

// cudaEventConsumer is responsible for consuming CUDA events from the eBPF probe, and delivering them
// to the appropriate stream handler.
type cudaEventConsumer struct {
	eventHandler   ddebpf.EventHandler
	once           sync.Once
	closed         chan struct{}
	streamHandlers *streamCollection
	wg             sync.WaitGroup
	running        atomic.Bool
	sysCtx         *systemContext
	cfg            *config.Config
	telemetry      *cudaEventConsumerTelemetry
	debugCollector *eventCollector
}

type cudaEventConsumerTelemetry struct {
	events             telemetry.Counter
	eventErrors        telemetry.Counter
	eventCounterByType map[gpuebpf.CudaEventType]telemetry.SimpleCounter
}

// newCudaEventConsumer creates a new CUDA event consumer.
func newCudaEventConsumer(sysCtx *systemContext, streamHandlers *streamCollection, eventHandler ddebpf.EventHandler, cfg *config.Config, telemetry telemetry.Component) *cudaEventConsumer {
	return &cudaEventConsumer{
		eventHandler:   eventHandler,
		closed:         make(chan struct{}),
		cfg:            cfg,
		sysCtx:         sysCtx,
		streamHandlers: streamHandlers,
		telemetry:      newCudaEventConsumerTelemetry(telemetry),
		debugCollector: newEventCollector(),
	}
}

func newCudaEventConsumerTelemetry(tm telemetry.Component) *cudaEventConsumerTelemetry {
	subsystem := consts.GpuTelemetryModule + "__consumer"

	events := tm.NewCounter(subsystem, "events", []string{"event_type"}, "Number of processed CUDA events received by the consumer")
	eventCounterByType := make(map[gpuebpf.CudaEventType]telemetry.SimpleCounter)

	for i := 0; i < int(gpuebpf.CudaEventTypeCount); i++ {
		eventType := gpuebpf.CudaEventType(i)
		eventCounterByType[eventType] = events.WithTags(map[string]string{"event_type": eventType.String()})
	}

	return &cudaEventConsumerTelemetry{
		events:             events,
		eventErrors:        tm.NewCounter(subsystem, "events__errors", []string{"event_type", "error"}, "Number of CUDA events that couldn't be processed due to an error"),
		eventCounterByType: eventCounterByType,
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
	health := health.RegisterLiveness(consts.GpuConsumerHealthName)
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
				c.sysCtx.cleanOld()
			case batchData, ok := <-dataChannel:
				if !ok {
					return
				}

				dataLen := len(batchData.Data)
				if dataLen < gpuebpf.SizeofCudaEventHeader {
					if logLimitProbe.ShouldLog() {
						log.Warnf("Not enough data to parse header, data size=%d, expecting at least %d", dataLen, gpuebpf.SizeofCudaEventHeader)
					}
					c.telemetry.eventErrors.Inc(telemetryEventHeader, telemetryEventErrorMismatch)
					continue
				}

				header := (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&batchData.Data[0]))
				dataPtr := unsafe.Pointer(&batchData.Data[0])
				err := c.handleEvent(header, dataPtr, dataLen)

				if err != nil && logLimitProbe.ShouldLog() {
					log.Warnf("Error processing CUDA event: %v", err)
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

func (c *cudaEventConsumer) handleEvent(header *gpuebpf.CudaEventHeader, dataPtr unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)
	c.telemetry.eventCounterByType[eventType].Inc()
	if isStreamSpecificEvent(eventType) {
		return c.handleStreamEvent(header, dataPtr, dataLen)
	}
	return c.handleGlobalEvent(header, dataPtr, dataLen)
}

func handleTypedEvent[K any](c *cudaEventConsumer, handler func(*K), eventType gpuebpf.CudaEventType, data unsafe.Pointer, dataLen int, expectedSize int) error {
	if dataLen != expectedSize {
		evStr := eventType.String()
		c.telemetry.eventErrors.Inc(evStr, telemetryEventErrorMismatch)
		return fmt.Errorf("not enough data to parse %s event, data size=%d, expecting %d", evStr, dataLen, expectedSize)
	}

	typedEvent := (*K)(data)

	handler(typedEvent)
	c.debugCollector.tryRecordEvent(typedEvent)

	return nil
}

func (c *cudaEventConsumer) handleStreamEvent(header *gpuebpf.CudaEventHeader, data unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)
	streamHandler, err := c.streamHandlers.getStream(header)

	if err != nil {
		return fmt.Errorf("error getting stream handler for stream id: %d : %w ", header.Stream_id, err)
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
		return fmt.Errorf("unknown event type: %d", header.Type)
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
		return fmt.Errorf("unknown event type: %d", header.Type)
	}
}

func (c *cudaEventConsumer) handleProcessExit(pid uint32) {
	c.streamHandlers.markProcessStreamsAsEnded(pid)
}

func (c *cudaEventConsumer) checkClosedProcesses() {
	seenPIDs := make(map[uint32]struct{})
	_ = kernel.WithAllProcs(c.cfg.ProcRoot, func(pid int) error {
		seenPIDs[uint32(pid)] = struct{}{}
		return nil
	})

	for handler := range c.streamHandlers.allStreams() {
		if _, ok := seenPIDs[handler.metadata.pid]; !ok {
			c.streamHandlers.markProcessStreamsAsEnded(handler.metadata.pid)
		}
	}
}
