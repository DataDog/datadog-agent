// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package entropy

import (
	"encoding/binary"
	"fmt"
	"math"
	"net/netip"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

const testInterval = 5 * time.Minute

// w is a window start: a multiple of the 5 min interval.
const w = int64(1_800_000_000)

var received = time.Unix(w+10, 0)

var exporter = netip.MustParseAddr("127.0.0.1")

func testConfig() Config {
	return Config{
		Interval:                 testInterval,
		MinRecords:               1,
		MaxExporters:             10,
		MaxInterfacesPerExporter: 10,
		MaxPeersPerInterface:     100,
		MaxTotalEntries:          1000,
	}
}

// ingressFlow is a record on input interface 1 of exporter 127.0.0.1, with no output interface,
// so only the ingress side is counted.
func ingressFlow(src byte, bytes, packets, samplingRate uint64) *common.Flow {
	return &common.Flow{
		Namespace:      "ns",
		ExporterAddr:   []byte{127, 0, 0, 1},
		SrcAddr:        []byte{10, 0, 0, src},
		DstAddr:        []byte{10, 0, 1, 1},
		InputInterface: 1,
		Bytes:          bytes,
		Packets:        packets,
		SamplingRate:   samplingRate,
	}
}

// closeWindowW receives flows during window w on an accumulator created at createdAt, and closes w.
func closeWindowW(t *testing.T, cfg Config, createdAt int64, flows []*common.Flow) Report {
	t.Helper()
	acc := NewAccumulator(cfg, time.Unix(createdAt, 0))
	for _, f := range flows {
		acc.Add(f, time.Unix(w+10, 0))
	}
	return acc.CloseWindows(time.Unix(w+300, 0))
}

func TestComputeEntropy(t *testing.T) {
	eightEqual := make([]*common.Flow, 0, 8)
	for i := range 8 {
		eightEqual = append(eightEqual, ingressFlow(byte(i+1), 100, 1, 1))
	}
	tests := []struct {
		name           string
		flows          []*common.Flow
		wantBytes      float64
		wantPackets    float64
		wantRecords    float64
		wantNormalized float64
		wantHasNorm    bool
		wantDistinct   int
	}{
		{
			name:         "one peer",
			flows:        []*common.Flow{ingressFlow(1, 500, 5, 1), ingressFlow(1, 500, 5, 1)},
			wantDistinct: 1,
		},
		{
			name:           "8 equal peers",
			flows:          eightEqual,
			wantBytes:      3,
			wantPackets:    3,
			wantRecords:    3,
			wantNormalized: 1,
			wantHasNorm:    true,
			wantDistinct:   8,
		},
		{
			// 100 bytes at 1:1 and 10 bytes at 1:10 are the same traffic once sampling is applied
			name:           "mixed sampling rates",
			flows:          []*common.Flow{ingressFlow(1, 100, 1, 1), ingressFlow(2, 10, 1, 10)},
			wantBytes:      1,
			wantPackets:    -(1.0/11*math.Log2(1.0/11) + 10.0/11*math.Log2(10.0/11)),
			wantRecords:    1,
			wantNormalized: 1,
			wantHasNorm:    true,
			wantDistinct:   2,
		},
		{
			name:           "no sampling rate counts as 1:1",
			flows:          []*common.Flow{ingressFlow(1, 100, 1, 1), ingressFlow(2, 100, 1, 0)},
			wantBytes:      1,
			wantPackets:    1,
			wantRecords:    1,
			wantNormalized: 1,
			wantHasNorm:    true,
			wantDistinct:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := closeWindowW(t, testConfig(), w-1, tt.flows)
			require.Len(t, report.Windows, 1)
			r := report.Windows[0]
			assert.Equal(t, StatusValid, r.Status)
			assert.InDelta(t, tt.wantBytes, r.ByteEntropy, 1e-9)
			assert.InDelta(t, tt.wantPackets, r.PacketEntropy, 1e-9)
			assert.InDelta(t, tt.wantRecords, r.RecordEntropy, 1e-9)
			assert.Equal(t, tt.wantHasNorm, r.HasNormalizedByteEntropy)
			assert.InDelta(t, tt.wantNormalized, r.NormalizedByteEntropy, 1e-9)
			assert.Equal(t, tt.wantDistinct, r.DistinctCount)
			assert.Equal(t, Ingress, r.ObservationPoint)
			// the missing output interface is dropped, not counted as a peer
			assert.Equal(t, uint64(len(tt.flows)), report.Dropped[DropKey{"ns", exporter, DropNoOutputInterface}])
		})
	}
}

