// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package listeners

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	winio "github.com/Microsoft/go-winio"
)

const pipeNamePrefix = `\\.\pipe\`

// NamedPipeListener implements the StatsdListener interface for named pipe protocol.
// It listens to a given pipe name and sends back packets ready to be processed.
// Origin detection is not implemented for named pipe.
type NamedPipeListener struct {
	pipe          net.Listener
	packetManager *packets.PacketManager
	// TODO: Migrate to `ConnectionTracker` instead
	connections       *namedPipeConnections
	trafficCapture    replay.Component // Currently ignored
	listenWg          sync.WaitGroup
	telemetryStore    *TelemetryStore
	internalTelemetry *listenerTelemetry
}

// NewNamedPipeListener returns an named pipe Statsd listener
func NewNamedPipeListener(pipeName string, packetOut chan packets.Packets,
	sharedPacketPoolManager *packets.PoolManager, cfg config.Reader, capture replay.Component, telemetryStore *TelemetryStore, packetsTelemetryStore *packets.TelemetryStore, telemetrycomp telemetry.Component) (*NamedPipeListener, error) {

	bufferSize := cfg.GetInt("dogstatsd_buffer_size")
	return newNamedPipeListener(
		pipeName,
		bufferSize,
		packets.NewPacketManagerFromConfig(packetOut, sharedPacketPoolManager, cfg, packetsTelemetryStore),
		capture,
		telemetryStore,
		telemetrycomp,
	)
}

func newNamedPipeListener(
	pipeName string,
	bufferSize int,
	packetManager *packets.PacketManager,
	capture replay.Component,
	telemetryStore *TelemetryStore,
	telemetrycomp telemetry.Component,
) (*NamedPipeListener, error) {

	namedPipeTelemetry := newListenerTelemetry("named_pipe", "named_pipe", telemetrycomp)

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
		connections: &namedPipeConnections{
			newConn:         make(chan net.Conn),
			connToClose:     make(chan net.Conn),
			closeAllConns:   make(chan struct{}),
			allConnsClosed:  make(chan struct{}),
			activeConnCount: atomic.NewInt32(0),
		},
		trafficCapture:    capture,
		telemetryStore:    telemetryStore,
		internalTelemetry: namedPipeTelemetry,
	}

	log.Debugf("dogstatsd-named-pipes: %s successfully initialized", pipe.Addr())
	return listener, nil
}

type namedPipeConnections struct {
	newConn         chan net.Conn
	connToClose     chan net.Conn
	closeAllConns   chan struct{}
	allConnsClosed  chan struct{}
	activeConnCount *atomic.Int32
}

func (l *namedPipeConnections) handleConnections() {
	connections := make(map[net.Conn]struct{})
	requestStop := false
	for stop := false; !stop; {
		select {
		case conn := <-l.newConn:
			connections[conn] = struct{}{}
			l.activeConnCount.Inc()
		case conn := <-l.connToClose:
			conn.Close()
			delete(connections, conn)
			l.activeConnCount.Dec()
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
				_ = conn.SetReadDeadline(time.Now())
			}

		}
	}
	l.allConnsClosed <- struct{}{}
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *NamedPipeListener) Listen() {
	l.listenWg.Add(1)

	go func() {
		defer l.listenWg.Done()
		l.listen()
	}()
}

func (l *NamedPipeListener) listen() {
	go l.connections.handleConnections()
	for {
		conn, err := l.pipe.Accept()
		switch {
		case err == nil:
			l.connections.newConn <- conn
			buffer := l.packetManager.CreateBuffer()
			go l.listenConnection(conn, buffer)

		case err.Error() == "use of closed network connection":
			{
				// Called when the pipe listener is closed from Stop()
				log.Debug("dogstatsd-named-pipes: stop listening")
				return
			}
		default:
			log.Error(err)
		}
	}
}

func (l *NamedPipeListener) listenConnection(conn net.Conn, buffer []byte) {
	log.Debugf("dogstatsd-named-pipes: start listening a new named pipe client on %s", conn.LocalAddr())
	startWriteIndex := 0
	var t1, t2 time.Time
	for {
		bytesRead, err := conn.Read(buffer[startWriteIndex:])

		t1 = time.Now()

		if err != nil {
			if err == io.EOF {
				log.Debugf("dogstatsd-named-pipes: client disconnected from %s", conn.LocalAddr())
				break
			}

			// NamedPipeListener.Stop uses a timeout to stop listening.
			if err == winio.ErrTimeout {
				log.Debugf("dogstatsd-named-pipes: stop listening a named pipe client on %s", conn.LocalAddr())
				break
			}
			log.Errorf("dogstatsd-named-pipes: error reading packet: %v", err.Error())
			l.internalTelemetry.onReadError()
		} else {
			endIndex := startWriteIndex + bytesRead

			// When there is no '\n', the message is partial. LastIndexByte returns -1 and messageSize is 0.
			// If there is a '\n', at least one message is completed and '\n' is part of this message.
			messageSize := bytes.LastIndexByte(buffer[:endIndex], '\n') + 1
			if messageSize > 0 {
				l.internalTelemetry.onReadSuccess(messageSize)

				// PacketAssembler merges multiple packets together and sends them when its buffer is full
				l.packetManager.PacketAssembler.AddMessage(buffer[:messageSize])
			}

			startWriteIndex = endIndex - messageSize

			// If the message is bigger than the buffer size, reset startWriteIndex to continue reading next messages.
			if startWriteIndex >= len(buffer) {
				startWriteIndex = 0
			} else {
				copy(buffer, buffer[messageSize:endIndex])
			}
		}

		t2 = time.Now()
		l.telemetryStore.tlmListener.Observe(float64(t2.Sub(t1).Nanoseconds()), "named_pipe", "named_pipe", "named_pipe")
	}
	l.connections.connToClose <- conn
}

// Stop closes the connection and stops listening
func (l *NamedPipeListener) Stop() {
	// Request closing connections
	l.connections.closeAllConns <- struct{}{}

	// Wait until all connections are closed
	<-l.connections.allConnsClosed

	l.packetManager.Close()
	l.pipe.Close()
	l.listenWg.Wait()
}

// getActiveConnectionsCount returns the number of active connections.
func (l *NamedPipeListener) getActiveConnectionsCount() int32 {
	return l.connections.activeConnCount.Load()
}
