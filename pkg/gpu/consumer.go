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

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
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

const processExitChannelSize = 100

// cudaEventConsumer is responsible for consuming CUDA events from the eBPF probe, and delivering them
// to the appropriate stream handler.
type cudaEventConsumer struct {
	eventHandler       ddebpf.EventHandler
	once               sync.Once
	closed             chan struct{}
	processExitChannel chan uint32
	streamHandlers     *streamCollection
	wg                 sync.WaitGroup
	running            atomic.Bool
	sysCtx             *systemContext
	cfg                *config.Config
	telemetry          *cudaEventConsumerTelemetry
	debugCollector     *eventCollector
	ringFlusher        perf.Flusher
}

type cudaEventConsumerTelemetry struct {
	events              telemetry.Counter
	eventErrors         telemetry.Counter
	eventCounterByType  map[gpuebpf.CudaEventType]telemetry.SimpleCounter
	droppedProcessExits telemetry.Counter
}

// newCudaEventConsumer creates a new CUDA event consumer.
func newCudaEventConsumer(sysCtx *systemContext, streamHandlers *streamCollection, eventHandler ddebpf.EventHandler, ringFlusher perf.Flusher, cfg *config.Config, telemetry telemetry.Component) *cudaEventConsumer {
	return &cudaEventConsumer{
		eventHandler:       eventHandler,
		closed:             make(chan struct{}),
		processExitChannel: make(chan uint32, processExitChannelSize),
		cfg:                cfg,
		sysCtx:             sysCtx,
		streamHandlers:     streamHandlers,
		telemetry:          newCudaEventConsumerTelemetry(telemetry),
		debugCollector:     newEventCollector(),
		ringFlusher:        ringFlusher,
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
		events:              events,
		eventErrors:         tm.NewCounter(subsystem, "events__errors", []string{"event_type", "error"}, "Number of CUDA events that couldn't be processed due to an error"),
		eventCounterByType:  eventCounterByType,
		droppedProcessExits: tm.NewCounter(subsystem, "dropped_process_exits", nil, "Number of process exits events that were dropped"),
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

	// Send events to the main event loop asynchronously, so that all process handling is done in the same goroutine.
	// That way we avoid race conditions between the process monitor and the event consumer.
	cleanupExit := processMonitor.SubscribeExit(func(pid uint32) {
		select {
		case c.processExitChannel <- pid:
		default:
			// If the channel is full, we don't want to block the main event
			// loop, so we just drop the event. The process exit will be caught
			// later with the full process scan. We increase a telemetry metric to track this.
			c.telemetry.droppedProcessExits.Inc()
		}
	})

	c.wg.Add(1)
	go func() {
		c.running.Store(true)
		processSync := time.NewTicker(c.cfg.ScanProcessesInterval)
		ringBufferFlush := time.NewTicker(c.cfg.RingBufferFlushInterval)

		defer func() {
			cleanupExit()
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			close(c.processExitChannel)
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
			case <-ringBufferFlush.C:
				c.ringFlusher.Flush()
			case pid, ok := <-c.processExitChannel:
				if !ok {
					return
				}
				c.handleProcessExit(pid)
			case batchData, ok := <-dataChannel:
				if !ok {
					return
				}

				dataLen := len(batchData.Data)
				if dataLen == 0 {
					// This was a flush event, with no data to process so we can skip it
					// with no warning log.
					continue
				}

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

func isStreamSpecificEvent(et gpuebpf.CudaEventType) bool {
	return et != gpuebpf.CudaEventTypeSetDevice &&
		et != gpuebpf.CudaEventTypeVisibleDevicesSet &&
		et != gpuebpf.CudaEventTypeSyncDevice
}

func (c *cudaEventConsumer) handleEvent(header *gpuebpf.CudaEventHeader, dataPtr unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)
	c.telemetry.eventCounterByType[eventType].Inc()
	if isStreamSpecificEvent(eventType) {
		return c.handleStreamEvent(header, dataPtr, dataLen)
	}
	return c.handleGlobalEvent(header, dataPtr, dataLen)
}

func handleTypedEventErr[K any](c *cudaEventConsumer, handler func(*K) error, eventType gpuebpf.CudaEventType, data unsafe.Pointer, dataLen int, expectedSize int) error {
	if dataLen != expectedSize {
		evStr := eventType.String()
		c.telemetry.eventErrors.Inc(evStr, telemetryEventErrorMismatch)
		return fmt.Errorf("not enough data to parse %s event, data size=%d, expecting %d", evStr, dataLen, expectedSize)
	}

	typedEvent := (*K)(data)
	if err := handler(typedEvent); err != nil {
		return err
	}
	c.debugCollector.tryRecordEvent(typedEvent)
	return nil
}

func handleTypedEvent[K any](c *cudaEventConsumer, handler func(*K), eventType gpuebpf.CudaEventType, data unsafe.Pointer, dataLen int, expectedSize int) error {
	return handleTypedEventErr(c, func(k *K) error {
		handler(k)
		return nil
	}, eventType, data, dataLen, expectedSize)
}

func (c *cudaEventConsumer) handleStreamEvent(header *gpuebpf.CudaEventHeader, data unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)
	streamHandler, err := c.streamHandlers.getStream(header)

	if err != nil {
		if logLimitProbe.ShouldLog() {
			log.Warnf("error getting stream handler for stream id %d: %v", header.Stream_id, err)
		}

		return err
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

func (c *cudaEventConsumer) handleVisibleDevicesSet(vds *gpuebpf.CudaVisibleDevicesSetEvent) {
	pid, _ := getPidTidFromHeader(&vds.Header)

	c.sysCtx.setUpdatedVisibleDevicesEnvVar(int(pid), unix.ByteSliceToString(vds.Devices[:]))
}

func (c *cudaEventConsumer) handleDeviceSync(event *gpuebpf.CudaSetDeviceEvent) error {
	streams, err := c.streamHandlers.getActiveDeviceStreams(&event.Header)
	if err != nil {
		return fmt.Errorf("cannot get streams for the active device: %w", err)
	}

	// we reproduce device sync behavior by dispatching a synthetic stream sync
	// event for all the streams on the TID's active device
	evt := gpuebpf.CudaSync{Header: event.Header}
	for _, stream := range streams {
		evt.Header.Stream_id = stream.metadata.streamID
		stream.handleSync(&evt)
	}

	return nil
}

func (c *cudaEventConsumer) handleGlobalEvent(header *gpuebpf.CudaEventHeader, data unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)
	switch eventType {
	case gpuebpf.CudaEventTypeSetDevice:
		return handleTypedEvent(c, c.handleSetDevice, eventType, data, dataLen, gpuebpf.SizeofCudaSetDeviceEvent)
	case gpuebpf.CudaEventTypeVisibleDevicesSet:
		return handleTypedEvent(c, c.handleVisibleDevicesSet, eventType, data, dataLen, gpuebpf.SizeofCudaVisibleDevicesSetEvent)
	case gpuebpf.CudaEventTypeSyncDevice:
		return handleTypedEventErr(c, c.handleDeviceSync, eventType, data, dataLen, gpuebpf.SizeofCudaSyncDeviceEvent)
	default:
		c.telemetry.eventErrors.Inc(telemetryEventTypeUnknown, telemetryEventErrorUnknownType)
		return fmt.Errorf("unknown event type: %d", header.Type)
	}
}

// handleProcessExit is called when a process exits. It marks all streams for that process as ended. Should only be called
// from the main event loop.
func (c *cudaEventConsumer) handleProcessExit(pid uint32) {
	c.streamHandlers.markProcessStreamsAsEnded(pid)
}

func (c *cudaEventConsumer) checkClosedProcesses() {
	seenPIDs := make(map[uint32]struct{})
	_ = kernel.WithAllProcs(c.cfg.ProcRoot, func(pid int) error {
		seenPIDs[uint32(pid)] = struct{}{}
		return nil
	})

	for _, handler := range c.streamHandlers.allStreams() {
		if _, ok := seenPIDs[handler.metadata.pid]; !ok {
			c.streamHandlers.markProcessStreamsAsEnded(handler.metadata.pid)
		}
	}
}
