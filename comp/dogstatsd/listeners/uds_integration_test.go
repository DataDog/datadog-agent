// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package listeners

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUDSPartialRead(t *testing.T) {
	socketPath := testSocketPath(t)
	sync := make(chan error)

	addr := &net.UnixAddr{Net: "unix", Name: socketPath}
	listen, err := net.ListenUnix(addr.Net, addr)
	require.NoError(t, err)

	payload := []byte("test_metric:1|c|#tags,tags,tags")

	go func() {
		sender, err := net.DialUnix(addr.Net, nil, addr)
		sync <- err
		if err != nil {
			return
		}

		err = binary.Write(sender, binary.LittleEndian, int32(len(payload)))
		sync <- err
		if err != nil {
			return
		}

		for _, b := range payload {
			n, err := sender.Write([]byte{b})
			if n < 1 {
				sync <- fmt.Errorf("aborted write: %w", err)
			}
			sync <- err
		}

		sync <- sender.Close()

		close(sync)
	}()

	conn, err := listen.Accept()
	require.NoError(t, err)

	mockConfig := map[string]any{
		"dogstatsd_stream_socket": socketPath,
	}

	packetsChannel := make(chan packets.Packets, 4)
	deps := fulfillDepsWithConfig(t, mockConfig)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)

	factory, err := udsStreamListenerFactory(packetsChannel, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	require.NoError(t, err)

	go factory.(*UDSStreamListener).handleConnection(conn.(*net.UnixConn), func(c netUnixConn) error { return c.Close() })

	for err := range sync {
		require.NoError(t, err)
	}

	pkts := <-packetsChannel
	assert.Equal(t, 1, len(pkts))
	assert.Equal(t, payload, pkts[0].Contents)
}
