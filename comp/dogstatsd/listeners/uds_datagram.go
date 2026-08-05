// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	pidmap "github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/def"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/fdhandoff"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// UDSDatagramListener implements the StatsdListener interface for Unix Domain (datagrams)
type UDSDatagramListener struct {
	UDSListener

	conn *net.UnixConn
}

// NewUDSDatagramListener returns an idle UDS datagram Statsd listener
func NewUDSDatagramListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager[packets.Packet], sharedOobPoolManager *packets.PoolManager[[]byte], cfg model.Reader, capture replay.Component, wmeta option.Option[workloadmeta.Component], pidMap pidmap.Component, telemetryStore *TelemetryStore, packetsTelemetryStore *packets.TelemetryStore, telemetryComponent telemetry.Component) (*UDSDatagramListener, error) {
	socketPath := cfg.GetString("dogstatsd_socket")
	transport := "unixgram"
	originDetection := cfg.GetBool("dogstatsd_origin_detection")

	var conn *net.UnixConn
	var err error
	if handoffPath := cfg.GetString("dogstatsd_socket_fd_from"); handoffPath != "" {
		conn, originDetection, err = adoptUDSDatagramConn(handoffPath, socketPath, originDetection)
	} else {
		conn, originDetection, err = listenUDSDatagram(socketPath, transport, originDetection)
	}
	if err != nil {
		return nil, err
	}

	l, err := NewUDSListener(packetOut, sharedPacketPoolManager, sharedOobPoolManager, cfg, capture, transport, wmeta, pidMap, telemetryStore, packetsTelemetryStore, telemetryComponent, originDetection)
	if err != nil {
		return nil, err
	}

	listener := &UDSDatagramListener{
		UDSListener: *l,
		conn:        conn,
	}

	log.Infof("dogstatsd-uds: %s successfully initialized", conn.LocalAddr())
	return listener, nil
}

// listenUDSDatagram binds the DogStatsD datagram socket, removing a stale socket
// file if needed. It returns the connection and whether origin detection could
// actually be enabled on it.
func listenUDSDatagram(socketPath string, transport string, originDetection bool) (*net.UnixConn, bool, error) {
	_, err := setupSocketBeforeListen(socketPath, transport)
	if err != nil {
		return nil, originDetection, err
	}

	conf := net.ListenConfig{
		Control: func(_, address string, c syscall.RawConn) (err error) {
			originDetection, err = setupUnixConn(c, originDetection, address)
			return
		},
	}

	connGeneric, err := conf.ListenPacket(context.Background(), transport, socketPath)
	if err != nil {
		return nil, originDetection, fmt.Errorf("can't listen: %s", err)
	}

	conn, ok := connGeneric.(*net.UnixConn)
	if !ok {
		return nil, originDetection, fmt.Errorf("unexpected return type from ListenPacket, expected UnixConn: %#v", connGeneric)
	}

	err = setSocketWriteOnly(socketPath)
	if err != nil {
		return nil, originDetection, err
	}

	return conn, originDetection, nil
}

// adoptUDSDatagramConn adopts the DogStatsD datagram socket held by a socket
// holder process, by receiving its file descriptor over the handoff socket. The
// socket is neither unlinked nor rebound, so clients keep talking to the same
// socket inode across Agent restarts.
func adoptUDSDatagramConn(handoffPath string, socketPath string, originDetection bool) (*net.UnixConn, bool, error) {
	conn, err := fdhandoff.ReceivePacketConn(handoffPath)
	if err != nil {
		return nil, originDetection, fmt.Errorf("can't adopt the dogstatsd socket: %s", err)
	}

	// Clients send to dogstatsd_socket, so a holder bound to another path means
	// the Agent listens on a socket nobody writes to and reports no error.
	if adopted := conn.LocalAddr().String(); adopted != socketPath {
		log.Warnf("dogstatsd-uds: %s handed off %s, which is not the configured dogstatsd_socket %s: clients writing to %s will not be read",
			handoffPath, adopted, socketPath, socketPath)
	}

	if originDetection {
		// The holder already enables credential passing, but the option is
		// idempotent and setting it here keeps origin detection working with a
		// holder that did not.
		rawConn, err := conn.SyscallConn()
		if err != nil {
			conn.Close()
			return nil, originDetection, fmt.Errorf("can't access the adopted dogstatsd socket: %s", err)
		}
		originDetection, _ = setupUnixConn(rawConn, originDetection, conn.LocalAddr().String())
	}

	log.Infof("dogstatsd-uds: adopted the socket held by %s", handoffPath)
	return conn, originDetection, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSDatagramListener) Listen() {
	l.listenWg.Add(1)
	go func() {
		defer l.listenWg.Done()
		l.listen()
	}()
}

func (l *UDSDatagramListener) listen() {
	log.Infof("dogstatsd-uds: starting to listen on %s", l.conn.LocalAddr())
	err := l.handleConnection(l.conn, func(conn netUnixConn) error {
		return conn.Close()
	})
	if err != nil {
		log.Errorf("dogstatsd-uds: error handling connection: %v", err)
	}

}

// Stop closes the UDS connection and stops listening
func (l *UDSDatagramListener) Stop() {
	err := l.conn.Close()
	if err != nil {
		log.Errorf("dogstatsd-uds: error closing connection: %s", err)
	}
	l.UDSListener.Stop()
}
