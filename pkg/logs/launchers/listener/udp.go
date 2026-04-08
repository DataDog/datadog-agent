// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/socket"
)

// The UDP listener is limited by the size of its read buffer,
// if the content of the message is bigger than the buffer length,
// it will arbitrary be truncated.

// A UDPListener opens a new UDP connection, keeps it alive and delegates the read operations to a tailer.
// When the source's Format is "syslog", it creates a DatagramTailer that produces
// structured messages. Otherwise, it creates an unstructured DatagramTailer.
type UDPListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	frameSize        int
	tailer           *tailer.DatagramTailer
	Conn             *net.UDPConn
}

// NewUDPListener returns an initialized UDPListener
func NewUDPListener(pipelineProvider pipeline.Provider, source *sources.LogSource, frameSize int) *UDPListener {
	return &UDPListener{
		pipelineProvider: pipelineProvider,
		source:           source,
		frameSize:        frameSize,
	}
}

// Start opens a new UDP connection and starts a tailer.
func (l *UDPListener) Start() {
	log.Infof("Starting UDP forwarder on port: %d, with read buffer size: %d", l.source.Config.Port, l.frameSize)
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
	if l.tailer != nil {
		log.Infof("Stopping UDP forwarder on port: %d", l.source.Config.Port)
		l.tailer.Stop()
	}
}

// startNewTailer starts a new DatagramTailer
func (l *UDPListener) startNewTailer() error {
	conn, err := l.newUDPConnection()
	if err != nil {
		return err
	}
	l.tailer = tailer.NewDatagramTailer(l.source, conn, l.pipelineProvider.NextPipelineChan(), true, l.frameSize)
	l.tailer.SetOnError(func() { l.resetTailer() })
	l.tailer.Start()
	return nil
}

// resetTailer tears down the current tailer and connection, then starts
// a fresh one. Called automatically on transient read errors.
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

// newUDPConnection returns a new UDP connection,
// returns an error if the creation failed.
func (l *UDPListener) newUDPConnection() (*net.UDPConn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	l.Conn = udpConn
	return udpConn, nil
}
