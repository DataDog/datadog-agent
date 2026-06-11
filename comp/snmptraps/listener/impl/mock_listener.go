// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listenerimpl

import (
	listener "github.com/DataDog/datadog-agent/comp/snmptraps/listener/def"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
)

// NewMockListener creates a mock listener for use in tests.
func NewMockListener() (listener.MockComponent, listener.Component) {
	l := &mockListener{
		packets: make(chan *packet.SnmpPacket, 100),
	}
	return l, l
}

type mockListener struct {
	packets packet.PacketsChannel
}

// Packets returns the packets channel to which the listener publishes.
func (t *mockListener) Packets() packet.PacketsChannel {
	return t.packets
}

// Start is a no-op
func (t *mockListener) Start() error {
	return nil
}

// Stop is a no-op
func (t *mockListener) Stop() {}

func (t *mockListener) Send(p *packet.SnmpPacket) {
	t.packets <- p
}
