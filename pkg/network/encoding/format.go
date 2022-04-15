// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"math"
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/gogo/protobuf/proto"
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
	httpStats *model.HTTPAggregations,
	dnsFormatter *dnsFormatter,
	ipc ipCache,
) *model.Connection {
	c := connPool.Get().(*model.Connection)
	c.Pid = int32(conn.Pid)
	c.Laddr = formatAddr(conn.Source, conn.SPort, ipc)
	c.Raddr = formatAddr(conn.Dest, conn.DPort, ipc)
	c.Family = formatFamily(conn.Family)
	c.Type = formatType(conn.Type)
	c.IsLocalPortEphemeral = formatEphemeralType(conn.SPortIsEphemeral)
	c.PidCreateTime = 0
	c.LastBytesSent = conn.LastSentBytes
	c.LastBytesReceived = conn.LastRecvBytes
	c.LastPacketsSent = conn.LastSentPackets
	c.LastPacketsReceived = conn.LastRecvPackets
	c.LastRetransmits = conn.LastRetransmits
	c.Direction = formatDirection(conn.Direction)
	c.NetNS = conn.NetNS
	c.RemoteNetworkId = ""
	c.IpTranslation = formatIPTranslation(conn.IPTranslation, ipc)
	c.Rtt = conn.RTT
	c.RttVar = conn.RTTVar
	c.IntraHost = conn.IntraHost
	c.LastTcpEstablished = conn.LastTCPEstablished
	c.LastTcpClosed = conn.LastTCPClosed

	c.RouteIdx = formatRouteIdx(conn.Via, routes)
	dnsFormatter.FormatConnectionDNS(conn, c)

	if httpStats != nil {
		c.HttpAggregations, _ = proto.Marshal(httpStats)
	}

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

// FormatHTTPStats converts the HTTP map into a suitable format for serialization
func FormatHTTPStats(httpData map[http.Key]http.RequestStats) map[http.Key]*model.HTTPAggregations {
	var (
		aggregationsByKey = make(map[http.Key]*model.HTTPAggregations, len(httpData))

		// Pre-allocate some of the objects
		dataPool = make([]model.HTTPStats_Data, len(httpData)*http.NumStatusClasses)
		ptrPool  = make([]*model.HTTPStats_Data, len(httpData)*http.NumStatusClasses)
		poolIdx  = 0
	)

	for key, stats := range httpData {
		path := key.Path
		method := key.Method
		key.Path = ""
		key.Method = http.MethodUnknown

		httpAggregations, ok := aggregationsByKey[key]
		if !ok {
			httpAggregations = &model.HTTPAggregations{
				EndpointAggregations: make([]*model.HTTPStats, 0, 10),
			}

			aggregationsByKey[key] = httpAggregations
		}

		ms := &model.HTTPStats{
			Path:                  path,
			Method:                model.HTTPMethod(method),
			StatsByResponseStatus: ptrPool[poolIdx : poolIdx+http.NumStatusClasses],
		}

		for i := 0; i < len(stats); i++ {
			data := &dataPool[poolIdx+i]
			ms.StatsByResponseStatus[i] = data
			data.Count = uint32(stats[i].Count)

			if latencies := stats[i].Latencies; latencies != nil {
				blob, _ := proto.Marshal(latencies.ToProto())
				data.Latencies = blob
			} else {
				data.FirstLatencySample = stats[i].FirstLatencySample
			}
		}

		poolIdx += http.NumStatusClasses
		httpAggregations.EndpointAggregations = append(httpAggregations.EndpointAggregations, ms)
	}

	return aggregationsByKey
}

// Build the key for the http map based on whether the local or remote side is http.
func httpKeyFromConn(c network.ConnectionStats) http.Key {
	// Retrieve translated addresses
	laddr, lport := network.GetNATLocalAddress(c)
	raddr, rport := network.GetNATRemoteAddress(c)

	// HTTP data is always indexed as (client, server), so we flip
	// the lookup key if necessary using the port range heuristic
	if network.IsEphemeralPort(int(lport)) {
		return http.NewKey(laddr, raddr, lport, rport, "", http.MethodUnknown)
	}

	return http.NewKey(raddr, laddr, rport, lport, "", http.MethodUnknown)
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
	if addr == nil {
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
