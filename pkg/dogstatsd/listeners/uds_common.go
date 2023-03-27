// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"expvar"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners/ratelimit"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	conn                    *net.UnixConn
	packetsBuffer           *packets.Buffer
	sharedPacketPoolManager *packets.PoolManager
	oobPoolManager          *packets.PoolManager
	trafficCapture          replay.Component
	OriginDetection         bool

	dogstatsdMemBasedRateLimiter bool
}

// NewUDSListener returns an idle UDS Statsd listener
func NewUDSListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager, capture replay.Component) (*UDSListener, error) {
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
			return nil, fmt.Errorf("dogstatsd-uds: cannot remove stale UNIX socket: %v", err)
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
		conn:            conn,
		packetsBuffer: packets.NewBuffer(uint(config.Datadog.GetInt("dogstatsd_packet_buffer_size")),
			config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout"), packetOut),
		sharedPacketPoolManager:      sharedPacketPoolManager,
		trafficCapture:               capture,
		dogstatsdMemBasedRateLimiter: config.Datadog.GetBool("dogstatsd_mem_based_rate_limiter.enabled"),
	}

	if listener.trafficCapture != nil {
		err = listener.trafficCapture.RegisterSharedPoolManager(listener.sharedPacketPoolManager)
		if err != nil {
			return nil, err
		}
	}

	// Init the oob buffer pool if origin detection is enabled
	if originDetection {

		pool := &sync.Pool{
			New: func() interface{} {
				s := make([]byte, getUDSAncillarySize())
				return &s
			},
		}

		listener.oobPoolManager = packets.NewPoolManager(pool)
		if listener.trafficCapture != nil {
			err = listener.trafficCapture.RegisterOOBPoolManager(listener.oobPoolManager)
			if err != nil {
				return nil, err
			}
		}
	}

	log.Debugf("dogstatsd-uds: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSListener) Listen() {
	t1 := time.Now()
	var t2 time.Time
	log.Infof("dogstatsd-uds: starting to listen on %s", l.conn.LocalAddr())

	var rateLimiter *ratelimit.MemBasedRateLimiter
	if l.dogstatsdMemBasedRateLimiter {
		var err error
		rateLimiter, err = ratelimit.BuildMemBasedRateLimiter()
		if err != nil {
			log.Errorf("Cannot use DogStatsD rate limiter: %v", err)
			rateLimiter = nil
		} else {
			log.Info("DogStatsD rate limiter enabled")
		}
	}

	for {
		var n int
		var err error
		// retrieve an available packet from the packet pool,
		// which will be pushed back by the server when processed.
		packet := l.sharedPacketPoolManager.Get().(*packets.Packet)
		udsPackets.Add(1)

		var capBuff *replay.CaptureBuffer
		if l.trafficCapture != nil && l.trafficCapture.IsOngoing() {
			capBuff = replay.CapPool.Get().(*replay.CaptureBuffer)
			capBuff.Pb.Ancillary = nil
			capBuff.Pb.Payload = nil
			capBuff.ContainerID = ""
		}

		if l.OriginDetection {
			// Read datagram + credentials in ancillary data
			oob := l.oobPoolManager.Get().(*[]byte)
			oobS := *oob
			var oobn int

			if rateLimiter != nil {
				if err = rateLimiter.MayWait(); err != nil {
					log.Error(err)
				}
			}

			t2 = time.Now()
			tlmListener.Observe(float64(t2.Sub(t1).Nanoseconds()), "uds")

			n, oobn, _, _, err = l.conn.ReadMsgUnix(packet.Buffer, oobS)

			t1 = time.Now()

			// Extract container id from credentials
			pid, container, taggingErr := processUDSOrigin(oobS[:oobn])

			if capBuff != nil {
				capBuff.Pb.Timestamp = time.Now().UnixNano()
				capBuff.Pid = int32(pid)
				capBuff.Oob = oob
				capBuff.Buff = packet
				capBuff.Pb.AncillarySize = int32(oobn)
				capBuff.Pb.Ancillary = oobS[:oobn]
				capBuff.Pb.PayloadSize = int32(n)
				capBuff.Pb.Payload = packet.Buffer[:n]
				capBuff.Pb.Pid = int32(pid)
			}

			if taggingErr != nil {
				log.Warnf("dogstatsd-uds: error processing origin, data will not be tagged : %v", taggingErr)
				udsOriginDetectionErrors.Add(1)
				tlmUDSOriginDetectionError.Inc()
			} else {
				packet.Origin = container
				if capBuff != nil {
					capBuff.ContainerID = container
				}
			}
			// Return the buffer back to the pool for reuse
			l.oobPoolManager.Put(oob)
		} else {
			if rateLimiter != nil {
				if err = rateLimiter.MayWait(); err != nil {
					log.Error(err)
				}
			}

			t2 = time.Now()
			tlmListener.Observe(float64(t2.Sub(t1).Nanoseconds()), "uds")

			// Read only datagram contents with no credentials
			n, _, err = l.conn.ReadFromUnix(packet.Buffer)

			t1 = time.Now()

			if capBuff != nil {
				capBuff.Pb.Timestamp = time.Now().UnixNano()
				capBuff.Buff = packet
				capBuff.Pb.Pid = 0
				capBuff.Pb.AncillarySize = int32(0)
				capBuff.Pb.PayloadSize = int32(n)
				capBuff.Pb.Payload = packet.Buffer[:n]
			}
		}

		if capBuff != nil {
			l.trafficCapture.Enqueue(capBuff)
		}

		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return
			}

			log.Errorf("dogstatsd-uds: error reading packet: %v", err)
			udsPacketReadingErrors.Add(1)
			tlmUDSPackets.Inc("error")
			continue
		}
		tlmUDSPackets.Inc("ok")

		udsBytes.Add(int64(n))
		tlmUDSPacketsBytes.Add(float64(n))
		packet.Contents = packet.Buffer[:n]
		packet.Source = packets.UDS

		// packetsBuffer handles the forwarding of the packets to the dogstatsd server intake channel
		l.packetsBuffer.Append(packet)
	}
}

// Stop closes the UDS connection and stops listening
func (l *UDSListener) Stop() {
	l.packetsBuffer.Close()
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
