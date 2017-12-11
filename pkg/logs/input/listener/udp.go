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

// A UDPListener listens for UDP connections and delegates the work to connHandler
type UDPListener struct {
	conn        *net.UDPConn
	connHandler *ConnectionHandler
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pp pipeline.Provider, source *config.IntegrationConfigLogSource) (*UDPListener, error) {
	log.Println("Starting UDP forwarder on port", source.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", source.Port))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	connHandler := &ConnectionHandler{
		pp:     pp,
		source: source,
	}
	return &UDPListener{
		conn:        conn,
		connHandler: connHandler,
	}, nil
}

// Start listens to UDP connections on another routine
func (udpListener *UDPListener) Start() {
	go udpListener.run()
}

// run lets connHandler handle new UDP connections
func (udpListener *UDPListener) run() {
	go udpListener.connHandler.handleConnection(udpListener.conn)
}
