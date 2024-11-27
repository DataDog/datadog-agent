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

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// cudaEventConsumer is responsible for consuming CUDA events from the eBPF probe, and delivering them
// to the appropriate stream handler.
type cudaEventConsumer struct {
	eventHandler   ddebpf.EventHandler
	once           sync.Once
	closed         chan struct{}
	streamHandlers map[streamKey]*StreamHandler
	wg             sync.WaitGroup
	running        atomic.Bool
	sysCtx         *systemContext
	cfg            *config.Config
}

// newCudaEventConsumer creates a new CUDA event consumer.
func newCudaEventConsumer(sysCtx *systemContext, eventHandler ddebpf.EventHandler, cfg *config.Config) *cudaEventConsumer {
	return &cudaEventConsumer{
		eventHandler:   eventHandler,
		closed:         make(chan struct{}),
		streamHandlers: make(map[streamKey]*StreamHandler),
		cfg:            cfg,
		sysCtx:         sysCtx,
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
		processSync := time.NewTicker(c.cfg.ScanTerminatedProcessesInterval)

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
					continue
				}

				header := (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&batchData.Data[0]))
				dataPtr := unsafe.Pointer(&batchData.Data[0])

				var err error
				if isStreamSpecificEvent(gpuebpf.CudaEventType(header.Type)) {
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

func (c *cudaEventConsumer) handleStreamEvent(header *gpuebpf.CudaEventHeader, data unsafe.Pointer, dataLen int) error {
	streamHandler := c.getStreamHandler(header)

	switch header.Type {
	case gpuebpf.CudaEventTypeKernelLaunch:
		if dataLen != gpuebpf.SizeofCudaKernelLaunch {
			return fmt.Errorf("Not enough data to parse kernel launch event, data size=%d, expecting %d", dataLen, gpuebpf.SizeofCudaKernelLaunch)

		}
		streamHandler.handleKernelLaunch((*gpuebpf.CudaKernelLaunch)(data))
	case gpuebpf.CudaEventTypeMemory:
		if dataLen != gpuebpf.SizeofCudaMemEvent {
			return fmt.Errorf("Not enough data to parse memory event, data size=%d, expecting %d", dataLen, gpuebpf.SizeofCudaMemEvent)

		}
		streamHandler.handleMemEvent((*gpuebpf.CudaMemEvent)(data))
	case gpuebpf.CudaEventTypeSync:
		if dataLen != gpuebpf.SizeofCudaSync {
			return fmt.Errorf("Not enough data to parse sync event, data size=%d, expecting %d", dataLen, gpuebpf.SizeofCudaSync)

		}
		streamHandler.handleSync((*gpuebpf.CudaSync)(data))
	default:
		return fmt.Errorf("Unknown event type: %d", header.Type)
	}

	return nil
}

func getPidTidFromHeader(header *gpuebpf.CudaEventHeader) (uint32, uint32) {
	tid := uint32(header.Pid_tgid & 0xFFFFFFFF)
	pid := uint32(header.Pid_tgid >> 32)
	return pid, tid
}

func (c *cudaEventConsumer) handleGlobalEvent(header *gpuebpf.CudaEventHeader, data unsafe.Pointer, dataLen int) error {
	switch header.Type {
	case gpuebpf.CudaEventTypeSetDevice:
		if dataLen != gpuebpf.SizeofCudaSetDeviceEvent {
			return fmt.Errorf("Not enough data to parse set device event, data size=%d, expecting %d", dataLen, gpuebpf.SizeofCudaSetDeviceEvent)

		}
		csde := (*gpuebpf.CudaSetDeviceEvent)(data)

		pid, tid := getPidTidFromHeader(header)
		c.sysCtx.setDeviceSelection(int(pid), int(tid), csde.Device)
	default:
		return fmt.Errorf("Unknown event type: %d", header.Type)
	}

	return nil
}

func (c *cudaEventConsumer) handleProcessExit(pid uint32) {
	for key, handler := range c.streamHandlers {
		if key.pid == pid {
			log.Debugf("Process %d ended, marking stream %d as ended", pid, key.stream)
			// the probe is responsible for deleting the stream handler
			_ = handler.markEnd()
		}
	}
}

func (c *cudaEventConsumer) getStreamKey(header *gpuebpf.CudaEventHeader) streamKey {
	pid, tid := getPidTidFromHeader(header)

	key := streamKey{
		pid:     pid,
		stream:  header.Stream_id,
		gpuUUID: "",
	}

	// Try to get the GPU device if we can, but do not fail if we can't as we want to report
	// the data even if we can't get the GPU UUID
	gpuDevice, err := c.sysCtx.getCurrentActiveGpuDevice(int(pid), int(tid))
	if err != nil {
		log.Warnf("Error getting GPU device for process %d: %v", pid, err)
	} else {
		var ret nvml.Return
		key.gpuUUID, ret = gpuDevice.GetUUID()
		if ret != nvml.SUCCESS {
			log.Warnf("Error getting GPU UUID for process %d: %v", pid, nvml.ErrorString(ret))
		}
	}

	return key
}

func (c *cudaEventConsumer) getStreamHandler(header *gpuebpf.CudaEventHeader) *StreamHandler {
	key := c.getStreamKey(header)
	if _, ok := c.streamHandlers[key]; !ok {
		cgroup := unix.ByteSliceToString(header.Cgroup[:])
		containerID, err := cgroups.ContainerFilter("", cgroup)
		if err != nil {
			// We don't want to return an error here, as we can still process the event without the container ID
			log.Errorf("error getting container ID for cgroup %s: %s", cgroup, err)
		}
		c.streamHandlers[key] = newStreamHandler(key.pid, containerID, c.sysCtx)
	}

	return c.streamHandlers[key]
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
		}
	}
}
