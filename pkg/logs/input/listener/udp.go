// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// A UDPListener listens for UDP connections and delegates the work to connHandler
type UDPListener struct {
	port        int
	conn        *net.UDPConn
	connHandler *ConnectionHandler
	stop        chan struct{}
	done        chan struct{}
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pp pipeline.Provider, source *config.LogSource) (*UDPListener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", source.Config.Port))
	if err != nil {
		source.Status.Error(err)
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		source.Status.Error(err)
		return nil, err
	}
	source.Status.Success()
	connHandler := NewConnectionHandler(pp, source)
	return &UDPListener{
		port:        source.Config.Port,
		conn:        conn,
		connHandler: connHandler,
		stop:        make(chan struct{}, 1),
		done:        make(chan struct{}, 1),
	}, nil
}

// Start listens to UDP connections on another routine
func (l *UDPListener) Start() {
	log.Info("Starting UDP forwarder on port ", l.port)
	l.connHandler.Start()
	go l.run()
}

// Stop closes the UDP connection
// it blocks until connHandler is flushed
func (l *UDPListener) Stop() {
	log.Info("Stopping UDP forwarder on port ", l.port)
	l.stop <- struct{}{}
	<-l.done
}

// run lets connHandler handle new UDP connections
func (l *UDPListener) run() {
	l.connHandler.HandleConnection(l.conn)
	<-l.stop
	l.connHandler.Stop()
	l.done <- struct{}{}
}
