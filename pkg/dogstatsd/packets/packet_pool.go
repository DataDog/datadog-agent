// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
)

var (
	tlmPacketPoolGet = telemetry.NewCounter("dogstatsd", "packet_pool_get",
		nil, "Count of get done in the packet pool")
	tlmPacketPoolPut = telemetry.NewCounter("dogstatsd", "packet_pool_put",
		nil, "Count of put done in the packet pool")
	tlmPacketPool = telemetry.NewGauge("dogstatsd", "packet_pool",
		nil, "Usage of the packet pool in dogstatsd")
)

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
	// telemetry
	tlmEnabled bool
}

// NewPacketPool creates a new pool with a specified buffer size
func NewPacketPool(bufferSize int) *PacketPool {
	return &PacketPool{
		pool: sync.Pool{
			New: func() interface{} {
				packet := &Packet{
					Buffer: make([]byte, bufferSize),
					Origin: NoOrigin,
				}
				packet.Contents = packet.Buffer[0:0]
				return packet
			},
		},
		// telemetry
		tlmEnabled: telemetry_utils.IsEnabled(),
	}
}

// Get gets a Packet object read for use.
func (p *PacketPool) Get() interface{} {
	if p.tlmEnabled {
		tlmPacketPoolGet.Inc()
		tlmPacketPool.Inc()
	}
	return p.pool.Get()
}

// Put resets the Packet origin and puts it back in the pool.
func (p *PacketPool) Put(x interface{}) {
	if p == nil {
		return
	}

	packet, ok := x.(*Packet)
	if ok && packet.Origin != NoOrigin {
		packet.Origin = NoOrigin
	}
	if p.tlmEnabled {
		tlmPacketPoolPut.Inc()
		tlmPacketPool.Dec()
	}
	p.pool.Put(packet)
}
