// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build windows

package listeners

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync/atomic"
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
	connections   *namedPipeConnections
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

	if !strings.HasPrefix(pipeName, pipeNamePrefix) {
		pipeName = pipeNamePrefix + pipeName
	}
	pipe, err := winio.ListenPipe(pipeName, &config)

	if err != nil {
		return nil, err
	}

	listener := &NamedPipeListener{
		pipe:          pipe,
		packetManager: packetManager,
		connections: &namedPipeConnections{
			newConn:         make(chan net.Conn),
			connToClose:     make(chan net.Conn),
			closeAllConns:   make(chan struct{}),
			allConnsClosed:  make(chan struct{}),
			activeConnCount: 0,
		},
	}

	log.Debugf("dogstatsd-named-pipes: %s successfully initialized", pipe.Addr())
	return listener, nil
}

// IsNamedPipeEndpoint detects if the endpoint has the named pipe prefix
func IsNamedPipeEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, pipeNamePrefix)
}

type namedPipeConnections struct {
	newConn         chan net.Conn
	connToClose     chan net.Conn
	closeAllConns   chan struct{}
	allConnsClosed  chan struct{}
	activeConnCount int32
}

func (l *namedPipeConnections) handleConnections() {
	connections := make(map[net.Conn]struct{})
	requestStop := false
	for stop := false; !stop; {
		select {
		case conn := <-l.newConn:
			connections[conn] = struct{}{}
			atomic.AddInt32(&l.activeConnCount, 1)
		case conn := <-l.connToClose:
			conn.Close()
			delete(connections, conn)
			atomic.AddInt32(&l.activeConnCount, -1)
			if requestStop && len(connections) == 0 {
				stop = true
			}
		case <-l.closeAllConns:
			requestStop = true
			if len(connections) == 0 {
				stop = true
			}
			for conn := range connections {
				// Stop the current execution of net.Conn.Read() and exit listen loop.
				conn.SetReadDeadline(time.Now())
			}

		}
	}
	l.allConnsClosed <- struct{}{}
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *NamedPipeListener) Listen() {
	go l.connections.handleConnections()
	for {
		conn, err := l.pipe.Accept()
		switch {
		case err == nil:
			l.connections.newConn <- conn
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
	startWriteIndex := 0
	for {
		bytesRead, err := conn.Read(buffer[startWriteIndex:])

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
			endIndex := startWriteIndex + bytesRead

			// When there is no '\n', the message is partial. LastIndexByte returns -1 and messageSize is 0.
			// If there is a '\n', at least one message is completed and '\n' is part of this message.
			messageSize := bytes.LastIndexByte(buffer[:endIndex], '\n') + 1
			if messageSize > 0 {
				namedPipeTelemetry.onReadSuccess(messageSize)

				// packetAssembler merges multiple packets together and sends them when its buffer is full
				l.packetManager.packetAssembler.addMessage(buffer[:messageSize])
			}

			startWriteIndex = endIndex - messageSize

			// If the message is bigger than the buffer size, reset startWriteIndex to continue reading next messages.
			if startWriteIndex >= len(buffer) {
				startWriteIndex = 0
			} else {
				copy(buffer, buffer[messageSize:endIndex])
			}
		}
	}
	l.connections.connToClose <- conn
}

// Stop closes the connection and stops listening
func (l *NamedPipeListener) Stop() {
	// Request closing connections
	l.connections.closeAllConns <- struct{}{}

	// Wait until all connections are closed
	<-l.connections.allConnsClosed

	l.packetManager.close()
	l.pipe.Close()
}

// getActiveConnectionsCount returns the number of active connections.
func (l *NamedPipeListener) getActiveConnectionsCount() int32 {
	return atomic.LoadInt32(&l.connections.activeConnCount)
}
