// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package packets

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const messageSeparator = byte('\n')

// Assembler merges multiple incoming datagrams into one "Packet" object to
// save space and make number of message in a single "Packet" more predictable
type Assembler struct {
	packet       *Packet
	packetLength int
	// assembled packets are pushed into this buffer
	packetsBuffer           *Buffer
	sharedPacketPoolManager *PoolManager[Packet]
	flushTimer              *time.Ticker
	closeChannel            chan struct{}
	doneChannel             chan struct{}
	packetSourceType        SourceType
	sync.Mutex
}

// NewAssembler creates a new Assembler instance using the specified flush duration, buffer and pool manager
func NewAssembler(flushTimer time.Duration, packetsBuffer *Buffer, sharedPacketPoolManager *PoolManager[Packet], packetSourceType SourceType) *Assembler {
	packetAssembler := &Assembler{
		// retrieve an available packet from the packet pool,
		// which will be pushed back by the server when processed.
		packet:                  sharedPacketPoolManager.Get(),
		sharedPacketPoolManager: sharedPacketPoolManager,
		packetsBuffer:           packetsBuffer,
		flushTimer:              time.NewTicker(flushTimer),
		packetSourceType:        packetSourceType,
		closeChannel:            make(chan struct{}),
		doneChannel:             make(chan struct{}),
	}
	go packetAssembler.flushLoop()
	return packetAssembler
}

func (p *Assembler) flushLoop() {
	for {
		select {
		case <-p.flushTimer.C:
			p.Lock()
			p.flush()
			p.Unlock()
		case <-p.closeChannel:
			p.Lock()
			p.flush()
			p.Unlock()
			close(p.doneChannel)
			return
		}
	}
}

// AddMessage adds a new dogstatsd message to the buffer
func (p *Assembler) AddMessage(message []byte) {
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

func (p *Assembler) flush() {
	if p.packetLength == 0 {
		return
	}
	p.packet.Contents = p.packet.Buffer[:p.packetLength]
	p.packet.Source = p.packetSourceType
	p.packetsBuffer.Append(p.packet)
	// retrieve an available packet from the packet pool,
	// which will be pushed back by the server when processed.
	p.packet = p.sharedPacketPoolManager.Get()
	p.packetLength = 0
}

// Close closes the packet assembler
func (p *Assembler) Close() {
	close(p.closeChannel)

	select {
	case <-p.doneChannel:
	case <-time.After(time.Second):
		log.Debug("Timeout flushing the dogstatsd assembler on stop")
	}
}
