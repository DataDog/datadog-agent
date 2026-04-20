// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const scopeName = "datadog/cnm"

// Metric names emitted per connection.
const (
	metricBytesSent      = "network.bytes.sent"
	metricBytesReceived  = "network.bytes.received"
	metricPacketsSent    = "network.packets.sent"
	metricPacketsRecv    = "network.packets.received"
	metricRetransmits    = "network.tcp.retransmits"
	metricTCPEstablished = "network.tcp.established"
	metricTCPClosed      = "network.tcp.closed"
	metricTCPRTT         = "network.tcp.rtt"
	metricTCPRTTVar      = "network.tcp.rtt_var"
)

// Attribute keys for connection identity.
const (
	attrSourceAddr   = "network.source.address"
	attrSourcePort   = "network.source.port"
	attrDestAddr     = "network.destination.address"
	attrDestPort     = "network.destination.port"
	attrTransport    = "network.transport"
	attrNetworkType  = "network.type"
	attrDirection    = "network.direction"
	attrPID          = "network.pid"
	attrNetNS        = "network.netns"
	attrContainerSrc = "container.id.source"
	attrContainerDst = "container.id.dest"
	attrIntraHost    = "network.intra_host"
	attrIsClosed     = "network.is_closed"
	attrProtoAPI     = "network.protocol.api"
	attrProtoApp     = "network.protocol.application"
	attrProtoEnc     = "network.protocol.encryption"
	attrCookie       = "network.cookie"
	attrLastUpdate   = "network.last_update_epoch"
	attrDurationNs   = "network.duration_ns"
	attrNATSrcAddr   = "network.nat.source.address"
	attrNATDstAddr   = "network.nat.destination.address"
)

// convertToMetrics transforms collected network connections into pmetric.Metrics.
func convertToMetrics(conns *network.Connections, hostname string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()

	if hostname != "" {
		rm.Resource().Attributes().PutStr("host.name", hostname)
	}

	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName(scopeName)
	sm.Scope().SetVersion(version.AgentVersion)

	now := pcommon.NewTimestampFromTime(time.Now())

	// Create metric definitions
	bytesSent := addSumMetric(sm, metricBytesSent, "By")
	bytesRecv := addSumMetric(sm, metricBytesReceived, "By")
	packetsSent := addSumMetric(sm, metricPacketsSent, "{packets}")
	packetsRecv := addSumMetric(sm, metricPacketsRecv, "{packets}")
	retransmits := addSumMetric(sm, metricRetransmits, "{count}")
	tcpEstablished := addSumMetric(sm, metricTCPEstablished, "{count}")
	tcpClosed := addSumMetric(sm, metricTCPClosed, "{count}")
	tcpRTT := addGaugeMetric(sm, metricTCPRTT, "us")
	tcpRTTVar := addGaugeMetric(sm, metricTCPRTTVar, "us")

	for i := range conns.Conns {
		conn := &conns.Conns[i]

		// Emit sum (monotonic) metrics
		addSumDataPoint(bytesSent, now, int64(conn.Monotonic.SentBytes), conn)
		addSumDataPoint(bytesRecv, now, int64(conn.Monotonic.RecvBytes), conn)
		addSumDataPoint(packetsSent, now, int64(conn.Monotonic.SentPackets), conn)
		addSumDataPoint(packetsRecv, now, int64(conn.Monotonic.RecvPackets), conn)
		addSumDataPoint(retransmits, now, int64(conn.Monotonic.Retransmits), conn)
		addSumDataPoint(tcpEstablished, now, int64(conn.Monotonic.TCPEstablished), conn)
		addSumDataPoint(tcpClosed, now, int64(conn.Monotonic.TCPClosed), conn)

		// Emit gauge metrics
		addGaugeDataPoint(tcpRTT, now, int64(conn.RTT), conn)
		addGaugeDataPoint(tcpRTTVar, now, int64(conn.RTTVar), conn)
	}

	return md
}

