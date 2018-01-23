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

// A TCPListener listens and accepts TCP connections and delegates the work to connHandler
type TCPListener struct {
	port        int
	listener    net.Listener
	connHandler *ConnectionHandler
	shouldStop  bool
	mu          *sync.Mutex
}

// NewTCPListener returns an initialized TCPListener
func NewTCPListener(pp pipeline.Provider, source *config.LogSource) (*TCPListener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", source.Config.Port))
	if err != nil {
		source.Status.Error(err)
		return nil, err
	}
	source.Status.Success()
	connHandler := NewConnectionHandler(pp, source)
	return &TCPListener{
		port:        source.Config.Port,
		listener:    listener,
		connHandler: connHandler,
		mu:          &sync.Mutex{},
	}, nil
}

// Start listens to TCP connections on another routine
func (l *TCPListener) Start() {
	log.Info("Starting TCP forwarder on port ", l.port)
	go l.run()
}

// Stop prevents listener to accept new incoming connections and close all open connections
func (l *TCPListener) Stop() {
	log.Info("Stopping TCP forwarder on port ", l.port)
	l.mu.Lock()
	l.shouldStop = true
	err := l.listener.Close()
	if err != nil {
		log.Warn(err)
	}
	l.connHandler.Stop()
	l.mu.Unlock()
}

// run accepts new TCP connections and lets connHandler handle them
func (l *TCPListener) run() {
	for {
		conn, err := l.listener.Accept()
		l.mu.Lock()
		if l.shouldStop {
			l.mu.Unlock()
			return
		}
		if err != nil {
			l.connHandler.source.Status.Error(err)
			log.Error("Can't listen: ", err)
			l.mu.Unlock()
			return
		}
		l.connHandler.source.Status.Success()
		go l.connHandler.HandleConnection(conn)
		l.mu.Unlock()
	}
}
