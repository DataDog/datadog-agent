// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package listeners

import (
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/nettest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
)

type udsListenerFactory func(packetOut chan packets.Packets, manager *packets.PoolManager[packets.Packet], cfg config.Component, pidMap pidmap.Component, telemetryStore *TelemetryStore, packetsTelemetryStore *packets.TelemetryStore, telemetry telemetry.Component) (StatsdListener, error)

func socketPathConfKey(transport string) string {
	if transport == "unix" {
		return "dogstatsd_stream_socket"
	}
	return "dogstatsd_socket"
}

func testSocketPath(t *testing.T) string {
	// https://github.com/golang/go/issues/62614
	path, err := nettest.LocalPath()
	assert.NoError(t, err)
	return path
}

func newPacketPoolManagerUDS(cfg config.Component, packetsTelemetryStore *packets.TelemetryStore) *packets.PoolManager[packets.Packet] {
	packetPoolUDS := packets.NewPool(cfg.GetInt("dogstatsd_buffer_size"), packetsTelemetryStore)
	return packets.NewPoolManager[packets.Packet](packetPoolUDS)
}

func testFileExistsNewUDSListener(t *testing.T, socketPath string, cfg map[string]interface{}, listenerFactory udsListenerFactory) {
	_, err := os.Create(socketPath)
	assert.Nil(t, err)
	defer os.Remove(socketPath)
	deps := fulfillDepsWithConfig(t, cfg)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	_, err = listenerFactory(nil, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	assert.Error(t, err)
}

func testSocketExistsNewUSDListener(t *testing.T, socketPath string, cfg map[string]interface{}, listenerFactory udsListenerFactory) {
	address, err := net.ResolveUnixAddr("unix", socketPath)
	assert.Nil(t, err)
	_, err = net.ListenUnix("unix", address)
	assert.Nil(t, err)
	testWorkingNewUDSListener(t, socketPath, cfg, listenerFactory)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string, cfg map[string]interface{}, listenerFactory udsListenerFactory) {
	deps := fulfillDepsWithConfig(t, cfg)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := listenerFactory(nil, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)
	fi, err := os.Stat(socketPath)
	require.Nil(t, err)
	assert.Equal(t, "Srwx-w--w-", fi.Mode().String())
}

func testNewUDSListener(t *testing.T, listenerFactory udsListenerFactory, transport string) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey(transport)] = socketPath

	t.Run("fail_file_exists", func(tt *testing.T) {
		testFileExistsNewUDSListener(tt, socketPath, mockConfig, listenerFactory)
	})
	t.Run("socket_exists", func(tt *testing.T) {
		testSocketExistsNewUSDListener(tt, socketPath, mockConfig, listenerFactory)
	})
	t.Run("working", func(tt *testing.T) {
		testWorkingNewUDSListener(tt, socketPath, mockConfig, listenerFactory)
	})
}

func testStartStopUDSListener(t *testing.T, listenerFactory udsListenerFactory, transport string) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey(transport)] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	deps := fulfillDepsWithConfig(t, mockConfig)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := listenerFactory(nil, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	s.Listen()

	conn, err := net.Dial(transport, socketPath)
	assert.Nil(t, err)
	conn.Close()

	s.Stop()

	_, err = net.Dial(transport, socketPath)
	assert.NotNil(t, err)
}

func defaultMUnixConn(addr net.Addr, streamMode bool) *mockUnixConn {
	return &mockUnixConn{addr: addr, streamMode: streamMode, stop: make(chan struct{}, 5)}
}

type mockUnixConn struct {
	addr       net.Addr
	buffer     [][]byte
	offset     int
	stop       chan struct{}
	streamMode bool
}

func (conn *mockUnixConn) Write(b []byte) (int, error) {
	if conn.streamMode {
		return conn.writeStream(b)
	}
	return conn.writeDatagram(b)
}

func (conn *mockUnixConn) writeDatagram(b []byte) (int, error) {
	conn.buffer = append(conn.buffer, b)
	return len(b), nil
}

func (conn *mockUnixConn) writeStream(b []byte) (int, error) {
	if len(conn.buffer) == 0 {
		conn.buffer = [][]byte{{}}
	}
	conn.buffer[0] = append(conn.buffer[0], b...)
	return len(b), nil
}

func (conn *mockUnixConn) Close() error {
	conn.stop <- struct{}{}
	return nil
}
func (conn *mockUnixConn) LocalAddr() net.Addr { return conn.addr }
func (conn *mockUnixConn) Read(b []byte) (int, error) {
	if conn.streamMode {
		return conn.readStream(b)
	}
	return conn.readDatagram(b)
}

func (conn *mockUnixConn) readDatagram(b []byte) (int, error) {
	if conn.offset >= len(conn.buffer) {
		select {
		case <-conn.stop:
		case <-time.After(time.Second * 2):
		}

		return 0, io.EOF
	}

	n := copy(b, conn.buffer[conn.offset])
	conn.offset++
	return n, nil
}

func (conn *mockUnixConn) readStream(b []byte) (int, error) {
	if conn.offset >= len(conn.buffer[0]) {
		select {
		case <-conn.stop:
		case <-time.After(time.Second * 2):
		}

		return 0, io.EOF
	}
	n := copy(b, conn.buffer[0][conn.offset:])
	conn.offset += n
	return n, nil
}

func (conn *mockUnixConn) ReadFromUnix(b []byte) (int, *net.UnixAddr, error) {
	n, _ := conn.Read(b)
	return n, nil, nil
}
func (conn *mockUnixConn) ReadMsgUnix(_ []byte, _ []byte) (n int, oobn int, flags int, addr *net.UnixAddr, err error) {
	return 0, 0, 0, nil, nil
}
func (conn *mockUnixConn) SyscallConn() (syscall.RawConn, error) {
	return nil, errors.New("Unimplemented")
}
func (conn *mockUnixConn) SetReadBuffer(_ int) error {
	return errors.New("Unimplemented")
}
func (conn *mockUnixConn) RemoteAddr() net.Addr {
	return conn.addr
}
func (conn *mockUnixConn) SetDeadline(_ time.Time) error {
	return errors.New("Unimplemented")
}
func (conn *mockUnixConn) SetReadDeadline(_ time.Time) error {
	return errors.New("Unimplemented")
}
func (conn *mockUnixConn) SetWriteDeadline(_ time.Time) error {
	return errors.New("Unimplemented")
}
