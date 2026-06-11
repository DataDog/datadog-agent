// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package impl

import (
	"net/netip"
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// helpers

func ipv4(a, b, c, d byte) []byte { return []byte{a, b, c, d} }

// makeFlow returns a minimal valid TCP flow pointing at 8.8.8.8:443
// exported from 192.0.2.1 in namespace "default".
func makeFlow() *common.Flow {
	return &common.Flow{
		Namespace:    "default",
		EtherType:    etherTypeIPv4,
		IPProtocol:   ipProtoTCP,
		SrcAddr:      ipv4(10, 0, 0, 1),
		DstAddr:      ipv4(8, 8, 8, 8),
		SrcPort:      54321,
		DstPort:      443,
		ExporterAddr: ipv4(192, 0, 2, 1),
	}
}

// ── Per-flow drop tests ─────────────────────────────────────────────────────

func TestFlowToConnection_DropNotIPv4(t *testing.T) {
	f := makeFlow()
	f.EtherType = 0x86DD // IPv6
	_, reason := flowToConnection(f)
	assert.Equal(t, "not_ipv4", reason)
}

func TestFlowToConnection_DropBadProtocol(t *testing.T) {
	f := makeFlow()
	f.IPProtocol = 47 // GRE
	_, reason := flowToConnection(f)
	assert.Equal(t, "bad_protocol", reason)
}

func TestFlowToConnection_DropLoopback(t *testing.T) {
	f := makeFlow()
	f.DstAddr = ipv4(127, 0, 0, 1)
	_, reason := flowToConnection(f)
	assert.Equal(t, "loopback", reason)
}

func TestFlowToConnection_DropMulticast(t *testing.T) {
	f := makeFlow()
	// 239.1.2.3 is in the administratively-scoped multicast range (239.0.0.0/8)
	// and is NOT a link-local multicast address, so it exercises the IsMulticast()
	// branch rather than the link-local branch.
	f.DstAddr = ipv4(239, 1, 2, 3)
	_, reason := flowToConnection(f)
	assert.Equal(t, "multicast", reason)
}

func TestFlowToConnection_DropLinkLocal(t *testing.T) {
	f := makeFlow()
	f.DstAddr = ipv4(169, 254, 1, 1) // link-local unicast
	_, reason := flowToConnection(f)
	assert.Equal(t, "link_local", reason)
}

func TestFlowToConnection_DropLinkLocalMulticast(t *testing.T) {
	f := makeFlow()
	f.DstAddr = ipv4(224, 0, 0, 1) // all-hosts — link-local multicast
	_, reason := flowToConnection(f)
	assert.Equal(t, "link_local", reason)
}

func TestFlowToConnection_DropBroadcast(t *testing.T) {
	f := makeFlow()
	f.DstAddr = ipv4(255, 255, 255, 255) // limited broadcast
	_, reason := flowToConnection(f)
	assert.Equal(t, "broadcast", reason)
}

func TestFlowToConnection_DropSelfExporter(t *testing.T) {
	f := makeFlow()
	f.DstAddr = ipv4(192, 0, 2, 1) // same as ExporterAddr
	f.ExporterAddr = ipv4(192, 0, 2, 1)
	_, reason := flowToConnection(f)
	assert.Equal(t, "self_exporter", reason)
}

// ── Happy-path protocol tests ───────────────────────────────────────────────

func TestFlowToConnection_TCP(t *testing.T) {
	f := makeFlow()
	conn, reason := flowToConnection(f)
	require.Empty(t, reason)

	assert.Equal(t, payload.ProtocolTCP, conn.Protocol)
	assert.Equal(t, agentmodel.ConnectionType_tcp, conn.Type)
	assert.Equal(t, netip.MustParseAddr("8.8.8.8"), conn.DestIP)
	assert.Equal(t, netip.MustParseAddrPort("8.8.8.8:443"), conn.Dest)
	assert.Equal(t, conn.Dest, conn.TranslatedDest)
	assert.Equal(t, agentmodel.ConnectionDirection_outgoing, conn.Direction)
	assert.Equal(t, agentmodel.ConnectionFamily_v4, conn.Family)
	assert.Equal(t, npmodel.OriginNetworkDevice, conn.Origin)
	assert.False(t, conn.IntraHost)
	assert.False(t, conn.SystemProbeConn)
	assert.Equal(t, []string{"default"}, conn.Namespaces)
	assert.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, conn.ExporterAddrs)
}

