// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// A TCPListener listens and accepts TCP connections and delegates the work to connHandler
type TCPListener struct {
	port        int
	listener    net.Listener
	connHandler *ConnectionHandler
	stop        chan struct{}
	done        chan struct{}
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
		stop:        make(chan struct{}, 1),
		done:        make(chan struct{}, 1),
	}, nil
}

// Start listens to TCP connections on another routine
func (l *TCPListener) Start() {
	log.Info("Starting TCP forwarder on port ", l.port)
	l.connHandler.Start()
	go l.run()
}

// Stop prevents the listener to accept new incoming connections
// it blocks until connHandler is flushed
func (l *TCPListener) Stop() {
	log.Info("Stopping TCP forwarder on port ", l.port)
	l.stop <- struct{}{}
	l.listener.Close()
	<-l.done
}

// run accepts new TCP connections and lets connHandler handle them
func (l *TCPListener) run() {
	defer func() {
		l.listener.Close()
		l.connHandler.Stop()
		l.done <- struct{}{}
	}()
	for {
		select {
		case <-l.stop:
			// stop accepting new connections
			return
		default:
			conn, err := l.listener.Accept()
			if err != nil {
				// an error occurred, stop from accepting new connections
				l.connHandler.source.Status.Error(err)
				log.Error("Can't listen: ", err)
				return
			}
			l.connHandler.source.Status.Success()
			go l.connHandler.HandleConnection(conn)
		}
	}
}
