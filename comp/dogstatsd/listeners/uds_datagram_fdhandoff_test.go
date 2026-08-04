// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

// The socket handoff relies on SCM_RIGHTS, which is not available on windows.

package listeners

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/fdhandoff"
)

// startHoldSocketServer binds socketPath the way the dsd-socket-holder binary
// does and serves its file descriptor on a handoff socket, whose path is
// returned.
func startHoldSocketServer(t *testing.T, socketPath string) string {
	t.Helper()

	socket, err := fdhandoff.BindDatagramSocket(socketPath)
	require.NoError(t, err)

	handoffPath := testSocketPath(t)
	server, err := fdhandoff.NewServer(handoffPath, 0700, socket)
	require.NoError(t, err)
	server.ErrorHandler = func(err error) { t.Errorf("handoff failed: %v", err) }

	go func() {
		// Serve returns when the listener is closed at the end of the test.
		_ = server.Serve()
	}()

	t.Cleanup(func() {
		assert.NoError(t, server.Close())
		assert.NoError(t, socket.Close())
	})

	return handoffPath
}

func TestUDSDatagramReceiveFromHandoff(t *testing.T) {
	socketPath := testSocketPath(t)
	handoffPath := startHoldSocketServer(t, socketPath)

	mockConfig := map[string]interface{}{}
	mockConfig["dogstatsd_socket"] = socketPath
	mockConfig["dogstatsd_socket_fd_from"] = handoffPath
	mockConfig["dogstatsd_origin_detection"] = false

	contents := []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	packetsChannel := make(chan packets.Packets)

	deps := fulfillDepsWithConfig(t, mockConfig)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := udsDatagramListenerFactory(packetsChannel, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	require.NoError(t, err)
	require.NotNil(t, s)
	defer s.Stop()

	// The listener adopted the socket held by the server, it did not rebind it.
	assert.Equal(t, socketPath, s.(*UDSDatagramListener).conn.LocalAddr().String())
	assert.Equal(t, "unixgram", s.(*UDSDatagramListener).conn.LocalAddr().Network())

	s.Listen()

	conn, err := net.Dial("unixgram", socketPath)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write(contents)
	require.NoError(t, err)

	select {
	case pkts := <-packetsChannel:
		require.Len(t, pkts, 1)
		assert.Equal(t, contents, pkts[0].Contents)
		assert.Equal(t, packets.UDS, pkts[0].Source)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

// TestUDSDatagramHandoffKeepsSocketAlive asserts that the socket file survives
// the Agent listener being stopped, which is the whole point of the handoff: a
// client can keep sending to the same socket while the Agent restarts.
func TestUDSDatagramHandoffKeepsSocketAlive(t *testing.T) {
	socketPath := testSocketPath(t)
	handoffPath := startHoldSocketServer(t, socketPath)

	mockConfig := map[string]interface{}{}
	mockConfig["dogstatsd_socket"] = socketPath
	mockConfig["dogstatsd_socket_fd_from"] = handoffPath
	mockConfig["dogstatsd_origin_detection"] = false

	deps := fulfillDepsWithConfig(t, mockConfig)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)

	// A client connected before the Agent starts, and kept across its restart.
	client, err := net.Dial("unixgram", socketPath)
	require.NoError(t, err)
	defer client.Close()

	for i := 0; i < 2; i++ {
		packetsChannel := make(chan packets.Packets)
		s, err := udsDatagramListenerFactory(packetsChannel, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
		require.NoError(t, err)
		s.Listen()

		_, err = client.Write([]byte("daemon:666|g"))
		require.NoError(t, err)

		select {
		case pkts := <-packetsChannel:
			require.Len(t, pkts, 1)
		case <-time.After(2 * time.Second):
			assert.FailNow(t, "Timeout on receive channel")
		}

		s.Stop()

		// Stopping the listener must not have removed the socket file.
		fi, err := os.Stat(socketPath)
		require.NoError(t, err)
		assert.NotZero(t, fi.Mode()&os.ModeSocket)
	}
}

// TestUDSDatagramNoHandoffBindsSocket asserts the default path is unchanged when
// dogstatsd_socket_fd_from is not set: the Agent binds the socket itself.
func TestUDSDatagramNoHandoffBindsSocket(t *testing.T) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig["dogstatsd_socket"] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	deps := fulfillDepsWithConfig(t, mockConfig)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := udsDatagramListenerFactory(nil, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	require.NoError(t, err)
	defer s.Stop()

	fi, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.Equal(t, "Srwx-w--w-", fi.Mode().String())
}
