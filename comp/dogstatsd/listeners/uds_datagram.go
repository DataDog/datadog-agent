// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// UDSDatagramListener implements the StatsdListener interface for Unix Domain (datagrams)
type UDSDatagramListener struct {
	UDSListener

	conn *net.UnixConn
}

// NewUDSDatagramListener returns an idle UDS datagram Statsd listener
func NewUDSDatagramListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager, sharedOobPoolManager *packets.PoolManager, cfg config.Reader, capture replay.Component, wmeta optional.Option[workloadmeta.Component], pidMap pidmap.Component) (*UDSDatagramListener, error) {
	socketPath := cfg.GetString("dogstatsd_socket")
	transport := "unixgram"

	address, err := setupSocketBeforeListen(socketPath, transport)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUnixgram(transport, address)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	err = setSocketWriteOnly(socketPath)
	if err != nil {
		return nil, err
	}

	l, err := NewUDSListener(packetOut, sharedPacketPoolManager, sharedOobPoolManager, cfg, capture, transport, wmeta, pidMap)
	if err != nil {
		return nil, err
	}

	listener := &UDSDatagramListener{
		UDSListener: *l,
		conn:        conn,
	}

	// Setup origin detection early
	l.OriginDetection, err = setupUnixConn(conn, l.OriginDetection, l.config)
	if err != nil {
		return nil, err
	}

	log.Infof("dogstatsd-uds: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSDatagramListener) Listen() {
	l.listenWg.Add(1)
	go func() {
		defer l.listenWg.Done()
		l.listen()
	}()
}

func (l *UDSDatagramListener) listen() {
	log.Infof("dogstatsd-uds: starting to listen on %s", l.conn.LocalAddr())
	_ = l.handleConnection(l.conn, func(conn *net.UnixConn) error {
		return conn.Close()
	})
}

// Stop closes the UDS connection and stops listening
func (l *UDSDatagramListener) Stop() {
	err := l.conn.Close()
	if err != nil {
		log.Errorf("dogstatsd-uds: error closing connection: %s", err)
	}
	l.UDSListener.Stop()
}
