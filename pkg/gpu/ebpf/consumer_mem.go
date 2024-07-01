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

type CudaMemEventConsumer struct {
	eventHandler ddebpf.EventHandler
	requests     chan chan struct{}
	once         sync.Once
	closed       chan struct{}
}

func NewCudaMemEventConsumer(eventHandler ddebpf.EventHandler) *CudaMemEventConsumer {
	return &CudaMemEventConsumer{
		eventHandler: eventHandler,
		closed:       make(chan struct{}),
	}
}

func (c *CudaMemEventConsumer) FlushPending() {
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

func (c *CudaMemEventConsumer) Stop() {
	if c == nil {
		return
	}
	c.eventHandler.Stop()
	c.once.Do(func() {
		close(c.closed)
	})
}

func (c *CudaMemEventConsumer) Start() {
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

				log.Infof("Rx data\n")

				if len(batchData.Data) != SizeofCudaMemEvent {
					log.Errorf("unknown type received from perf buffer, skipping. data size=%d, expecting %d", len(batchData.Data), SizeofCudaKernelLaunch)
				}

				ckl := (*CudaMemEvent)(unsafe.Pointer(&batchData.Data[0]))

				log.Infof("cuda mem event launch: %+v", ckl)

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
