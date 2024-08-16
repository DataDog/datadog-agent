// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"sync"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type CudaEventConsumer struct {
	eventHandler   ddebpf.EventHandler
	requests       chan chan struct{}
	once           sync.Once
	closed         chan struct{}
	streamHandlers map[StreamKey]*StreamHandler
}

func NewCudaEventConsumer(eventHandler ddebpf.EventHandler) *CudaEventConsumer {
	return &CudaEventConsumer{
		eventHandler:   eventHandler,
		closed:         make(chan struct{}),
		streamHandlers: make(map[StreamKey]*StreamHandler),
	}
}

func (c *CudaEventConsumer) FlushPending() {
	if c == nil {
		return
	}

	select {
	case <-c.closed:
		return
	default:
	}

	wait := make(chan struct{})
	select {
	case <-c.closed:
	case c.requests <- wait:
		<-wait
	}
}

func (c *CudaEventConsumer) Stop() {
	if c == nil {
		return
	}
	c.eventHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *CudaEventConsumer) Start() {
	if c == nil {
		return
	}
	health := health.RegisterLiveness("gpu-tracer-cuda-events")
	processMonitor := monitor.GetProcessMonitor()
	processMonitor.SubscribeExit(c.handleProcessExit)

	go func() {
		defer func() {
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
		}()

		dataChannel := c.eventHandler.DataChannel()
		lostChannel := c.eventHandler.LostChannel()
		for {
			select {
			case <-c.closed:
				return
			case <-health.C:
			case batchData, ok := <-dataChannel:
				if !ok {
					return
				}

				if len(batchData.Data) < gpuebpf.SizeofCudaEventHeader {
					log.Errorf("Not enough data to parse header, data size=%d, expecting at least %d", len(batchData.Data), gpuebpf.SizeofCudaEventHeader)
					continue
				}

				header := (*gpuebpf.CudaEventHeader)(unsafe.Pointer(&batchData.Data[0]))

				pid := uint32(header.Pid_tgid >> 32)
				tid := uint32(header.Pid_tgid)
				streamKey := StreamKey{Pid: pid, Tid: tid, Stream: header.Stream_id}

				if _, ok := c.streamHandlers[streamKey]; !ok {
					c.streamHandlers[streamKey] = newStreamHandler()
				}

				switch header.Type {
				case gpuebpf.CudaEventTypeKernelLaunch:
					if len(batchData.Data) != gpuebpf.SizeofCudaKernelLaunch {
						log.Errorf("Not enough data to parse kernel launch event, data size=%d, expecting at least %d", len(batchData.Data), gpuebpf.SizeofCudaKernelLaunch)
						continue
					}
					ckl := (*gpuebpf.CudaKernelLaunch)(unsafe.Pointer(&batchData.Data[0]))
					c.streamHandlers[streamKey].handleKernelLaunch(ckl)
				case gpuebpf.CudaEventTypeMemory:
					if len(batchData.Data) != gpuebpf.SizeofCudaMemEvent {
						log.Errorf("Not enough data to parse memory event, data size=%d, expecting at least %d", len(batchData.Data), gpuebpf.SizeofCudaMemEvent)
						continue
					}
					cme := (*gpuebpf.CudaMemEvent)(unsafe.Pointer(&batchData.Data[0]))
					c.streamHandlers[streamKey].handleMemEvent(cme)
				case gpuebpf.CudaEventTypeSync:
					if len(batchData.Data) != gpuebpf.SizeofCudaSync {
						log.Errorf("Not enough data to parse sync event, data size=%d, expecting at least %d", len(batchData.Data), gpuebpf.SizeofCudaSync)
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
}

func (c *CudaEventConsumer) handleProcessExit(pid uint32) {
	for key, handler := range c.streamHandlers {
		if key.Pid == pid {
			log.Debugf("Process %d ended, marking stream as ended", pid)
			_ = handler.markProcessEnded()
		}
	}
}
