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
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners/ratelimit"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
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
	packetOut               chan packets.Packets
	sharedPacketPoolManager *packets.PoolManager[packets.Packet]
	oobPoolManager          *packets.PoolManager[[]byte]
	trafficCapture          replay.Component
	pidMap                  pidmap.Component
	OriginDetection         bool
	config                  model.Reader

	wmeta option.Option[workloadmeta.Component]

	transport string

	dogstatsdMemBasedRateLimiter bool

	packetBufferSize         uint
	packetBufferFlushTimeout time.Duration
	telemetryWithListenerID  bool

	listenWg *sync.WaitGroup

	// telemetry
	telemetry             telemetry.Component
	telemetryStore        *TelemetryStore
	packetsTelemetryStore *packets.TelemetryStore
}

// Wrapper for net.UnixConn
type netUnixConn interface {
	Close() error
	LocalAddr() net.Addr
	Read(b []byte) (int, error)
	ReadFromUnix(b []byte) (int, *net.UnixAddr, error)
	ReadMsgUnix(b []byte, oob []byte) (n int, oobn int, flags int, addr *net.UnixAddr, err error)
	SyscallConn() (syscall.RawConn, error)
	SetReadBuffer(bytes int) error
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Write(b []byte) (n int, err error)
}

// CloseFunction is a function that closes a connection
type CloseFunction func(unixConn netUnixConn) error

func setupUnixConn(conn syscall.RawConn, originDetection bool, address string) (bool, error) {
	if originDetection {
		err := enableUDSPassCred(conn)
		if err != nil {
			log.Errorf("dogstatsd-uds: error enabling origin detection: %s", err)
			originDetection = false
		} else {
			log.Debugf("dogstatsd-uds: enabling origin detection on %s", address)
		}
	}

	return originDetection, nil
}

