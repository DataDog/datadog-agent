// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
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
	pool *ddsync.TypedPool[Packet]
	// telemetry
	tlmEnabled       bool
	packetsTelemetry *TelemetryStore
}

var usedByTestTelemetry = false

// NewPool creates a new pool with a specified buffer size
func NewPool(bufferSize int, packetsTelemetry *TelemetryStore) *Pool {
	return &Pool{
		pool: ddsync.NewTypedPool(func() *Packet {
			packet := &Packet{
				Buffer: make([]byte, bufferSize),
				Origin: NoOrigin,
			}
			packet.Contents = packet.Buffer[0:0]
			return packet
		}),
		// telemetry
		tlmEnabled:       usedByTestTelemetry || utils.IsTelemetryEnabled(pkgconfigsetup.Datadog()),
		packetsTelemetry: packetsTelemetry,
	}
}

// Get gets a Packet object read for use.
func (p *Pool) Get() *Packet {
	if p.tlmEnabled {
		p.packetsTelemetry.tlmPoolGet.Inc()
		p.packetsTelemetry.tlmPool.Inc()
	}
	return p.pool.Get()
}

// Put resets the Packet origin and puts it back in the pool.
func (p *Pool) Put(packet *Packet) {
	if packet == nil {
		return
	}

	if packet.Origin != NoOrigin {
		packet.Origin = NoOrigin
	}
	if p.tlmEnabled {
		p.packetsTelemetry.tlmPoolPut.Inc()
		p.packetsTelemetry.tlmPool.Dec()
	}
	p.pool.Put(packet)
}
