// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build (linux && linux_bpf) || darwin

package ebpfless

import (
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
)

func TestUpdateRstFlagUsesCrossPlatformFailureKeys(t *testing.T) {
	tcp := &layers.TCP{RST: true}
	processor := &TCPProcessor{}

	for _, tc := range []struct {
		name     string
		state    connStatus
		expected uint16
	}{
		{
			name:     "refused",
			state:    connStatAttempted,
			expected: network.TCPFailureErrnoConnRefused,
		},
		{
			name:     "reset",
			state:    connStatEstablished,
			expected: network.TCPFailureErrnoConnReset,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &network.ConnectionStats{}
			state := &connectionState{tcpState: tc.state}

			processor.updateRstFlag(conn, state, 0, tcp, 0)

			require.Equal(t, map[uint16]uint32{tc.expected: 1}, conn.TCPFailures)
			require.Equal(t, uint16(1), conn.Monotonic.TCPClosed)
			require.True(t, conn.IsClosed)
			require.Equal(t, connStatClosed, state.tcpState)
		})
	}
}

func TestUpdateRstFlagIgnoresAlreadyClosedConnection(t *testing.T) {
	conn := &network.ConnectionStats{}
	state := &connectionState{tcpState: connStatClosed}

	(&TCPProcessor{}).updateRstFlag(conn, state, 0, &layers.TCP{RST: true}, 0)

	require.Empty(t, conn.TCPFailures)
	require.Zero(t, conn.Monotonic.TCPClosed)
	require.False(t, conn.IsClosed)
}
