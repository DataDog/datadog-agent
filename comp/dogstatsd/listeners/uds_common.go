// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"expvar"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
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
	packetOut               chan packets.Packets
	sharedPacketPoolManager *packets.PoolManager
	oobPoolManager          *packets.PoolManager
	trafficCapture          replay.Component
	OriginDetection         bool
	config                  config.Reader

	transport string

	dogstatsdMemBasedRateLimiter bool

	packetBufferSize         uint
	packetBufferFlushTimeout time.Duration
	telemetryWithListenerID  bool

	listenWg *sync.WaitGroup
}

// CloseFunction is a function that closes a connection
type CloseFunction func(unixConn *net.UnixConn) error

func setupUnixConn(conn *net.UnixConn, originDetection bool, config config.Reader) (bool, error) {
	panic("not called")
}

func setupSocketBeforeListen(socketPath string, transport string) (*net.UnixAddr, error) {
	panic("not called")
}

func setSocketWriteOnly(socketPath string) error {
	panic("not called")
}

// NewUDSOobPoolManager returns an UDS OOB pool manager
func NewUDSOobPoolManager() *packets.PoolManager {
	panic("not called")
}

// NewUDSListener returns an idle UDS Statsd listener
func NewUDSListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager, sharedOobPacketPoolManager *packets.PoolManager, cfg config.Reader, capture replay.Component, transport string) (*UDSListener, error) {
	panic("not called")
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSListener) handleConnection(conn *net.UnixConn, closeFunc CloseFunction) error {
	panic("not called")
}

func (l *UDSListener) getConnID(conn *net.UnixConn) string {
	panic("not called")
}
func (l *UDSListener) getListenerID(conn *net.UnixConn) string {
	panic("not called")
}

// Stop closes the UDS connection and stops listening
func (l *UDSListener) Stop() {
	panic("not called")
}

func (l *UDSListener) clearTelemetry(id string) {
	panic("not called")
}
