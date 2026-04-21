// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnmexporter

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// mockForwarder captures submitted payloads for verification.
type mockForwarder struct {
	mu       sync.Mutex
	payloads []transaction.BytesPayloads
	headers  []http.Header
}

func (m *mockForwarder) SubmitConnectionChecks(
	payload transaction.BytesPayloads,
	extra http.Header,
) (chan defaultforwarder.Response, error) {
	m.mu.Lock()
	m.payloads = append(m.payloads, payload)
	m.headers = append(m.headers, extra)
	m.mu.Unlock()
	ch := make(chan defaultforwarder.Response)
	close(ch)
	return ch, nil
}

func TestExporterConsumeEmptyMetrics(t *testing.T) {
	fwd := &mockForwarder{}
	exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), fwd)

	md := pmetric.NewMetrics()
	require.NoError(t, exp.ConsumeMetrics(context.Background(), md))

	assert.Empty(t, fwd.payloads)
}

func TestExporterReconstructsConnections(t *testing.T) {
	exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), nil)

	md := buildTestMetrics(3)
	conns := exp.reconstructConnections(md)

	require.Len(t, conns, 3)
	// reconstructConnections groups by tuple via map, so iteration order is non-deterministic.
	// Sort by PID for stable assertions.
	sort.Slice(conns, func(i, j int) bool { return conns[i].Pid < conns[j].Pid })

	for i, conn := range conns {
		assert.Equal(t, uint64((i+1)*1000), conn.Monotonic.SentBytes)
		assert.Equal(t, uint64((i+1)*2000), conn.Monotonic.RecvBytes)
		assert.Equal(t, uint32(15000), conn.RTT)
	}
}

func TestExporterReconstructsConnectionAttributes(t *testing.T) {
	exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), nil)

	md := buildTestMetrics(1)
	conns := exp.reconstructConnections(md)

	require.Len(t, conns, 1)
	conn := conns[0]

	assert.Equal(t, "10.0.1.1", conn.Source.String())
	assert.Equal(t, "10.0.2.1", conn.Dest.String())
	assert.Equal(t, uint16(10001), conn.SPort)
	assert.Equal(t, uint16(443), conn.DPort)
	assert.False(t, conn.IntraHost)
	assert.False(t, conn.IsClosed)
}

func TestExporterReconstructsNAT(t *testing.T) {
	exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), nil)

	md := buildTestMetricsWithNAT()
	conns := exp.reconstructConnections(md)

	require.Len(t, conns, 1)
	require.NotNil(t, conns[0].IPTranslation)
	assert.Equal(t, "192.168.1.1", conns[0].IPTranslation.ReplSrcIP.String())
	assert.Equal(t, "192.168.1.2", conns[0].IPTranslation.ReplDstIP.String())
}

func TestExporterSubmitsToForwarder(t *testing.T) {
	fwd := &mockForwarder{}
	exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), fwd)
	exp.hostname = "test-host"

	md := buildTestMetrics(5)
	require.NoError(t, exp.ConsumeMetrics(context.Background(), md))

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	require.NotEmpty(t, fwd.payloads, "expected at least one payload submission")
	require.NotEmpty(t, fwd.headers)

	// Verify hostname header (X-Dd-Hostname)
	assert.Equal(t, "test-host", fwd.headers[0].Get("X-Dd-Hostname"))
}

func TestExporterBatching(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.MaxConnsPerMessage = 2 // Small batch size for testing

	fwd := &mockForwarder{}
	exp := newCNMExporter(cfg, zap.NewNop(), fwd)
	exp.hostname = "test-host"

	md := buildTestMetrics(5)
	require.NoError(t, exp.ConsumeMetrics(context.Background(), md))

	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	// 5 connections / 2 per batch = 3 batches
	assert.Equal(t, 3, len(fwd.payloads))
}

func TestExporterExtractsHostname(t *testing.T) {
	t.Run("from exporter field", func(t *testing.T) {
		exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), nil)
		exp.hostname = "configured-host"

		md := buildTestMetrics(1)
		assert.Equal(t, "configured-host", exp.extractHostname(md))
	})

	t.Run("from resource attributes", func(t *testing.T) {
		exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), nil)

		md := buildTestMetrics(1)
		assert.Equal(t, "test-host", exp.extractHostname(md))
	})

	t.Run("empty when neither set", func(t *testing.T) {
		exp := newCNMExporter(createDefaultConfig().(*Config), zap.NewNop(), nil)

		md := pmetric.NewMetrics()
		assert.Equal(t, "", exp.extractHostname(md))
	})
}

