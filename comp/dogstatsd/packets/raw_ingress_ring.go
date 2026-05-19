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

	shard        *RawIngressShard
	compactShard *CompactRawIngressShard
	idx          int
}

// Release marks the ring storage backing this packet as reusable.
func (p RawPacket) Release() {
	if p.shard != nil {
		p.shard.release(p.idx)
		return
	}
	if p.compactShard != nil {
		p.compactShard.release(p.idx)
	}
}

// RawPacketReservation is a reserved, not-yet-committed ring slot. Listeners
// read directly into Buffer and then Commit the number of bytes read.
type RawPacketReservation struct {
	shard        *RawIngressShard
	compactShard *CompactRawIngressShard
	idx          int
	buf          []byte
}

// Buffer returns the writable fixed-size slot buffer.
func (r RawPacketReservation) Buffer() []byte {
	return r.buf
}

// Commit publishes a reserved slot to consumers.
func (r RawPacketReservation) Commit(n int, meta RawPacketMeta) {
	if r.shard != nil {
		r.shard.commit(r.idx, n, meta)
		return
	}
	if r.compactShard != nil {
		r.compactShard.commit(r.buf, n, meta)
	}
}

// Abort publishes an empty aborted slot so consumers can advance past a failed
// read while preserving reservation order.
func (r RawPacketReservation) Abort() {
	if r.shard != nil {
		r.shard.abort(r.idx)
		return
	}
	if r.compactShard != nil {
		r.compactShard.abort(r.buf)
	}
}

// RawPacketWriter reserves writable buffers for listener reads.
type RawPacketWriter interface {
	Reserve() (RawPacketReservation, bool)
	Len() int
}

// RawIngressReader is the worker-side cursor over a raw ingress shard.
type RawIngressReader interface {
	TryNext() (RawPacket, bool)
	Notify() <-chan struct{}
}

// RawIngressBatchReader is an optional worker-side cursor that can peek and
// release several raw packets with one shard lock acquisition. Batches are
// always returned from the shard head and must be released in order with
// ReleaseBatch after processing.
type RawIngressBatchReader interface {
	RawIngressReader
	TryNextBatch(dst []RawPacket) []RawPacket
	ReleaseBatch(n int)
}

type rawIngressSlot struct {
	buf         []byte
	n           int
	meta        RawPacketMeta
	committedAt int64
	ready       bool
	aborted     bool
}

type rawIngressTelemetry struct {
	bytes              telemetry.Gauge
	slots              telemetry.Gauge
	packets            telemetry.Gauge
	consumerLagRecords telemetry.Gauge
	consumerLagBytes   telemetry.Gauge
	oldestAgeNS        telemetry.Gauge
	oldestTimestampNS  telemetry.Gauge
	blockedNS          telemetry.Counter
	stats              telemetry.Counter
	shard              string
}

func newRawIngressTelemetry(telemetrycomp telemetry.Component) rawIngressTelemetry {
	if telemetrycomp == nil {
		return rawIngressTelemetry{}
	}
	return rawIngressTelemetry{
		bytes: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "bytes",
			[]string{"shard"}, "Bytes currently retained by the experimental DogStatsD raw ingress ring"),
		slots: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "slots",
			[]string{"shard"}, "Slots currently retained by the experimental DogStatsD raw ingress ring"),
		packets: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "packets",
			[]string{"shard"}, "Packets currently retained by the experimental DogStatsD raw ingress ring"),
		consumerLagRecords: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "consumer_lag_records",
			[]string{"shard"}, "Committed records currently waiting for the experimental DogStatsD raw ingress ring consumer"),
		consumerLagBytes: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "consumer_lag_bytes",
			[]string{"shard"}, "Committed bytes currently waiting for the experimental DogStatsD raw ingress ring consumer"),
		oldestAgeNS: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "oldest_record_age_ns",
			[]string{"shard"}, "Age in nanoseconds of the oldest committed record retained by the experimental DogStatsD raw ingress ring"),
		oldestTimestampNS: telemetrycomp.NewGauge("dogstatsd_ingress_ring", "oldest_record_timestamp_ns",
			[]string{"shard"}, "Unix timestamp in nanoseconds of the oldest committed record retained by the experimental DogStatsD raw ingress ring"),
		blockedNS: telemetrycomp.NewCounter("dogstatsd_ingress_ring", "blocked_ns",
			[]string{"shard"}, "Nanoseconds spent blocked reserving or appending to the experimental DogStatsD raw ingress ring"),
		stats: telemetrycomp.NewCounter("dogstatsd_ingress_ring", "stats",
			[]string{"shard", "stat"}, "Experimental DogStatsD raw ingress ring counters"),
	}
}

