// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listener

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
)

type mockListener struct {
	packets packet.PacketsChannel
}

func newMock() MockComponent {
	return &mockListener{
		packets: make(chan *packet.SnmpPacket, 100),
	}
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
