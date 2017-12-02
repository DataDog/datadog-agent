// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listener

import (
	"fmt"
	"log"
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// A UdpListener listens to bytes on a udp port and sends log lines to
// an output channel
type UdpListener struct {
	conn *net.UDPConn
	anl  *AbstractNetworkListener
}

// NewUdpListener returns an initialized NewUdpListener
func NewUdpListener(pp *pipeline.PipelineProvider, source *config.IntegrationConfigLogSource) (*AbstractNetworkListener, error) {
	log.Println("Starting UDP forwarder on port", source.Port)

	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", source.Port))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	udpListener := &UdpListener{
		conn: conn,
	}
	anl := &AbstractNetworkListener{
		listener: udpListener,
		pp:       pp,
		source:   source,
	}
	udpListener.anl = anl
	return anl, nil
}

// run lets the listener handle incoming udp messages
func (udpListener *UdpListener) run() {
	go udpListener.anl.handleConnection(udpListener.conn)
}

func (udpListener *UdpListener) readMessage(conn net.Conn, inBuf []byte) (int, error) {
	n, _, err := udpListener.conn.ReadFromUDP(inBuf)
	return n, err
}