func addSumMetric(sm pmetric.ScopeMetrics, name, unit string) pmetric.Metric {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	return m
}

func addGaugeMetric(sm pmetric.ScopeMetrics, name, unit string) pmetric.Metric {
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	m.SetUnit(unit)
	m.SetEmptyGauge()
	return m
}

func addSumDataPoint(m pmetric.Metric, ts pcommon.Timestamp, val int64, conn *network.ConnectionStats) {
	dp := m.Sum().DataPoints().AppendEmpty()
	dp.SetTimestamp(ts)
	dp.SetIntValue(val)
	setConnectionAttributes(dp.Attributes(), conn)
}

func addGaugeDataPoint(m pmetric.Metric, ts pcommon.Timestamp, val int64, conn *network.ConnectionStats) {
	dp := m.Gauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(ts)
	dp.SetIntValue(val)
	setConnectionAttributes(dp.Attributes(), conn)
}

func setConnectionAttributes(attrs pcommon.Map, conn *network.ConnectionStats) {
	attrs.PutStr(attrSourceAddr, conn.Source.String())
	attrs.PutInt(attrSourcePort, int64(conn.SPort))
	attrs.PutStr(attrDestAddr, conn.Dest.String())
	attrs.PutInt(attrDestPort, int64(conn.DPort))
	attrs.PutStr(attrTransport, transportString(conn.Type))
	attrs.PutStr(attrNetworkType, familyString(conn.Family))
	attrs.PutStr(attrDirection, directionString(conn.Direction))
	attrs.PutInt(attrPID, int64(conn.Pid))
	attrs.PutInt(attrNetNS, int64(conn.NetNS))
	attrs.PutBool(attrIntraHost, conn.IntraHost)
	attrs.PutBool(attrIsClosed, conn.IsClosed)
	attrs.PutInt(attrCookie, int64(conn.Cookie))
	attrs.PutInt(attrLastUpdate, int64(conn.LastUpdateEpoch))
	attrs.PutInt(attrDurationNs, conn.Duration.Nanoseconds())

	if conn.ContainerID.Source != nil {
		if s, ok := conn.ContainerID.Source.Get().(string); ok && s != "" {
			attrs.PutStr(attrContainerSrc, s)
		}
	}
	if conn.ContainerID.Dest != nil {
		if s, ok := conn.ContainerID.Dest.Get().(string); ok && s != "" {
			attrs.PutStr(attrContainerDst, s)
		}
	}

	if api := conn.ProtocolStack.API.String(); api != "Unknown" {
		attrs.PutStr(attrProtoAPI, api)
	}
	if app := conn.ProtocolStack.Application.String(); app != "Unknown" {
		attrs.PutStr(attrProtoApp, app)
	}
	if enc := conn.ProtocolStack.Encryption.String(); enc != "Unknown" {
		attrs.PutStr(attrProtoEnc, enc)
	}

	if conn.IPTranslation != nil {
		attrs.PutStr(attrNATSrcAddr, conn.IPTranslation.ReplSrcIP.String())
		attrs.PutStr(attrNATDstAddr, conn.IPTranslation.ReplDstIP.String())
	}
}

func transportString(t network.ConnectionType) string {
	switch t {
	case network.TCP:
		return "tcp"
	case network.UDP:
		return "udp"
	default:
		return "unknown"
	}
}

func familyString(f network.ConnectionFamily) string {
	switch f {
	case network.AFINET:
		return "ipv4"
	case network.AFINET6:
		return "ipv6"
	default:
		return "unknown"
	}
}

func directionString(d network.ConnectionDirection) string {
	switch d {
	case network.INCOMING:
		return "incoming"
	case network.OUTGOING:
		return "outgoing"
	case network.LOCAL:
		return "local"
	case network.NONE:
		return "none"
	default:
		return "unknown"
	}
}
