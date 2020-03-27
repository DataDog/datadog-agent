// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmPacketsBufferDropped = telemetry.NewCounter("dogstatsd", "packets_buffer_dropped",
		nil, "Count of dropped packets")
	tlmPacketsBufferProcessed = telemetry.NewCounter("dogstatsd", "packets_buffer_processed",
		nil, "Count of processed packets")
)

// packetsBuffer is a buffer of packets that will automatically flush to configurable channel
// when it is full or after a configurable duration
type packetsBuffer struct {
	packets       Packets
	flushTimer    *time.Ticker
	bufferSize    uint
	outputChannel chan Packets
	closeChannel  chan struct{}
	packetPool    *PacketPool
	m             sync.Mutex
	tlmEnabled    bool
}

func newPacketsBuffer(packetPool *PacketPool, bufferSize uint, flushTimer time.Duration, outputChannel chan Packets) *packetsBuffer {
	pb := &packetsBuffer{
		bufferSize:    bufferSize,
		flushTimer:    time.NewTicker(flushTimer),
		outputChannel: outputChannel,
		packets:       make(Packets, 0, bufferSize),
		closeChannel:  make(chan struct{}),
		packetPool:    packetPool,
		tlmEnabled:    telemetry.IsEnabled(),
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
	}
}

func (pb *packetsBuffer) flush() {
	if len(pb.packets) > 0 {
		select {
		case pb.outputChannel <- pb.packets:
			if pb.tlmEnabled {
				tlmPacketsBufferProcessed.Inc()
			}
		default:
			if pb.tlmEnabled {
				tlmPacketsBufferDropped.Inc()
			}
			for _, packet := range pb.packets {
				pb.packetPool.Put(packet)
			}
		}
		pb.packets = make(Packets, 0, pb.bufferSize)
	}
}

func (pb *packetsBuffer) close() {
	close(pb.closeChannel)
}
