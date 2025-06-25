// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// UDSStreamListener implements the StatsdListener interface for Unix Domain (streams)
type UDSStreamListener struct {
	UDSListener
	connTracker *ConnectionTracker
	conn        *net.UnixListener
}

// NewUDSStreamListener returns an idle UDS datagram Statsd listener
func NewUDSStreamListener(packetOut chan packets.Packets, sharedPacketPoolManager *packets.PoolManager[packets.Packet], sharedOobPacketPoolManager *packets.PoolManager[[]byte], cfg model.Reader, capture replay.Component, wmeta option.Option[workloadmeta.Component], pidMap pidmap.Component, telemetryStore *TelemetryStore, packetsTelemetryStore *packets.TelemetryStore, telemetry telemetry.Component) (*UDSStreamListener, error) {
	socketPath := cfg.GetString("dogstatsd_stream_socket")
	transport := "unix"

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

	unixListener, err := conf.Listen(context.Background(), transport, socketPath)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	conn, ok := unixListener.(*net.UnixListener)
	if !ok {
		return nil, fmt.Errorf("unexpected return type from Listen, expected UnixConn: %#v", unixListener)
	}

	err = setSocketWriteOnly(socketPath)
	if err != nil {
		return nil, err
	}

	l, err := NewUDSListener(packetOut, sharedPacketPoolManager, sharedOobPacketPoolManager, cfg, capture, transport, wmeta, pidMap, telemetryStore, packetsTelemetryStore, telemetry, originDetection)
	if err != nil {
		return nil, err
	}

	listener := &UDSStreamListener{
		UDSListener: *l,
		connTracker: NewConnectionTracker(transport, 1*time.Second),
		conn:        conn,
	}

	log.Infof("dogstatsd-uds-stream: %s successfully initialized", conn.Addr())
	return listener, nil
}

// Listen runs the intake loop. Should be called in its own goroutine
func (l *UDSStreamListener) Listen() {
	l.listenWg.Add(1)
	go func() {
		defer l.listenWg.Done()
		l.listen()
	}()
}

func (l *UDSStreamListener) listen() {

	l.connTracker.Start()
	log.Infof("dogstatsd-uds-stream: starting to listen on %s", l.conn.Addr())
	for {
		conn, err := l.conn.AcceptUnix()
		if err != nil {
			if !strings.HasSuffix(err.Error(), " use of closed network connection") {
				log.Errorf("dogstatsd-uds: error accepting connection: %v", err)
			}
			break
		}
		go func() {
			l.connTracker.Track(conn)
			err = l.handleConnection(conn, func(c netUnixConn) error {
				l.connTracker.Close(c)
				return nil
			})
			if err != nil {
				log.Errorf("dogstatsd-uds-stream: error handling connection: %v", err)
			}
		}()
	}
}

// Stop closes the UDS connection and stops listening
func (l *UDSStreamListener) Stop() {
	_ = l.conn.Close()
	l.connTracker.Stop()
	l.UDSListener.Stop()
}