func TestAccumulator_Windows(t *testing.T) {
	acc := NewAccumulator(testConfig(), time.Unix(w-1, 0))

	acc.Add(ingressFlow(1, 100, 1, 1), time.Unix(w+10, 0))
	assert.Empty(t, acc.CloseWindows(time.Unix(w+299, 0)).Windows, "window w is still open")

	acc.Add(ingressFlow(2, 100, 1, 1), time.Unix(w+310, 0))
	report := acc.CloseWindows(time.Unix(w+305, 0))
	require.Len(t, report.Windows, 1)
	assert.Equal(t, time.Unix(w, 0), report.Windows[0].Start)
	assert.Equal(t, time.Unix(w+300, 0), report.Windows[0].End)
	assert.Equal(t, uint64(1), report.Windows[0].RecordCount)

	// the exporter's end time is ignored, even when it's 0 or far in the future
	stale, future := ingressFlow(3, 100, 1, 1), ingressFlow(4, 100, 1, 1)
	stale.EndTimestamp, future.EndTimestamp = 0, uint64(w+100_000)
	acc.Add(stale, time.Unix(w+320, 0))
	acc.Add(future, time.Unix(w+330, 0))
	// the Agent's clock went back into the closed window w
	acc.Add(ingressFlow(5, 100, 1, 1), time.Unix(w+20, 0))

	report = acc.CloseWindows(time.Unix(w+600, 0))
	require.Len(t, report.Windows, 1)
	assert.Equal(t, time.Unix(w+300, 0), report.Windows[0].Start)
	assert.Equal(t, uint64(3), report.Windows[0].RecordCount)
	// both sides of a dropped record are counted
	assert.Equal(t, uint64(2), report.Dropped[DropKey{"ns", exporter, DropLate}])
	assert.Equal(t, 0, report.StateEntries)
}

func TestAccumulator_QualityStatus(t *testing.T) {
	twoPeers := []*common.Flow{ingressFlow(1, 100, 1, 1), ingressFlow(2, 100, 1, 1)}
	tests := []struct {
		name         string
		configure    func(*Config)
		createdAt    int64
		flows        []*common.Flow
		wantStatus   QualityStatus
		wantDistinct int
	}{
		{
			name:         "valid",
			flows:        twoPeers,
			wantStatus:   StatusValid,
			wantDistinct: 2,
		},
		{
			name:         "below min records",
			configure:    func(c *Config) { c.MinRecords = 3 },
			flows:        twoPeers,
			wantStatus:   StatusInsufficientData,
			wantDistinct: 2,
		},
		{
			name:         "below min bytes",
			configure:    func(c *Config) { c.MinBytes = 1000 },
			flows:        twoPeers,
			wantStatus:   StatusInsufficientData,
			wantDistinct: 2,
		},
		{
			name:       "peer limit suppresses instead of truncating",
			configure:  func(c *Config) { c.MaxPeersPerInterface = 1 },
			flows:      twoPeers,
			wantStatus: StatusStateLimit,
		},
		{
			name:       "total entries limit",
			configure:  func(c *Config) { c.MaxTotalEntries = 1 },
			flows:      twoPeers,
			wantStatus: StatusStateLimit,
		},
		{
			name:         "accumulator started mid-window",
			createdAt:    w + 5,
			flows:        twoPeers,
			wantStatus:   StatusProcessRestart,
			wantDistinct: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig()
			if tt.configure != nil {
				tt.configure(&cfg)
			}
			createdAt := w - 1
			if tt.createdAt != 0 {
				createdAt = tt.createdAt
			}
			report := closeWindowW(t, cfg, createdAt, tt.flows)
			require.Len(t, report.Windows, 1)
			r := report.Windows[0]
			assert.Equal(t, tt.wantStatus, r.Status)
			assert.Equal(t, tt.wantDistinct, r.DistinctCount)
			if tt.wantStatus != StatusValid {
				assert.Zero(t, r.ByteEntropy)
				assert.False(t, r.HasNormalizedByteEntropy)
			}
		})
	}

	t.Run("exporter limit drops records for new exporters", func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxExporters = 1
		other := ingressFlow(1, 100, 1, 1)
		other.ExporterAddr = []byte{127, 0, 0, 2}
		report := closeWindowW(t, cfg, w-1, []*common.Flow{ingressFlow(1, 100, 1, 1), other})
		require.Len(t, report.Windows, 1)
		assert.Equal(t, uint64(1), report.Dropped[DropKey{"ns", netip.MustParseAddr("127.0.0.2"), DropStateLimit}])
	})

	t.Run("peer limit on one interface leaves its neighbor valid", func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxPeersPerInterface = 1
		neighbor := ingressFlow(3, 100, 1, 1)
		neighbor.InputInterface = 2
		report := closeWindowW(t, cfg, w-1, []*common.Flow{ingressFlow(1, 100, 1, 1), ingressFlow(2, 100, 1, 1), neighbor})
		statuses := make(map[uint32]QualityStatus)
		for _, r := range report.Windows {
			statuses[r.InterfaceIndex] = r.Status
		}
		assert.Equal(t, map[uint32]QualityStatus{1: StatusStateLimit, 2: StatusValid}, statuses)
	})
}

