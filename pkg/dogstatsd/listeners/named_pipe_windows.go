// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build windows

package listeners

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/Microsoft/go-winio"
)

var namedPipeTelemetry = newListenerTelemetry("named_pipe", "Named Pipe")

const pipeNamePrefix = `\\.\pipe\`

// NamedPipeListener implements the StatsdListener interface for named pipe protocol.
// It listens to a given pipe name and sends back packets ready to be processed.
// Origin detection is not implemented for named pipe.
type NamedPipeListener struct {
	pipe          net.Listener
	packetManager *packetManager
	connections   map[net.Conn]struct{}
	mux           sync.Mutex
}

// NewNamedPipeListener returns an named pipe Statsd listener
func NewNamedPipeListener(pipeName string, packetOut chan Packets, sharedPacketPool *PacketPool) (*NamedPipeListener, error) {
	bufferSize := config.Datadog.GetInt("dogstatsd_buffer_size")
	return newNamedPipeListener(
		pipeName,
		bufferSize,
		newPacketManagerFromConfig(
			packetOut,
			sharedPacketPool))
}

func newNamedPipeListener(
	pipeName string,
	bufferSize int,
	packetManager *packetManager) (*NamedPipeListener, error) {

	config := winio.PipeConfig{
		InputBufferSize:  int32(bufferSize),
		OutputBufferSize: 0,
	}
	pipePath := pipeNamePrefix + pipeName
	pipe, err := winio.ListenPipe(pipePath, &config)

	if err != nil {
		return nil, err
	}

	listener := &NamedPipeListener{
		pipe:          pipe,
		packetManager: packetManager,
		connections:   make(map[net.Conn]struct{}),
	}

	log.Debugf("dogstatsd-named-pipes: %s successfully initialized", pipe.Addr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *NamedPipeListener) Listen() {
	for {
		conn, err := l.pipe.Accept()
		switch {
		case err == nil:
			l.mux.Lock()
			l.connections[conn] = struct{}{}
			l.mux.Unlock()
			buffer := l.packetManager.createBuffer()
			go l.listenConnection(conn, buffer)

		case err.Error() == "use of closed network connection":
			{
				// Called when the pipe listener is closed from Stop()
				log.Info("dogstatsd-named-pipes: stop listening")
				return
			}
		default:
			log.Error(err)
		}
	}
}

func (l *NamedPipeListener) listenConnection(conn net.Conn, buffer []byte) {
	log.Infof("dogstatsd-named-pipes: start listening a new named pipe client on %s", conn.LocalAddr())
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Infof("dogstatsd-named-pipes: client disconnected from %s", conn.LocalAddr())
				break
			}

			// NamedPipeListener.Stop uses a timeout to stop listening.
			if err == winio.ErrTimeout {
				log.Infof("dogstatsd-named-pipes: stop listening a named pipe client on %s", conn.LocalAddr())
				break
			}
			log.Errorf("dogstatsd-named-pipes: error reading packet: %v", err.Error())
			namedPipeTelemetry.onReadError()
		} else {
			namedPipeTelemetry.onReadSuccess(n)

			// packetAssembler merges multiple packets together and sends them when its buffer is full
			l.packetManager.packetAssembler.addMessage(buffer[:n])
		}
	}
	conn.Close()
	l.mux.Lock()
	defer l.mux.Unlock()
	delete(l.connections, conn)
}

// Stop closes the connection and stops listening
func (l *NamedPipeListener) Stop() {
	l.mux.Lock()
	defer l.mux.Unlock()
	for conn := range l.connections {
		// Stop the current execution of net.Conn.Read() and exit listen loop.
		conn.SetReadDeadline(time.Now())
	}

	l.packetManager.close()
	l.pipe.Close()
}

// GetActiveConnectionsCount returns the number of active connections.
func (l *NamedPipeListener) GetActiveConnectionsCount() int {
	l.mux.Lock()
	defer l.mux.Unlock()
	return len(l.connections)
}
