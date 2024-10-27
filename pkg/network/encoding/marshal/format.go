// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"math"

	"github.com/twmb/murmur3"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const maxRoutes = math.MaxInt32

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

func mergeDynamicTags(dynamicTags ...map[string]struct{}) (out map[string]struct{}) {
	for _, tags := range dynamicTags {
		if out == nil {
			out = tags
			continue
		}
		for k, v := range tags {
			out[k] = v
		}
	}
	return
}

// FormatConnection converts a ConnectionStats into an model.Connection
func FormatConnection(builder *model.ConnectionBuilder, conn network.ConnectionStats, routes map[string]RouteIdx,
	httpEncoder *httpEncoder, http2Encoder *http2Encoder, kafkaEncoder *kafkaEncoder, postgresEncoder *postgresEncoder,
	redisEncoder *redisEncoder, dnsFormatter *dnsFormatter, ipc ipCache, tagsSet *network.TagsSet) {

	builder.SetPid(int32(conn.Pid))

	var containerID string
	if conn.ContainerID.Source != nil {
		containerID = conn.ContainerID.Source.Get().(string)
	}
	builder.SetLaddr(func(w *model.AddrBuilder) {
		w.SetIp(ipc.Get(conn.Source))
		w.SetPort(int32(conn.SPort))
		w.SetContainerId(containerID)
	})

	containerID = ""
	if conn.ContainerID.Dest != nil {
		containerID = conn.ContainerID.Dest.Get().(string)
	}
	builder.SetRaddr(func(w *model.AddrBuilder) {
		w.SetIp(ipc.Get(conn.Dest))
		w.SetPort(int32(conn.DPort))
		w.SetContainerId(containerID)
	})
	builder.SetFamily(uint64(formatFamily(conn.Family)))
	builder.SetType(uint64(formatType(conn.Type)))
	builder.SetIsLocalPortEphemeral(uint64(formatEphemeralType(conn.SPortIsEphemeral)))
	builder.SetLastBytesSent(conn.Last.SentBytes)
	builder.SetLastBytesReceived(conn.Last.RecvBytes)
	builder.SetLastPacketsSent(conn.Last.SentPackets)
	builder.SetLastRetransmits(conn.Last.Retransmits)
	builder.SetLastPacketsReceived(conn.Last.RecvPackets)
	builder.SetDirection(uint64(formatDirection(conn.Direction)))
	builder.SetNetNS(conn.NetNS)
	if conn.IPTranslation != nil {
		builder.SetIpTranslation(func(w *model.IPTranslationBuilder) {
			ipt := formatIPTranslation(conn.IPTranslation, ipc)
			w.SetReplSrcPort(ipt.ReplSrcPort)
			w.SetReplDstPort(ipt.ReplDstPort)
			w.SetReplSrcIP(ipt.ReplSrcIP)
			w.SetReplDstIP(ipt.ReplDstIP)
		})
	}
	builder.SetRtt(conn.RTT)
	builder.SetRttVar(conn.RTTVar)
	builder.SetIntraHost(conn.IntraHost)
	builder.SetLastTcpEstablished(uint32(conn.Last.TCPEstablished))
	builder.SetLastTcpClosed(uint32(conn.Last.TCPClosed))
	builder.SetProtocol(func(w *model.ProtocolStackBuilder) {
		ps := FormatProtocolStack(conn.ProtocolStack, conn.StaticTags)
		for _, p := range ps.Stack {
			w.AddStack(uint64(p))
		}
	})

	builder.SetRouteIdx(formatRouteIdx(conn.Via, routes))
	dnsFormatter.FormatConnectionDNS(conn, builder)

	if len(conn.TCPFailures) > 0 {
		builder.AddTcpFailuresByErrCode(func(w *model.Connection_TcpFailuresByErrCodeEntryBuilder) {
			for k, v := range conn.TCPFailures {
				w.SetKey(k)
				w.SetValue(v)
			}
		})
	}

	httpStaticTags, httpDynamicTags := httpEncoder.GetHTTPAggregationsAndTags(conn, builder)
	http2StaticTags, http2DynamicTags := http2Encoder.WriteHTTP2AggregationsAndTags(conn, builder)

	staticTags := httpStaticTags | http2StaticTags
	dynamicTags := mergeDynamicTags(httpDynamicTags, http2DynamicTags)

	staticTags |= kafkaEncoder.WriteKafkaAggregations(conn, builder)
	staticTags |= postgresEncoder.WritePostgresAggregations(conn, builder)
	staticTags |= redisEncoder.WriteRedisAggregations(conn, builder)

	conn.StaticTags |= staticTags
	tags, tagChecksum := formatTags(conn, tagsSet, dynamicTags)
	for _, t := range tags {
		builder.AddTags(t)
	}
	builder.SetTagsChecksum(tagChecksum)
}

