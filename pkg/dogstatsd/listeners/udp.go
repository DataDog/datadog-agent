// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"
	"net"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	udpExpvar = expvar.NewMap("dogstatsd-udp")
)

// UDPListener implements the StatsdListener interface for UDP protocol.
// It listens to a given UDP address and sends back packets ready to be
// processed.
// Origin detection is not implemented for UDP.
type UDPListener struct {
	conn       net.PacketConn
	packetPool *PacketPool
	packetOut  chan *Packet
}

// NewUDPListener returns an idle UDP Statsd listener
func NewUDPListener(packetOut chan *Packet, packetPool *PacketPool) (*UDPListener, error) {
	var conn net.PacketConn
	var err error
	var url string

	if config.Datadog.GetBool("dogstatsd_non_local_traffic") == true {
		// Listen to all network interfaces
		url = fmt.Sprintf(":%d", config.Datadog.GetInt("dogstatsd_port"))
	} else {
		url = fmt.Sprintf("localhost:%d", config.Datadog.GetInt("dogstatsd_port"))
	}

	conn, err = net.ListenPacket("udp", url)

	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	listener := &UDPListener{
		packetOut:  packetOut,
		packetPool: packetPool,
		conn:       conn,
	}
	log.Debugf("dogstatsd-udp: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDPListener) Listen() {
	log.Infof("dogstatsd-udp: starting to listen on %s", l.conn.LocalAddr())
	for {
		packet := l.packetPool.Get()
		n, _, err := l.conn.ReadFrom(packet.buffer)
		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd-udp: error reading packet: %v", err)
			udpExpvar.Add("PacketReadingErrors", 1)
			continue
		}

		packet.Contents = packet.buffer[:n]
		l.packetOut <- packet
	}
}

// Stop closes the UDP connection and stops listening
func (l *UDPListener) Stop() {
	l.conn.Close()
}
