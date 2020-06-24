// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import (
	"sync"
	"time"
)

const messageSeparator = byte('\n')

// packetAssembler merges multiple incoming datagrams into one "Packet" object to
// save space and make number of message in a single "Packet" more predictable
type packetAssembler struct {
	packet       *Packet
	packetLength int
	// assembled packets are pushed into this buffer
	packetsBuffer    *packetsBuffer
	sharedPacketPool *PacketPool
	flushTimer       *time.Ticker
	closeChannel     chan struct{}
	sync.Mutex
}

func newPacketAssembler(flushTimer time.Duration, packetsBuffer *packetsBuffer, sharedPacketPool *PacketPool) *packetAssembler {
	packetAssembler := &packetAssembler{
		// retrieve an available packet from the packet pool,
		// which will be pushed back by the server when processed.
		packet:           sharedPacketPool.Get(),
		sharedPacketPool: sharedPacketPool,
		packetsBuffer:    packetsBuffer,
		flushTimer:       time.NewTicker(flushTimer),
		closeChannel:     make(chan struct{}),
	}
	go packetAssembler.flushLoop()
	return packetAssembler
}

func (p *packetAssembler) flushLoop() {
	for {
		select {
		case <-p.flushTimer.C:
			p.Lock()
			p.flush()
			p.Unlock()
		case <-p.closeChannel:
			return
		}
	}
}

func (p *packetAssembler) addMessage(message []byte) {
	p.Lock()
	if p.packetLength == 0 {
		p.packetLength = copy(p.packet.buffer, message)
	} else if len(p.packet.buffer) >= len(message)+p.packetLength+1 {
		p.packet.buffer[p.packetLength] = messageSeparator
		n := copy(p.packet.buffer[p.packetLength+1:], message)
		p.packetLength += n + 1
	} else {
		p.flush()
		p.packetLength = copy(p.packet.buffer, message)
	}
	p.Unlock()
}

func (p *packetAssembler) flush() {
	if p.packetLength == 0 {
		return
	}
	p.packet.Contents = p.packet.buffer[:p.packetLength]
	p.packetsBuffer.append(p.packet)
	// retrieve an available packet from the packet pool,
	// which will be pushed back by the server when processed.
	p.packet = p.sharedPacketPool.Get()
	p.packetLength = 0
}

func (p *packetAssembler) close() {
	p.Lock()
	close(p.closeChannel)
	p.Unlock()
}
