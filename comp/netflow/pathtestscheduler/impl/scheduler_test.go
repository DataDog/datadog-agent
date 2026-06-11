// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package impl

import (
	"iter"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

// fakeNpCollector records calls to ScheduleNetworkPathTests for assertions.
type fakeNpCollector struct {
	calls [][]npmodel.NetworkPathConnection
}

func (f *fakeNpCollector) ScheduleNetworkPathTests(conns iter.Seq[npmodel.NetworkPathConnection]) {
	var batch []npmodel.NetworkPathConnection
	for conn := range conns {
		batch = append(batch, conn)
	}
	f.calls = append(f.calls, batch)
}

func (f *fakeNpCollector) totalConns() int {
	total := 0
	for _, batch := range f.calls {
		total += len(batch)
	}
	return total
}

func (f *fakeNpCollector) callCount() int {
	return len(f.calls)
}

// ── Builder helpers ──────────────────────────────────────────────────────────

// newTestScheduler builds a schedulerImpl directly (bypassing fx) for unit tests.
func newTestScheduler(t *testing.T, overrides map[string]any) (*schedulerImpl, *fakeNpCollector, *teststatsd.Client) {
	t.Helper()
	mockConfig := config.NewMockWithOverrides(t, overrides)
	cfg, err := newSchedulerConfig(mockConfig)
	require.NoError(t, err)

	collector := &fakeNpCollector{}
	statsd := &teststatsd.Client{}
	logger := logmock.New(t)

	s := &schedulerImpl{
		logger:      logger,
		statsd:      statsd,
		cfg:         cfg,
		npcollector: collector,
	}
	return s, collector, statsd
}

// makeValidFlow returns a minimal valid TCP flow for 8.8.8.8:443 exported by 192.0.2.1.
func makeValidFlow(dstOctet byte) *common.Flow {
	return &common.Flow{
		Namespace:    "default",
		EtherType:    etherTypeIPv4,
		IPProtocol:   ipProtoTCP,
		SrcAddr:      []byte{10, 0, 0, 1},
		DstAddr:      []byte{8, 8, 8, dstOctet},
		SrcPort:      54321,
		DstPort:      443,
		ExporterAddr: []byte{192, 0, 2, 1},
	}
}

// ── Tests ────────────────────────────────────────────────────────────────────

// TestScheduleFromFlows_Disabled verifies that when enabled=false the scheduler
// is a no-op: npcollector is never called and no metrics are emitted.
func TestScheduleFromFlows_Disabled(t *testing.T) {
	s, collector, statsd := newTestScheduler(t, map[string]any{
		// enabled defaults to false; make it explicit.
		"network_path.netflow_monitoring.enabled": false,
	})

	s.ScheduleFromFlows([]*common.Flow{makeValidFlow(8)})

	assert.Equal(t, 0, collector.callCount(), "npcollector should not be called when disabled")
	assert.Empty(t, statsd.CountCalls, "no metrics should be emitted when disabled")
}

// TestScheduleFromFlows_NilInput verifies that nil input is safe when enabled.
func TestScheduleFromFlows_NilInput(t *testing.T) {
	s, collector, _ := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled": true,
	})

	require.NotPanics(t, func() { s.ScheduleFromFlows(nil) })
	assert.Equal(t, 0, collector.callCount(), "npcollector should not be called for nil input")
}

// TestScheduleFromFlows_EmptyInput verifies that an empty slice is safe when enabled.
func TestScheduleFromFlows_EmptyInput(t *testing.T) {
	s, collector, _ := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled": true,
	})

	require.NotPanics(t, func() { s.ScheduleFromFlows([]*common.Flow{}) })
	assert.Equal(t, 0, collector.callCount(), "npcollector should not be called for empty input")
}

// TestScheduleFromFlows_HappyPath verifies that valid flows are converted and
// forwarded to npcollector.
func TestScheduleFromFlows_HappyPath(t *testing.T) {
	s, collector, statsd := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": 100,
	})

	flows := []*common.Flow{
		makeValidFlow(1),
		makeValidFlow(2),
		makeValidFlow(3),
	}

	s.ScheduleFromFlows(flows)

	require.Equal(t, 1, collector.callCount(), "npcollector should be called once")
	assert.Equal(t, 3, collector.totalConns(), "all 3 unique destinations should be forwarded")

	// flows_received metric should be emitted.
	require.NotEmpty(t, statsd.CountCalls)
	var found bool
	for _, call := range statsd.CountCalls {
		if call.Name == metricFlowsReceived {
			assert.Equal(t, int64(3), int64(call.Value))
			found = true
		}
	}
	assert.True(t, found, "flows_received metric should be emitted")
}

