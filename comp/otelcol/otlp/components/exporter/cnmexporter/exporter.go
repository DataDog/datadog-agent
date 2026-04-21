// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnmexporter

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// cnmExporter reconstructs Datadog-native CollectorConnections protobuf payloads
// from the OTLP metrics produced by the CNM receiver, then submits them via the
// connections forwarder.
type cnmExporter struct {
	cfg       *Config
	logger    *zap.Logger
	forwarder connectionsforwarder.Component
	hostname  string
	tagger    tagger.Component
	groupID   atomic.Int32
}

func newCNMExporter(cfg *Config, logger *zap.Logger, forwarder connectionsforwarder.Component) *cnmExporter {
	return &cnmExporter{
		cfg:       cfg,
		logger:    logger,
		forwarder: forwarder,
	}
}

// ConsumeMetrics processes OTLP metrics from the CNM receiver, reconstructs
// connection data, encodes as CollectorConnections protobuf, and submits.
func (e *cnmExporter) ConsumeMetrics(_ context.Context, md pmetric.Metrics) error {
	conns := e.reconstructConnections(md)
	if len(conns) == 0 {
		return nil
	}

	hostname := e.extractHostname(md)
	groupID := e.groupID.Add(1)

	return e.encodeAndSubmit(conns, hostname, groupID)
}

// Shutdown is called when the exporter is being shut down.
func (e *cnmExporter) Shutdown(_ context.Context) error {
	return nil
}

// reconstructConnections extracts per-connection data from OTLP metrics.
// It groups data points by connection tuple attributes and rebuilds ConnectionStats.
func (e *cnmExporter) reconstructConnections(md pmetric.Metrics) []network.ConnectionStats {
	// Index connections by a composite key derived from tuple attributes.
	connMap := make(map[connKey]*network.ConnectionStats)

	for i := 0; i < md.ResourceMetrics().Len(); i++ {
		rm := md.ResourceMetrics().At(i)
		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			sm := rm.ScopeMetrics().At(j)
			for k := 0; k < sm.Metrics().Len(); k++ {
				m := sm.Metrics().At(k)
				e.processMetric(m, connMap)
			}
		}
	}

	result := make([]network.ConnectionStats, 0, len(connMap))
	for _, cs := range connMap {
		result = append(result, *cs)
	}
	return result
}

// connKey is a composite key for grouping metrics belonging to the same connection.
type connKey struct {
	srcAddr   string
	srcPort   int64
	dstAddr   string
	dstPort   int64
	transport string
	family    string
	direction string
	pid       int64
	netns     int64
}

func (e *cnmExporter) processMetric(m pmetric.Metric, connMap map[connKey]*network.ConnectionStats) {
	switch m.Type() {
	case pmetric.MetricTypeSum:
		for i := 0; i < m.Sum().DataPoints().Len(); i++ {
			dp := m.Sum().DataPoints().At(i)
			cs := e.getOrCreateConn(dp.Attributes(), connMap)
			applySumValue(m.Name(), dp.IntValue(), cs)
		}
	case pmetric.MetricTypeGauge:
		for i := 0; i < m.Gauge().DataPoints().Len(); i++ {
			dp := m.Gauge().DataPoints().At(i)
			cs := e.getOrCreateConn(dp.Attributes(), connMap)
			applyGaugeValue(m.Name(), dp.IntValue(), cs)
		}
	}
}

func (e *cnmExporter) getOrCreateConn(attrs pcommon.Map, connMap map[connKey]*network.ConnectionStats) *network.ConnectionStats {
	key := extractConnKey(attrs)
	cs, ok := connMap[key]
	if !ok {
		cs = &network.ConnectionStats{}
		populateFromAttributes(cs, attrs)
		connMap[key] = cs
	}
	return cs
}

func extractConnKey(attrs pcommon.Map) connKey {
	return connKey{
		srcAddr:   getStr(attrs, "network.source.address"),
		srcPort:   getInt(attrs, "network.source.port"),
		dstAddr:   getStr(attrs, "network.destination.address"),
		dstPort:   getInt(attrs, "network.destination.port"),
		transport: getStr(attrs, "network.transport"),
		family:    getStr(attrs, "network.type"),
		direction: getStr(attrs, "network.direction"),
		pid:       getInt(attrs, "network.pid"),
		netns:     getInt(attrs, "network.netns"),
	}
}

