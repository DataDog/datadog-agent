// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"fmt"
	"time"

	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// mockSource is a test double for the connectionsSource interface.
type mockSource struct {
	conns *network.Connections
	err   error
}

func (m *mockSource) RegisterClient(_ string) error { return nil }
func (m *mockSource) GetActiveConnections(_ string) (*network.Connections, func(), error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.conns, func() {}, nil
}

func testConfig() *Config {
	cfg := defaultConfig().(*Config)
	cfg.CheckInterval = 100 * time.Millisecond
	return cfg
}

func makeTestConnection(pid int32) network.ConnectionStats {
	return network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Pid:       uint32(pid),
			Source:    util.AddressFromString(fmt.Sprintf("10.0.1.%d", pid)),
			Dest:      util.AddressFromString(fmt.Sprintf("10.0.2.%d", pid)),
			SPort:     uint16(10000 + pid),
			DPort:     443,
			Type:      network.TCP,
			Family:    network.AFINET,
			Direction: network.OUTGOING,
			NetNS:     4026531840,
		},
		Monotonic: network.StatCounters{
			SentBytes:      uint64(pid) * 1000,
			RecvBytes:      uint64(pid) * 2000,
			SentPackets:    uint64(pid) * 10,
			RecvPackets:    uint64(pid) * 20,
			Retransmits:    uint32(pid), //nolint:gosec
			TCPEstablished: 1,
			TCPClosed:      0,
		},
		RTT:       15000,
		RTTVar:    3000,
		Cookie:    uint64(pid) * 12345,
		Duration:  30 * time.Second,
		IntraHost: false,
		IsClosed:  false,
	}
}

func makeTestConnections(n int) *network.Connections {
	conns := make([]network.ConnectionStats, n)
	for i := 0; i < n; i++ {
		conns[i] = makeTestConnection(int32(i + 1))
	}
	return &network.Connections{
		BufferedData:  network.BufferedData{Conns: conns},
		DNS:           map[util.Address][]dns.Hostname{},
		ConnTelemetry: map[network.ConnTelemetryType]int64{},
	}
}

func makeUDPConnection(pid int32) network.ConnectionStats {
	conn := makeTestConnection(pid)
	conn.Type = network.UDP
	conn.Direction = network.INCOMING
	conn.RTT = 0
	conn.RTTVar = 0
	return conn
}

func makeConnectionWithNAT(pid int32) network.ConnectionStats {
	conn := makeTestConnection(pid)
	conn.IPTranslation = &network.IPTranslation{
		ReplSrcIP:   util.AddressFromString("192.168.1.1"),
		ReplDstIP:   util.AddressFromString("192.168.1.2"),
		ReplSrcPort: 8080,
		ReplDstPort: 80,
	}
	return conn
}

func makeConnectionWithContainerIDs(pid int32) network.ConnectionStats {
	conn := makeTestConnection(pid)
	conn.ContainerID.Source = intern.GetByString(fmt.Sprintf("container-src-%d", pid))
	conn.ContainerID.Dest = intern.GetByString(fmt.Sprintf("container-dst-%d", pid))
	return conn
}

func makeConnectionWithProtocolStack(pid int32) network.ConnectionStats {
	conn := makeTestConnection(pid)
	conn.ProtocolStack = protocols.Stack{
		Application: protocols.HTTP,
		Encryption:  protocols.TLS,
	}
	return conn
}