// TestScheduleFromFlows_MaxDestinationsCap verifies that when connections exceed
// max_destinations_per_flush, only the cap number are forwarded and the overflow
// is reported via the max_destinations_cap drop metric.
func TestScheduleFromFlows_MaxDestinationsCap(t *testing.T) {
	const cap = 10
	const total = 100
	s, collector, statsd := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": cap,
	})

	// Generate `total` flows with unique destinations 8.8.X.Y to ensure all are
	// distinct after aggregation.
	flows := make([]*common.Flow, total)
	for i := 0; i < total; i++ {
		flows[i] = &common.Flow{
			Namespace:    "default",
			EtherType:    etherTypeIPv4,
			IPProtocol:   ipProtoTCP,
			SrcAddr:      []byte{10, 0, 0, 1},
			DstAddr:      []byte{8, 8, byte(i / 256), byte(i % 256)},
			SrcPort:      54321,
			DstPort:      443,
			ExporterAddr: []byte{192, 0, 2, 1},
		}
	}

	s.ScheduleFromFlows(flows)

	require.Equal(t, 1, collector.callCount())
	assert.Equal(t, cap, collector.totalConns(), "only cap connections should be forwarded")

	// Verify the max_destinations_cap drop metric was emitted with the overflow count.
	var capDropped int64
	for _, call := range statsd.CountCalls {
		if call.Name == metricDropped {
			for _, tag := range call.Tags {
				if tag == "reason:max_destinations_cap" {
					capDropped += int64(call.Value)
				}
			}
		}
	}
	assert.Equal(t, int64(total-cap), capDropped,
		"drop metric should report %d overflow connections", total-cap)
}

// TestScheduleFromFlows_DestExcludes verifies that connections whose destination
// falls within an excluded CIDR are dropped.
func TestScheduleFromFlows_DestExcludes(t *testing.T) {
	s, collector, statsd := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": 100,
		// Exclude 8.8.8.0/24 — our test flows will be split between
		// 8.8.8.x (excluded) and 9.9.9.x (allowed).
		"network_path.netflow_monitoring.dest_excludes": []string{"8.8.8.0/24"},
	})

	flows := []*common.Flow{
		// Should be excluded (dest in 8.8.8.0/24).
		{
			Namespace: "default", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{8, 8, 8, 8},
			SrcPort: 54321, DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
		// Should be allowed (9.9.9.9 not in excluded range).
		{
			Namespace: "default", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{9, 9, 9, 9},
			SrcPort: 54321, DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
	}

	s.ScheduleFromFlows(flows)

	require.Equal(t, 1, collector.callCount())
	assert.Equal(t, 1, collector.totalConns(), "excluded destination should be dropped")

	// Verify the dest_excluded drop metric was emitted.
	var excludedDropped int64
	for _, call := range statsd.CountCalls {
		if call.Name == metricDropped {
			for _, tag := range call.Tags {
				if tag == "reason:dest_excluded" {
					excludedDropped += int64(call.Value)
				}
			}
		}
	}
	assert.Equal(t, int64(1), excludedDropped, "dest_excluded drop metric should be 1")
}

// TestScheduleFromFlows_DropCountMetrics verifies that converter drop reasons
// (e.g. not_ipv4, loopback) produce per-reason drop metrics.
func TestScheduleFromFlows_DropCountMetrics(t *testing.T) {
	s, _, statsd := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled": true,
	})

	flows := []*common.Flow{
		// IPv6 — will be dropped by converter with reason "not_ipv4".
		{
			Namespace: "default", EtherType: 0x86DD, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{8, 8, 8, 8},
			SrcPort: 54321, DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
		// Loopback destination — will be dropped with reason "loopback".
		{
			Namespace: "default", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{127, 0, 0, 1},
			SrcPort: 54321, DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
	}

	s.ScheduleFromFlows(flows)

	dropReasons := make(map[string]int64)
	for _, call := range statsd.CountCalls {
		if call.Name == metricDropped {
			for _, tag := range call.Tags {
				if len(tag) > 7 && tag[:7] == "reason:" {
					dropReasons[tag[7:]] += int64(call.Value)
				}
			}
		}
	}

	assert.Equal(t, int64(1), dropReasons["not_ipv4"], "should see one not_ipv4 drop")
	assert.Equal(t, int64(1), dropReasons["loopback"], "should see one loopback drop")
}

// TestScheduleFromFlows_ConnectionsEmittedMetric verifies the connections_emitted
// metric reflects the number of connections actually forwarded to npcollector.
func TestScheduleFromFlows_ConnectionsEmittedMetric(t *testing.T) {
	s, _, statsd := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": 100,
	})

	s.ScheduleFromFlows([]*common.Flow{makeValidFlow(1), makeValidFlow(2)})

	var emitted int64
	for _, call := range statsd.CountCalls {
		if call.Name == metricConnectionsEmitted {
			emitted += int64(call.Value)
		}
	}
	assert.Equal(t, int64(2), emitted, "connections_emitted should equal the number forwarded")
}

