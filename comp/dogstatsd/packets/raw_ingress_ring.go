// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// RawPacketMeta is metadata associated with bytes read into a raw ingress ring.
type RawPacketMeta struct {
	Source     SourceType
	ListenerID string
	Origin     string
	ProcessID  uint32
}

// RawPacket is a committed packet record read from a raw ingress ring. The
// Contents slice points directly at ring-owned storage and is valid until
// Release is called.
type RawPacket struct {
	Contents   []byte
	Origin     string
	ProcessID  uint32
	ListenerID string
	Source     SourceType

	shard *RawIngressShard
	idx   int
}

// Release marks the ring slot backing this packet as reusable.
func (p RawPacket) Release() {
	if p.shard != nil {
		p.shard.release(p.idx)
	}
}

// RawPacketReservation is a reserved, not-yet-committed ring slot. Listeners
// read directly into Buffer and then Commit the number of bytes read.
type RawPacketReservation struct {
	shard *RawIngressShard
	idx   int
	buf   []byte
}

// Buffer returns the writable fixed-size slot buffer.
func (r RawPacketReservation) Buffer() []byte {
	return r.buf
}

// Commit publishes a reserved slot to consumers.
func (r RawPacketReservation) Commit(n int, meta RawPacketMeta) {
	if r.shard == nil {
		return
	}
	r.shard.commit(r.idx, n, meta)
}

// Abort publishes an empty aborted slot so consumers can advance past a failed
// read while preserving reservation order.
func (r RawPacketReservation) Abort() {
	if r.shard == nil {
		return
	}
	r.shard.abort(r.idx)
}

// RawPacketWriter reserves ring slots for listener reads.
type RawPacketWriter interface {
	Reserve() (RawPacketReservation, bool)
	Len() int
}

type rawIngressSlot struct {
	buf     []byte
	n       int
	meta    RawPacketMeta
	ready   bool
	aborted bool
}

type rawIngressTelemetry struct {
	bytes     telemetry.Gauge
	slots     telemetry.Gauge
	packets   telemetry.Gauge
	blockedNS telemetry.Counter
	stats     telemetry.Counter
	shard     string
}

// RawIngressShard is a single-consumer, multi-producer, preallocated raw packet
// ring. Slots are fixed-size byte buffers so UDS datagram listeners can reserve
// a slot and read socket bytes directly into ring-owned storage.
type RawIngressShard struct {
	mu        sync.Mutex
	notFull   *sync.Cond
	notify    chan struct{}
	stopped   bool
	slots     []rawIngressSlot
	head      int
	tail      int
	used      int
	bytes     int64
	packets   int64
	telemetry rawIngressTelemetry
}

// NewRawIngressShard creates a preallocated raw ingress shard.
func NewRawIngressShard(slotCount int, slotSize int, telemetrycomp telemetry.Component, shard string) *RawIngressShard {
	if slotCount <= 0 {
		slotCount = 1
	}
	if slotSize <= 0 {
		slotSize = 1
	}
	shardRing := &RawIngressShard{
		notify: make(chan struct{}, 1),
		slots:  make([]rawIngressSlot, slotCount),
	}
	shardRing.notFull = sync.NewCond(&shardRing.mu)
	for i := range shardRing.slots {
		shardRing.slots[i].buf = make([]byte, slotSize)
	}
	if telemetrycomp != nil {
		shardRing.telemetry = rawIngressTelemetry{
			bytes: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "bytes",
				[]string{"shard"}, "Bytes currently retained by the experimental DogStatsD raw ingress ring"),
			slots: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "slots",
				[]string{"shard"}, "Slots currently retained by the experimental DogStatsD raw ingress ring"),
			packets: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "packets",
				[]string{"shard"}, "Packets currently retained by the experimental DogStatsD raw ingress ring"),
			blockedNS: telemetrycomp.NewCounter("dogstatsd_ingress_ring", "blocked_ns",
				[]string{"shard"}, "Nanoseconds spent blocked reserving the experimental DogStatsD raw ingress ring"),
			stats: telemetrycomp.NewCounter("dogstatsd_ingress_ring", "stats",
				[]string{"shard", "stat"}, "Experimental DogStatsD raw ingress ring counters"),
			shard: shard,
		}
	}
	return shardRing
}

// Reserve returns a writable slot, blocking when the ring is full. A false
// return means the ring was stopped.
func (s *RawIngressShard) Reserve() (RawPacketReservation, bool) {
	var blockStart time.Time
	blocked := false

	s.mu.Lock()
	for !s.stopped && s.used == len(s.slots) {
		if !blocked {
			blocked = true
			blockStart = time.Now()
		}
		s.notFull.Wait()
	}
	if s.stopped {
		s.mu.Unlock()
		return RawPacketReservation{}, false
	}
	if blocked && s.telemetry.blockedNS != nil {
		s.telemetry.blockedNS.Add(float64(time.Since(blockStart).Nanoseconds()), s.telemetry.shard)
	}
	idx := s.tail
	s.tail = (s.tail + 1) % len(s.slots)
	s.used++
	s.updateGaugesLocked()
	s.mu.Unlock()

	if s.telemetry.stats != nil {
		s.telemetry.stats.Inc(s.telemetry.shard, "reserved_slots")
	}
	return RawPacketReservation{shard: s, idx: idx, buf: s.slots[idx].buf}, true
}

