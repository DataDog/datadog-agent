// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"sync"
	"time"
)

// packetsBuffer is a buffer of packets that will automatically flush to the given
// output channel when it is full or after a configurable duration.
type packetsBuffer struct {
	packets       Packets
	flushTimer    *time.Ticker
	bufferSize    uint
	outputChannel chan Packets
	closeChannel  chan struct{}
	m             sync.Mutex
}

func newPacketsBuffer(bufferSize uint, flushTimer time.Duration, outputChannel chan Packets) *packetsBuffer {
	pb := &packetsBuffer{
		bufferSize:    bufferSize,
		flushTimer:    time.NewTicker(flushTimer),
		outputChannel: outputChannel,
		packets:       make(Packets, 0, bufferSize),
		closeChannel:  make(chan struct{}),
	}
	go pb.flushLoop()
	return pb
}

func (pb *packetsBuffer) flushLoop() {
	for {
		select {
		case <-pb.flushTimer.C:
			pb.m.Lock()
			pb.flush()
			tlmPacketsBufferFlushedTimer.Inc()
			pb.m.Unlock()
		case <-pb.closeChannel:
			return
		}
	}
}

func (pb *packetsBuffer) append(packet *Packet) {
	pb.m.Lock()
	defer pb.m.Unlock()
	pb.packets = append(pb.packets, packet)
	if uint(len(pb.packets)) >= pb.bufferSize {
		pb.flush()
		tlmPacketsBufferFlushedFull.Inc()
	}
}

func (pb *packetsBuffer) flush() {
	if len(pb.packets) > 0 {
		t1 := time.Now()
		pb.outputChannel <- pb.packets
		t2 := time.Now()
		tlmListenerChannel.Observe(float64(t2.Sub(t1).Nanoseconds()))

		tlmPacketsChannelSize.Set(float64(len(pb.outputChannel)))
		pb.packets = make(Packets, 0, pb.bufferSize)
	}
}

func (pb *packetsBuffer) close() {
	close(pb.closeChannel)
}