// TestScheduleFromFlows_AllDropped verifies that when all connections are dropped
// (e.g. all excluded) npcollector is not called.
func TestScheduleFromFlows_AllDropped(t *testing.T) {
	s, collector, _ := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":       true,
		"network_path.netflow_monitoring.dest_excludes": []string{"8.8.8.0/24"},
	})

	// Only flows with excluded destinations.
	flows := []*common.Flow{
		{
			Namespace: "default", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{8, 8, 8, 1},
			SrcPort: 54321, DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
	}

	s.ScheduleFromFlows(flows)

	assert.Equal(t, 0, collector.callCount(), "npcollector should not be called when all connections are dropped")
}

// TestScheduleFromFlows_AggregationDeduplication verifies that two flows with the
// same (dst, port, protocol) are aggregated into a single connection.
func TestScheduleFromFlows_AggregationDeduplication(t *testing.T) {
	s, collector, _ := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": 100,
	})

	// Same destination, two different exporters — should aggregate to one connection.
	flows := []*common.Flow{
		{
			Namespace: "ns1", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{8, 8, 8, 8},
			SrcPort: 54321, DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
		{
			Namespace: "ns2", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 2}, DstAddr: []byte{8, 8, 8, 8},
			SrcPort: 12345, DstPort: 443, ExporterAddr: []byte{192, 0, 2, 2},
		},
	}

	s.ScheduleFromFlows(flows)

	require.Equal(t, 1, collector.callCount())
	require.Equal(t, 1, collector.totalConns(), "two flows to same dst should aggregate to one connection")

	conn := collector.calls[0][0]
	assert.Equal(t, netip.MustParseAddr("8.8.8.8"), conn.DestIP)
	// Both namespaces should be present.
	assert.ElementsMatch(t, []string{"ns1", "ns2"}, conn.Namespaces)
	// Both exporter addresses should be present.
	assert.ElementsMatch(t, []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
	}, conn.ExporterAddrs)
}

// TestScheduleFromFlows_MultipleDestExcludes verifies that multiple excluded CIDRs
// all apply correctly.
func TestScheduleFromFlows_MultipleDestExcludes(t *testing.T) {
	s, collector, _ := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": 100,
		"network_path.netflow_monitoring.dest_excludes": []string{
			"8.8.0.0/16",
			"9.9.0.0/16",
		},
	})

	flows := []*common.Flow{
		// Excluded by first CIDR.
		{
			Namespace: "default", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{8, 8, 8, 8},
			DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
		// Excluded by second CIDR.
		{
			Namespace: "default", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{9, 9, 9, 9},
			DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
		// Not excluded — 1.1.1.1 is not in either range.
		{
			Namespace: "default", EtherType: etherTypeIPv4, IPProtocol: ipProtoTCP,
			SrcAddr: []byte{10, 0, 0, 1}, DstAddr: []byte{1, 1, 1, 1},
			DstPort: 443, ExporterAddr: []byte{192, 0, 2, 1},
		},
	}

	s.ScheduleFromFlows(flows)

	require.Equal(t, 1, collector.callCount())
	assert.Equal(t, 1, collector.totalConns(), "only non-excluded destination should reach npcollector")
	assert.Equal(t, netip.MustParseAddr("1.1.1.1"), collector.calls[0][0].DestIP)
}

// TestScheduleFromFlows_UniqueFlowsPerDstOctet is a regression test ensuring
// that generating 100 unique destinations each hits its own slot.
func TestScheduleFromFlows_UniqueFlowsPerDstOctet(t *testing.T) {
	const count = 20
	s, collector, _ := newTestScheduler(t, map[string]any{
		"network_path.netflow_monitoring.enabled":                    true,
		"network_path.netflow_monitoring.max_destinations_per_flush": count + 10,
	})

	flows := make([]*common.Flow, count)
	for i := 0; i < count; i++ {
		flows[i] = &common.Flow{
			Namespace:    "default",
			EtherType:    etherTypeIPv4,
			IPProtocol:   ipProtoTCP,
			SrcAddr:      []byte{10, 0, 0, 1},
			DstAddr:      []byte{8, 8, 8, byte(i + 1)},
			DstPort:      int32(i + 1), // distinct ports
			ExporterAddr: []byte{192, 0, 2, 1},
		}
	}

	s.ScheduleFromFlows(flows)

	assert.Equal(t, count, collector.totalConns())
}
