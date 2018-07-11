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

// A UDPListener opens a new UDP connection, keeps it alive and delegates the read operations to a reader.
type UDPListener struct {
	pp     pipeline.Provider
	source *config.LogSource
	reader *Reader
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pp pipeline.Provider, source *config.LogSource) *UDPListener {
	return &UDPListener{
		pp:     pp,
		source: source,
	}
}

// Start opens a new UDP connection and starts a reader.
func (l *UDPListener) Start() {
	log.Infof("Starting UDP forwarder on port: %d", l.source.Config.Port)
	conn, err := l.newUDPConnection()
	if err != nil {
		log.Errorf("Can't open UDP connection on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.reader = l.newReader(conn)
	l.reader.Start()
}

// Stop stops the reader.
func (l *UDPListener) Stop() {
	log.Infof("Stopping UDP forwarder on port: %d", l.source.Config.Port)
	l.reader.Stop()
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

// newReader returns a new reader that reads from conn.
func (l *UDPListener) newReader(conn net.Conn) *Reader {
	return NewReader(l.source, conn, l.pp.NextPipelineChan(), l.handleUngracefulStop)
}

// handleUngracefulStop restarts a reader when the previous one ungracefully stopped
// from reading data from its connection.
func (l *UDPListener) handleUngracefulStop(reader *Reader) {
	log.Info("Restarting a new UDP connection on port: %d", l.source.Config.Port)
	reader.Stop()
	conn, err := l.newUDPConnection()
	if err != nil {
		log.Errorf("Could not restart UDP connection on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
	l.reader = l.newReader(conn)
	l.reader.Start()
}
