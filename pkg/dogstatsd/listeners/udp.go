// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	udpExpvars             = expvar.NewMap("dogstatsd-udp")
	udpPacketReadingErrors = expvar.Int{}
	udpPackets             = expvar.Int{}
	udpBytes               = expvar.Int{}
)

func init() {
	udpExpvars.Set("PacketReadingErrors", &udpPacketReadingErrors)
	udpExpvars.Set("Packets", &udpPackets)
	udpExpvars.Set("Bytes", &udpBytes)
}

// UDPListener implements the StatsdListener interface for UDP protocol.
// It listens to a given UDP address and sends back packets ready to be
// processed.
// Origin detection is not implemented for UDP.
type UDPListener struct {
	conn            *net.UDPConn
	packetsBuffer   *packets.Buffer
	packetAssembler *packets.Assembler
	buffer          []byte
	trafficCapture  replay.Component // Currently ignored
}

// NewUDPListener returns an idle UDP Statsd listener
func NewUDPListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager, capture replay.Component) (*UDPListener, error) {
	var err error
	var url string

	if config.Datadog.GetBool("dogstatsd_non_local_traffic") == true {
		// Listen to all network interfaces
		url = fmt.Sprintf(":%d", config.Datadog.GetInt("dogstatsd_port"))
	} else {
		url = net.JoinHostPort(config.GetBindHost(), config.Datadog.GetString("dogstatsd_port"))
	}

	addr, err := net.ResolveUDPAddr("udp", url)
	if err != nil {
		return nil, fmt.Errorf("could not resolve udp addr: %s", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	if rcvbuf := config.Datadog.GetInt("dogstatsd_so_rcvbuf"); rcvbuf != 0 {
		if err := conn.SetReadBuffer(rcvbuf); err != nil {
			return nil, fmt.Errorf("could not set socket rcvbuf: %s", err)
		}
	}

	bufferSize := config.Datadog.GetInt("dogstatsd_buffer_size")
	packetsBufferSize := config.Datadog.GetInt("dogstatsd_packet_buffer_size")
	flushTimeout := config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout")

	buffer := make([]byte, bufferSize)
	packetsBuffer := packets.NewBuffer(uint(packetsBufferSize), flushTimeout, packetOut)
	packetAssembler := packets.NewAssembler(flushTimeout, packetsBuffer, sharedPacketPoolManager, packets.UDP)

	listener := &UDPListener{
		conn:            conn,
		packetsBuffer:   packetsBuffer,
		packetAssembler: packetAssembler,
		buffer:          buffer,
		trafficCapture:  capture,
	}
	log.Debugf("dogstatsd-udp: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDPListener) Listen() {
	var t1, t2 time.Time
	log.Infof("dogstatsd-udp: starting to listen on %s", l.conn.LocalAddr())
	for {
		n, _, err := l.conn.ReadFrom(l.buffer)
		t1 = time.Now()
		udpPackets.Add(1)

		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd-udp: error reading packet: %v", err)
			udpPacketReadingErrors.Add(1)
			tlmUDPPackets.Inc("error")
		} else {
			tlmUDPPackets.Inc("ok")

			udpBytes.Add(int64(n))
			tlmUDPPacketsBytes.Add(float64(n))

			// packetAssembler merges multiple packets together and sends them when its buffer is full
			l.packetAssembler.AddMessage(l.buffer[:n])
		}

		t2 = time.Now()
		tlmListener.Observe(float64(t2.Sub(t1).Nanoseconds()), "udp")
	}
}

// Stop closes the UDP connection and stops listening
func (l *UDPListener) Stop() {
	l.packetAssembler.Close()
	l.packetsBuffer.Close()
	l.conn.Close()
}
