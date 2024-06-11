// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// Pool wraps the sync.Pool class for *Packet type.
// It allows to avoid allocating one object per packet.
//
// Caution: as objects get reused, byte slices extracted from
// packet.Contents will change when the object is reused. You
// need to hold on to the object until you extracted all the
// information and parsed it into strings/float/int.
//
// Strings extracted with `string(Contents[n:m]) don't share the
// origin []byte storage, so they will be unaffected.
type Pool struct {
	pool sync.Pool
	// telemetry
	tlmEnabled       bool
	packetsTelemetry *TelemetryStore
}

// NewPool creates a new pool with a specified buffer size
func NewPool(bufferSize int, packetsTelemetry *TelemetryStore) *Pool {
	return &Pool{
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
		tlmEnabled:       utils.IsTelemetryEnabled(config.Datadog()),
		packetsTelemetry: packetsTelemetry,
	}
}

// Get gets a Packet object read for use.
func (p *Pool) Get() interface{} {
	if p.tlmEnabled {
		p.packetsTelemetry.tlmPoolGet.Inc()
		p.packetsTelemetry.tlmPool.Inc()
	}
	return p.pool.Get()
}

// Put resets the Packet origin and puts it back in the pool.
func (p *Pool) Put(x interface{}) {
	if x == nil {
		return
	}

	// we don't really need the assertion of the user is sensible
	packet, ok := x.(*Packet)
	if ok && packet.Origin != NoOrigin {
		packet.Origin = NoOrigin
	}
	if p.tlmEnabled {
		p.packetsTelemetry.tlmPoolPut.Inc()
		p.packetsTelemetry.tlmPool.Dec()
	}
	p.pool.Put(packet)
}
