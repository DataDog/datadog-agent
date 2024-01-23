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

// UDSStreamListener implements the StatsdListener interface for Unix Domain (streams)
type UDSStreamListener struct {
	UDSListener
	connTracker *ConnectionTracker
	conn        *net.UnixListener
}

// NewUDSStreamListener returns an idle UDS datagram Statsd listener
func NewUDSStreamListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager, sharedOobPacketPoolManager *packets.PoolManager, cfg config.Reader, capture replay.Component) (*UDSStreamListener, error) {
	panic("not called")
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSStreamListener) Listen() {
	panic("not called")
}

func (l *UDSStreamListener) listen() {
	panic("not called")
}

// Stop closes the UDS connection and stops listening
func (l *UDSStreamListener) Stop() {
	panic("not called")
}
