// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"encoding/binary"
	"errors"
	"expvar"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners/ratelimit"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	"github.com/DataDog/datadog-agent/pkg/config"
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
	packetsBuffer           *packets.Buffer
	sharedPacketPoolManager *packets.PoolManager
	oobPoolManager          *packets.PoolManager
	trafficCapture          replay.Component
	OriginDetection         bool
	config                  config.Reader

	network string

	dogstatsdMemBasedRateLimiter bool
}

func setupUnixConn(conn *net.UnixConn, originDetection bool, config config.ConfigReader) error {
	if originDetection {
		err := enableUDSPassCred(conn)
		if err != nil {
			log.Errorf("dogstatsd-uds: error enabling origin detection: %s", err)
			originDetection = false
		} else {
			log.Debugf("dogstatsd-uds: enabling origin detection on %s", conn.LocalAddr())
		}
	}

	if rcvbuf := config.GetInt("dogstatsd_so_rcvbuf"); rcvbuf != 0 {
		if err := conn.SetReadBuffer(rcvbuf); err != nil {
			return fmt.Errorf("could not set socket rcvbuf: %s", err)
		}
	}
	return nil
}

func setupSocketBeforeListen(socketPath string, network string) (*net.UnixAddr, error) {
	address, addrErr := net.ResolveUnixAddr(network, socketPath)
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
	return address, nil
}

func setSocketWriteOnly(socketPath string) error {
	err := os.Chmod(socketPath, 0722)
	if err != nil {
		return fmt.Errorf("can't set the socket at write only: %s", err)
	}
	return nil
}

// NewUDSOobPoolManager returns an UDS OOB pool manager
func NewUDSOobPoolManager() *packets.PoolManager {
	pool := &sync.Pool{
		New: func() interface{} {
			s := make([]byte, getUDSAncillarySize())
			return &s
		},
	}

	return packets.NewPoolManager(pool)
}

