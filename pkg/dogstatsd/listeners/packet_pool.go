// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import "sync"

// PacketPool wraps the sync.Pool class for *Packet type.
// It allows to avoid allocating one object per packet.
type PacketPool struct {
	pool sync.Pool
}

// NewPacketPool creates a new pool with a specified buffer size
func NewPacketPool(bufferSize int) *PacketPool {
	return &PacketPool{
		pool: sync.Pool{
			New: func() interface{} {
				packet := &Packet{
					buffer: make([]byte, bufferSize),
					Origin: NoOrigin,
				}
				packet.Contents = packet.buffer[0:0]
				return packet
			},
		},
	}
}

// Get gets a packet object read for use.
func (p *PacketPool) Get() *Packet {
	return p.pool.Get().(*Packet)
}

// Put resets the packet origin and puts it back in the pool.
func (p *PacketPool) Put(packet *Packet) {
	if packet.Origin != NoOrigin {
		packet.Origin = NoOrigin
	}
	p.pool.Put(packet)
}
