// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package impl contains the implementation of the pathtestscheduler component.
package impl

import (
	"net/netip"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// IP protocol numbers per IANA.
const (
	ipProtoICMP = 1
	ipProtoTCP  = 6
	ipProtoUDP  = 17
)

// EtherType for IPv4.
const etherTypeIPv4 = 0x0800

// dropReason is a human-readable token used as a metric tag for dropped flows.
// An empty string means the flow was not dropped.
type dropReason = string

// flowToConnection converts a single NetFlow record into a NetworkPathConnection.
// When a non-empty dropReason is returned the caller should count the drop and
// discard the (zero-value) connection.
//
// Drop conditions (in order evaluated):
//  1. EtherType != IPv4 (0x0800)       → "not_ipv4"
//  2. IPProtocol not in {TCP,UDP,ICMP}  → "bad_protocol"
//  3. DstAddr unspecified               → "unspecified"
//  4. DstAddr is loopback               → "loopback"
//  5. DstAddr is link-local             → "link_local"
//  6. DstAddr is multicast              → "multicast"
//  7. DstAddr is broadcast (255.255.255.255) → "broadcast"
//  8. DstAddr == ExporterAddr           → "self_exporter"
func flowToConnection(flow *common.Flow) (npmodel.NetworkPathConnection, dropReason) {
	// Stage 1a: EtherType check — only IPv4 is supported.
	if flow.EtherType != etherTypeIPv4 {
		return npmodel.NetworkPathConnection{}, "not_ipv4"
	}

	// Stage 1b: Protocol check — must be TCP, UDP, or ICMP.
	var connType agentmodel.ConnectionType
	var proto payload.Protocol
	switch flow.IPProtocol {
	case ipProtoTCP:
		connType = agentmodel.ConnectionType_tcp
		proto = payload.ProtocolTCP
	case ipProtoUDP:
		connType = agentmodel.ConnectionType_udp
		proto = payload.ProtocolUDP
	case ipProtoICMP:
		// ConnectionType has no ICMP variant; leave connType as its zero value.
		// Protocol carries the truth here (see plan §2 Q2 resolution).
		proto = payload.ProtocolICMP
	default:
		return npmodel.NetworkPathConnection{}, "bad_protocol"
	}

	// Stage 1c: Parse destination address.
	// netip.AddrFromSlice returns an IPv4-in-IPv6 address for 4-byte slices —
	// call Unmap() to get a plain IPv4 netip.Addr.
	dstAddr, ok := netip.AddrFromSlice(flow.DstAddr)
	if !ok {
		return npmodel.NetworkPathConnection{}, "unspecified"
	}
	dstAddr = dstAddr.Unmap()

	// Confirm we actually got an IPv4 address (belt-and-suspenders after EtherType check).
	if !dstAddr.Is4() {
		return npmodel.NetworkPathConnection{}, "not_ipv4"
	}

	// Drop unroutable or otherwise unsuitable destinations.
	if dstAddr.IsUnspecified() {
		return npmodel.NetworkPathConnection{}, "unspecified"
	}
	if dstAddr.IsLoopback() {
		return npmodel.NetworkPathConnection{}, "loopback"
	}
	if dstAddr.IsLinkLocalUnicast() || dstAddr.IsLinkLocalMulticast() {
		return npmodel.NetworkPathConnection{}, "link_local"
	}
	if dstAddr.IsMulticast() {
		return npmodel.NetworkPathConnection{}, "multicast"
	}
	// IPv4 limited broadcast (255.255.255.255).
	if dstAddr == netip.MustParseAddr("255.255.255.255") {
		return npmodel.NetworkPathConnection{}, "broadcast"
	}

	// Stage 1d: Parse exporter address and reject self-exporter flows.
	exporterAddr, ok := netip.AddrFromSlice(flow.ExporterAddr)
	if ok {
		exporterAddr = exporterAddr.Unmap()
		if exporterAddr == dstAddr {
			return npmodel.NetworkPathConnection{}, "self_exporter"
		}
	}

	// Stage 1e: Parse source address (informational; failures don't drop the flow).
	srcAddr, _ := netip.AddrFromSlice(flow.SrcAddr)
	srcAddr = srcAddr.Unmap()

	// Build Dest AddrPort: use port 0 when DstPort is zero or negative (ephemeral).
	var dstPort uint16
	if flow.DstPort > 0 {
		dstPort = uint16(flow.DstPort)
	}
	dest := netip.AddrPortFrom(dstAddr, dstPort)

	// Build Source AddrPort: same rule for SrcPort.
	var srcPort uint16
	if flow.SrcPort > 0 {
		srcPort = uint16(flow.SrcPort)
	}
	src := netip.AddrPortFrom(srcAddr, srcPort)

	// Build ExporterAddrs slice. A zero-value/invalid exporter is omitted.
	var exporterAddrs []netip.Addr
	if ok && exporterAddr.IsValid() {
		exporterAddrs = []netip.Addr{exporterAddr}
	}

	conn := npmodel.NetworkPathConnection{
		Source:          src,
		Dest:            dest,
		TranslatedDest:  dest,
		Type:            connType,
		Protocol:        proto,
		Direction:       agentmodel.ConnectionDirection_outgoing,
		Family:          agentmodel.ConnectionFamily_v4,
		Domain:          flow.DstReverseDNSHostname,
		IntraHost:       false,
		SystemProbeConn: false,
		Origin:          npmodel.OriginNetworkDevice,
		DestIP:          dstAddr,
		// Namespaces and ExporterAddrs contain single-element slices here;
		// AggregateFlows (stage 2) will union across all flows in the batch.
		Namespaces:    []string{flow.Namespace},
		ExporterAddrs: exporterAddrs,
	}
	return conn, ""
}

// aggregationKey is the grouping key used for cross-flow aggregation.
// Flows sharing the same destination address, port, and protocol are merged
// into a single NetworkPathConnection.
type aggregationKey struct {
	destAddr netip.Addr
	destPort uint16
	protocol payload.Protocol
}

// AggregateFlows converts a batch of NetFlow records into a slice of
// NetworkPathConnection values (one per unique destination) and a map of
// drop counts keyed by drop reason.
//
// Stage 2 — cross-flow aggregation:
// For each group of flows that share the same (DstAddr, DstPort, Protocol):
//   - Pick the first surviving flow's fields as the representative.
//   - Union the Namespaces and ExporterAddrs from all flows in the group.
//   - Domain is the first non-empty value seen in the group (stable, first-seen order).
func AggregateFlows(flows []*common.Flow) (connections []npmodel.NetworkPathConnection, dropCounts map[string]int) {
	dropCounts = make(map[string]int)

	// ordered slice of keys to preserve deterministic output order.
	var keys []aggregationKey
	type groupState struct {
		conn        npmodel.NetworkPathConnection
		nsSet       map[string]struct{}
		exporterSet map[netip.Addr]struct{}
	}
	groups := make(map[aggregationKey]*groupState)

	for _, flow := range flows {
		conn, reason := flowToConnection(flow)
		if reason != "" {
			dropCounts[reason]++
			continue
		}

		key := aggregationKey{
			destAddr: conn.DestIP,
			destPort: conn.Dest.Port(),
			protocol: conn.Protocol,
		}

		gs, exists := groups[key]
		if !exists {
			// First flow for this destination — use it as the representative.
			gs = &groupState{
				conn:        conn,
				nsSet:       make(map[string]struct{}),
				exporterSet: make(map[netip.Addr]struct{}),
			}
			groups[key] = gs
			keys = append(keys, key)
		}

		// Union namespaces.
		for _, ns := range conn.Namespaces {
			if ns != "" {
				gs.nsSet[ns] = struct{}{}
			}
		}

		// Union exporter addresses.
		for _, addr := range conn.ExporterAddrs {
			if addr.IsValid() {
				gs.exporterSet[addr] = struct{}{}
			}
		}

		// Domain: keep the first non-empty value (first-seen wins).
		if gs.conn.Domain == "" && conn.Domain != "" {
			gs.conn.Domain = conn.Domain
		}
	}

	// Build output slice in stable (insertion) order.
	connections = make([]npmodel.NetworkPathConnection, 0, len(keys))
	for _, key := range keys {
		gs := groups[key]

		// Materialise the deduplicated namespace set.
		namespaces := make([]string, 0, len(gs.nsSet))
		for ns := range gs.nsSet {
			namespaces = append(namespaces, ns)
		}

		// Materialise the deduplicated exporter set.
		exporterAddrs := make([]netip.Addr, 0, len(gs.exporterSet))
		for addr := range gs.exporterSet {
			exporterAddrs = append(exporterAddrs, addr)
		}

		gs.conn.Namespaces = namespaces
		gs.conn.ExporterAddrs = exporterAddrs
		connections = append(connections, gs.conn)
	}

	return connections, dropCounts
}
