// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"sync"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// A UDPListener listens for UDP connections and delegates the work to connHandler
type UDPListener struct {
	port        int
	conn        *net.UDPConn
	connHandler *ConnectionHandler
	mu          *sync.Mutex
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pp pipeline.Provider, source *config.IntegrationConfigLogSource) (*UDPListener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", source.Port))
	if err != nil {
		source.Tracker.TrackError(err)
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		source.Tracker.TrackError(err)
		return nil, err
	}
	source.Tracker.TrackSuccess()
	connHandler := NewConnectionHandler(pp, source)
	return &UDPListener{
		port:        source.Port,
		conn:        conn,
		connHandler: connHandler,
		mu:          &sync.Mutex{},
	}, nil
}

// Start listens to UDP connections on another routine
func (l *UDPListener) Start() {
	log.Info("Starting UDP forwarder on port ", l.port)
	go l.run()
}

// Stop closes the UDP connection
func (l *UDPListener) Stop() {
	log.Info("Stopping UDP forwarder on port ", l.port)
	l.mu.Lock()
	l.connHandler.Stop()
	l.mu.Unlock()
}

// run lets connHandler handle new UDP connections
func (l *UDPListener) run() {
	l.mu.Lock()
	l.connHandler.HandleConnection(l.conn)
	l.mu.Unlock()
}
