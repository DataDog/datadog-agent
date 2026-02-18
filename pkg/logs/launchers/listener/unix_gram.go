// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"net"
	"os"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/socket"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// UnixgramListener listens on a Unix domain datagram socket and reads
// datagrams using a DatagramTailer.
type UnixgramListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	frameSize        int
	tailer           *tailer.DatagramTailer
	conn             net.PacketConn
}

// NewUnixgramListener returns an initialized UnixgramListener.
func NewUnixgramListener(pipelineProvider pipeline.Provider, source *sources.LogSource, frameSize int) *UnixgramListener {
	return &UnixgramListener{
		pipelineProvider: pipelineProvider,
		source:           source,
		frameSize:        frameSize,
	}
}

// Start creates the Unix datagram socket and starts the tailer.
func (l *UnixgramListener) Start() {
	log.Infof("Starting Unix datagram listener on %s", l.source.Config.SocketPath)
	err := l.startTailer()
	if err != nil {
		log.Errorf("Can't start Unix datagram listener on %s: %v", l.source.Config.SocketPath, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
}

// Stop stops the tailer and cleans up the socket file.
func (l *UnixgramListener) Stop() {
	log.Infof("Stopping Unix datagram listener on %s", l.source.Config.SocketPath)
	if l.tailer != nil {
		l.tailer.Stop()
	}
	// Clean up the socket file
	os.Remove(l.source.Config.SocketPath) //nolint:errcheck
}

// startTailer opens the Unix datagram connection and starts the tailer.
func (l *UnixgramListener) startTailer() error {
	// Remove stale socket file if it exists
	os.Remove(l.source.Config.SocketPath) //nolint:errcheck

	addr, err := net.ResolveUnixAddr("unixgram", l.source.Config.SocketPath)
	if err != nil {
		return err
	}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return err
	}
	l.conn = conn
	l.tailer = tailer.NewDatagramTailer(l.source, conn, l.pipelineProvider.NextPipelineChan(), false, l.frameSize)
	l.tailer.SetOnError(func() { l.resetTailer() })
	l.tailer.Start()
	return nil
}

// resetTailer tears down the current tailer and connection, then starts
// a fresh one. Called automatically on transient read errors.
func (l *UnixgramListener) resetTailer() {
	log.Infof("Resetting the Unix datagram connection on %s", l.source.Config.SocketPath)
	l.tailer.Stop()
	err := l.startTailer()
	if err != nil {
		log.Errorf("Could not reset the Unix datagram connection on %s: %v", l.source.Config.SocketPath, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
}
