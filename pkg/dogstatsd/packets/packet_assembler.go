// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package packets

import (
	"sync"
	"time"
)

const messageSeparator = byte('\n')

// PacketAssembler merges multiple incoming datagrams into one "Packet" object to
// save space and make number of message in a single "Packet" more predictable
type PacketAssembler struct {
	packet       *Packet
	packetLength int
	// assembled packets are pushed into this buffer
	packetsBuffer           *PacketsBuffer
	sharedPacketPoolManager *PoolManager
	flushTimer              *time.Ticker
	closeChannel            chan struct{}
	packetSourceType        SourceType
	sync.Mutex
}

func NewPacketAssembler(flushTimer time.Duration, packetsBuffer *PacketsBuffer, sharedPacketPoolManager *PoolManager, packetSourceType SourceType) *PacketAssembler {
	packetAssembler := &PacketAssembler{
		// retrieve an available packet from the packet pool,
		// which will be pushed back by the server when processed.
		packet:                  sharedPacketPoolManager.Get().(*Packet),
		sharedPacketPoolManager: sharedPacketPoolManager,
		packetsBuffer:           packetsBuffer,
		flushTimer:              time.NewTicker(flushTimer),
		packetSourceType:        packetSourceType,
		closeChannel:            make(chan struct{}),
	}
	go packetAssembler.flushLoop()
	return packetAssembler
}

func (p *PacketAssembler) flushLoop() {
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

func (p *PacketAssembler) AddMessage(message []byte) {
	p.Lock()
	if p.packetLength == 0 {
		p.packetLength = copy(p.packet.Buffer, message)
	} else if len(p.packet.Buffer) >= len(message)+p.packetLength+1 {
		p.packet.Buffer[p.packetLength] = messageSeparator
		n := copy(p.packet.Buffer[p.packetLength+1:], message)
		p.packetLength += n + 1
	} else {
		p.flush()
		p.packetLength = copy(p.packet.Buffer, message)
	}
	p.Unlock()
}

func (p *PacketAssembler) flush() {
	if p.packetLength == 0 {
		return
	}
	p.packet.Contents = p.packet.Buffer[:p.packetLength]
	p.packet.Source = p.packetSourceType
	p.packetsBuffer.Append(p.packet)
	// retrieve an available packet from the packet pool,
	// which will be pushed back by the server when processed.
	p.packet = p.sharedPacketPoolManager.Get().(*Packet)
	p.packetLength = 0
}

func (p *PacketAssembler) Close() {
	p.Lock()
	close(p.closeChannel)
	p.Unlock()
}