// buildTestMetrics creates pmetric.Metrics that mimic the CNM receiver output.
func buildTestMetrics(n int) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("host.name", "test-host")

	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("datadog/cnm")

	now := pcommon.NewTimestampFromTime(time.Now())

	addSumMetric := func(name string) pmetric.Metric {
		m := sm.Metrics().AppendEmpty()
		m.SetName(name)
		s := m.SetEmptySum()
		s.SetIsMonotonic(true)
		s.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		return m
	}

	addGaugeMetric := func(name string) pmetric.Metric {
		m := sm.Metrics().AppendEmpty()
		m.SetName(name)
		m.SetEmptyGauge()
		return m
	}

	bytesSent := addSumMetric("network.bytes.sent")
	bytesRecv := addSumMetric("network.bytes.received")
	packetsSent := addSumMetric("network.packets.sent")
	packetsRecv := addSumMetric("network.packets.received")
	retransmits := addSumMetric("network.tcp.retransmits")
	established := addSumMetric("network.tcp.established")
	closed := addSumMetric("network.tcp.closed")
	rtt := addGaugeMetric("network.tcp.rtt")
	rttVar := addGaugeMetric("network.tcp.rtt_var")

	for i := 1; i <= n; i++ {
		addDP := func(m pmetric.Metric, val int64) {
			var dp pmetric.NumberDataPoint
			switch m.Type() {
			case pmetric.MetricTypeSum:
				dp = m.Sum().DataPoints().AppendEmpty()
			case pmetric.MetricTypeGauge:
				dp = m.Gauge().DataPoints().AppendEmpty()
			}
			dp.SetTimestamp(now)
			dp.SetIntValue(val)
			setTestAttrs(dp.Attributes(), i)
		}

		addDP(bytesSent, int64(i*1000))
		addDP(bytesRecv, int64(i*2000))
		addDP(packetsSent, int64(i*10))
		addDP(packetsRecv, int64(i*20))
		addDP(retransmits, int64(i))
		addDP(established, 1)
		addDP(closed, 0)
		addDP(rtt, 15000)
		addDP(rttVar, 3000)
	}

	return md
}

func buildTestMetricsWithNAT() pmetric.Metrics {
	md := buildTestMetrics(1)
	// Add NAT attributes to all data points
	rm := md.ResourceMetrics().At(0)
	sm := rm.ScopeMetrics().At(0)
	for i := 0; i < sm.Metrics().Len(); i++ {
		m := sm.Metrics().At(i)
		switch m.Type() {
		case pmetric.MetricTypeSum:
			for j := 0; j < m.Sum().DataPoints().Len(); j++ {
				dp := m.Sum().DataPoints().At(j)
				dp.Attributes().PutStr("network.nat.source.address", "192.168.1.1")
				dp.Attributes().PutStr("network.nat.destination.address", "192.168.1.2")
			}
		case pmetric.MetricTypeGauge:
			for j := 0; j < m.Gauge().DataPoints().Len(); j++ {
				dp := m.Gauge().DataPoints().At(j)
				dp.Attributes().PutStr("network.nat.source.address", "192.168.1.1")
				dp.Attributes().PutStr("network.nat.destination.address", "192.168.1.2")
			}
		}
	}
	return md
}

func setTestAttrs(attrs pcommon.Map, idx int) {
	attrs.PutStr("network.source.address", "10.0.1."+strconv.Itoa(idx))
	attrs.PutInt("network.source.port", int64(10000+idx))
	attrs.PutStr("network.destination.address", "10.0.2."+strconv.Itoa(idx))
	attrs.PutInt("network.destination.port", 443)
	attrs.PutStr("network.transport", "tcp")
	attrs.PutStr("network.type", "ipv4")
	attrs.PutStr("network.direction", "outgoing")
	attrs.PutInt("network.pid", int64(idx))
	attrs.PutInt("network.netns", 4026531840)
	attrs.PutBool("network.intra_host", false)
	attrs.PutBool("network.is_closed", false)
	attrs.PutInt("network.cookie", int64(idx*12345))
	attrs.PutInt("network.last_update_epoch", 0)
	attrs.PutInt("network.duration_ns", int64(30*time.Second))
}
