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
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const telemetryEventErrorMismatch = "size_mismatch"
const telemetryEventErrorUnknownType = "unknown_type"
const telemetryEventErrorHandlerError = "handler_error"
const telemetryEventTypeUnknown = "unknown"
const telemetryEventHeader = "header"

const processExitChannelSize = 100

// cudaEventConsumer is responsible for consuming CUDA events from the eBPF probe, and delivering them
// to the appropriate stream handler.
type cudaEventConsumer struct {
	deps               cudaEventConsumerDependencies
	once               sync.Once
	closed             chan struct{}
	processExitChannel chan uint32
	wg                 sync.WaitGroup
	running            atomic.Bool
	telemetry          *cudaEventConsumerTelemetry
	debugCollector     *eventCollector
}

type cudaEventConsumerTelemetry struct {
	events              telemetry.Counter
	eventErrors         telemetry.Counter
	streamGetErrors     telemetry.Counter
	eventCounterByType  map[gpuebpf.CudaEventType]telemetry.SimpleCounter
	droppedProcessExits telemetry.Counter
}

type cudaEventConsumerDependencies struct {
	// sysCtx is the system context
	sysCtx *systemContext
	// cfg is the configuration
	cfg *config.Config
	// telemetry is the telemetry component
	telemetry telemetry.Component
	// processMonitor allows subscribing to process start and exit events
	processMonitor uprobes.ProcessMonitor
	// streamHandlers holds all the stream handlers for the different streams
	streamHandlers *streamCollection
	// eventHandler is the event handler for eBPF events
	eventHandler ddebpf.EventHandler
	// ringFlusher allows flushing the ring buffer
	ringFlusher perf.Flusher
}

// newCudaEventConsumer creates a new CUDA event consumer.
func newCudaEventConsumer(deps cudaEventConsumerDependencies) *cudaEventConsumer {
	return &cudaEventConsumer{
		deps:               deps,
		closed:             make(chan struct{}),
		processExitChannel: make(chan uint32, processExitChannelSize),
		telemetry:          newCudaEventConsumerTelemetry(deps.telemetry),
		debugCollector:     newEventCollector(),
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
		streamGetErrors:     tm.NewCounter(subsystem, "stream_get_errors", nil, "Number of errors when getting a stream handler"),
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
		c.wg.Wait()
		// Close the channel after both goroutines have exited to avoid
		// sending on a closed channel if checkClosedProcesses is still running
		close(c.processExitChannel)
	})
}

// Start starts the CUDA event consumer.
func (c *cudaEventConsumer) Start() {
	if c == nil {
		return
	}

	c.startProcessChecker()
	c.startMainLoop()
	log.Trace("CUDA event consumer started")
}

// startProcessChecker starts a goroutine that periodically checks for closed processes
// and sends their PIDs through the processExitChannel. This avoids blocking the main
// consumer loop with process scanning.
func (c *cudaEventConsumer) startProcessChecker() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		processSync := time.NewTicker(c.deps.cfg.ScanProcessesInterval)
		defer processSync.Stop()

		for {
			select {
			case <-c.closed:
				return
			case <-processSync.C:
				c.checkClosedProcesses()
			}
		}
	}()
}