func TestFlowToConnection_UDP(t *testing.T) {
	f := makeFlow()
	f.IPProtocol = ipProtoUDP
	f.DstPort = 53
	conn, reason := flowToConnection(f)
	require.Empty(t, reason)

	assert.Equal(t, payload.ProtocolUDP, conn.Protocol)
	assert.Equal(t, agentmodel.ConnectionType_udp, conn.Type)
	assert.Equal(t, uint16(53), conn.Dest.Port())
}

func TestFlowToConnection_ICMP(t *testing.T) {
	f := makeFlow()
	f.IPProtocol = ipProtoICMP
	f.DstPort = 0 // ICMP has no port
	conn, reason := flowToConnection(f)
	require.Empty(t, reason)

	// §2 Q2 resolution: Protocol carries ProtocolICMP; Type stays at zero value.
	assert.Equal(t, payload.ProtocolICMP, conn.Protocol)
	assert.Equal(t, agentmodel.ConnectionType(0), conn.Type, "ICMP should leave ConnectionType at zero value")
	assert.Equal(t, uint16(0), conn.Dest.Port(), "ICMP dest port must be 0")
}

// ── Port-handling edge cases ────────────────────────────────────────────────

func TestFlowToConnection_DstPortZero(t *testing.T) {
	f := makeFlow()
	f.DstPort = 0
	conn, reason := flowToConnection(f)
	require.Empty(t, reason)
	assert.Equal(t, uint16(0), conn.Dest.Port())
}

func TestFlowToConnection_DstPortEphemeral(t *testing.T) {
	// DstPort == -1 means "ephemeral"; must produce port 0.
	f := makeFlow()
	f.DstPort = -1
	conn, reason := flowToConnection(f)
	require.Empty(t, reason)
	assert.Equal(t, uint16(0), conn.Dest.Port())
}

// ── Domain field ────────────────────────────────────────────────────────────

func TestFlowToConnection_Domain(t *testing.T) {
	f := makeFlow()
	f.DstReverseDNSHostname = "dns.google"
	conn, reason := flowToConnection(f)
	require.Empty(t, reason)
	assert.Equal(t, "dns.google", conn.Domain)
}

// ── AggregateFlows tests ────────────────────────────────────────────────────

// TestAggregateFlows_ThreeFlowsSameDestTwoNamespacesTwoExporters verifies that
// three flows to the same destination from two different namespaces and two
// different exporters produce ONE connection with both namespaces and both
// exporter addresses unioned.
func TestAggregateFlows_ThreeFlowsSameDestTwoNamespacesTwoExporters(t *testing.T) {
	flows := []*common.Flow{
		{
			Namespace:    "ns-a",
			EtherType:    etherTypeIPv4,
			IPProtocol:   ipProtoTCP,
			SrcAddr:      ipv4(10, 0, 0, 1),
			DstAddr:      ipv4(8, 8, 8, 8),
			DstPort:      443,
			ExporterAddr: ipv4(192, 0, 2, 1),
		},
		{
			Namespace:    "ns-b",
			EtherType:    etherTypeIPv4,
			IPProtocol:   ipProtoTCP,
			SrcAddr:      ipv4(10, 0, 0, 2),
			DstAddr:      ipv4(8, 8, 8, 8),
			DstPort:      443,
			ExporterAddr: ipv4(192, 0, 2, 2), // different exporter
		},
		{
			// Third flow: same destination, same namespace as first, same exporter.
			// Should not duplicate entries in the union sets.
			Namespace:    "ns-a",
			EtherType:    etherTypeIPv4,
			IPProtocol:   ipProtoTCP,
			SrcAddr:      ipv4(10, 0, 0, 3),
			DstAddr:      ipv4(8, 8, 8, 8),
			DstPort:      443,
			ExporterAddr: ipv4(192, 0, 2, 1), // same as first exporter
		},
	}

	connections, dropCounts := AggregateFlows(flows)

	require.Empty(t, dropCounts, "no drops expected")
	require.Len(t, connections, 1, "three flows to the same destination must produce exactly one connection")

	conn := connections[0]
	assert.Equal(t, netip.MustParseAddr("8.8.8.8"), conn.DestIP)
	assert.Equal(t, uint16(443), conn.Dest.Port())
	assert.Equal(t, payload.ProtocolTCP, conn.Protocol)

	// Both namespaces must appear, each exactly once.
	assert.ElementsMatch(t, []string{"ns-a", "ns-b"}, conn.Namespaces)

	// Both exporters must appear, each exactly once.
	assert.ElementsMatch(t,
		[]netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")},
		conn.ExporterAddrs,
	)
}

