// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"net"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// imdsReassemblerMaxFlows bounds the number of concurrently tracked IMDS flows.
	imdsReassemblerMaxFlows = 1024
	// imdsReassemblerFlowTTL is how long an incomplete flow buffer is kept before it is
	// considered stale and reset on the next segment for that flow.
	imdsReassemblerFlowTTL = 5 * time.Second
	// imdsReassemblerMaxBytes bounds the bytes buffered for a single in-flight message.
	imdsReassemblerMaxBytes = 64 * 1024
)

// imdsFlowKey identifies a unidirectional TCP flow (source -> destination). Requests and
// responses therefore map to distinct keys (the 4-tuple is swapped), so a key's buffer only
// ever carries a single HTTP message stream.
type imdsFlowKey struct {
	srcIP   [16]byte
	dstIP   [16]byte
	srcPort uint16
	dstPort uint16
	netns   uint32
}

// imdsSegment is a single captured TCP segment payload.
type imdsSegment struct {
	seq  uint32
	data []byte
}

// imdsFlowBuffer accumulates the ordered payload of one HTTP message on a flow.
type imdsFlowBuffer struct {
	segments   []imdsSegment
	size       int
	lastUpdate time.Time
}

// imdsReassembler reassembles IMDS HTTP messages (mostly credential responses) that span
// multiple TCP segments. eBPF delivers one event per captured IMDS packet together with the
// segment's TCP sequence number; this buffers those payloads per flow, orders/dedupes them by
// sequence number, and yields the full byte stream once a complete HTTP message is present.
type imdsReassembler struct {
	sync.Mutex
	flows  *lru.Cache[imdsFlowKey, *imdsFlowBuffer]
	ttl    time.Duration
	maxLen int

	// incompleteSegments counts segments received that did not complete an HTTP message and were
	// therefore ignored while waiting for more segments. It is flushed in SendStats.
	incompleteSegments *atomic.Uint64
}

// newIMDSReassembler returns a ready-to-use reassembler.
func newIMDSReassembler() (*imdsReassembler, error) {
	flows, err := lru.New[imdsFlowKey, *imdsFlowBuffer](imdsReassemblerMaxFlows)
	if err != nil {
		return nil, err
	}
	return &imdsReassembler{
		flows:              flows,
		ttl:                imdsReassemblerFlowTTL,
		maxLen:             imdsReassemblerMaxBytes,
		incompleteSegments: atomic.NewUint64(0),
	}, nil
}

// onIncompleteSegment records that an IMDS segment was received that did not complete a message.
func (r *imdsReassembler) onIncompleteSegment() {
	r.incompleteSegments.Inc()
}

// SendStats sends the IMDS reassembler metrics: the size of the flow cache, the distribution of
// buffered segments (sequences) per flow, and the number of incomplete segments received since the
// last flush.
func (r *imdsReassembler) SendStats(statsdClient statsd.ClientInterface) error {
	// snapshot the per-flow segment counts under the lock, then emit outside of it
	r.Lock()
	cacheSize := r.flows.Len()
	seqsPerEntry := make([]int, 0, cacheSize)
	for _, buf := range r.flows.Values() {
		seqsPerEntry = append(seqsPerEntry, len(buf.segments))
	}
	r.Unlock()

	if err := statsdClient.Gauge(metrics.MetricIMDSReassemblerCacheSize, float64(cacheSize), nil, 1.0); err != nil {
		return err
	}

	for _, count := range seqsPerEntry {
		if err := statsdClient.Histogram(metrics.MetricIMDSReassemblerSequencesPerEntry, float64(count), nil, 1.0); err != nil {
			return err
		}
	}

	if incomplete := r.incompleteSegments.Swap(0); incomplete > 0 {
		if err := statsdClient.Count(metrics.MetricIMDSReassemblerIncompleteSegment, int64(incomplete), nil, 1.0); err != nil {
			return err
		}
	}

	return nil
}

