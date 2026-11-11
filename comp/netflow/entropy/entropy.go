// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package entropy computes per-interface Shannon entropy of NetFlow traffic across peers,
// in windows by the time the Agent receives each record, before records are merged.
// Exporter timestamps aren't used: exporter clocks can be minutes off.
package entropy

import (
	"math"
	"net/netip"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// MetricVersion changes when the meaning of the values changes, not when thresholds do.
const MetricVersion = 1

// ObservationPoint is the interface side a distribution describes.
type ObservationPoint string

const (
	// Ingress is keyed by source IP on the input interface.
	Ingress ObservationPoint = "ingress"
	// Egress is keyed by destination IP on the output interface.
	Egress ObservationPoint = "egress"
)

// QualityStatus is whether a window's values can be used.
type QualityStatus string

const (
	// StatusValid passed every gate.
	StatusValid QualityStatus = "valid"
	// StatusInsufficientData is below MinRecords or MinBytes.
	StatusInsufficientData QualityStatus = "insufficient_data"
	// StatusStateLimit hit a state limit.
	StatusStateLimit QualityStatus = "state_limit"
	// StatusProcessRestart started before the accumulator, so it's partial.
	StatusProcessRestart QualityStatus = "process_restart"
)

// DropReason is why one side of a record wasn't counted.
type DropReason string

const (
	// DropNoInputInterface is an ingress side whose record has no input interface.
	DropNoInputInterface DropReason = "no_input_interface"
	// DropNoOutputInterface is an egress side whose record has no output interface.
	DropNoOutputInterface DropReason = "no_output_interface"
	// DropInvalidIP is a missing or unparseable exporter or peer IP.
	DropInvalidIP DropReason = "invalid_ip"
	// DropLate is for an already closed window, when the Agent's clock goes back.
	DropLate DropReason = "late"
	// DropStateLimit is for a new exporter or interface past its limit.
	DropStateLimit DropReason = "state_limit"
)

// DropKey is the exporter a drop came from (Exporter is zero when its IP was invalid) and why.
type DropKey struct {
	Namespace string
	Exporter  netip.Addr
	Reason    DropReason
}

// Config holds window and limit settings.
type Config struct {
	Interval time.Duration

	MinRecords uint64
	MinBytes   uint64

	MaxExporters             int
	MaxInterfacesPerExporter int
	MaxPeersPerInterface     int
	// MaxTotalEntries is across all open windows.
	MaxTotalEntries int
}

type exporterKey struct {
	namespace string
	exporter  netip.Addr
}

type ifaceKey struct {
	exporterKey
	ifIndex uint32
	point   ObservationPoint
}

type counts struct {
	bytes   uint64
	packets uint64
	records uint64
}

type ifaceState struct {
	peers    map[netip.Addr]counts
	records  uint64
	bytes    uint64
	packets  uint64
	limitHit bool
}

type window struct {
	ifaces    map[ifaceKey]*ifaceState
	exporters map[exporterKey]int // interface count
}

// Accumulator is safe for concurrent use: Add runs on the receive loop, CloseWindows on the flush loop.
type Accumulator struct {
	cfg         Config
	intervalSec int64

	mu           sync.Mutex
	windows      map[int64]*window // by start, unix seconds
	closedBefore int64             // windows starting earlier are closed
	firstWindow  int64             // windows starting here or earlier are partial
	totalEntries int
	dropped      map[DropKey]uint64
}

// NewAccumulator treats the window containing now as partial.
func NewAccumulator(cfg Config, now time.Time) *Accumulator {
	intervalSec := max(int64(cfg.Interval/time.Second), 1)
	a := &Accumulator{
		cfg:         cfg,
		intervalSec: intervalSec,
		windows:     make(map[int64]*window),
		dropped:     make(map[DropKey]uint64),
	}
	a.firstWindow = a.windowStart(now.Unix())
	a.closedBefore = a.firstWindow
	return a
}

func (a *Accumulator) windowStart(ts int64) int64 {
	return ts - ts%a.intervalSec
}

// Add counts a record received at now towards its ingress and egress interfaces.
func (a *Accumulator) Add(flow *common.Flow, now time.Time) {
	exporter, exporterOK := netip.AddrFromSlice(flow.ExporterAddr)
	src, srcOK := netip.AddrFromSlice(flow.SrcAddr)
	dst, dstOK := netip.AddrFromSlice(flow.DstAddr)
	// IPv4-mapped addresses print as IPv4, like the aggregator's exporter_ip tags
	exporter, src, dst = exporter.Unmap(), src.Unmap(), dst.Unmap()
	start := a.windowStart(now.Unix())

	// 0 means the exporter didn't report a rate: unsampled, as in the NetFlow backend
	rate := max(flow.SamplingRate, 1)
	weight := flow.Bytes * rate
	packets := flow.Packets * rate

	a.mu.Lock()
	defer a.mu.Unlock()

	if !exporterOK {
		a.dropped[DropKey{Namespace: flow.Namespace, Reason: DropInvalidIP}] += 2
		return
	}
	if start < a.closedBefore {
		a.dropped[DropKey{flow.Namespace, exporter, DropLate}] += 2
		return
	}

	exp := exporterKey{namespace: flow.Namespace, exporter: exporter}
	sides := [2]struct {
		ifIndex  uint32
		point    ObservationPoint
		peer     netip.Addr
		ok       bool
		noIfDrop DropReason
	}{
		{flow.InputInterface, Ingress, src, srcOK, DropNoInputInterface},
		{flow.OutputInterface, Egress, dst, dstOK, DropNoOutputInterface},
	}
	for _, side := range sides {
		if side.ifIndex == 0 {
			a.dropped[DropKey{flow.Namespace, exporter, side.noIfDrop}]++
			continue
		}
		if !side.ok {
			a.dropped[DropKey{flow.Namespace, exporter, DropInvalidIP}]++
			continue
		}
		state := a.iface(start, ifaceKey{exporterKey: exp, ifIndex: side.ifIndex, point: side.point})
		if state == nil {
			a.dropped[DropKey{flow.Namespace, exporter, DropStateLimit}]++
			continue
		}
		a.addToIface(state, side.peer, weight, packets)
	}
}

// iface returns nil when a new exporter or interface is past its limit.
func (a *Accumulator) iface(start int64, key ifaceKey) *ifaceState {
	w, ok := a.windows[start]
	if !ok {
		w = &window{ifaces: make(map[ifaceKey]*ifaceState), exporters: make(map[exporterKey]int)}
		a.windows[start] = w
	}
	if state, ok := w.ifaces[key]; ok {
		return state
	}
	ifaceCount, exporterKnown := w.exporters[key.exporterKey]
	if !exporterKnown && len(w.exporters) >= a.cfg.MaxExporters {
		return nil
	}
	if ifaceCount >= a.cfg.MaxInterfacesPerExporter {
		return nil
	}
	state := &ifaceState{peers: make(map[netip.Addr]counts)}
	w.ifaces[key] = state
	w.exporters[key.exporterKey] = ifaceCount + 1
	return state
}

func (a *Accumulator) addToIface(state *ifaceState, peer netip.Addr, weight, packets uint64) {
	if state.limitHit {
		return
	}
	c, known := state.peers[peer]
	if !known && (len(state.peers) >= a.cfg.MaxPeersPerInterface || a.totalEntries >= a.cfg.MaxTotalEntries) {
		// suppress rather than keep a truncated distribution
		state.limitHit = true
		a.totalEntries -= len(state.peers)
		state.peers = nil
		return
	}
	if !known {
		a.totalEntries++
	}
	c.bytes += weight
	c.packets += packets
	c.records++
	state.peers[peer] = c
	state.bytes += weight
	state.packets += packets
	state.records++
}

// WindowResult is one closed interface window.
type WindowResult struct {
	Namespace        string
	Exporter         netip.Addr
	InterfaceIndex   uint32
	ObservationPoint ObservationPoint
	Start            time.Time
	End              time.Time
	Status           QualityStatus

	// in bits, set only when valid
	ByteEntropy   float64
	PacketEntropy float64
	RecordEntropy float64
	// ByteEntropy / log2(DistinctCount), set only when DistinctCount >= 2
	NormalizedByteEntropy    float64
	HasNormalizedByteEntropy bool

	TotalWeight   uint64
	TotalPackets  uint64
	RecordCount   uint64
	DistinctCount int
}

// Report holds closed windows, drops since the last call, and peer entries still held.
type Report struct {
	Windows      []WindowResult
	Dropped      map[DropKey]uint64
	StateEntries int
}

// CloseWindows closes windows that ended by now. It computes outside the lock.
func (a *Accumulator) CloseWindows(now time.Time) Report {
	a.mu.Lock()
	a.closedBefore = max(a.closedBefore, a.windowStart(now.Unix()))

	closed := make(map[int64]*window)
	for start, w := range a.windows {
		if start < a.closedBefore {
			closed[start] = w
			delete(a.windows, start)
			for _, state := range w.ifaces {
				a.totalEntries -= len(state.peers)
			}
		}
	}
	dropped := a.dropped
	a.dropped = make(map[DropKey]uint64)
	report := Report{Dropped: dropped, StateEntries: a.totalEntries}
	firstWindow := a.firstWindow
	a.mu.Unlock()

	for start, w := range closed {
		for key, state := range w.ifaces {
			report.Windows = append(report.Windows, a.result(start, start <= firstWindow, key, state))
		}
	}
	return report
}

func (a *Accumulator) result(start int64, partial bool, key ifaceKey, state *ifaceState) WindowResult {
	r := WindowResult{
		Namespace:        key.namespace,
		Exporter:         key.exporter,
		InterfaceIndex:   key.ifIndex,
		ObservationPoint: key.point,
		Start:            time.Unix(start, 0),
		End:              time.Unix(start+a.intervalSec, 0),
		TotalWeight:      state.bytes,
		TotalPackets:     state.packets,
		RecordCount:      state.records,
		DistinctCount:    len(state.peers),
	}
	switch {
	case state.limitHit:
		r.Status = StatusStateLimit
	case partial:
		r.Status = StatusProcessRestart
	case state.records < a.cfg.MinRecords || state.bytes < a.cfg.MinBytes:
		r.Status = StatusInsufficientData
	default:
		r.Status = StatusValid
	}
	if r.Status != StatusValid {
		return r
	}

	r.ByteEntropy, r.PacketEntropy, r.RecordEntropy = shannonEntropies(state)
	if r.DistinctCount >= 2 {
		r.NormalizedByteEntropy = r.ByteEntropy / math.Log2(float64(r.DistinctCount))
		r.HasNormalizedByteEntropy = true
	}
	return r
}

// shannonEntropies uses H = log2(T) - sum(w*log2(w))/T for bytes, packets and records in one pass.
func shannonEntropies(state *ifaceState) (bytesH, packetsH, recordsH float64) {
	var bytesSum, packetsSum, recordsSum float64
	for _, c := range state.peers {
		bytesSum += wLog2w(c.bytes)
		packetsSum += wLog2w(c.packets)
		recordsSum += wLog2w(c.records)
	}
	return entropyFrom(state.bytes, bytesSum), entropyFrom(state.packets, packetsSum), entropyFrom(state.records, recordsSum)
}

func wLog2w(w uint64) float64 {
	if w == 0 {
		return 0
	}
	f := float64(w)
	return f * math.Log2(f)
}

func entropyFrom(total uint64, sum float64) float64 {
	if total == 0 {
		return 0
	}
	t := float64(total)
	// floating point error can go slightly below 0
	return max(math.Log2(t)-sum/t, 0)
}
