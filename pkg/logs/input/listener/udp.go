// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"strings"

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
	err := l.startNewTailer()
	if err != nil {
		log.Errorf("Can't start UDP forwarder on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
}

// Stop stops the tailer.
func (l *UDPListener) Stop() {
	log.Infof("Stopping UDP forwarder on port: %d", l.source.Config.Port)
	l.tailer.Stop()
}

// startNewTailer starts a new Tailer
func (l *UDPListener) startNewTailer() error {
	conn, err := l.newUDPConnection()
	if err != nil {
		return err
	}
	l.tailer = NewTailer(l.source, conn, l.pp.NextPipelineChan(), l.read)
	l.tailer.Start()
	return nil
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

// read reads data from the tailer connection, returns an error if it failed and reset the tailer.
func (l *UDPListener) read(tailer *Tailer) ([]byte, error) {
	inBuf := make([]byte, 4096)
	n, err := tailer.conn.Read(inBuf)
	switch {
	case err != nil && l.isClosedConnError(err):
		return nil, err
	case err != nil:
		go l.resetTailer()
		return nil, err
	default:
		return inBuf[:n], nil
	}
}

// resetTailer creates a new tailer.
func (l *UDPListener) resetTailer() {
	log.Infof("Resetting the UDP connection on port: %d", l.source.Config.Port)
	l.tailer.Stop()
	err := l.startNewTailer()
	if err != nil {
		log.Errorf("Could not reset the UDP connection on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
}

// isConnClosedError returns true if the error is related to a closed connection,
// for more details, see: https://golang.org/src/internal/poll/fd.go#L18.
func (l *UDPListener) isClosedConnError(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}
