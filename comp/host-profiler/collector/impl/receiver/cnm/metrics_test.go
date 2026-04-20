// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/network"
)

func TestConvertSingleConnection(t *testing.T) {
	conns := makeTestConnections(1)
	md := convertToMetrics(conns, "test-host")

	require.Equal(t, 1, md.ResourceMetrics().Len())
	rm := md.ResourceMetrics().At(0)

	// Verify resource attributes
	hostAttr, ok := rm.Resource().Attributes().Get("host.name")
	require.True(t, ok)
	assert.Equal(t, "test-host", hostAttr.Str())

	sm := rm.ScopeMetrics().At(0)
	assert.Equal(t, scopeName, sm.Scope().Name())
	assert.Equal(t, 9, sm.Metrics().Len())

	// Verify bytes sent metric
	bytesSent := findMetric(sm, metricBytesSent)
	require.NotNil(t, bytesSent)
	require.Equal(t, 1, bytesSent.Sum().DataPoints().Len())
	dp := bytesSent.Sum().DataPoints().At(0)
	assert.Equal(t, int64(1000), dp.IntValue()) // pid=1, 1*1000

	// Verify connection attributes
	srcAddr, ok := dp.Attributes().Get(attrSourceAddr)
	require.True(t, ok)
	assert.Equal(t, "10.0.1.1", srcAddr.Str())

	dstAddr, ok := dp.Attributes().Get(attrDestAddr)
	require.True(t, ok)
	assert.Equal(t, "10.0.2.1", dstAddr.Str())

	srcPort, ok := dp.Attributes().Get(attrSourcePort)
	require.True(t, ok)
	assert.Equal(t, int64(10001), srcPort.Int())

	transport, ok := dp.Attributes().Get(attrTransport)
	require.True(t, ok)
	assert.Equal(t, "tcp", transport.Str())

	netType, ok := dp.Attributes().Get(attrNetworkType)
	require.True(t, ok)
	assert.Equal(t, "ipv4", netType.Str())

	dir, ok := dp.Attributes().Get(attrDirection)
	require.True(t, ok)
	assert.Equal(t, "outgoing", dir.Str())
}

func TestConvertMultipleConnections(t *testing.T) {
	n := 10
	conns := makeTestConnections(n)
	md := convertToMetrics(conns, "test-host")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)

	// Each of the 9 metrics should have n data points
	for i := 0; i < sm.Metrics().Len(); i++ {
		m := sm.Metrics().At(i)
		switch m.Type() {
		case pmetric.MetricTypeSum:
			assert.Equal(t, n, m.Sum().DataPoints().Len(), "metric %s", m.Name())
		case pmetric.MetricTypeGauge:
			assert.Equal(t, n, m.Gauge().DataPoints().Len(), "metric %s", m.Name())
		default:
			t.Errorf("unexpected metric type %v for %s", m.Type(), m.Name())
		}
	}
}

func TestConvertTCPMetrics(t *testing.T) {
	conns := makeTestConnections(1)
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)

	rtt := findMetric(sm, metricTCPRTT)
	require.NotNil(t, rtt)
	assert.Equal(t, int64(15000), rtt.Gauge().DataPoints().At(0).IntValue())

	rttVar := findMetric(sm, metricTCPRTTVar)
	require.NotNil(t, rttVar)
	assert.Equal(t, int64(3000), rttVar.Gauge().DataPoints().At(0).IntValue())

	retransmits := findMetric(sm, metricRetransmits)
	require.NotNil(t, retransmits)
	assert.Equal(t, int64(1), retransmits.Sum().DataPoints().At(0).IntValue())

	established := findMetric(sm, metricTCPEstablished)
	require.NotNil(t, established)
	assert.Equal(t, int64(1), established.Sum().DataPoints().At(0).IntValue())
}

func TestConvertUDPConnection(t *testing.T) {
	conn := makeUDPConnection(1)
	conns := &network.Connections{
		BufferedData:  network.BufferedData{Conns: []network.ConnectionStats{conn}},
		ConnTelemetry: map[network.ConnTelemetryType]int64{},
	}
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)

	bytesSent := findMetric(sm, metricBytesSent)
	require.NotNil(t, bytesSent)
	dp := bytesSent.Sum().DataPoints().At(0)

	transport, ok := dp.Attributes().Get(attrTransport)
	require.True(t, ok)
	assert.Equal(t, "udp", transport.Str())

	dir, ok := dp.Attributes().Get(attrDirection)
	require.True(t, ok)
	assert.Equal(t, "incoming", dir.Str())
}

func TestConvertWithNAT(t *testing.T) {
	conn := makeConnectionWithNAT(1)
	conns := &network.Connections{
		BufferedData:  network.BufferedData{Conns: []network.ConnectionStats{conn}},
		ConnTelemetry: map[network.ConnTelemetryType]int64{},
	}
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	dp := findMetric(sm, metricBytesSent).Sum().DataPoints().At(0)

	natSrc, ok := dp.Attributes().Get(attrNATSrcAddr)
	require.True(t, ok)
	assert.Equal(t, "192.168.1.1", natSrc.Str())

	natDst, ok := dp.Attributes().Get(attrNATDstAddr)
	require.True(t, ok)
	assert.Equal(t, "192.168.1.2", natDst.Str())
}

