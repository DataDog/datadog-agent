// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	udsExpvars               = expvar.NewMap("dogstatsd-uds")
	udsOriginDetectionErrors = expvar.Int{}
	udsPacketReadingErrors   = expvar.Int{}
	udsPackets               = expvar.Int{}
	udsBytes                 = expvar.Int{}
)

func init() {
	udsExpvars.Set("OriginDetectionErrors", &udsOriginDetectionErrors)
	udsExpvars.Set("PacketReadingErrors", &udsPacketReadingErrors)
	udsExpvars.Set("Packets", &udsPackets)
	udsExpvars.Set("Bytes", &udsBytes)
}

// UDSListener implements the StatsdListener interface for Unix Domain
// Socket datagram protocol. It listens to a given socket path and sends
// back packets ready to be processed.
// Origin detection will be implemented for UDS.
type UDSListener struct {
	conn            *net.UnixConn
	packetBuffer    *packetBuffer
	packetPool      *PacketPool
	oobPool         *sync.Pool // For origin detection ancilary data
	OriginDetection bool
}

// NewUDSListener returns an idle UDS Statsd listener
func NewUDSListener(packetOut chan Packets, packetPool *PacketPool) (*UDSListener, error) {
	socketPath := config.Datadog.GetString("dogstatsd_socket")
	originDetection := config.Datadog.GetBool("dogstatsd_origin_detection")

	address, addrErr := net.ResolveUnixAddr("unixgram", socketPath)
	if addrErr != nil {
		return nil, fmt.Errorf("dogstatsd-uds: can't ResolveUnixAddr: %v", addrErr)
	}
	fileInfo, err := os.Stat(socketPath)
	// Socket file already exists
	if err == nil {
		// Make sure it's a UNIX socket
		if fileInfo.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("dogstatsd-uds: cannot reuse %s socket path: path already exists and is not a UNIX socket", socketPath)
		}
		err = os.Remove(socketPath)
		if err != nil {
			return nil, fmt.Errorf("dogstatsd-usd: cannot remove stale UNIX socket: %v", err)
		}
	}

	conn, err := net.ListenUnixgram("unixgram", address)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}
	err = os.Chmod(socketPath, 0722)
	if err != nil {
		return nil, fmt.Errorf("can't set the socket at write only: %s", err)
	}

	if originDetection {
		err = enableUDSPassCred(conn)
		if err != nil {
			log.Errorf("dogstatsd-uds: error enabling origin detection: %s", err)
			originDetection = false
		} else {
			log.Debugf("dogstatsd-uds: enabling origin detection on %s", conn.LocalAddr())

		}
	}

	if rcvbuf := config.Datadog.GetInt("dogstatsd_so_rcvbuf"); rcvbuf != 0 {
		if err := conn.SetReadBuffer(rcvbuf); err != nil {
			return nil, fmt.Errorf("could not set socket rcvbuf: %s", err)
		}
	}

	listener := &UDSListener{
		OriginDetection: originDetection,
		packetPool:      packetPool,
		conn:            conn,
		packetBuffer: newPacketBuffer(uint(config.Datadog.GetInt("dogstatsd_packet_buffer_size")),
			config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout"), packetOut),
	}

	// Init the oob buffer pool if origin detection is enabled
	if originDetection {
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
		udsPackets.Add(1)
		if l.OriginDetection {
			// Read datagram + credentials in ancilary data
			oob := l.oobPool.Get().([]byte)
			var oobn int
			n, oobn, _, _, err = l.conn.ReadMsgUnix(packet.buffer, oob)
			// Extract container id from credentials
			container, taggingErr := processUDSOrigin(oob[:oobn])
			if taggingErr != nil {
				log.Warnf("dogstatsd-uds: error processing origin, data will not be tagged : %v", taggingErr)
				udsOriginDetectionErrors.Add(1)
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
			udsPacketReadingErrors.Add(1)
			continue
		}

		udsBytes.Add(int64(n))
		packet.Contents = packet.buffer[:n]

		// packetBuffer handles the forwarding of the packets to the dogstatsd server intake channel
		l.packetBuffer.append(packet)
	}
}

// Stop closes the UDS connection and stops listening
func (l *UDSListener) Stop() {
	l.packetBuffer.close()
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
