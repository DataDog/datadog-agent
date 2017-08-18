// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"
	"net"
	"os"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	socketExpvar = expvar.NewMap("dogstatsd-uds")
)

// UDSListener implements the StatsdListener interface for Unix Domain
// Socket datagram protocol. It listens to a given socket path and sends
// back packets ready to be processed.
// Origin detection will be implemented for UDS.
type UDSListener struct {
	conn            *net.UnixConn
	packetOut       chan *Packet
	bufferSize      int
	oobSize         int
	OriginDetection bool
}

// NewUDSListener returns an idle UDS Statsd listener
func NewUDSListener(packetOut chan *Packet) (*UDSListener, error) {
	socketPath := config.Datadog.GetString("dogstatsd_socket")
	originDection := config.Datadog.GetBool("dogstatsd_origin_detection")

	address, addrErr := net.ResolveUnixAddr("unixgram", socketPath)
	if addrErr != nil {
		return nil, fmt.Errorf("dogstatsd-uds: can't ResolveUnixAddr: %v", addrErr)
	}
	conn, err := net.ListenUnixgram("unixgram", address)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	if originDection {
		err = enableUDSPassCred(conn)
		if err != nil {
			log.Errorf("dogstatsd-uds: error enabling origin detection: %s", err)
			originDection = false
		} else {
			log.Debugf("dogstatsd-uds: enabling origin detection on %s", conn.LocalAddr())
			// FIXME: remove when fully implemented
			log.Warnf("dogstatsd-uds: origin detection feature is not complete yet")
		}
	}

	listener := &UDSListener{
		OriginDetection: originDection,
		oobSize:         getUDSAncillarySize(),
		bufferSize:      config.Datadog.GetInt("dogstatsd_buffer_size"),
		packetOut:       packetOut,
		conn:            conn,
	}

	log.Debugf("dogstatsd-uds: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSListener) Listen() {
	log.Infof("dogstatsd-uds: starting to listen on %s", l.conn.LocalAddr())
	for {
		buf := make([]byte, l.bufferSize)
		var n int
		var err error
		packet := &Packet{}

		if l.OriginDetection {
			// Read datagram + credentials in ancilary data
			oob := make([]byte, l.oobSize)
			var oobn int
			n, oobn, _, _, err = l.conn.ReadMsgUnix(buf, oob)

			// Extract container id from credentials
			container, err := processUDSOrigin(oob[:oobn])
			if err != nil {
				log.Warnf("dogstatsd-uds: error processing origin, data will not be tagged : %v", err)
				socketExpvar.Add("OriginDetectionErrors", 1)
			} else {
				packet.Origin = container
			}
		} else {
			// Read only datagram contents with no credentials
			n, _, err = l.conn.ReadFromUnix(buf)
		}

		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd-uds: error reading packet: %v", err)
			socketExpvar.Add("PacketReadingErrors", 1)
			continue
		}

		packet.Contents = buf[:n]
		l.packetOut <- packet
	}
}

// Stop closes the UDS connection and stops listening
func (l *UDSListener) Stop() {
	l.conn.Close()

	// Socket cleanup on exit
	socketPath := config.Datadog.GetString("dogstatsd_socket")
	if len(socketPath) > 0 {
		err := os.Remove(socketPath)
		if err != nil {
			log.Infof("dogstatsd-uds: error removing socket file: %s", err)
		}
	}
}