// TestAggregateFlows_MultipleDestinations verifies that flows to different
// destinations produce separate connections.
func TestAggregateFlows_MultipleDestinations(t *testing.T) {
	flows := []*common.Flow{
		{
			Namespace:    "default",
			EtherType:    etherTypeIPv4,
			IPProtocol:   ipProtoTCP,
			SrcAddr:      ipv4(10, 0, 0, 1),
			DstAddr:      ipv4(8, 8, 8, 8),
			DstPort:      443,
			ExporterAddr: ipv4(192, 0, 2, 1),
		},
		{
			Namespace:    "default",
			EtherType:    etherTypeIPv4,
			IPProtocol:   ipProtoTCP,
			SrcAddr:      ipv4(10, 0, 0, 1),
			DstAddr:      ipv4(1, 1, 1, 1),
			DstPort:      443,
			ExporterAddr: ipv4(192, 0, 2, 1),
		},
	}

	connections, dropCounts := AggregateFlows(flows)

	require.Empty(t, dropCounts)
	assert.Len(t, connections, 2, "two distinct destinations must produce two connections")
}

// TestAggregateFlows_DropCountsAccumulate verifies that drop reasons are tallied correctly.
func TestAggregateFlows_DropCountsAccumulate(t *testing.T) {
	flows := []*common.Flow{
		// valid flow
		makeFlow(),
		// two non-IPv4 flows
		func() *common.Flow {
			f := makeFlow()
			f.EtherType = 0x86DD
			return f
		}(),
		func() *common.Flow {
			f := makeFlow()
			f.EtherType = 0x86DD
			return f
		}(),
		// one loopback
		func() *common.Flow {
			f := makeFlow()
			f.DstAddr = ipv4(127, 0, 0, 1)
			return f
		}(),
	}

	connections, dropCounts := AggregateFlows(flows)

	assert.Len(t, connections, 1)
	assert.Equal(t, 2, dropCounts["not_ipv4"])
	assert.Equal(t, 1, dropCounts["loopback"])
}

// TestAggregateFlows_DomainFirstNonEmpty verifies the first-non-empty domain selection.
func TestAggregateFlows_DomainFirstNonEmpty(t *testing.T) {
	flows := []*common.Flow{
		{
			Namespace:             "default",
			EtherType:             etherTypeIPv4,
			IPProtocol:            ipProtoTCP,
			SrcAddr:               ipv4(10, 0, 0, 1),
			DstAddr:               ipv4(8, 8, 8, 8),
			DstPort:               443,
			ExporterAddr:          ipv4(192, 0, 2, 1),
			DstReverseDNSHostname: "", // no domain yet
		},
		{
			Namespace:             "default",
			EtherType:             etherTypeIPv4,
			IPProtocol:            ipProtoTCP,
			SrcAddr:               ipv4(10, 0, 0, 2),
			DstAddr:               ipv4(8, 8, 8, 8),
			DstPort:               443,
			ExporterAddr:          ipv4(192, 0, 2, 1),
			DstReverseDNSHostname: "dns.google", // first non-empty
		},
		{
			Namespace:             "default",
			EtherType:             etherTypeIPv4,
			IPProtocol:            ipProtoTCP,
			SrcAddr:               ipv4(10, 0, 0, 3),
			DstAddr:               ipv4(8, 8, 8, 8),
			DstPort:               443,
			ExporterAddr:          ipv4(192, 0, 2, 1),
			DstReverseDNSHostname: "other.name", // second non-empty — should NOT win
		},
	}

	connections, _ := AggregateFlows(flows)
	require.Len(t, connections, 1)
	assert.Equal(t, "dns.google", connections[0].Domain, "first non-empty domain must win")
}

// TestAggregateFlows_EmptyInput verifies that an empty input produces zero connections.
func TestAggregateFlows_EmptyInput(t *testing.T) {
	connections, dropCounts := AggregateFlows(nil)
	assert.Empty(t, connections)
	assert.Empty(t, dropCounts)
}

// TestAggregateFlows_ICMP_Protocol verifies §2 Q2: ICMP flows produce
// Protocol=ProtocolICMP with port=0 even after aggregation.
func TestAggregateFlows_ICMP_Protocol(t *testing.T) {
	f := makeFlow()
	f.IPProtocol = ipProtoICMP
	f.DstPort = 0

	connections, dropCounts := AggregateFlows([]*common.Flow{f})

	require.Empty(t, dropCounts)
	require.Len(t, connections, 1)
	conn := connections[0]
	assert.Equal(t, payload.ProtocolICMP, conn.Protocol, "ICMP flow must have ProtocolICMP after aggregation")
	assert.Equal(t, uint16(0), conn.Dest.Port(), "ICMP dest port must be 0 after aggregation")
}