func TestConvertWithoutNAT(t *testing.T) {
	conns := makeTestConnections(1) // no IPTranslation set
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	dp := findMetric(sm, metricBytesSent).Sum().DataPoints().At(0)

	_, ok := dp.Attributes().Get(attrNATSrcAddr)
	assert.False(t, ok, "NAT attributes should not be present without IPTranslation")
}

func TestConvertWithContainerIDs(t *testing.T) {
	conn := makeConnectionWithContainerIDs(1)
	conns := &network.Connections{
		BufferedData:  network.BufferedData{Conns: []network.ConnectionStats{conn}},
		ConnTelemetry: map[network.ConnTelemetryType]int64{},
	}
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	dp := findMetric(sm, metricBytesSent).Sum().DataPoints().At(0)

	srcCID, ok := dp.Attributes().Get(attrContainerSrc)
	require.True(t, ok)
	assert.Equal(t, "container-src-1", srcCID.Str())

	dstCID, ok := dp.Attributes().Get(attrContainerDst)
	require.True(t, ok)
	assert.Equal(t, "container-dst-1", dstCID.Str())
}

func TestConvertWithProtocolStack(t *testing.T) {
	conn := makeConnectionWithProtocolStack(1)
	conns := &network.Connections{
		BufferedData:  network.BufferedData{Conns: []network.ConnectionStats{conn}},
		ConnTelemetry: map[network.ConnTelemetryType]int64{},
	}
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	dp := findMetric(sm, metricBytesSent).Sum().DataPoints().At(0)

	app, ok := dp.Attributes().Get(attrProtoApp)
	require.True(t, ok)
	assert.Equal(t, "HTTP", app.Str())

	enc, ok := dp.Attributes().Get(attrProtoEnc)
	require.True(t, ok)
	assert.Equal(t, "TLS", enc.Str())

	// API should not be set (it's Unknown)
	_, ok = dp.Attributes().Get(attrProtoAPI)
	assert.False(t, ok)
}

func TestConvertMonotonicCountersAreCumulative(t *testing.T) {
	conns := makeTestConnections(1)
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)

	for _, name := range []string{metricBytesSent, metricBytesReceived, metricPacketsSent, metricPacketsRecv, metricRetransmits, metricTCPEstablished, metricTCPClosed} {
		m := findMetric(sm, name)
		require.NotNil(t, m, "metric %s not found", name)
		assert.True(t, m.Sum().IsMonotonic(), "metric %s should be monotonic", name)
		assert.Equal(t, pmetric.AggregationTemporalityCumulative, m.Sum().AggregationTemporality(), "metric %s should be cumulative", name)
	}
}

func TestConvertResourceAttributes(t *testing.T) {
	conns := makeTestConnections(1)

	t.Run("with hostname", func(t *testing.T) {
		md := convertToMetrics(conns, "my-host")
		rm := md.ResourceMetrics().At(0)
		h, ok := rm.Resource().Attributes().Get("host.name")
		require.True(t, ok)
		assert.Equal(t, "my-host", h.Str())
	})

	t.Run("without hostname", func(t *testing.T) {
		md := convertToMetrics(conns, "")
		rm := md.ResourceMetrics().At(0)
		_, ok := rm.Resource().Attributes().Get("host.name")
		assert.False(t, ok)
	})
}

func TestConvertEmptyConnections(t *testing.T) {
	conns := makeTestConnections(0)
	md := convertToMetrics(conns, "test-host")

	// Still has structure, but metrics have zero data points
	require.Equal(t, 1, md.ResourceMetrics().Len())
	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	assert.Equal(t, 9, sm.Metrics().Len())

	for i := 0; i < sm.Metrics().Len(); i++ {
		m := sm.Metrics().At(i)
		switch m.Type() {
		case pmetric.MetricTypeSum:
			assert.Equal(t, 0, m.Sum().DataPoints().Len(), "metric %s", m.Name())
		case pmetric.MetricTypeGauge:
			assert.Equal(t, 0, m.Gauge().DataPoints().Len(), "metric %s", m.Name())
		}
	}
}

func TestConvertClosedConnection(t *testing.T) {
	conns := makeTestConnections(1)
	conns.Conns[0].IsClosed = true
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	dp := findMetric(sm, metricBytesSent).Sum().DataPoints().At(0)

	closed, ok := dp.Attributes().Get(attrIsClosed)
	require.True(t, ok)
	assert.True(t, closed.Bool())
}

func TestConvertIntraHostConnection(t *testing.T) {
	conns := makeTestConnections(1)
	conns.Conns[0].IntraHost = true
	md := convertToMetrics(conns, "")

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	dp := findMetric(sm, metricBytesSent).Sum().DataPoints().At(0)

	intra, ok := dp.Attributes().Get(attrIntraHost)
	require.True(t, ok)
	assert.True(t, intra.Bool())
}

// findMetric searches for a metric by name in a ScopeMetrics.
func findMetric(sm pmetric.ScopeMetrics, name string) *pmetric.Metric {
	for i := 0; i < sm.Metrics().Len(); i++ {
		m := sm.Metrics().At(i)
		if m.Name() == name {
			return &m
		}
	}
	return nil
}
