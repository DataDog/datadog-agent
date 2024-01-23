// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"net"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// UDSDatagramListener implements the StatsdListener interface for Unix Domain (datagrams)
type UDSDatagramListener struct {
	UDSListener

	conn *net.UnixConn
}

// NewUDSDatagramListener returns an idle UDS datagram Statsd listener
func NewUDSDatagramListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager, sharedOobPoolManager *packets.PoolManager, cfg config.Reader, capture replay.Component) (*UDSDatagramListener, error) {
	panic("not called")
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSDatagramListener) Listen() {
	panic("not called")
}

func (l *UDSDatagramListener) listen() {
	panic("not called")
}

// Stop closes the UDS connection and stops listening
func (l *UDSDatagramListener) Stop() {
	panic("not called")
}
