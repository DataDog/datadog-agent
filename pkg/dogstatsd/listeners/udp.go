// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import (
	"fmt"
	"net"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var udpTelemetry = newListenerTelemetry("udp", "UDP")

// UDPListener implements the StatsdListener interface for UDP protocol.
// It listens to a given UDP address and sends back packets ready to be
// processed.
// Origin detection is not implemented for UDP.
type UDPListener struct {
	conn          *net.UDPConn
	packetManager *packetManager
}

// NewUDPListener returns an idle UDP Statsd listener
func NewUDPListener(packetOut chan Packets, sharedPacketPool *PacketPool) (*UDPListener, error) {
	var err error
	var url string

	if config.Datadog.GetBool("dogstatsd_non_local_traffic") == true {
		// Listen to all network interfaces
		url = fmt.Sprintf(":%d", config.Datadog.GetInt("dogstatsd_port"))
	} else {
		url = net.JoinHostPort(config.Datadog.GetString("bind_host"), config.Datadog.GetString("dogstatsd_port"))
	}

	addr, err := net.ResolveUDPAddr("udp", url)
	if err != nil {
		return nil, fmt.Errorf("could not resolve udp addr: %s", err)
	}
	conn, err := net.ListenUDP("udp", addr)

	if rcvbuf := config.Datadog.GetInt("dogstatsd_so_rcvbuf"); rcvbuf != 0 {
		if err := conn.SetReadBuffer(rcvbuf); err != nil {
			return nil, fmt.Errorf("could not set socket rcvbuf: %s", err)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	listener := &UDPListener{
		conn:          conn,
		packetManager: newPacketManagerFromConfig(packetOut, sharedPacketPool),
	}
	log.Debugf("dogstatsd-udp: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDPListener) Listen() {
	log.Infof("dogstatsd-udp: starting to listen on %s", l.conn.LocalAddr())
	for {
		n, _, err := l.conn.ReadFrom(l.packetManager.buffer)
		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd-udp: error reading packet: %v", err)
			udpTelemetry.onReadError()
			continue
		}
		udpTelemetry.onReadSuccess(n)

		// packetAssembler merges multiple packets together and sends them when its buffer is full
		l.packetManager.packetAssembler.addMessage(l.packetManager.buffer[:n])
	}
}

// Stop closes the UDP connection and stops listening
func (l *UDPListener) Stop() {
	l.packetManager.close()
	l.conn.Close()
}
