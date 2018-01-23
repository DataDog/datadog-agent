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
	listener    net.Listener
	connHandler *ConnectionHandler
}

// NewTCPListener returns an initialized TCPListener
func NewTCPListener(pp pipeline.Provider, source *config.LogSource) (*TCPListener, error) {
	log.Info("Starting TCP forwarder on port ", source.Config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", source.Config.Port))
	if err != nil {
		source.Status.Error(err)
		return nil, err
	}
	source.Status.Success()
	connHandler := &ConnectionHandler{
		pp:     pp,
		source: source,
	}
	return &TCPListener{
		listener:    listener,
		connHandler: connHandler,
	}, nil
}

// Start listens to TCP connections on another routine
func (tcpListener *TCPListener) Start() {
	go tcpListener.run()
}

// run accepts new TCP connections and lets connHandler handle them
func (tcpListener *TCPListener) run() {
	for {
		conn, err := tcpListener.listener.Accept()
		if err != nil {
			tcpListener.connHandler.source.Status.Error(err)
			log.Error("Can't listen: ", err)
			return
		}
		tcpListener.connHandler.source.Status.Success()
		go tcpListener.connHandler.handleConnection(conn)
	}
}
