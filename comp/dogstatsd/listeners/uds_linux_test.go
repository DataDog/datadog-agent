// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Origin detection is linux-only

// Most of it is tested by test/integration/dogstatsd/origin_detection_test.go
// that requires a docker environment to run

package listeners

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/fx"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	replayfx "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx"
	replayimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestUDSPassCred(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "dsd.socket")

	cfg := map[string]interface{}{}
	cfg["dogstatsd_socket"] = socketPath
	cfg["dogstatsd_origin_detection"] = true

	deps := fulfillDepsWithConfig(t, cfg)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	listernersTelemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	pool := packets.NewPool(deps.Config, 512, packetsTelemetryStore)
	poolManager := packets.NewPoolManager(pool)
	s, err := NewUDSDatagramListener(nil, poolManager, nil, deps.Config, nil, option.None[workloadmeta.Component](), deps.PidMap, listernersTelemetryStore, packetsTelemetryStore, deps.Telemetry)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)

	// Test socket has PASSCRED option set to 1
	f, err := s.conn.File()
	require.Nil(t, err)
	defer f.Close()

	enabled, err := unix.GetsockoptInt(int(f.Fd()), unix.SOL_SOCKET, unix.SO_PASSCRED)
	assert.Nil(t, err)
	assert.Equal(t, enabled, 1)
}

func TestUDSRepeatedCapture(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "capture.socket")
	deps := fulfillDepsWithConfig(t, map[string]interface{}{
		"dogstatsd_socket":             socketPath,
		"dogstatsd_origin_detection":   true,
		"dogstatsd_packet_buffer_size": 1,
	})
	capture := fxutil.Test[replay.Component](t,
		replayfx.Module(),
		fx.Provide(func() config.Component { return deps.Config }),
		fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
	)
	packetTelemetry := packets.NewTelemetryStore(nil, deps.Telemetry)
	manager := packets.NewPoolManager(packets.NewPool(deps.Config, 8192, packetTelemetry))
	oob := NewUDSOobPoolManager()
	require.NoError(t, capture.RegisterSharedPoolManager(manager))
	require.NoError(t, capture.RegisterOOBPoolManager(oob))
	packetOut := make(chan packets.Packets, 16)
	listener, err := NewUDSDatagramListener(packetOut, manager, oob, deps.Config, capture,
		option.None[workloadmeta.Component](), deps.PidMap, NewTelemetryStore(nil, deps.Telemetry), packetTelemetry, deps.Telemetry)
	require.NoError(t, err)
	listener.Listen()
	defer listener.Stop()
	defer capture.StopCapture()
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	require.NoError(t, err)
	defer conn.Close()

	send := func(payload string) {
		t.Helper()
		_, err := conn.Write([]byte(payload))
		require.NoError(t, err)
		select {
		case batch := <-packetOut:
			require.Len(t, batch, 1)
			require.Equal(t, payload, string(batch[0].Contents))
			manager.Put(batch[0])
		case <-time.After(5 * time.Second):
			t.Fatal("listener did not deliver packet")
		}
	}
	start := func() string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.Chmod(dir, 0777))
		path, err := capture.StartCapture(dir, time.Minute, false)
		require.NoError(t, err)
		return path
	}
	finish := func() {
		t.Helper()
		capture.StopCapture()
		require.Eventually(t, func() bool { return !capture.IsOngoing() }, 5*time.Second, time.Millisecond)
		require.Zero(t, manager.Count())
		require.Zero(t, oob.Count())
	}

	for i := range 2 {
		path := start()
		// The listener may already be blocked in a read begun before capture.
		send("prime:1|c")
		payload := fmt.Sprintf("capture:%d|c", i)
		send(payload)
		finish()
		reader, err := replayimpl.NewTrafficCaptureReader(path, 1, false)
		require.NoError(t, err)
		reader.Seek(0)
		found := false
		for {
			msg, err := reader.ReadNext()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if string(msg.Payload) == payload {
				found = true
				require.NotEmpty(t, msg.Ancillary)
				messages, err := unix.ParseSocketControlMessage(msg.Ancillary)
				require.NoError(t, err)
				require.Len(t, messages, 1)
				credentials, err := unix.ParseUnixCredentials(&messages[0])
				require.NoError(t, err)
				require.Equal(t, int32(os.Getpid()), credentials.Pid)
			}
		}
		require.True(t, found, "second and subsequent captures must contain traffic")
		require.NoError(t, reader.Close())
		for range 16 {
			send("normal:1|c")
		}
		require.Zero(t, manager.Count())
		require.Zero(t, oob.Count())
	}
	// A failed read during capture must also release the listener's ownership.
	start()
	send("prime:1|c")
	listener.Stop()
	finish()
}