// TryNext returns the next committed packet if the head slot is ready.
func (s *RawIngressShard) TryNext() (RawPacket, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.used > 0 {
		slot := &s.slots[s.head]
		if !slot.ready {
			return RawPacket{}, false
		}
		if slot.aborted {
			s.releaseHeadLocked()
			continue
		}
		packet := RawPacket{
			Contents:   slot.buf[:slot.n],
			Origin:     slot.meta.Origin,
			ProcessID:  slot.meta.ProcessID,
			ListenerID: slot.meta.ListenerID,
			Source:     slot.meta.Source,
			shard:      s,
			idx:        s.head,
		}
		return packet, true
	}
	return RawPacket{}, false
}

// Notify returns a channel signaled when new committed packets may be available.
func (s *RawIngressShard) Notify() <-chan struct{} {
	return s.notify
}

// Len returns the number of reserved or committed slots in the shard.
func (s *RawIngressShard) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}

// Stop unblocks producers waiting in Reserve.
func (s *RawIngressShard) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.notFull.Broadcast()
	s.signalNotifyLocked()
	s.mu.Unlock()
}

func (s *RawIngressShard) commit(idx int, n int, meta RawPacketMeta) {
	s.mu.Lock()
	if n < 0 {
		n = 0
	}
	slot := &s.slots[idx]
	if n > len(slot.buf) {
		n = len(slot.buf)
	}
	slot.n = n
	slot.meta = meta
	slot.ready = true
	slot.aborted = false
	s.bytes += int64(n)
	s.packets++
	s.updateGaugesLocked()
	s.signalNotifyLocked()
	s.mu.Unlock()

	if s.telemetry.stats != nil {
		s.telemetry.stats.Inc(s.telemetry.shard, "committed_packets")
		s.telemetry.stats.Add(float64(n), s.telemetry.shard, "committed_bytes")
	}
}

func (s *RawIngressShard) abort(idx int) {
	s.mu.Lock()
	slot := &s.slots[idx]
	slot.n = 0
	slot.meta = RawPacketMeta{}
	slot.ready = true
	slot.aborted = true
	s.signalNotifyLocked()
	s.mu.Unlock()

	if s.telemetry.stats != nil {
		s.telemetry.stats.Inc(s.telemetry.shard, "aborted_slots")
	}
}

func (s *RawIngressShard) release(idx int) {
	s.mu.Lock()
	if s.used == 0 || idx != s.head {
		s.mu.Unlock()
		return
	}
	s.releaseHeadLocked()
	s.mu.Unlock()
}

func (s *RawIngressShard) releaseHeadLocked() {
	slot := &s.slots[s.head]
	s.bytes -= int64(slot.n)
	if s.bytes < 0 {
		s.bytes = 0
	}
	if !slot.aborted {
		s.packets--
		if s.packets < 0 {
			s.packets = 0
		}
	}
	slot.n = 0
	slot.meta = RawPacketMeta{}
	slot.ready = false
	slot.aborted = false
	s.head = (s.head + 1) % len(s.slots)
	s.used--
	s.updateGaugesLocked()
	s.notFull.Signal()
}

func (s *RawIngressShard) signalNotifyLocked() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *RawIngressShard) updateGaugesLocked() {
	if s.telemetry.bytes != nil {
		s.telemetry.bytes.Set(float64(s.bytes), s.telemetry.shard)
	}
	if s.telemetry.slots != nil {
		s.telemetry.slots.Set(float64(s.used), s.telemetry.shard)
	}
	if s.telemetry.packets != nil {
		s.telemetry.packets.Set(float64(s.packets), s.telemetry.shard)
	}
}

// RawIngressShards is a sharded raw packet writer used by listeners. Workers
// consume their corresponding shard directly.
type RawIngressShards struct {
	shards []*RawIngressShard
	next   atomic.Uint64
}

// NewRawIngressShards creates byte-budgeted raw ingress shards.
func NewRawIngressShards(shardCount int, maxBytes int64, slotSize int, telemetrycomp telemetry.Component) *RawIngressShards {
	if shardCount <= 0 {
		shardCount = 1
	}
	if maxBytes <= 0 {
		maxBytes = int64(slotSize)
	}
	bytesPerShard := maxBytes / int64(shardCount)
	if bytesPerShard < int64(slotSize) {
		bytesPerShard = int64(slotSize)
	}
	slotsPerShard := int(bytesPerShard / int64(slotSize))
	if slotsPerShard <= 0 {
		slotsPerShard = 1
	}
	shards := make([]*RawIngressShard, shardCount)
	for i := range shards {
		shards[i] = NewRawIngressShard(slotsPerShard, slotSize, telemetrycomp, strconv.Itoa(i))
	}
	return &RawIngressShards{shards: shards}
}

// Reserve reserves a slot on the next shard.
func (s *RawIngressShards) Reserve() (RawPacketReservation, bool) {
	if len(s.shards) == 0 {
		return RawPacketReservation{}, false
	}
	idx := int(s.next.Add(1)-1) % len(s.shards)
	return s.shards[idx].Reserve()
}

// Len returns total reserved or committed slots across shards.
func (s *RawIngressShards) Len() int {
	total := 0
	for _, shard := range s.shards {
		total += shard.Len()
	}
	return total
}

// Shard returns the shard assigned to a worker.
func (s *RawIngressShards) Shard(worker int) *RawIngressShard {
	if len(s.shards) == 0 {
		return nil
	}
	return s.shards[worker%len(s.shards)]
}

// Stop unblocks all shard producers.
func (s *RawIngressShards) Stop() {
	for _, shard := range s.shards {
		shard.Stop()
	}
}
