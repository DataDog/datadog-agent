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

// A UDPListener opens a new UDP connection, keeps it alive and delegates the read operations to a tailer.
type UDPListener struct {
	pp     pipeline.Provider
	source *config.LogSource
	tailer *Tailer
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pp pipeline.Provider, source *config.LogSource) *UDPListener {
	return &UDPListener{
		pp:     pp,
		source: source,
	}
}

// Start opens a new UDP connection and starts a tailer.
func (l *UDPListener) Start() {
	log.Infof("Starting UDP forwarder on port: %d", l.source.Config.Port)
	conn, err := l.newUDPConnection()
	if err != nil {
		log.Errorf("Can't open UDP connection on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.tailer = l.newTailer(conn)
	l.tailer.Start()
}

// Stop stops the tailer.
func (l *UDPListener) Stop() {
	log.Infof("Stopping UDP forwarder on port: %d", l.source.Config.Port)
	l.tailer.Stop()
}

// newUDPConnection returns a new UDP connection,
// returns an error if the creation failed.
func (l *UDPListener) newUDPConnection() (net.Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp", udpAddr)
}

// newTailer returns a new tailer that reads from conn.
func (l *UDPListener) newTailer(conn net.Conn) *Tailer {
	return NewTailer(l.source, conn, l.pp.NextPipelineChan(), true, l.handleUngracefulStop)
}

// handleUngracefulStop restarts a tailer when the previous one ungracefully stopped
// from reading data from its connection.
func (l *UDPListener) handleUngracefulStop(tailer *Tailer) {
	log.Info("Restarting a new UDP connection on port: %d", l.source.Config.Port)
	tailer.Stop()
	conn, err := l.newUDPConnection()
	if err != nil {
		log.Errorf("Could not restart UDP connection on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
	l.tailer = l.newTailer(conn)
	l.tailer.Start()
}
