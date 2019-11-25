// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package listeners

import (
	"sync"
	"time"
)

// packetBuffer is a buffer of packet that will automatically flush to configurable channel
// when it is full or after a configurable duration
type packetBuffer struct {
	packets       Packets
	flushTimer    *time.Ticker
	bufferSize    uint
	outputChannel chan Packets
	closeChannel  chan struct{}
	m             sync.Mutex
}

func newPacketBuffer(bufferSize uint, flushTimer time.Duration, outputChannel chan Packets) *packetBuffer {
	pb := &packetBuffer{
		bufferSize:    bufferSize,
		flushTimer:    time.NewTicker(flushTimer),
		outputChannel: outputChannel,
		packets:       make(Packets, 0, bufferSize),
		closeChannel:  make(chan struct{}),
	}
	go pb.flushLoop()
	return pb
}

func (pb *packetBuffer) flushLoop() {
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

func (pb *packetBuffer) append(packet *Packet) {
	pb.m.Lock()
	defer pb.m.Unlock()
	pb.packets = append(pb.packets, packet)
	if uint(len(pb.packets)) == pb.bufferSize {
		pb.flush()
	}
}

func (pb *packetBuffer) flush() {
	if len(pb.packets) > 0 {
		pb.outputChannel <- pb.packets
		pb.packets = make(Packets, 0, pb.bufferSize)
	}
}

func (pb *packetBuffer) close() {
	close(pb.closeChannel)
}