// FormatCompilationTelemetry converts telemetry from its internal representation to a protobuf message
func FormatCompilationTelemetry(builder *model.ConnectionsBuilder, telByAsset map[string]network.RuntimeCompilationTelemetry) {
	if telByAsset == nil {
		return
	}

	for asset, tel := range telByAsset {
		builder.AddCompilationTelemetryByAsset(func(kv *model.Connections_CompilationTelemetryByAssetEntryBuilder) {
			kv.SetKey(asset)
			kv.SetValue(func(w *model.RuntimeCompilationTelemetryBuilder) {
				w.SetRuntimeCompilationEnabled(tel.RuntimeCompilationEnabled)
				w.SetRuntimeCompilationResult(uint64(tel.RuntimeCompilationResult))
				w.SetRuntimeCompilationDuration(tel.RuntimeCompilationDuration)
			})
		})
	}
}

// FormatConnectionTelemetry converts telemetry from its internal representation to a protobuf message
func FormatConnectionTelemetry(builder *model.ConnectionsBuilder, tel map[network.ConnTelemetryType]int64) {
	// Fetch USM payload telemetry
	ret := GetUSMPayloadTelemetry()

	// Merge it with NPM telemetry
	for k, v := range tel {
		ret[string(k)] = v
	}

	for k, v := range ret {
		builder.AddConnTelemetryMap(func(w *model.Connections_ConnTelemetryMapEntryBuilder) {
			w.SetKey(k)
			w.SetValue(v)
		})
	}

}

// FormatCORETelemetry writes the CORETelemetryByAsset map into a connections payload
func FormatCORETelemetry(builder *model.ConnectionsBuilder, telByAsset map[string]int32) {
	if telByAsset == nil {
		return
	}

	for asset, tel := range telByAsset {
		builder.AddCORETelemetryByAsset(func(w *model.Connections_CORETelemetryByAssetEntryBuilder) {
			w.SetKey(asset)
			w.SetValue(uint64(tel))
		})
	}
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

func formatTags(c network.ConnectionStats, tagsSet *network.TagsSet, connDynamicTags map[string]struct{}) ([]uint32, uint32) {
	var checksum uint32

	staticTags := network.GetStaticTags(c.StaticTags)
	tagsIdx := make([]uint32, 0, len(staticTags)+len(connDynamicTags)+len(c.Tags))

	for _, tag := range staticTags {
		checksum ^= murmur3.StringSum32(tag)
		tagsIdx = append(tagsIdx, tagsSet.Add(tag))
	}

	// Dynamic tags
	for tag := range connDynamicTags {
		checksum ^= murmur3.StringSum32(tag)
		tagsIdx = append(tagsIdx, tagsSet.Add(tag))
	}

	// other tags, e.g., from process env vars like DD_ENV, etc.
	for _, tag := range c.Tags {
		t := tag.Get().(string)
		checksum ^= murmur3.StringSum32(t)
		tagsIdx = append(tagsIdx, tagsSet.Add(t))
	}

	return tagsIdx, checksum
}