func populateFromAttributes(cs *network.ConnectionStats, attrs pcommon.Map) {
	srcAddr := getStr(attrs, "network.source.address")
	dstAddr := getStr(attrs, "network.destination.address")
	cs.Source = util.AddressFromString(srcAddr)
	cs.Dest = util.AddressFromString(dstAddr)
	cs.SPort = uint16(getInt(attrs, "network.source.port"))
	cs.DPort = uint16(getInt(attrs, "network.destination.port"))
	cs.Pid = uint32(getInt(attrs, "network.pid"))
	cs.NetNS = uint32(getInt(attrs, "network.netns"))

	switch getStr(attrs, "network.transport") {
	case "tcp":
		cs.Type = network.TCP
	case "udp":
		cs.Type = network.UDP
	}

	switch getStr(attrs, "network.type") {
	case "ipv4":
		cs.Family = network.AFINET
	case "ipv6":
		cs.Family = network.AFINET6
	}

	switch getStr(attrs, "network.direction") {
	case "incoming":
		cs.Direction = network.INCOMING
	case "outgoing":
		cs.Direction = network.OUTGOING
	case "local":
		cs.Direction = network.LOCAL
	case "none":
		cs.Direction = network.NONE
	}

	cs.IntraHost = getBool(attrs, "network.intra_host")
	cs.IsClosed = getBool(attrs, "network.is_closed")
	cs.Cookie = uint64(getInt(attrs, "network.cookie"))
	cs.LastUpdateEpoch = uint64(getInt(attrs, "network.last_update_epoch"))

	durationNs := getInt(attrs, "network.duration_ns")
	if durationNs > 0 {
		cs.Duration = time.Duration(durationNs)
	}

	// NAT translation
	natSrc := getStr(attrs, "network.nat.source.address")
	natDst := getStr(attrs, "network.nat.destination.address")
	if natSrc != "" || natDst != "" {
		cs.IPTranslation = &network.IPTranslation{
			ReplSrcIP: util.AddressFromString(natSrc),
			ReplDstIP: util.AddressFromString(natDst),
		}
	}
}

func applySumValue(name string, val int64, cs *network.ConnectionStats) {
	switch name {
	case "network.bytes.sent":
		cs.Monotonic.SentBytes = uint64(val)
	case "network.bytes.received":
		cs.Monotonic.RecvBytes = uint64(val)
	case "network.packets.sent":
		cs.Monotonic.SentPackets = uint64(val)
	case "network.packets.received":
		cs.Monotonic.RecvPackets = uint64(val)
	case "network.tcp.retransmits":
		cs.Monotonic.Retransmits = uint32(val)
	case "network.tcp.established":
		cs.Monotonic.TCPEstablished = uint16(val)
	case "network.tcp.closed":
		cs.Monotonic.TCPClosed = uint16(val)
	}
}

func applyGaugeValue(name string, val int64, cs *network.ConnectionStats) {
	switch name {
	case "network.tcp.rtt":
		cs.RTT = uint32(val)
	case "network.tcp.rtt_var":
		cs.RTTVar = uint32(val)
	}
}

func (e *cnmExporter) extractHostname(md pmetric.Metrics) string {
	if e.hostname != "" {
		return e.hostname
	}
	if md.ResourceMetrics().Len() > 0 {
		rm := md.ResourceMetrics().At(0)
		if v, ok := rm.Resource().Attributes().Get("host.name"); ok {
			return v.Str()
		}
	}
	return ""
}

// Attribute accessors
func getStr(m pcommon.Map, key string) string {
	v, ok := m.Get(key)
	if !ok {
		return ""
	}
	return v.Str()
}

func getInt(m pcommon.Map, key string) int64 {
	v, ok := m.Get(key)
	if !ok {
		return 0
	}
	return v.Int()
}

func getBool(m pcommon.Map, key string) bool {
	v, ok := m.Get(key)
	if !ok {
		return false
	}
	return v.Bool()
}
