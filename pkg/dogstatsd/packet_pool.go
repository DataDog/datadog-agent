// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import "sync"

// PacketPool wraps the sync.Pool class for *Packet type.
// It allows to avoid allocating one object per packet.
//
// Caution: as objects get reused, byte slices extracted from
// packet.Contents will change when the object is reused. You
// need to hold on to the object until you extracted all the
// information and parsed it into strings/float/int.
//
// Strings extracted with `string(Contents[n:m]) don't share the
// origin []byte storage, so they will be unaffected.
type PacketPool struct {
	pool sync.Pool
}

// NewPacketPool creates a new pool with a specified buffer size
func NewPacketPool(bufferSize int) *PacketPool {
	pool := &PacketPool{pool: sync.Pool{}}
	pool.pool.New = func() interface{} {
		packet := &Packet{
			buffer: make([]byte, bufferSize),
			Origin: NoOrigin,
		}
		packet.Contents = packet.buffer[0:0]
		packet.pool = pool
		return packet
	}
	return pool
}

// Get gets a Packet object read for use.
func (p *PacketPool) Get() *Packet {
	return p.pool.Get().(*Packet)
}

// Put resets the Packet origin and puts it back in the pool.
func (p *PacketPool) Put(packet *Packet) {
	if packet.Origin != NoOrigin {
		packet.Origin = NoOrigin
	}
	p.pool.Put(packet)
}
