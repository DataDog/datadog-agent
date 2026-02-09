// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type mockBoundPortLookup map[uint16]network.ConnectionType

func (m mockBoundPortLookup) Find(proto network.ConnectionType, port uint16) bool {
	foundProto, ok := m[port]
	return ok && foundProto == proto
}

func TestGuessConnectionDirection(t *testing.T) {
	remoteAddr := util.AddressFromString("8.8.8.8")
	loopbackAddr := util.AddressFromString("127.0.0.1")

	tests := []struct {
		name        string
		conn        *network.ConnectionStats
		pktType     uint8
		ports       mockBoundPortLookup
		expectedDir network.ConnectionDirection
		expectedErr bool
	}{
		{
			name: "already has direction",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source:    remoteAddr,
					Dest:      remoteAddr,
					SPort:     12345,
					DPort:     8080,
					Type:      network.TCP,
					Direction: network.OUTGOING,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       nil,
			expectedDir: network.OUTGOING,
		},
		{
			name: "source port is bound",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  8080,
					DPort:  12345,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{8080: network.TCP},
			expectedDir: network.INCOMING,
		},
		{
			name: "loopback dest port is bound",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: loopbackAddr,
					Dest:   loopbackAddr,
					SPort:  12345,
					DPort:  8080,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{8080: network.TCP},
			expectedDir: network.OUTGOING,
		},
		{
			name: "loopback dest port not bound falls through",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: loopbackAddr,
					Dest:   loopbackAddr,
					SPort:  12345,
					DPort:  8080,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{},
			expectedDir: network.INCOMING,
		},
		{
			name: "non-loopback dest port bound is not checked",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  12345,
					DPort:  8080,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{8080: network.TCP},
			expectedDir: network.INCOMING,
		},
		{
			name: "source system port is incoming",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  80,
					DPort:  12345,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_OUTGOING,
			ports:       mockBoundPortLookup{},
			expectedDir: network.INCOMING,
		},
		{
			name: "dest system port is outgoing",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  12345,
					DPort:  443,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{},
			expectedDir: network.OUTGOING,
		},
		{
			name: "PACKET_HOST fallback is incoming",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  12345,
					DPort:  8080,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{},
			expectedDir: network.INCOMING,
		},
		{
			name: "PACKET_OUTGOING fallback is outgoing",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  12345,
					DPort:  8080,
					Type:   network.TCP,
				},
			},
			pktType:     unix.PACKET_OUTGOING,
			ports:       mockBoundPortLookup{},
			expectedDir: network.OUTGOING,
		},
		{
			name: "unknown packet type returns error",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  12345,
					DPort:  8080,
					Type:   network.TCP,
				},
			},
			pktType:     99,
			ports:       mockBoundPortLookup{},
			expectedDir: network.UNKNOWN,
			expectedErr: true,
		},
		{
			name: "UDP source port bound",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  5353,
					DPort:  12345,
					Type:   network.UDP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{5353: network.UDP},
			expectedDir: network.INCOMING,
		},
		{
			name: "bound port wrong protocol does not match",
			conn: &network.ConnectionStats{
				ConnectionTuple: network.ConnectionTuple{
					Source: remoteAddr,
					Dest:   remoteAddr,
					SPort:  8080,
					DPort:  12345,
					Type:   network.UDP,
				},
			},
			pktType:     unix.PACKET_HOST,
			ports:       mockBoundPortLookup{8080: network.TCP},
			expectedDir: network.INCOMING,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := guessConnectionDirection(tt.conn, tt.pktType, tt.ports)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expectedDir, dir)
		})
	}
}
