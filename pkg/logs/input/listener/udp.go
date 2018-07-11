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

// A UDPListener opens a new UDP connection, keeps it alive and delegates the read operations to a Worker.
type UDPListener struct {
	pp     pipeline.Provider
	source *config.LogSource
	worker *Worker
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pp pipeline.Provider, source *config.LogSource) *UDPListener {
	return &UDPListener{
		pp:     pp,
		source: source,
	}
}

// Start opens a new UDP connection and starts a worker.
func (l *UDPListener) Start() {
	log.Infof("Starting UDP forwarder on port: %d", l.source.Config.Port)
	conn, err := l.newUDPConnection()
	if err != nil {
		log.Errorf("Can't open a UDP connection: %v", err)
		l.source.Status.Error(err)
		return
	}
	l.worker = l.newWorker(conn)
	l.worker.Start()
}

// Stop stops the worker.
func (l *UDPListener) Stop() {
	log.Infof("Stopping UDP forwarder on port: %d", l.source.Config.Port)
	l.worker.Stop()
}

// newUDPConnection returns a new UDP connection,
// returns an error if the creation failed.
func (l *UDPListener) newUDPConnection() (net.Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// newWorker returns a new worker that reads from conn.
func (l *UDPListener) newWorker(conn net.Conn) *Worker {
	return NewWorker(l.source, conn, l.pp.NextPipelineChan(), true, l.recoverFromError)
}

// recoverFromError restarts a worker when the previous one gracefully stopped,
// from reading data from its connection.
func (l *UDPListener) recoverFromError() {
	log.Info("Restarting a new UDP connection on port: %d", l.source.Config.Port)
	l.worker.Stop()
	conn, err := l.newUDPConnection()
	if err != nil {
		log.Errorf("Could not restart a UDP connection: %v", err)
		l.source.Status.Error(err)
		return
	}
	l.worker = l.newWorker(conn)
	l.worker.Start()
}
