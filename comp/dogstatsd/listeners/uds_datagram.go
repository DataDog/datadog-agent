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

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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

	_, err := setupSocketBeforeListen(socketPath, transport)
	if err != nil {
		return nil, err
	}

	originDetection := cfg.GetBool("dogstatsd_origin_detection")

	conf := net.ListenConfig{
		Control: func(_, address string, c syscall.RawConn) (err error) {
			originDetection, err = setupUnixConn(c, originDetection, address)
			return
		},
	}

	connGeneric, err := conf.ListenPacket(context.Background(), transport, socketPath)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	conn, ok := connGeneric.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("unexpected return type from ListenPacket, expected UnixConn: %#v", connGeneric)
	}

	err = setSocketWriteOnly(socketPath)
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
