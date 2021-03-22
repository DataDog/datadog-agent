// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmBufferFlushedTimer = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_timer",
		nil, "Count of packets buffer flush triggered by the timer")
	tlmBufferFlushedFull = telemetry.NewCounter("dogstatsd", "packets_buffer_flush_full",
		nil, "Count of packets buffer flush triggered because the buffer is full")
	tlmPacketsChannelSize = telemetry.NewGauge("dogstatsd", "packets_channel_size",
		nil, "Number of packets in the packets channel")
)

// Buffer is a buffer of packets that will automatically flush to the given
// output channel when it is full or after a configurable duration.
type Buffer struct {
	packets       Packets
	flushTimer    *time.Ticker
	bufferSize    uint
	outputChannel chan Packets
	closeChannel  chan struct{}
	m             sync.Mutex
}

func NewBuffer(bufferSize uint, flushTimer time.Duration, outputChannel chan Packets) *Buffer {
	pb := &Buffer{
		bufferSize:    bufferSize,
		flushTimer:    time.NewTicker(flushTimer),
		outputChannel: outputChannel,
		packets:       make(Packets, 0, bufferSize),
		closeChannel:  make(chan struct{}),
	}
	go pb.flushLoop()
	return pb
}

func (pb *Buffer) flushLoop() {
	for {
		select {
		case <-pb.flushTimer.C:
			pb.m.Lock()
			pb.flush()
			tlmBufferFlushedTimer.Inc()
			pb.m.Unlock()
		case <-pb.closeChannel:
			return
		}
	}
}

func (pb *Buffer) Append(packet *Packet) {
	pb.m.Lock()
	defer pb.m.Unlock()
	pb.packets = append(pb.packets, packet)
	if uint(len(pb.packets)) >= pb.bufferSize {
		pb.flush()
		tlmBufferFlushedFull.Inc()
	}
}

func (pb *Buffer) flush() {
	if len(pb.packets) > 0 {
		pb.outputChannel <- pb.packets
		tlmPacketsChannelSize.Set(float64(len(pb.outputChannel)))
		pb.packets = make(Packets, 0, pb.bufferSize)
	}
}

func (pb *Buffer) Close() {
	close(pb.closeChannel)
}
