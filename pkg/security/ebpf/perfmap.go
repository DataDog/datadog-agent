// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package ebpf

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	bpflib "github.com/iovisor/gobpf/elf"
)

var (
	defaultBufferLength  = 256
	defaultLostEventSize = 10
)

// PerfMapHandler represents the handler for events pushed into
// a perf event array. It gets passed a buffer holding an event.
type PerfMapHandler func([]byte)

// PerfMapLostHandler represents the handler for lost events of
// a perf event array. It gets passed the number of lost events.
type PerfMapLostHandler func(uint64)

// PerfMap represents an eBPF perf event array
type PerfMap struct {
	*bpflib.PerfMap

	handler      func([]byte)
	lostHandler  func(uint64)
	eventChannel chan []byte
	lostChannel  chan uint64

	// https://github.com/golang/go/issues/36606
	padding       int32
	receivedCount int64
	lostCount     int64
}

// Start the goroutine handling the received and losts events
// of the perf event array
func (p *PerfMap) Start() error {
	p.PollStart()

	go func() {
		for {
			select {
			case data, ok := <-p.eventChannel:
				if !ok {
					log.Infof("Exiting closed connections polling")
					return
				}
				atomic.AddInt64(&p.receivedCount, 1)

				p.handler(data)
			case lostCount, ok := <-p.lostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&p.lostCount, int64(lostCount))

				if p.lostHandler != nil {
					p.lostHandler(lostCount)
				}
			}
		}
	}()

	return nil
}

// Stop the received and lost events handlers
func (p *PerfMap) Stop() {
	p.PollStop()
}
