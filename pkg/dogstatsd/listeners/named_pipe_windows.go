// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build windows

package listeners

import (
	"expvar"
	"io"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/Microsoft/go-winio"
)

type namedPipeTelemetry struct {
	packetReadingErrors expvar.Int
	packets             expvar.Int
	bytes               expvar.Int
	tlmPackets          telemetry.Counter
	tlmPacketsBytes     telemetry.Counter
}

func newNamedPipeTelemetry() *namedPipeTelemetry {
	expvars := expvar.NewMap("dogstatsd-named-pipe")
	packetReadingErrors := expvar.Int{}
	packets := expvar.Int{}
	bytes := expvar.Int{}

	tlmPackets := telemetry.NewCounter("dogstatsd", "named_pipe_packets",
		[]string{"state"}, "Dogstatsd named pipe packets count")
	tlmPacketsBytes := telemetry.NewCounter("dogstatsd", "named_pipe_packets_bytes",
		nil, "Dogstatsd named pipe packets bytes count")
	expvars.Set("PacketReadingErrors", &packetReadingErrors)
	expvars.Set("Packets", &packets)
	expvars.Set("Bytes", &bytes)

	return &namedPipeTelemetry{
		packetReadingErrors: packetReadingErrors,
		tlmPackets:          tlmPackets,
		packets:             packets,
		bytes:               bytes,
		tlmPacketsBytes:     tlmPacketsBytes,
	}
}

func (t *namedPipeTelemetry) onReadSuccess(n int) {
	t.packets.Add(1)
	t.tlmPackets.Inc("ok")
	t.bytes.Add(int64(n))
	t.tlmPacketsBytes.Add(float64(n))
}

func (t *namedPipeTelemetry) onReadError() {
	t.packetReadingErrors.Add(1)
	t.tlmPackets.Inc("error")
}

// NamedPipeListener implements the StatsdListener interface for named pipe protocol.
// It listens to a given pipe name and sends back packets ready to be processed.
// Origin detection is not implemented for named pipe.
type NamedPipeListener struct {
	pipe            net.Listener
	packetsBuffer   *packetsBuffer
	packetAssembler *packetAssembler
	buffer          []byte
	connections     []net.Conn
	telemetry       *namedPipeTelemetry
}

// NewNamedPipeListener returns an named pipe Statsd listener
func NewNamedPipeListener(pipeName string, packetOut chan Packets, sharedPacketPool *PacketPool) (*NamedPipeListener, error) {
	bufferSize := config.Datadog.GetInt("dogstatsd_buffer_size")
	packetsBufferSize := config.Datadog.GetInt("dogstatsd_packet_buffer_size")
	flushTimeout := config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout")

	buffer := make([]byte, bufferSize)
	packetsBuffer := newPacketsBuffer(uint(packetsBufferSize), flushTimeout, packetOut)
	packetAssembler := newPacketAssembler(flushTimeout, packetsBuffer, sharedPacketPool)

	config := winio.PipeConfig{
		InputBufferSize:  int32(bufferSize),
		OutputBufferSize: 0,
	}
	pipePath := `\\.\pipe\` + pipeName
	pipe, err := winio.ListenPipe(pipePath, &config)

	if err != nil {
		return nil, err
	}

	listener := &NamedPipeListener{
		pipe:            pipe,
		packetsBuffer:   packetsBuffer,
		packetAssembler: packetAssembler,
		buffer:          buffer,
		telemetry:       newNamedPipeTelemetry(),
	}

	log.Debugf("dogstatsd-named-pipe: %s successfully initialized", pipe.Addr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *NamedPipeListener) Listen() {
	for {
		conn, err := l.pipe.Accept()
		switch {
		case err == nil:
			l.connections = append(l.connections, conn)
			go l.listenConnection(conn)
		case err.Error() == "use of closed network connection":
			{
				log.Info("dogstatsd-named-pipes: stop listening")
				return
			}
		default:
			log.Error(err)
		}
	}
}

func (l *NamedPipeListener) listenConnection(conn net.Conn) {
	log.Infof("dogstatsd-named-pipes: start listening a new named pipe client on %s", conn.LocalAddr())
	for {
		n, err := conn.Read(l.buffer)
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
			log.Errorf("dogstatsd-named-pipe: error reading packet: %v", err.Error())
			l.telemetry.onReadError()
		} else {
			l.telemetry.onReadSuccess(n)

			// packetAssembler merges multiple packets together and sends them when its buffer is full
			l.packetAssembler.addMessage(l.buffer[:n])
		}
	}
	conn.Close()
}

// Stop closes the connection and stops listening
func (l *NamedPipeListener) Stop() {
	for _, conn := range l.connections {
		// Stop the current execution of net.Conn.Read() and exit listen loop.
		conn.SetReadDeadline(time.Now())
	}

	l.packetAssembler.close()
	l.packetsBuffer.close()
	l.pipe.Close()
}
