// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package listeners

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	pidmap "github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func udsStreamListenerFactory(packetOut chan packets.Packets, manager *packets.PoolManager[packets.Packet], cfg config.Component, pidMap pidmap.Component, telemetryStore *TelemetryStore, packetsTelemetryStore *packets.TelemetryStore, telemetry telemetry.Component) (StatsdListener, error) {
	return NewUDSStreamListener(packetOut, manager, nil, cfg, nil, option.None[workloadmeta.Component](), pidMap, telemetryStore, packetsTelemetryStore, telemetry)
}

func TestNewUDSStreamListener(t *testing.T) {
	testNewUDSListener(t, udsStreamListenerFactory, "unix")
}

func TestStartStopUDSStreamListener(t *testing.T) {
	testStartStopUDSListener(t, udsStreamListenerFactory, "unix")
}

func TestUDSStreamReceive(t *testing.T) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey("unix")] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	var contents0 = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")
	var contents1 = []byte("daemon:999|g|#sometag1:somevalue1")

	packetsChannel := make(chan packets.Packets)

	deps := fulfillDepsWithConfig(t, mockConfig)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := udsStreamListenerFactory(packetsChannel, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	mConn := defaultMUnixConn(s.(*UDSStreamListener).conn.Addr(), true)
	defer s.Stop()

	binary.Write(mConn, binary.LittleEndian, int32(len(contents0)))
	mConn.Write(contents0)

	binary.Write(mConn, binary.LittleEndian, int32(len(contents1)))
	mConn.Write(contents1)

	go s.(*UDSStreamListener).handleConnection(mConn, func(c netUnixConn) error { return c.Close() })

	select {
	case pkts := <-packetsChannel:
		assert.Equal(t, 2, len(pkts))

		packet := pkts[0]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents0)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

		packet = pkts[1]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents1)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

// trackingPacketPool makes release observable — every Get must be matched by
// exactly one Put; duck-types the pool interface.
type trackingPacketPool struct {
	sync.Mutex
	outstanding int
	returned    int
}

func (p *trackingPacketPool) Get() *packets.Packet {
	p.Lock()
	defer p.Unlock()
	p.outstanding++
	return &packets.Packet{Buffer: make([]byte, 8192)}
}

func (p *trackingPacketPool) Put(_ *packets.Packet) {
	p.Lock()
	defer p.Unlock()
	p.outstanding--
	p.returned++
}

func (p *trackingPacketPool) stats() (outstanding, returned int) {
	p.Lock()
	defer p.Unlock()
	return p.outstanding, p.returned
}

// TestUDSStreamFailedReadsReleaseBuffers: framing errors that drop the
// connection must still return the read buffers, though neither capture nor
// server receives the packet.
func TestUDSStreamFailedReadsReleaseBuffers(t *testing.T) {
	handle := func(t *testing.T, newConn func(addr net.Addr) *mockUnixConn) {
		t.Helper()
		pool := &trackingPacketPool{}
		deps := fulfillDepsWithConfig(t, map[string]interface{}{
			socketPathConfKey("unix"):      testSocketPath(t),
			"dogstatsd_origin_detection":   false,
			"dogstatsd_stream_log_too_big": false,
		})
		packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
		s, err := NewUDSStreamListener(nil, packets.NewPoolManager[packets.Packet](pool), nil, deps.Config, nil,
			option.None[workloadmeta.Component](), deps.PidMap, NewTelemetryStore(nil, deps.Telemetry), packetsTelemetryStore, deps.Telemetry)
		require.NoError(t, err)
		defer s.Stop()

		mConn := newConn(s.conn.Addr())
		require.NoError(t, s.handleConnection(mConn, func(c netUnixConn) error { return c.Close() }))

		outstanding, returned := pool.stats()
		require.Zero(t, outstanding, "failed reads must return the packet buffer to the pool")
		require.Equal(t, 1, returned)
	}

	t.Run("connection closed while reading the length header", func(t *testing.T) {
		handle(t, func(addr net.Addr) *mockUnixConn {
			mConn := defaultMUnixConn(addr, true)
			mConn.Write([]byte{1}) // partial length header
			mConn.Close()
			return mConn
		})
	})

	t.Run("packet too large", func(t *testing.T) {
		handle(t, func(addr net.Addr) *mockUnixConn {
			mConn := defaultMUnixConn(addr, true)
			binary.Write(mConn, binary.LittleEndian, uint32(8193))
			return mConn
		})
	})

	t.Run("connection closed before the payload", func(t *testing.T) {
		handle(t, func(addr net.Addr) *mockUnixConn {
			mConn := defaultMUnixConn(addr, true)
			binary.Write(mConn, binary.LittleEndian, uint32(8))
			mConn.Close()
			return mConn
		})
	})
}