// NewUDSListener returns an idle UDS Statsd listener
func NewUDSListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager, sharedOobPacketPoolManager *packets.PoolManager, cfg config.ConfigReader, capture replay.Component, network string) (*UDSListener, error) {
	originDetection := cfg.GetBool("dogstatsd_origin_detection")

	listener := &UDSListener{
		OriginDetection: originDetection,
		packetsBuffer: packets.NewBuffer(uint(cfg.GetInt("dogstatsd_packet_buffer_size")),
			cfg.GetDuration("dogstatsd_packet_buffer_flush_timeout"), packetOut),
		sharedPacketPoolManager:      sharedPacketPoolManager,
		trafficCapture:               capture,
		dogstatsdMemBasedRateLimiter: cfg.GetBool("dogstatsd_mem_based_rate_limiter.enabled"),
		config:                       cfg,
		network:                      network,
	}

	// Init the oob buffer pool if origin detection is enabled
	if originDetection {
		listener.oobPoolManager = sharedOobPacketPoolManager
		if listener.oobPoolManager == nil {
			listener.oobPoolManager = NewUDSOobPoolManager()
		}

	}

	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSListener) handleConnection(conn *net.UnixConn) error {
	defer func() {
		tlmUDSConnections.Dec(l.network)
		_ = conn.Close()
	}()
	tlmUDSConnections.Inc(l.network)

	err := setupUnixConn(conn, l.OriginDetection, l.config)
	if err != nil {
		return err
	}

	t1 := time.Now()
	var t2 time.Time
	log.Debugf("dogstatsd-uds: starting to handle %s", conn.LocalAddr())

	var rateLimiter *ratelimit.MemBasedRateLimiter
	if l.dogstatsdMemBasedRateLimiter {
		var err error
		rateLimiter, err = ratelimit.BuildMemBasedRateLimiter(l.config)
		if err != nil {
			log.Errorf("Cannot use DogStatsD rate limiter: %v", err)
			rateLimiter = nil
		} else {
			log.Info("DogStatsD rate limiter enabled")
		}
	}

	for {
		var n int
		var oobn int
		var oob *[]byte
		var oobS []byte
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
			capBuff.Pb.Pid = 0
			capBuff.Pb.AncillarySize = int32(0)
			capBuff.Pb.PayloadSize = int32(0)
			capBuff.ContainerID = ""
		}

		if l.OriginDetection {
			// Read datagram + credentials in ancillary data
			oob = l.oobPoolManager.Get().(*[]byte)
			oobS = *oob
		}

		if rateLimiter != nil {
			if err = rateLimiter.MayWait(); err != nil {
				log.Error(err)
			}
		}

		t2 = time.Now()
		tlmListener.Observe(float64(t2.Sub(t1).Nanoseconds()), l.network, "uds")

		var expectedPacketLength int32
		var maxPacketLength int32
		if l.network == "unix" {
			// Read the expected packet length (in stream mode)
			err = binary.Read(conn, binary.BigEndian, &expectedPacketLength)
			switch {
			case err == io.EOF, errors.Is(err, io.ErrUnexpectedEOF):
				log.Debugf("dogstatsd-uds: %s connection closed", l.network)
				return nil
			}
			maxPacketLength = expectedPacketLength
		} else {
			maxPacketLength = int32(len(packet.Buffer))
		}

		for err == nil {
			if oob != nil {
				n, oobn, _, _, err = conn.ReadMsgUnix(packet.Buffer[n:maxPacketLength], oobS[oobn:])
			} else {
				n, _, err = conn.ReadFromUnix(packet.Buffer[n:maxPacketLength])
			}
			if n == 0 && oobn == 0 {
				log.Debugf("dogstatsd-uds: %s connection closed", l.network)
				return nil
			}
			// If framing is disabled (unixgram, unixpacket), we always will have read the whole packet
			if expectedPacketLength == 0 {
				break
			}
			// Otherwise see if we need to continue to accumulate bytes or not
			if int32(n) == expectedPacketLength {
				break
			}
			if int32(n) > expectedPacketLength {
				log.Info("dogstatsd-uds: read length mismatch, dropping connection")
				return nil
			}
		}

		t1 = time.Now()

		if oob != nil {
			// Extract container id from credentials
			pid, container, taggingErr := processUDSOrigin(oobS[:oobn])
			if taggingErr != nil {
				log.Warnf("dogstatsd-uds: error processing origin, data will not be tagged : %v", taggingErr)
				udsOriginDetectionErrors.Add(1)
				tlmUDSOriginDetectionError.Inc(l.network)
			} else {
				packet.Origin = container
				if capBuff != nil {
					capBuff.ContainerID = container
				}
			}
			if capBuff != nil {
				capBuff.Oob = oob
				capBuff.Pid = int32(pid)
				capBuff.Pb.AncillarySize = int32(oobn)
				capBuff.Pb.Ancillary = oobS[:oobn]
			}

			// Return the buffer back to the pool for reuse
			l.oobPoolManager.Put(oob)
		}

		if capBuff != nil {
			capBuff.Buff = packet
			capBuff.Pb.Timestamp = time.Now().UnixNano()
			capBuff.Pb.PayloadSize = int32(n)
			capBuff.Pb.Payload = packet.Buffer[:n]

			l.trafficCapture.Enqueue(capBuff)
		}

		if err != nil {
			// connection has been closed
			if strings.HasSuffix(err.Error(), " use of closed network connection") {
				return nil
			}

			log.Errorf("dogstatsd-uds: error reading packet: %v", err)
			udsPacketReadingErrors.Add(1)
			tlmUDSPackets.Inc(l.network, "error")
			continue
		}
		tlmUDSPackets.Inc(l.network, "ok")

		udsBytes.Add(int64(n))
		tlmUDSPacketsBytes.Add(float64(n), l.network)
		packet.Contents = packet.Buffer[:n]
		packet.Source = packets.UDS

		// packetsBuffer handles the forwarding of the packets to the dogstatsd server intake channel
		l.packetsBuffer.Append(packet)
	}
}

// Stop closes the UDS connection and stops listening
func (l *UDSListener) Stop() {
	l.packetsBuffer.Close()
	// Socket cleanup on exit is not necessary as sockets are automatically removed by go.
}
