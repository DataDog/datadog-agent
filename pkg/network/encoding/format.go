// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"math"
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const maxRoutes = math.MaxInt32

var connPool = sync.Pool{
	New: func() interface{} {
		return new(model.Connection)
	},
}

// RouteIdx stores the route and the index into the route collection for a route
type RouteIdx struct {
	Idx   int32
	Route model.Route
}

type ipCache map[util.Address]string

func (ipc ipCache) Get(addr util.Address) string {
	if v, ok := ipc[addr]; ok {
		return v
	}

	v := addr.String()
	ipc[addr] = v
	return v
}

// FormatConnection converts a ConnectionStats into an model.Connection
func FormatConnection(
	conn network.ConnectionStats,
	routes map[string]RouteIdx,
	httpEncoder *httpEncoder,
	dnsFormatter *dnsFormatter,
	ipc ipCache,
	tagsSet *network.TagsSet,
) *model.Connection {
	c := connPool.Get().(*model.Connection)
	c.Pid = int32(conn.Pid)
	c.Laddr = formatAddr(conn.Source, conn.SPort, ipc)
	c.Raddr = formatAddr(conn.Dest, conn.DPort, ipc)
	c.Family = formatFamily(conn.Family)
	c.Type = formatType(conn.Type)
	c.IsLocalPortEphemeral = formatEphemeralType(conn.SPortIsEphemeral)
	c.PidCreateTime = 0
	c.LastBytesSent = conn.Last.SentBytes
	c.LastBytesReceived = conn.Last.RecvBytes
	c.LastPacketsSent = conn.Last.SentPackets
	c.LastPacketsReceived = conn.Last.RecvPackets
	c.LastRetransmits = conn.Last.Retransmits
	c.Direction = formatDirection(conn.Direction)
	c.NetNS = conn.NetNS
	c.RemoteNetworkId = ""
	c.IpTranslation = formatIPTranslation(conn.IPTranslation, ipc)
	c.Rtt = conn.RTT
	c.RttVar = conn.RTTVar
	c.IntraHost = conn.IntraHost
	c.LastTcpEstablished = conn.Last.TCPEstablished
	c.LastTcpClosed = conn.Last.TCPClosed

	c.RouteIdx = formatRouteIdx(conn.Via, routes)
	dnsFormatter.FormatConnectionDNS(conn, c)

	httpStats, tags := httpEncoder.GetHTTPAggregationsAndTags(conn)
	if httpStats != nil {
		c.HttpAggregations, _ = proto.Marshal(httpStats)
	}

	conn.Tags |= tags
	c.Tags = formatTags(tagsSet, conn)

	return c
}

// FormatCompilationTelemetry converts telemetry from its internal representation to a protobuf message
func FormatCompilationTelemetry(telByAsset map[string]network.RuntimeCompilationTelemetry) map[string]*model.RuntimeCompilationTelemetry {
	if telByAsset == nil {
		return nil
	}

	ret := make(map[string]*model.RuntimeCompilationTelemetry)
	for asset, tel := range telByAsset {
		t := &model.RuntimeCompilationTelemetry{}
		t.RuntimeCompilationEnabled = tel.RuntimeCompilationEnabled
		t.RuntimeCompilationResult = model.RuntimeCompilationResult(tel.RuntimeCompilationResult)
		t.KernelHeaderFetchResult = model.KernelHeaderFetchResult(tel.KernelHeaderFetchResult)
		t.RuntimeCompilationDuration = tel.RuntimeCompilationDuration
		ret[asset] = t
	}
	return ret
}

// FormatConnectionTelemetry converts telemetry from its internal representation to a protobuf message
func FormatConnectionTelemetry(tel map[network.ConnTelemetryType]int64) map[string]int64 {
	if tel == nil {
		return nil
	}

	ret := make(map[string]int64)
	for k, v := range tel {
		ret[string(k)] = v
	}
	return ret
}

func returnToPool(c *model.Connections) {
	if c.Conns != nil {
		for _, c := range c.Conns {
			c.Reset()
			connPool.Put(c)
		}
	}
	if c.Dns != nil {
		for _, e := range c.Dns {
			e.Reset()
			dnsPool.Put(e)
		}
	}
}

func formatAddr(addr util.Address, port uint16, ipc ipCache) *model.Addr {
	if addr.IsZero() {
		return nil
	}

	return &model.Addr{Ip: ipc.Get(addr), Port: int32(port)}
}

func formatFamily(f network.ConnectionFamily) model.ConnectionFamily {
	switch f {
	case network.AFINET:
		return model.ConnectionFamily_v4
	case network.AFINET6:
		return model.ConnectionFamily_v6
	default:
		return -1
	}
}

func formatType(f network.ConnectionType) model.ConnectionType {
	switch f {
	case network.TCP:
		return model.ConnectionType_tcp
	case network.UDP:
		return model.ConnectionType_udp
	default:
		return -1
	}
}

func formatDirection(d network.ConnectionDirection) model.ConnectionDirection {
	switch d {
	case network.INCOMING:
		return model.ConnectionDirection_incoming
	case network.OUTGOING:
		return model.ConnectionDirection_outgoing
	case network.LOCAL:
		return model.ConnectionDirection_local
	case network.NONE:
		return model.ConnectionDirection_none
	default:
		return model.ConnectionDirection_unspecified
	}
}

func formatEphemeralType(e network.EphemeralPortType) model.EphemeralPortState {
	switch e {
	case network.EphemeralTrue:
		return model.EphemeralPortState_ephemeralTrue
	case network.EphemeralFalse:
		return model.EphemeralPortState_ephemeralFalse
	default:
		return model.EphemeralPortState_ephemeralUnspecified
	}
}

func formatIPTranslation(ct *network.IPTranslation, ipc ipCache) *model.IPTranslation {
	if ct == nil {
		return nil
	}

	return &model.IPTranslation{
		ReplSrcIP:   ipc.Get(ct.ReplSrcIP),
		ReplDstIP:   ipc.Get(ct.ReplDstIP),
		ReplSrcPort: int32(ct.ReplSrcPort),
		ReplDstPort: int32(ct.ReplDstPort),
	}
}

func formatRouteIdx(v *network.Via, routes map[string]RouteIdx) int32 {
	if v == nil || routes == nil {
		return -1
	}

	if len(routes) == maxRoutes {
		return -1
	}

	k := routeKey(v)
	if len(k) == 0 {
		return -1
	}

	if idx, ok := routes[k]; ok {
		return idx.Idx
	}

	routes[k] = RouteIdx{
		Idx:   int32(len(routes)),
		Route: model.Route{Subnet: &model.Subnet{Alias: v.Subnet.Alias}},
	}

	return int32(len(routes)) - 1
}

func routeKey(v *network.Via) string {
	return v.Subnet.Alias
}

func formatTags(tagsSet *network.TagsSet, c network.ConnectionStats) (tagsIdx []uint32) {
	for _, tag := range network.GetStaticTags(c.Tags) {
		tagsIdx = append(tagsIdx, tagsSet.Add(tag))
	}
	return tagsIdx
}