// flowKeyFromNetworkContext builds the unidirectional flow key from a network context.
func flowKeyFromNetworkContext(nc *model.NetworkContext) imdsFlowKey {
	key := imdsFlowKey{
		srcPort: nc.Source.Port,
		dstPort: nc.Destination.Port,
		netns:   nc.Device.NetNS,
	}
	copyIP(key.srcIP[:], nc.Source.IPNet.IP)
	copyIP(key.dstIP[:], nc.Destination.IPNet.IP)
	return key
}

func copyIP(dst []byte, ip net.IP) {
	if v4 := ip.To16(); v4 != nil {
		copy(dst, v4)
	}
}

// process feeds one captured IMDS segment into the reassembler. It returns the full HTTP
// message bytes and true once a complete message has been assembled for the flow; otherwise it
// returns (nil, false) and keeps buffering. The payload is copied, so the caller may reuse it.
func (r *imdsReassembler) process(key imdsFlowKey, seq uint32, payload []byte) (full []byte, complete bool) {
	if len(payload) == 0 {
		return nil, false
	}

	r.Lock()
	defer r.Unlock()

	// a non-empty segment that doesn't complete a message has been buffered: record it
	defer func() {
		if !complete {
			r.onIncompleteSegment()
		}
	}()

	now := time.Now()

	buf, ok := r.flows.Get(key)
	if !ok || now.Sub(buf.lastUpdate) > r.ttl {
		// new flow, or the previous (incomplete) message went stale: start fresh
		buf = &imdsFlowBuffer{}
		r.flows.Add(key, buf)
	}
	buf.lastUpdate = now

	// store a copy of the payload, deduping exact retransmits of the same sequence number
	if !buf.hasSeq(seq) {
		data := make([]byte, len(payload))
		copy(data, payload)
		buf.segments = append(buf.segments, imdsSegment{seq: seq, data: data})
		buf.size += len(data)
	}

	// give up on an oversized in-flight message to bound memory
	if buf.size > r.maxLen {
		r.flows.Remove(key)
		return nil, false
	}

	full = buf.contiguous()
	if full == nil {
		return nil, false
	}

	if !model.IMDSMessageComplete(full) {
		return nil, false
	}

	// a full message has been assembled: reset the flow so the next segments start a new
	// message (handles keep-alive connections that reuse the same flow)
	r.flows.Remove(key)
	return full, true
}

func (b *imdsFlowBuffer) hasSeq(seq uint32) bool {
	for i := range b.segments {
		if b.segments[i].seq == seq {
			return true
		}
	}
	return false
}

// contiguous assembles the gap-free byte run that begins at the message-start segment. It
// returns nil if no start segment has been seen yet (e.g. body segments arrived before the
// headers). Segments are ordered by their sequence offset relative to the base using wrapping
// 32-bit arithmetic, and overlaps from retransmits are trimmed.
func (b *imdsFlowBuffer) contiguous() []byte {
	if len(b.segments) == 0 {
		return nil
	}

	// anchor the base sequence number on the segment that begins an HTTP message, so that
	// out-of-order body segments don't mis-anchor reassembly
	baseSeq := uint32(0)
	found := false
	for i := range b.segments {
		if model.IMDSLooksLikeMessageStart(b.segments[i].data) {
			baseSeq = b.segments[i].seq
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	ordered := make([]imdsSegment, len(b.segments))
	copy(ordered, b.segments)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].seq-baseSeq < ordered[j].seq-baseSeq
	})

	var out []byte
	var expected uint32 // offset relative to baseSeq
	for _, seg := range ordered {
		off := seg.seq - baseSeq
		switch {
		case off == expected:
			out = append(out, seg.data...)
			expected += uint32(len(seg.data))
		case off < expected:
			// overlapping retransmit: append only the bytes past what we already have
			skip := expected - off
			if skip < uint32(len(seg.data)) {
				out = append(out, seg.data[skip:]...)
				expected += uint32(len(seg.data)) - skip
			}
		default:
			// gap: wait for the missing segment
			return out
		}
	}
	return out
}
