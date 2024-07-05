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
	eventHandler ddebpf.EventHandler
	requests     chan chan struct{}
	once         sync.Once
	closed       chan struct{}
}

func NewCudaEventConsumer(eventHandler ddebpf.EventHandler) *CudaEventConsumer {
	return &CudaEventConsumer{
		eventHandler: eventHandler,
		closed:       make(chan struct{}),
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

				ckl := (*CudaEventHeader)(unsafe.Pointer(&batchData.Data[0]))

				log.Infof("cuda kernel launch: %+v", ckl)

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
