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
// For examples for |MSG| := |F1|F2|F3| where |F1| + |F2| > BUF_LEN and |F1| < BUF_LEN :
// sending: |F1|
// sending: |F2|
// sending: |F3|
// would result in sending |MSG| to the logs-backend.
// sending: |MSG|
// would result in sending TRUNC(|F1|+|F2|) to the logs-backend.

// A UDPListener opens a new UDP connection, keeps it alive and delegates the read operations to a tailer.
type UDPListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	frameSize        int
	tailer           *tailer.Tailer
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

// startNewTailer starts a new Tailer
func (l *UDPListener) startNewTailer() error {
	conn, err := l.newUDPConnection()
	if err != nil {
		return err
	}
	l.tailer = tailer.NewTailer(l.source, conn, l.pipelineProvider.NextPipelineChan(), l.read)
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
func (l *UDPListener) read(tailer *tailer.Tailer) ([]byte, error) {
	frame := make([]byte, l.frameSize+1)
	n, err := tailer.Conn.Read(frame)
	switch {
	case err != nil && isClosedConnError(err):
		return nil, err
	case err != nil:
		go l.resetTailer()
		return nil, err
	default:
		// make sure all logs are separated by line feeds, otherwise they don't get properly split downstream
		if n > l.frameSize {
			// the message is bigger than the length of the read buffer,
			// the trailing part of the content will be dropped.
			frame[l.frameSize] = '\n'
		} else if n > 0 && frame[n-1] != '\n' {
			frame[n] = '\n'
			n++
		}
		return frame[:n], nil
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
