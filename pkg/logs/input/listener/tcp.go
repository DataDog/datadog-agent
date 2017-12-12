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

// A TCPListener listens to bytes on a tcp connection and sends log lines to
// an output channel
type TCPListener struct {
	listener net.Listener
	anl      *AbstractNetworkListener
}

// NewTCPListener returns an initialized NewTCPListener
func NewTCPListener(pp pipeline.Provider, source *config.IntegrationConfigLogSource) (*AbstractNetworkListener, error) {
	log.Println("Starting TCP forwarder on port", source.Port)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", source.Port))
	if err != nil {
		return nil, err
	}
	tcpListener := &TCPListener{
		listener: listener,
	}
	anl := &AbstractNetworkListener{
		listener: tcpListener,
		pp:       pp,
		source:   source,
	}
	tcpListener.anl = anl
	return anl, nil
}

// run lets the listener handle incoming tcp connections
func (tcpListener *TCPListener) run() {
	for {
		conn, err := tcpListener.listener.Accept()
		if err != nil {
			log.Println("Can't listen:", err)
			return
		}
		go tcpListener.anl.handleConnection(conn)
	}
}

func (tcpListener *TCPListener) readMessage(conn net.Conn, inBuf []byte) (int, error) {
	return conn.Read(inBuf)
}
