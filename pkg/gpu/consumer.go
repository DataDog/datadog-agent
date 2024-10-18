// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// cudaEventConsumer is responsible for consuming CUDA events from the eBPF probe, and delivering them
// to the appropriate stream handler.
type cudaEventConsumer struct {
	eventHandler   ddebpf.EventHandler
	once           sync.Once
	closed         chan struct{}
	streamHandlers map[model.StreamKey]*StreamHandler
	wg             sync.WaitGroup
	running        atomic.Bool
	cfg            *Config
}

// NewCudaEventConsumer creates a new CUDA event consumer.
func NewCudaEventConsumer(eventHandler ddebpf.EventHandler, cfg *Config) *cudaEventConsumer {
	return &cudaEventConsumer{
		eventHandler:   eventHandler,
		closed:         make(chan struct{}),
		streamHandlers: make(map[model.StreamKey]*StreamHandler),
		cfg:            cfg,
	}
}

// Stop stops the CUDA event consumer.
func (c *cudaEventConsumer) Stop() {
	if c == nil {
		return
	}
	c.eventHandler.Stop()
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

				pid := uint32(header.Pid_tgid >> 32)
				streamKey := model.StreamKey{Pid: pid, Stream: header.Stream_id}

				if _, ok := c.streamHandlers[streamKey]; !ok {
					c.streamHandlers[streamKey] = newStreamHandler()
				}

				switch header.Type {
				case gpuebpf.CudaEventTypeKernelLaunch:
					if dataLen != gpuebpf.SizeofCudaKernelLaunch {
						log.Errorf("Not enough data to parse kernel launch event, data size=%d, expecting %d", dataLen, gpuebpf.SizeofCudaKernelLaunch)
						continue
					}
					ckl := (*gpuebpf.CudaKernelLaunch)(unsafe.Pointer(&batchData.Data[0]))
					c.streamHandlers[streamKey].handleKernelLaunch(ckl)
				case gpuebpf.CudaEventTypeMemory:
					if dataLen != gpuebpf.SizeofCudaMemEvent {
						log.Errorf("Not enough data to parse memory event, data size=%d, expecting %d", dataLen, gpuebpf.SizeofCudaMemEvent)
						continue
					}
					cme := (*gpuebpf.CudaMemEvent)(unsafe.Pointer(&batchData.Data[0]))
					c.streamHandlers[streamKey].handleMemEvent(cme)
				case gpuebpf.CudaEventTypeSync:
					if dataLen != gpuebpf.SizeofCudaSync {
						log.Errorf("Not enough data to parse sync event, data size=%d, expecting %d", dataLen, gpuebpf.SizeofCudaSync)
						continue
					}
					cs := (*gpuebpf.CudaSync)(unsafe.Pointer(&batchData.Data[0]))
					c.streamHandlers[streamKey].handleSync(cs)
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

func (c *cudaEventConsumer) handleProcessExit(pid uint32) {
	for key, handler := range c.streamHandlers {
		if key.Pid == pid {
			log.Debugf("Process %d ended, marking stream %d as ended", pid, key.Stream)
			// the probe is responsible for deleting the stream handler
			_ = handler.markEnd()
		}
	}
}

func (c *cudaEventConsumer) checkClosedProcesses() {
	seenPIDs := make(map[uint32]struct{})
	_ = kernel.WithAllProcs("/proc", func(pid int) error {
		seenPIDs[uint32(pid)] = struct{}{}
		return nil
	})

	for key, handler := range c.streamHandlers {
		if _, ok := seenPIDs[key.Pid]; !ok {
			log.Debugf("Process %d ended, marking stream %d as ended", key.Pid, key.Stream)
			_ = handler.markEnd()
		}
	}
}

func (c *cudaEventConsumer) cleanupHandlersMarkedFinished() {
	for key, handler := range c.streamHandlers {
		if handler.processEnded {
			delete(c.streamHandlers, key)
		}
	}
}