func TestAccumulator_DropReasons(t *testing.T) {
	noInterfaces := ingressFlow(1, 100, 1, 1)
	noInterfaces.InputInterface = 0
	badPeer := ingressFlow(2, 100, 1, 1)
	badPeer.SrcAddr = nil
	badExporter := ingressFlow(3, 100, 1, 1)
	badExporter.ExporterAddr = nil

	report := closeWindowW(t, testConfig(), w-1, []*common.Flow{noInterfaces, badPeer, badExporter})
	assert.Equal(t, map[DropKey]uint64{
		{"ns", exporter, DropNoInputInterface}:  1,
		{"ns", exporter, DropNoOutputInterface}: 2,
		{"ns", exporter, DropInvalidIP}:         1,
		{"ns", netip.Addr{}, DropInvalidIP}:     2,
	}, report.Dropped)

	sender := mocksender.NewMockSender(t, "")
	sender.SetupAcceptAll()
	report.Submit(sender, logmock.New(t))
	sender.AssertMetric(t, "Count", "datadog.netflow.entropy.records_dropped", 2, "",
		[]string{"reason:no_output_interface", "device_namespace:ns", "exporter_ip:127.0.0.1"})
	sender.AssertMetric(t, "Count", "datadog.netflow.entropy.records_dropped", 2, "",
		[]string{"reason:invalid_ip", "device_namespace:ns"})
}

// benchFlows returns n records with distinct source and destination IPs spread over 100 interfaces,
// so each record is its own flow context and adds a peer on both sides.
func benchFlows(n int) []*common.Flow {
	flows := make([]*common.Flow, n)
	for i := range n {
		src := make([]byte, 4)
		dst := make([]byte, 4)
		binary.BigEndian.PutUint32(src, uint32(0x0a000000+i))
		binary.BigEndian.PutUint32(dst, uint32(0x0b000000+i))
		flows[i] = &common.Flow{
			Namespace:       "ns",
			ExporterAddr:    []byte{127, 0, 0, 1},
			SrcAddr:         src,
			DstAddr:         dst,
			InputInterface:  uint32(i%100 + 1),
			OutputInterface: uint32((i+50)%100 + 1),
			Bytes:           uint64(100 + i%1000),
			Packets:         1,
			SamplingRate:    1,
		}
	}
	return flows
}

func benchConfig() Config {
	cfg := testConfig()
	cfg.MaxPeersPerInterface = 50_000
	cfg.MaxTotalEntries = 2_000_000
	cfg.MaxInterfacesPerExporter = 2_000
	return cfg
}

// BenchmarkAccumulator_Add measures one record on the receive path, with the window already holding n flow contexts.
func BenchmarkAccumulator_Add(b *testing.B) {
	for _, n := range []int{100_000, 500_000} {
		b.Run(fmt.Sprintf("contexts=%d", n), func(b *testing.B) {
			flows := benchFlows(n)
			acc := NewAccumulator(benchConfig(), time.Unix(w-1, 0))
			for _, f := range flows {
				acc.Add(f, received)
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				acc.Add(flows[i%n], received)
				i++
			}
		})
	}
}

// BenchmarkAccumulator_Window measures filling and closing one window of n flow contexts,
// and reports the heap held by the open window.
func BenchmarkAccumulator_Window(b *testing.B) {
	for _, n := range []int{100_000, 500_000} {
		b.Run(fmt.Sprintf("contexts=%d", n), func(b *testing.B) {
			flows := benchFlows(n)
			b.ReportAllocs()
			var heapPerWindow uint64
			for b.Loop() {
				acc := NewAccumulator(benchConfig(), time.Unix(w-1, 0))
				var before, after runtime.MemStats
				if heapPerWindow == 0 {
					runtime.GC()
					runtime.ReadMemStats(&before)
				}
				for _, f := range flows {
					acc.Add(f, received)
				}
				if heapPerWindow == 0 {
					runtime.GC()
					runtime.ReadMemStats(&after)
					heapPerWindow = after.HeapAlloc - before.HeapAlloc
				}
				acc.CloseWindows(time.Unix(w+300, 0))
			}
			b.ReportMetric(float64(heapPerWindow)/(1<<20), "MiB/window")
			b.ReportMetric(float64(heapPerWindow)/float64(2*n), "B/peer_entry")
		})
	}
}