func setupSocketBeforeListen(socketPath string, transport string) (*net.UnixAddr, error) {
	address, addrErr := net.ResolveUnixAddr(transport, socketPath)
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
func NewUDSOobPoolManager() *packets.PoolManager[[]byte] {
	pool := ddsync.NewSlicePool[byte](getUDSAncillarySize(), getUDSAncillarySize())
	return packets.NewPoolManager[[]byte](pool)
}

// NewUDSListener returns an idle UDS Statsd listener
func NewUDSListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager[packets.Packet], sharedOobPacketPoolManager *packets.PoolManager[[]byte], cfg model.Reader, capture replay.Component, transport string, wmeta option.Option[workloadmeta.Component], pidMap pidmap.Component, telemetryStore *TelemetryStore, packetsTelemetryStore *packets.TelemetryStore, telemetry telemetry.Component, originDetection bool) (*UDSListener, error) {
	listener := &UDSListener{
		OriginDetection:              originDetection,
		packetOut:                    packetOut,
		sharedPacketPoolManager:      sharedPacketPoolManager,
		trafficCapture:               capture,
		pidMap:                       pidMap,
		dogstatsdMemBasedRateLimiter: cfg.GetBool("dogstatsd_mem_based_rate_limiter.enabled"),
		config:                       cfg,
		transport:                    transport,
		packetBufferSize:             uint(cfg.GetInt("dogstatsd_packet_buffer_size")),
		packetBufferFlushTimeout:     cfg.GetDuration("dogstatsd_packet_buffer_flush_timeout"),
		telemetryWithListenerID:      cfg.GetBool("dogstatsd_telemetry_enabled_listener_id"),
		listenWg:                     &sync.WaitGroup{},
		wmeta:                        wmeta,
		telemetryStore:               telemetryStore,
		packetsTelemetryStore:        packetsTelemetryStore,
		telemetry:                    telemetry,
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
func (l *UDSListener) handleConnection(conn netUnixConn, closeFunc CloseFunction) error {
	listenerID := l.getListenerID(conn)
	tlmListenerID := listenerID
	telemetryWithFullListenerID := l.telemetryWithListenerID
	if !telemetryWithFullListenerID {
		// In case we don't want the full listener id, we only keep the transport.
		tlmListenerID = "uds-" + conn.LocalAddr().Network()
	}

	packetsBuffer := packets.NewBuffer(
		l.packetBufferSize,
		l.packetBufferFlushTimeout,
		l.packetOut,
		tlmListenerID,
		l.packetsTelemetryStore,
	)
	l.telemetryStore.tlmUDSConnections.Inc(tlmListenerID, l.transport)
	defer func() {
		_ = closeFunc(conn)
		packetsBuffer.Close()
		if telemetryWithFullListenerID {
			l.clearTelemetry(tlmListenerID)
		}
		l.telemetryStore.tlmUDSConnections.Dec(tlmListenerID, l.transport)
	}()

	t1 := time.Now()
	var t2 time.Time
	log.Debugf("dogstatsd-uds: starting to handle %s", conn.LocalAddr())

	var rateLimiter *ratelimit.MemBasedRateLimiter
	if l.dogstatsdMemBasedRateLimiter {
		var err error
		rateLimiter, err = ratelimit.BuildMemBasedRateLimiter(l.config, l.telemetry)
		if err != nil {
			log.Errorf("Cannot use DogStatsD rate limiter: %v", err)
			rateLimiter = nil
		} else {
			log.Info("DogStatsD rate limiter enabled")
		}
	}

	if rcvbuf := l.config.GetInt("dogstatsd_so_rcvbuf"); rcvbuf != 0 {
		if err := conn.SetReadBuffer(rcvbuf); err != nil {
			log.Warnf("could not set socket rcvbuf: %s", err)
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
		packet := l.sharedPacketPoolManager.Get()
		udsPackets.Add(1)

		var capBuff *replay.CaptureBuffer
		if l.trafficCapture != nil && l.trafficCapture.IsOngoing() {
			capBuff = new(replay.CaptureBuffer)
			capBuff.Pb.Ancillary = nil
			capBuff.Pb.Payload = nil
			capBuff.Pb.Pid = 0
			capBuff.Pb.AncillarySize = int32(0)
			capBuff.Pb.PayloadSize = int32(0)
			capBuff.ContainerID = ""
		}

		if l.OriginDetection {
			// Read datagram + credentials in ancillary data
			oob = l.oobPoolManager.Get()
			oobS = *oob
		}

		if rateLimiter != nil {
			if err = rateLimiter.MayWait(); err != nil {
				log.Error(err)
			}
		}

		t2 = time.Now()
		l.telemetryStore.tlmListener.Observe(float64(t2.Sub(t1).Nanoseconds()), tlmListenerID, l.transport, "uds")

		var expectedPacketLength uint32
		var maxPacketLength uint32
		if l.transport == "unix" {
			// Read the expected packet length (in stream mode)
			b := []byte{0, 0, 0, 0}
			_, err = io.ReadFull(conn, b)
			if err != nil {
				switch {
				case errors.Is(err, io.EOF):
					log.Debugf("dogstatsd-uds: %s connection closed", l.transport)
				case errors.Is(err, io.ErrUnexpectedEOF):
					log.Errorf("dogstatsd-uds: %s connection closed while reading payload length", l.transport)
				default:
					log.Errorf("dogstatsd-uds: %s: error reading payload length: %v", l.transport, err)
				}
				return nil
			}
			expectedPacketLength = binary.LittleEndian.Uint32(b)
			if expectedPacketLength > uint32(len(packet.Buffer)) {
				log.Info("dogstatsd-uds: packet length too large, dropping connection")
				return nil
			}
			maxPacketLength = expectedPacketLength
		} else {
			maxPacketLength = uint32(len(packet.Buffer))
		}

		for err == nil {
			var nRead int
			if oob != nil {
				nRead, oobn, _, _, err = conn.ReadMsgUnix(packet.Buffer[n:maxPacketLength], oobS)
			} else {
				nRead, _, err = conn.ReadFromUnix(packet.Buffer[n:maxPacketLength])
			}
			n += nRead

			if nRead == 0 && oobn == 0 && l.transport == "unix" {
				log.Debugf("dogstatsd-uds: %s connection closed", l.transport)
				return nil
			}
			// If framing is disabled (unixgram, unixpacket), we always will have read the whole packet
			if expectedPacketLength == 0 {
				break
			}
			// Otherwise see if we need to continue to accumulate bytes or not
			if uint32(n) == expectedPacketLength {
				break
			}
			if uint32(n) > expectedPacketLength {
				log.Info("dogstatsd-uds: read length mismatch, dropping connection")
				return nil
			}
		}

		t1 = time.Now()

		if oob != nil {
			// Extract container id from credentials
			pid, container, taggingErr := processUDSOrigin(oobS[:oobn], l.wmeta, l.pidMap)
			if taggingErr != nil {
				log.Warnf("dogstatsd-uds: error processing origin, data will not be tagged : %v", taggingErr)
				udsOriginDetectionErrors.Add(1)
				l.telemetryStore.tlmUDSOriginDetectionError.Inc(tlmListenerID, l.transport)
			} else {
				packet.ProcessID = uint32(pid)
				packet.Origin = container
				if capBuff != nil {
					capBuff.ContainerID = container
				}
			}
			if capBuff != nil {
				capBuff.Oob = oob
				capBuff.Pid = int32(pid)
				capBuff.Pb.Pid = int32(pid)
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
			if errors.Is(err, net.ErrClosed) {
				return nil
			}

			log.Errorf("dogstatsd-uds: error reading packet: %v", err)
			udsPacketReadingErrors.Add(1)
			l.telemetryStore.tlmUDSPackets.Inc(tlmListenerID, l.transport, "error")
			continue
		}
		l.telemetryStore.tlmUDSPackets.Inc(tlmListenerID, l.transport, "ok")

		udsBytes.Add(int64(n))
		l.telemetryStore.tlmUDSPacketsBytes.Add(float64(n), tlmListenerID, l.transport)
		packet.Contents = packet.Buffer[:n]
		packet.Source = packets.UDS
		packet.ListenerID = listenerID

		// packetsBuffer handles the forwarding of the packets to the dogstatsd server intake channel
		packetsBuffer.Append(packet)
	}
}

func (l *UDSListener) getConnID(conn netUnixConn) string {
	// We use the file descriptor as a unique identifier for the connection. This might
	// increase the cardinality in the backend, but this option is not designed to be enabled
	// all the time. Plus is it useful to debug issues with the UDS listener since we will be
	// able to use external tools to get additional stats about the socket/fd.
	var fdConn uintptr
	rawConn, err := conn.SyscallConn()
	if err != nil {
		log.Errorf("dogstatsd-uds: error getting file from connection: %s", err)
	} else {
		_ = rawConn.Control(func(fd uintptr) { fdConn = fd })
	}
	return strconv.Itoa(int(fdConn))
}
func (l *UDSListener) getListenerID(conn netUnixConn) string {
	listenerID := "uds-" + conn.LocalAddr().Network()
	connID := l.getConnID(conn)
	if connID != "" {
		listenerID += "-" + connID
	}
	return listenerID
}

// Stop closes the UDS connection and stops listening
func (l *UDSListener) Stop() {
	// Socket cleanup on exit is not necessary as sockets are automatically removed by go.
	l.listenWg.Wait()
}

func (l *UDSListener) clearTelemetry(id string) {
	if id == "" {
		return
	}
	// Since the listener id is volatile we need to make sure we clear the telemetry.
	l.telemetryStore.tlmListener.Delete(id, l.transport)
	l.telemetryStore.tlmUDSConnections.Delete(id, l.transport)
	l.telemetryStore.tlmUDSPackets.Delete(id, l.transport, "error")
	l.telemetryStore.tlmUDSPackets.Delete(id, l.transport, "ok")
	l.telemetryStore.tlmUDSPacketsBytes.Delete(id, l.transport)
}