func (t rawIngressTelemetry) forShard(shard string) rawIngressTelemetry {
	t.shard = shard
	return t
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
	return newRawIngressShardWithTelemetry(slotCount, slotSize, newRawIngressTelemetry(telemetrycomp).forShard(shard))
}

func newRawIngressShardWithTelemetry(slotCount int, slotSize int, telemetry rawIngressTelemetry) *RawIngressShard {
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
	shardRing.telemetry = telemetry
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
		if blocked {
			s.telemetry.stats.Inc(s.telemetry.shard, "blocked_reservations")
			s.telemetry.stats.Inc(s.telemetry.shard, "backpressure_events")
		}
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
		s.updateOldestRecordGaugesLocked()
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

// TryNextBatch returns up to cap(dst) committed packets from the head of the
// shard. Returned packets remain owned by the ring and must be released with
// ReleaseBatch in the same order after processing.
func (s *RawIngressShard) TryNextBatch(dst []RawPacket) []RawPacket {
	dst = dst[:0]
	if cap(dst) == 0 {
		return dst
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for s.used > 0 && len(dst) < cap(dst) {
		slot := &s.slots[s.head]
		if !slot.ready {
			return dst
		}
		if !slot.aborted {
			break
		}
		s.releaseHeadLocked()
	}

	if s.used > 0 {
		s.updateOldestRecordGaugesLocked()
	}
	idx := s.head
	for scanned := 0; scanned < s.used && len(dst) < cap(dst); scanned++ {
		slot := &s.slots[idx]
		if !slot.ready || slot.aborted {
			break
		}
		dst = append(dst, RawPacket{
			Contents:   slot.buf[:slot.n],
			Origin:     slot.meta.Origin,
			ProcessID:  slot.meta.ProcessID,
			ListenerID: slot.meta.ListenerID,
			Source:     slot.meta.Source,
			shard:      s,
			idx:        idx,
		})
		idx = (idx + 1) % len(s.slots)
	}
	return dst
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
	slot.committedAt = time.Now().UnixNano()
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
	slot.committedAt = 0
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

// ReleaseBatch releases n packets from the shard head after a successful
// TryNextBatch call.
func (s *RawIngressShard) ReleaseBatch(n int) {
	if n <= 0 {
		return
	}
	s.mu.Lock()
	for i := 0; i < n && s.used > 0; i++ {
		slot := &s.slots[s.head]
		if !slot.ready {
			break
		}
		s.releaseHeadLocked()
	}
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
	slot.committedAt = 0
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
	if s.telemetry.consumerLagRecords != nil {
		s.telemetry.consumerLagRecords.Set(float64(s.packets), s.telemetry.shard)
	}
	if s.telemetry.consumerLagBytes != nil {
		s.telemetry.consumerLagBytes.Set(float64(s.bytes), s.telemetry.shard)
	}
	s.updateOldestRecordGaugesLocked()
}

func (s *RawIngressShard) updateOldestRecordGaugesLocked() {
	if s.telemetry.oldestAgeNS == nil && s.telemetry.oldestTimestampNS == nil {
		return
	}
	oldest := s.oldestCommittedTimestampLocked()
	if oldest == 0 {
		if s.telemetry.oldestAgeNS != nil {
			s.telemetry.oldestAgeNS.Set(0, s.telemetry.shard)
		}
		if s.telemetry.oldestTimestampNS != nil {
			s.telemetry.oldestTimestampNS.Set(0, s.telemetry.shard)
		}
		return
	}
	if s.telemetry.oldestTimestampNS != nil {
		s.telemetry.oldestTimestampNS.Set(float64(oldest), s.telemetry.shard)
	}
	if s.telemetry.oldestAgeNS != nil {
		age := time.Now().UnixNano() - oldest
		if age < 0 {
			age = 0
		}
		s.telemetry.oldestAgeNS.Set(float64(age), s.telemetry.shard)
	}
}

func (s *RawIngressShard) oldestCommittedTimestampLocked() int64 {
	var oldest int64
	for i := range s.slots {
		slot := &s.slots[i]
		if !slot.ready || slot.aborted || slot.committedAt == 0 {
			continue
		}
		if oldest == 0 || slot.committedAt < oldest {
			oldest = slot.committedAt
		}
	}
	return oldest
}

type compactRawIngressRecord struct {
	offset      int
	n           int
	padding     int
	meta        RawPacketMeta
	committedAt int64
}

// CompactRawIngressShard is a single-consumer, multi-producer, byte-compact raw
// packet ring. Listeners read into a reusable scratch buffer and commit copies
// only the bytes actually read into this shard's preallocated byte ring. This
// trades one copy for a denser ring than fixed dogstatsd_buffer_size slots.
type CompactRawIngressShard struct {
	mu      sync.Mutex
	notFull *sync.Cond
	notify  chan struct{}
	stopped bool

	buf     []byte
	records []compactRawIngressRecord

	headRecord  int
	tailRecord  int
	usedRecords int
	head        int
	tail        int
	usedBytes   int64
	packets     int64

	directReserve        bool
	inFlight             bool
	inFlightOffset       int
	inFlightPadding      int
	inFlightReserved     int
	inFlightPreviousTail int

	slotSize    int
	scratchPool sync.Pool
	telemetry   rawIngressTelemetry
}

// NewCompactRawIngressShard creates a preallocated byte-compact raw ingress
// shard. maxBytes bounds retained ring bytes; slotSize is the maximum UDS
// datagram read buffer size.
func NewCompactRawIngressShard(maxBytes int64, slotSize int, telemetrycomp telemetry.Component, shard string) *CompactRawIngressShard {
	return newCompactRawIngressShardWithTelemetry(maxBytes, slotSize, false, newRawIngressTelemetry(telemetrycomp).forShard(shard))
}

// NewDirectCompactRawIngressShard creates a byte-compact raw ingress shard that
// reserves ring-owned storage directly for listener reads. This avoids the
// scratch-to-ring copy but allows only one outstanding reservation per shard.
func NewDirectCompactRawIngressShard(maxBytes int64, slotSize int, telemetrycomp telemetry.Component, shard string) *CompactRawIngressShard {
	return newCompactRawIngressShardWithTelemetry(maxBytes, slotSize, true, newRawIngressTelemetry(telemetrycomp).forShard(shard))
}

func newCompactRawIngressShardWithTelemetry(maxBytes int64, slotSize int, directReserve bool, telemetry rawIngressTelemetry) *CompactRawIngressShard {
	if slotSize <= 0 {
		slotSize = 1
	}
	if maxBytes < int64(slotSize) {
		maxBytes = int64(slotSize)
	}
	recordCount := int(maxBytes / 512)
	if recordCount < 4 {
		recordCount = 4
	}
	shardRing := &CompactRawIngressShard{
		notify:        make(chan struct{}, 1),
		buf:           make([]byte, int(maxBytes)),
		records:       make([]compactRawIngressRecord, recordCount),
		directReserve: directReserve,
		slotSize:      slotSize,
		telemetry:     telemetry,
	}
	shardRing.notFull = sync.NewCond(&shardRing.mu)
	shardRing.scratchPool.New = func() any {
		return make([]byte, slotSize)
	}
	return shardRing
}

// Reserve returns a reusable scratch buffer for a listener read. The commit path
// appends the actual bytes read into the compact byte ring and blocks there when
// the byte/record budget is full. In direct-reserve mode, Reserve instead blocks
// before the socket read and returns ring-owned storage directly.
func (s *CompactRawIngressShard) Reserve() (RawPacketReservation, bool) {
	if s.directReserve {
		return s.reserveDirect()
	}

	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return RawPacketReservation{}, false
	}
	buf := s.scratchPool.Get().([]byte)
	if cap(buf) < s.slotSize {
		buf = make([]byte, s.slotSize)
	}
	return RawPacketReservation{compactShard: s, buf: buf[:s.slotSize]}, true
}

func (s *CompactRawIngressShard) reserveDirect() (RawPacketReservation, bool) {
	var blockStart time.Time
	blocked := false

	s.mu.Lock()
	for !s.stopped && (s.inFlight || !s.canAppendLocked(s.slotSize)) {
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

	s.inFlightPreviousTail = s.tail
	offset, padding := s.reserveBytesLocked(s.slotSize)
	s.inFlight = true
	s.inFlightOffset = offset
	s.inFlightPadding = padding
	s.inFlightReserved = s.slotSize
	s.usedBytes += int64(padding + s.slotSize)
	s.updateGaugesLocked()
	s.mu.Unlock()

	if s.telemetry.stats != nil {
		if blocked {
			s.telemetry.stats.Inc(s.telemetry.shard, "blocked_reservations")
			s.telemetry.stats.Inc(s.telemetry.shard, "backpressure_events")
		}
		s.telemetry.stats.Inc(s.telemetry.shard, "reserved_direct_slots")
	}
	return RawPacketReservation{compactShard: s, buf: s.buf[offset : offset+s.slotSize]}, true
}

// TryNext returns the next committed packet if one is available.
func (s *CompactRawIngressShard) TryNext() (RawPacket, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.usedRecords == 0 {
		return RawPacket{}, false
	}
	s.updateOldestRecordGaugesLocked()
	record := &s.records[s.headRecord]
	packet := RawPacket{
		Contents:     s.buf[record.offset : record.offset+record.n],
		Origin:       record.meta.Origin,
		ProcessID:    record.meta.ProcessID,
		ListenerID:   record.meta.ListenerID,
		Source:       record.meta.Source,
		compactShard: s,
		idx:          s.headRecord,
	}
	return packet, true
}

// TryNextBatch returns up to cap(dst) committed packets from the compact shard
// head. Returned packet contents point into the ring byte buffer and remain
// valid until ReleaseBatch is called.
func (s *CompactRawIngressShard) TryNextBatch(dst []RawPacket) []RawPacket {
	dst = dst[:0]
	if cap(dst) == 0 {
		return dst
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.usedRecords > 0 {
		s.updateOldestRecordGaugesLocked()
	}
	idx := s.headRecord
	for scanned := 0; scanned < s.usedRecords && len(dst) < cap(dst); scanned++ {
		record := &s.records[idx]
		dst = append(dst, RawPacket{
			Contents:     s.buf[record.offset : record.offset+record.n],
			Origin:       record.meta.Origin,
			ProcessID:    record.meta.ProcessID,
			ListenerID:   record.meta.ListenerID,
			Source:       record.meta.Source,
			compactShard: s,
			idx:          idx,
		})
		idx = (idx + 1) % len(s.records)
	}
	return dst
}

// Notify returns a channel signaled when committed packets may be available.
func (s *CompactRawIngressShard) Notify() <-chan struct{} {
	return s.notify
}

// Len returns the number of retained records in the compact shard.
func (s *CompactRawIngressShard) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usedRecords
}

// Stop unblocks producers waiting to append.
func (s *CompactRawIngressShard) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.notFull.Broadcast()
	s.signalNotifyLocked()
	s.mu.Unlock()
}

func (s *CompactRawIngressShard) commit(buf []byte, n int, meta RawPacketMeta) {
	if s.directReserve {
		s.commitDirect(n, meta)
		return
	}

	if n < 0 {
		n = 0
	}
	if n > len(buf) {
		n = len(buf)
	}
	_ = s.append(buf[:n], meta)
	s.putScratch(buf)
}

func (s *CompactRawIngressShard) abort(buf []byte) {
	if s.directReserve {
		s.abortDirect()
		return
	}

	s.putScratch(buf)
	if s.telemetry.stats != nil {
		s.telemetry.stats.Inc(s.telemetry.shard, "aborted_scratch")
	}
}

func (s *CompactRawIngressShard) commitDirect(n int, meta RawPacketMeta) {
	if n < 0 {
		n = 0
	}

	s.mu.Lock()
	if !s.inFlight {
		s.mu.Unlock()
		return
	}
	if n > s.inFlightReserved {
		n = s.inFlightReserved
	}
	offset := s.inFlightOffset
	padding := s.inFlightPadding
	reserved := s.inFlightReserved
	unused := reserved - n
	if unused > 0 {
		s.usedBytes -= int64(unused)
		if s.usedBytes < 0 {
			s.usedBytes = 0
		}
		s.tail = offset + n
		if s.tail == len(s.buf) {
			s.tail = 0
		}
	}

	idx := s.tailRecord
	s.records[idx] = compactRawIngressRecord{offset: offset, n: n, padding: padding, meta: meta, committedAt: time.Now().UnixNano()}
	s.tailRecord = (s.tailRecord + 1) % len(s.records)
	s.usedRecords++
	s.packets++
	s.inFlight = false
	s.inFlightOffset = 0
	s.inFlightPadding = 0
	s.inFlightReserved = 0
	s.inFlightPreviousTail = 0
	s.updateGaugesLocked()
	s.notFull.Broadcast()
	s.signalNotifyLocked()
	s.mu.Unlock()

	if s.telemetry.stats != nil {
		s.telemetry.stats.Inc(s.telemetry.shard, "committed_packets")
		s.telemetry.stats.Add(float64(n), s.telemetry.shard, "committed_bytes")
		if padding > 0 {
			s.telemetry.stats.Add(float64(padding), s.telemetry.shard, "padding_bytes")
		}
		if unused > 0 {
			s.telemetry.stats.Add(float64(unused), s.telemetry.shard, "reclaimed_direct_bytes")
		}
	}
}

func (s *CompactRawIngressShard) abortDirect() {
	s.mu.Lock()
	if !s.inFlight {
		s.mu.Unlock()
		return
	}
	s.usedBytes -= int64(s.inFlightPadding + s.inFlightReserved)
	if s.usedBytes < 0 {
		s.usedBytes = 0
	}
	s.tail = s.inFlightPreviousTail
	if s.usedRecords == 0 {
		s.head = 0
		s.tail = 0
		s.usedBytes = 0
	}
	s.inFlight = false
	s.inFlightOffset = 0
	s.inFlightPadding = 0
	s.inFlightReserved = 0
	s.inFlightPreviousTail = 0
	s.updateGaugesLocked()
	s.notFull.Broadcast()
	s.mu.Unlock()

	if s.telemetry.stats != nil {
		s.telemetry.stats.Inc(s.telemetry.shard, "aborted_direct_slots")
	}
}

func (s *CompactRawIngressShard) append(data []byte, meta RawPacketMeta) bool {
	n := len(data)
	var blockStart time.Time
	blocked := false

	s.mu.Lock()
	for !s.stopped && !s.canAppendLocked(n) {
		if !blocked {
			blocked = true
			blockStart = time.Now()
		}
		s.notFull.Wait()
	}
	if s.stopped {
		s.mu.Unlock()
		return false
	}
	if blocked && s.telemetry.blockedNS != nil {
		s.telemetry.blockedNS.Add(float64(time.Since(blockStart).Nanoseconds()), s.telemetry.shard)
	}

	offset, padding := s.reserveBytesLocked(n)
	copy(s.buf[offset:offset+n], data)
	idx := s.tailRecord
	s.records[idx] = compactRawIngressRecord{offset: offset, n: n, padding: padding, meta: meta, committedAt: time.Now().UnixNano()}
	s.tailRecord = (s.tailRecord + 1) % len(s.records)
	s.usedRecords++
	s.usedBytes += int64(padding + n)
	s.packets++
	s.updateGaugesLocked()
	s.signalNotifyLocked()
	s.mu.Unlock()

	if s.telemetry.stats != nil {
		if blocked {
			s.telemetry.stats.Inc(s.telemetry.shard, "blocked_appends")
			s.telemetry.stats.Inc(s.telemetry.shard, "backpressure_events")
		}
		s.telemetry.stats.Inc(s.telemetry.shard, "committed_packets")
		s.telemetry.stats.Add(float64(n), s.telemetry.shard, "committed_bytes")
		if padding > 0 {
			s.telemetry.stats.Add(float64(padding), s.telemetry.shard, "padding_bytes")
		}
	}
	return true
}

func (s *CompactRawIngressShard) canAppendLocked(n int) bool {
	if n > len(s.buf) || s.usedRecords == len(s.records) {
		return false
	}
	needed := s.bytesNeededLocked(n)
	return int64(needed) <= int64(len(s.buf))-s.usedBytes
}

func (s *CompactRawIngressShard) bytesNeededLocked(n int) int {
	if n == 0 {
		return 0
	}
	if s.usedBytes == 0 {
		return n
	}
	if s.tail >= s.head {
		end := len(s.buf) - s.tail
		if n <= end {
			return n
		}
		return end + n
	}
	if n <= s.head-s.tail {
		return n
	}
	return len(s.buf) + 1
}

func (s *CompactRawIngressShard) reserveBytesLocked(n int) (offset int, padding int) {
	if s.usedBytes == 0 {
		s.head = 0
		s.tail = 0
	}
	if n > 0 && s.tail >= s.head && len(s.buf)-s.tail < n {
		padding = len(s.buf) - s.tail
		s.tail = 0
	}
	offset = s.tail
	s.tail += n
	if s.tail == len(s.buf) {
		s.tail = 0
	}
	return offset, padding
}

func (s *CompactRawIngressShard) release(idx int) {
	s.mu.Lock()
	if s.usedRecords == 0 || idx != s.headRecord {
		s.mu.Unlock()
		return
	}
	s.releaseHeadLocked()
	s.mu.Unlock()
}

// ReleaseBatch releases n packets from the compact shard head after a
// successful TryNextBatch call.
func (s *CompactRawIngressShard) ReleaseBatch(n int) {
	if n <= 0 {
		return
	}
	s.mu.Lock()
	for i := 0; i < n && s.usedRecords > 0; i++ {
		s.releaseHeadLocked()
	}
	s.mu.Unlock()
}

func (s *CompactRawIngressShard) releaseHeadLocked() {
	record := s.records[s.headRecord]
	s.records[s.headRecord] = compactRawIngressRecord{}
	s.headRecord = (s.headRecord + 1) % len(s.records)
	s.usedRecords--
	s.usedBytes -= int64(record.padding + record.n)
	if s.usedBytes < 0 {
		s.usedBytes = 0
	}
	s.packets--
	if s.packets < 0 {
		s.packets = 0
	}
	if s.usedRecords == 0 {
		if s.inFlight {
			s.head = s.inFlightOffset
		} else {
			s.head = 0
			s.tail = 0
			s.usedBytes = 0
		}
	} else if record.padding > 0 {
		s.head = record.n
	} else {
		s.head = record.offset + record.n
		if s.head == len(s.buf) {
			s.head = 0
		}
	}
	s.updateGaugesLocked()
	s.notFull.Signal()
}

func (s *CompactRawIngressShard) putScratch(buf []byte) {
	if buf == nil || cap(buf) < s.slotSize {
		return
	}
	s.scratchPool.Put(buf[:s.slotSize])
}

func (s *CompactRawIngressShard) signalNotifyLocked() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *CompactRawIngressShard) updateGaugesLocked() {
	if s.telemetry.bytes != nil {
		s.telemetry.bytes.Set(float64(s.usedBytes), s.telemetry.shard)
	}
	if s.telemetry.slots != nil {
		s.telemetry.slots.Set(float64(s.usedRecords), s.telemetry.shard)
	}
	if s.telemetry.packets != nil {
		s.telemetry.packets.Set(float64(s.packets), s.telemetry.shard)
	}
	if s.telemetry.consumerLagRecords != nil {
		s.telemetry.consumerLagRecords.Set(float64(s.packets), s.telemetry.shard)
	}
	if s.telemetry.consumerLagBytes != nil {
		s.telemetry.consumerLagBytes.Set(float64(s.committedBytesLocked()), s.telemetry.shard)
	}
	s.updateOldestRecordGaugesLocked()
}

func (s *CompactRawIngressShard) committedBytesLocked() int64 {
	committedBytes := s.usedBytes
	if s.inFlight {
		committedBytes -= int64(s.inFlightPadding + s.inFlightReserved)
	}
	if committedBytes < 0 {
		return 0
	}
	return committedBytes
}

func (s *CompactRawIngressShard) updateOldestRecordGaugesLocked() {
	if s.telemetry.oldestAgeNS == nil && s.telemetry.oldestTimestampNS == nil {
		return
	}
	oldest := int64(0)
	if s.usedRecords > 0 {
		oldest = s.records[s.headRecord].committedAt
	}
	if oldest == 0 {
		if s.telemetry.oldestAgeNS != nil {
			s.telemetry.oldestAgeNS.Set(0, s.telemetry.shard)
		}
		if s.telemetry.oldestTimestampNS != nil {
			s.telemetry.oldestTimestampNS.Set(0, s.telemetry.shard)
		}
		return
	}
	if s.telemetry.oldestTimestampNS != nil {
		s.telemetry.oldestTimestampNS.Set(float64(oldest), s.telemetry.shard)
	}
	if s.telemetry.oldestAgeNS != nil {
		age := time.Now().UnixNano() - oldest
		if age < 0 {
			age = 0
		}
		s.telemetry.oldestAgeNS.Set(float64(age), s.telemetry.shard)
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
	telemetry := newRawIngressTelemetry(telemetrycomp)
	for i := range shards {
		shards[i] = newRawIngressShardWithTelemetry(slotsPerShard, slotSize, telemetry.forShard(strconv.Itoa(i)))
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

// CompactRawIngressShards is a sharded compact raw packet writer used by UDS
// datagram listeners. Workers consume their corresponding shard directly.
type CompactRawIngressShards struct {
	shards []*CompactRawIngressShard
	next   atomic.Uint64
}

// NewCompactRawIngressShards creates byte-budgeted compact raw ingress shards.
func NewCompactRawIngressShards(shardCount int, maxBytes int64, slotSize int, telemetrycomp telemetry.Component) *CompactRawIngressShards {
	return newCompactRawIngressShards(shardCount, maxBytes, slotSize, false, telemetrycomp)
}

// NewDirectCompactRawIngressShards creates byte-budgeted compact raw ingress
// shards that reserve ring-owned storage directly for listener reads.
func NewDirectCompactRawIngressShards(shardCount int, maxBytes int64, slotSize int, telemetrycomp telemetry.Component) *CompactRawIngressShards {
	return newCompactRawIngressShards(shardCount, maxBytes, slotSize, true, telemetrycomp)
}

func newCompactRawIngressShards(shardCount int, maxBytes int64, slotSize int, directReserve bool, telemetrycomp telemetry.Component) *CompactRawIngressShards {
	if shardCount <= 0 {
		shardCount = 1
	}
	if slotSize <= 0 {
		slotSize = 1
	}
	if maxBytes <= 0 {
		maxBytes = int64(slotSize)
	}
	bytesPerShard := maxBytes / int64(shardCount)
	if bytesPerShard < int64(slotSize) {
		bytesPerShard = int64(slotSize)
	}
	shards := make([]*CompactRawIngressShard, shardCount)
	telemetry := newRawIngressTelemetry(telemetrycomp)
	for i := range shards {
		shards[i] = newCompactRawIngressShardWithTelemetry(bytesPerShard, slotSize, directReserve, telemetry.forShard(strconv.Itoa(i)))
	}
	return &CompactRawIngressShards{shards: shards}
}

// Reserve reserves scratch space associated with the next compact shard.
func (s *CompactRawIngressShards) Reserve() (RawPacketReservation, bool) {
	if len(s.shards) == 0 {
		return RawPacketReservation{}, false
	}
	idx := int(s.next.Add(1)-1) % len(s.shards)
	return s.shards[idx].Reserve()
}

// Len returns total retained records across compact shards.
func (s *CompactRawIngressShards) Len() int {
	total := 0
	for _, shard := range s.shards {
		total += shard.Len()
	}
	return total
}

// Shard returns the compact shard assigned to a worker.
func (s *CompactRawIngressShards) Shard(worker int) *CompactRawIngressShard {
	if len(s.shards) == 0 {
		return nil
	}
	return s.shards[worker%len(s.shards)]
}

// Stop unblocks all compact shard producers.
func (s *CompactRawIngressShards) Stop() {
	for _, shard := range s.shards {
		shard.Stop()
	}
}
