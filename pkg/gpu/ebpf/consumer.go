// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"sync"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type CudaEventConsumer struct {
	eventHandler   ddebpf.EventHandler
	requests       chan chan struct{}
	once           sync.Once
	closed         chan struct{}
	streamHandlers map[streamKey]*streamHandler
}

func NewCudaEventConsumer(eventHandler ddebpf.EventHandler) *CudaEventConsumer {
	return &CudaEventConsumer{
		eventHandler:   eventHandler,
		closed:         make(chan struct{}),
		streamHandlers: make(map[streamKey]*streamHandler),
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
	health := health.RegisterLiveness("gpu-tracer-cuda-kernel-launch")

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

				if len(batchData.Data) < SizeofCudaEventHeader {
					log.Errorf("Not enough data to parse header, data size=%d, expecting at least %d", len(batchData.Data), SizeofCudaEventHeader)
					continue
				}

				header := (*CudaEventHeader)(unsafe.Pointer(&batchData.Data[0]))

				pid := uint32(header.Pid_tgid >> 32)
				tid := uint32(header.Pid_tgid)
				streamKey := streamKey{pid: pid, tid: tid, stream: header.Stream_id}

				if _, ok := c.streamHandlers[streamKey]; !ok {
					c.streamHandlers[streamKey] = &streamHandler{}
				}

				switch header.Type {
				case CudaEventTypeKernelLaunch:
					if len(batchData.Data) != SizeofCudaKernelLaunch {
						log.Errorf("Not enough data to parse kernel launch event, data size=%d, expecting at least %d", len(batchData.Data), SizeofCudaKernelLaunch)
						continue
					}
					ckl := (*CudaKernelLaunch)(unsafe.Pointer(&batchData.Data[0]))
					c.streamHandlers[streamKey].handleKernelLaunch(ckl)
				case CudaEventTypeMemory:
					if len(batchData.Data) != SizeofCudaMemEvent {
						log.Errorf("Not enough data to parse memory event, data size=%d, expecting at least %d", len(batchData.Data), SizeofCudaMemEvent)
						continue
					}
					cme := (*CudaMemEvent)(unsafe.Pointer(&batchData.Data[0]))
					c.streamHandlers[streamKey].handleMemEvent(cme)
				case CudaEventTypeSync:
					if len(batchData.Data) != SizeofCudaSync {
						log.Errorf("Not enough data to parse sync event, data size=%d, expecting at least %d", len(batchData.Data), SizeofCudaSync)
						continue
					}
					cs := (*CudaSync)(unsafe.Pointer(&batchData.Data[0]))
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
