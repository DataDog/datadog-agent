// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// A UDPListener listens for UDP connections and delegates the work to connHandler
type UDPListener struct {
	conn        *net.UDPConn
	connHandler *ConnectionHandler
	mu          *sync.Mutex
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pp pipeline.Provider, source *config.IntegrationConfigLogSource, timeout time.Duration) (*UDPListener, error) {
	log.Info("Starting UDP forwarder on port ", source.Port)
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
	connHandler := NewConnectionHandler(pp, source, timeout)
	return &UDPListener{
		conn:        conn,
		connHandler: connHandler,
		mu:          &sync.Mutex{},
	}, nil
}

// Start listens to UDP connections on another routine
func (l *UDPListener) Start() {
	go l.run()
}

// Stop closes the UDP connection
func (l *UDPListener) Stop() {
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