// startMainLoop starts the main consumer loop goroutine that processes CUDA events
// and handles various control signals.
func (c *cudaEventConsumer) startMainLoop() {
	c.wg.Add(1)
	go func() {
		c.running.Store(true)
		health := health.RegisterLiveness(consts.GpuConsumerHealthName)

		// Send events to the main event loop asynchronously, so that all process handling is done in the same goroutine.
		// That way we avoid race conditions between the process monitor and the event consumer.
		cleanupExit := c.deps.processMonitor.SubscribeExit(func(pid uint32) {
			select {
			case c.processExitChannel <- pid:
			default:
				// If the channel is full, we don't want to block the main event
				// loop, so we just drop the event. The process exit will be caught
				// later with the full process scan. We increase a telemetry metric to track this.
				c.telemetry.droppedProcessExits.Inc()
			}
		})

		ringBufferFlush := time.NewTicker(c.deps.cfg.RingBufferFlushInterval)

		defer func() {
			cleanupExit()
			ringBufferFlush.Stop()
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			c.wg.Done()
			log.Trace("CUDA event consumer stopped")
			c.running.Store(false)
		}()

		dataChannel := c.deps.eventHandler.DataChannel()
		lostChannel := c.deps.eventHandler.LostChannel()
		for {
			select {
			case <-c.closed:
				return
			case <-health.C:
			case <-ringBufferFlush.C:
				c.deps.ringFlusher.Flush()
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
}

func isStreamSpecificEvent(et gpuebpf.CudaEventType) bool {
	return et != gpuebpf.CudaEventTypeSetDevice &&
		et != gpuebpf.CudaEventTypeVisibleDevicesSet &&
		et != gpuebpf.CudaEventTypeSyncDevice
}

func (c *cudaEventConsumer) handleEvent(header *gpuebpf.CudaEventHeader, dataPtr unsafe.Pointer, dataLen int) error {
	eventType := gpuebpf.CudaEventType(header.Type)

	counter, ok := c.telemetry.eventCounterByType[eventType]
	if !ok {
		c.telemetry.eventErrors.Inc(telemetryEventTypeUnknown, telemetryEventErrorUnknownType)
		return fmt.Errorf("unknown event type: %d", header.Type)
	}
	counter.Inc()

	var err error
	if isStreamSpecificEvent(eventType) {
		err = c.handleStreamEvent(header, dataPtr, dataLen)
	} else {
		err = c.handleGlobalEvent(header, dataPtr, dataLen)
	}

	if err != nil {
		c.telemetry.eventErrors.Inc(eventType.String(), telemetryEventErrorHandlerError)
	}
	return err
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
	streamHandler, err := c.deps.streamHandlers.getStream(header)

	if err != nil {
		if logLimitProbe.ShouldLog() {
			log.Warnf("error getting stream handler for stream id %d: %v", header.Stream_id, err)
		}
		c.telemetry.streamGetErrors.Inc()
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
	c.deps.sysCtx.setDeviceSelection(int(pid), int(tid), csde.Device)
}

func (c *cudaEventConsumer) handleVisibleDevicesSet(vds *gpuebpf.CudaVisibleDevicesSetEvent) {
	pid, _ := getPidTidFromHeader(&vds.Header)

	c.deps.sysCtx.setUpdatedVisibleDevicesEnvVar(int(pid), unix.ByteSliceToString(vds.Devices[:]))
}

func (c *cudaEventConsumer) handleDeviceSync(event *gpuebpf.CudaSetDeviceEvent) error {
	streams, err := c.deps.streamHandlers.getActiveDeviceStreams(&event.Header)
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
	c.deps.streamHandlers.markProcessStreamsAsEnded(pid)
}

func (c *cudaEventConsumer) checkClosedProcesses() {
	seenPIDs := make(map[uint32]struct{})
	_ = kernel.WithAllProcs(c.deps.cfg.ProcRoot, func(pid int) error {
		seenPIDs[uint32(pid)] = struct{}{}
		return nil
	})

	// Track which PIDs we've already sent to avoid duplicates in this scan
	sentPIDs := make(map[uint32]struct{})
	for _, handler := range c.deps.streamHandlers.allStreams() {
		pid := handler.metadata.pid
		if _, ok := seenPIDs[pid]; !ok {
			if _, sent := sentPIDs[pid]; !sent {
				select {
				case c.processExitChannel <- pid:
					sentPIDs[pid] = struct{}{}
				default:
					// Channel is full, the process exit will be caught in the next scan
					c.telemetry.droppedProcessExits.Inc()
				}
			}
		}
	}
}
