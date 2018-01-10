// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

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
	packetPool      *PacketPool
	oobPool         *sync.Pool // For origin detection ancilary data
	OriginDetection bool
}

// NewUDSListener returns an idle UDS Statsd listener
func NewUDSListener(packetOut chan *Packet, packetPool *PacketPool) (*UDSListener, error) {
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

		}
	}

	listener := &UDSListener{
		OriginDetection: originDection,
		packetOut:       packetOut,
		packetPool:      packetPool,
		conn:            conn,
	}

	// Init the oob buffer pool if origin detection is enabled
	if originDection {
		listener.oobPool = &sync.Pool{
			New: func() interface{} {
				return make([]byte, getUDSAncillarySize())
			},
		}
	}

	log.Debugf("dogstatsd-uds: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSListener) Listen() {
	log.Infof("dogstatsd-uds: starting to listen on %s", l.conn.LocalAddr())
	for {
		var n int
		var err error
		packet := l.packetPool.Get()

		if l.OriginDetection {
			// Read datagram + credentials in ancilary data
			oob := l.oobPool.Get().([]byte)
			var oobn int
			n, oobn, _, _, err = l.conn.ReadMsgUnix(packet.buffer, oob)

			// Extract container id from credentials
			container, err := processUDSOrigin(oob[:oobn])
			if err != nil {
				log.Warnf("dogstatsd-uds: error processing origin, data will not be tagged : %v", err)
				socketExpvar.Add("OriginDetectionErrors", 1)
			} else {
				packet.Origin = container
			}
			// Return the buffer back to the pool for reuse
			l.oobPool.Put(oob)
		} else {
			// Read only datagram contents with no credentials
			n, _, err = l.conn.ReadFromUnix(packet.buffer)
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

		packet.Contents = packet.buffer[:n]
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
