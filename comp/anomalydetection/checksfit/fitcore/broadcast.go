// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`): the transport
// comes from commit 4961722de9009afdbbb711fc0adf14a8f6ff9277, and the broadcast
// transport from commit 788233d2ffcc1e9d19b8e8202ca7908b64c64687
// (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

package fitcore

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// broadcastInner is the publisher-local state shared with the control worker.
// The registry fields are guarded by mu; live shared controls use atomics.
type broadcastInner struct {
	shared         *mapping
	protocol       ProtocolDescriptor
	session        uint64
	capacity       int
	maxSubscribers int
	name           string
	setupTimeout   time.Duration

	mu      sync.Mutex
	changed chan struct{}

	activeSlots     []uint32 // dense list iterated on the publication path
	activeIndex     []int32  // reverse position by slot ID, -1 when inactive
	freeSlots       []uint32 // free-list stack
	cachedFree      int
	cachedFreeValid bool
	pin             int // slot currently registered as the wait target, -1 when none
	pending         int // reserved-but-not-active handshakes

	shutdown   atomic.Bool
	handshakes atomic.Int64
}

// BroadcastOutcome reports a broadcast send. It always carries the number of
// records already published, even for cancellation or a fatal failure. Accepted
// means published, not decoded or handled; a caller cannot retry a whole batch
// after a partial result.
type BroadcastOutcome struct {
	Accepted          int
	Rejection         Rejection
	NotificationError error
	Cancelled         bool
	Failure           error
}

type chunkResult struct {
	staged            int
	rejection         Rejection
	notificationError error
	cancelled         bool
	failure           error
}

func newBroadcastInner(shared *mapping, protocol ProtocolDescriptor, session uint64, capacity, maxSubscribers int, name string, setupTimeout time.Duration) *broadcastInner {
	freeSlots := make([]uint32, maxSubscribers)
	for i := 0; i < maxSubscribers; i++ {
		freeSlots[maxSubscribers-1-i] = uint32(i)
	}
	activeIndex := make([]int32, maxSubscribers)
	for i := range activeIndex {
		activeIndex[i] = -1
	}
	return &broadcastInner{
		shared:         shared,
		protocol:       protocol,
		session:        session,
		capacity:       capacity,
		maxSubscribers: maxSubscribers,
		name:           name,
		setupTimeout:   setupTimeout,
		changed:        make(chan struct{}),
		activeSlots:    make([]uint32, 0, maxSubscribers),
		activeIndex:    activeIndex,
		freeSlots:      freeSlots,
		pin:            -1,
	}
}

func (b *broadcastInner) writeWord() *uint32 {
	return b.shared.word(broadcastWriteOffset)
}

func (b *broadcastInner) slotBase(slot uint32) int {
	return broadcastSlotsOffset + int(slot)*broadcastSlotStride
}

func (b *broadcastInner) slotRead(slot uint32) *uint32 {
	return b.shared.word(b.slotBase(slot) + broadcastSlotReadCursor)
}

func (b *broadcastInner) slotState(slot uint32) *uint32 {
	return b.shared.word(b.slotBase(slot) + broadcastSlotState)
}

func (b *broadcastInner) slotGeneration(slot uint32) *uint32 {
	return b.shared.word(b.slotBase(slot) + broadcastSlotGeneration)
}

func (b *broadcastInner) slotFlag(slot uint32) *uint32 {
	return b.shared.word(b.slotBase(slot) + broadcastSlotProducerWaiting)
}

// notifyChangedLocked wakes every waiter blocked on subscriber membership. The
// caller must hold mu.
func (b *broadcastInner) notifyChangedLocked() {
	close(b.changed)
	b.changed = make(chan struct{})
}

func (b *broadcastInner) loadWriteLocked() (int, error) {
	w := int(atomic.LoadUint32(b.writeWord()))
	if w >= b.capacity || w%8 != 0 {
		return 0, invalid("broadcast write cursor is outside the aligned ring")
	}
	return w, nil
}

// scanFreeLocked computes the conservative free-byte budget from the slowest
// active subscriber. The caller must hold mu.
func (b *broadcastInner) scanFreeLocked() (int, error) {
	w := int(atomic.LoadUint32(b.writeWord()))
	if w >= b.capacity || w%8 != 0 {
		return 0, invalid("broadcast write cursor is outside the aligned ring")
	}
	maxLag := 0
	for _, slot := range b.activeSlots {
		r := int(atomic.LoadUint32(b.slotRead(slot)))
		if r >= b.capacity || r%8 != 0 {
			return 0, invalid("subscriber read cursor is outside the aligned ring")
		}
		lag := (w + b.capacity - r) % b.capacity
		if lag > maxLag {
			maxLag = lag
		}
	}
	if maxLag > b.capacity-gap {
		return 0, invalid("broadcast ring exceeds its reserved gap")
	}
	return b.capacity - gap - maxLag, nil
}

// sendBatch publishes the fitting prefix, blocking for capacity and for a first
// subscriber. It never drops records; a stopped subscriber stalls the call.
func (b *broadcastInner) sendBatch(records []Record, reg *waitRegistry, ctx context.Context) BroadcastOutcome {
	outcome := BroadcastOutcome{Rejection: RejectionNone}
	if checkCancelled(ctx) {
		outcome.Cancelled = true
		return outcome
	}
	for outcome.Accepted < len(records) {
		chunk := b.stageChunk(records, outcome.Accepted, reg, ctx)
		outcome.Accepted += chunk.staged
		if chunk.notificationError != nil {
			outcome.NotificationError = chunk.notificationError
		}
		if chunk.rejection != RejectionNone {
			outcome.Rejection = chunk.rejection
			break
		}
		if chunk.cancelled {
			outcome.Cancelled = true
			break
		}
		if chunk.failure != nil {
			outcome.Failure = chunk.failure
			break
		}
	}
	return outcome
}

func (b *broadcastInner) stageChunk(records []Record, start int, reg *waitRegistry, ctx context.Context) chunkResult {
	b.mu.Lock()
	for {
		if b.shutdown.Load() {
			b.mu.Unlock()
			return chunkResult{failure: fmt.Errorf("broadcast publisher is shut down")}
		}
		if checkCancelled(ctx) {
			b.mu.Unlock()
			return chunkResult{cancelled: true}
		}
		if len(b.activeSlots) == 0 {
			changed := b.changed
			b.mu.Unlock()
			if !waitChanged(ctx, changed) {
				return chunkResult{cancelled: true}
			}
			b.mu.Lock()
			continue
		}
		if !b.cachedFreeValid {
			free, err := b.scanFreeLocked()
			if err != nil {
				b.mu.Unlock()
				return chunkResult{failure: err}
			}
			b.cachedFree = free
			b.cachedFreeValid = true
		}
		cursor, err := b.loadWriteLocked()
		if err != nil {
			b.mu.Unlock()
			return chunkResult{failure: err}
		}
		free := b.cachedFree
		staged := 0
		rejection := RejectionNone
		published := false
		ring := b.shared.ring()

		for start+staged < len(records) {
			record := records[start+staged]
			if !b.protocol.supports(record.Kind) {
				rejection = RejectionInvalidType
				break
			}
			size, reason := broadcastRecordSize(len(record.Payload), b.capacity)
			if reason != RejectionNone {
				rejection = reason
				break
			}
			tail := b.capacity - cursor
			if size <= tail {
				if free < size {
					if staged > 0 || published {
						break
					}
					observed, cancelled, failure := b.waitForSpaceLocked(size, reg, ctx)
					if failure != nil {
						b.mu.Unlock()
						return chunkResult{failure: failure}
					}
					if cancelled {
						b.mu.Unlock()
						return chunkResult{cancelled: true}
					}
					free = observed
					cursor, err = b.loadWriteLocked()
					if err != nil {
						b.mu.Unlock()
						return chunkResult{failure: err}
					}
					continue
				}
				writeBroadcastRecord(ring, cursor, record, size)
				cursor = (cursor + size) % b.capacity
				free -= size
				staged++
				continue
			}
			// The record does not fit before the physical end: wrap first.
			if free < tail {
				if staged > 0 || published {
					break
				}
				observed, cancelled, failure := b.waitForSpaceLocked(tail, reg, ctx)
				if failure != nil {
					b.mu.Unlock()
					return chunkResult{failure: failure}
				}
				if cancelled {
					b.mu.Unlock()
					return chunkResult{cancelled: true}
				}
				free = observed
				cursor, err = b.loadWriteLocked()
				if err != nil {
					b.mu.Unlock()
					return chunkResult{failure: err}
				}
				continue
			}
			clear(ring[cursor : cursor+recordHeader])
			cursor = 0
			free -= tail
			published = true
			if free >= size {
				writeBroadcastRecord(ring, 0, record, size)
				cursor = size
				free -= size
				staged++
			} else {
				// Publish the padding alone, then the next loop iteration waits
				// for the record's own space.
				break
			}
		}

		if staged > 0 || published {
			atomic.StoreUint32(b.writeWord(), uint32(cursor))
			b.cachedFree = free
			b.cachedFreeValid = true
			notifyErr := wakeAllWord(b.writeWord())
			b.mu.Unlock()
			return chunkResult{staged: staged, rejection: rejection, notificationError: notifyErr}
		}
		if rejection != RejectionNone {
			b.mu.Unlock()
			return chunkResult{rejection: rejection}
		}
		b.mu.Unlock()
		return chunkResult{failure: invalid("broadcast send made no progress")}
	}
}

// waitForSpaceLocked blocks until at least needed bytes are free. It releases
// mu for the native wait and reacquires it before returning. The caller must
// hold mu.
func (b *broadcastInner) waitForSpaceLocked(needed int, reg *waitRegistry, ctx context.Context) (int, bool, error) {
	free, err := b.scanFreeLocked()
	if err != nil {
		return 0, false, err
	}
	b.cachedFree = free
	b.cachedFreeValid = true
	if free >= needed {
		return free, false, nil
	}
	w, err := b.loadWriteLocked()
	if err != nil {
		return 0, false, err
	}
	limit := -1
	maxLag := 0
	for _, slot := range b.activeSlots {
		r := int(atomic.LoadUint32(b.slotRead(slot)))
		if r >= b.capacity || r%8 != 0 {
			return 0, false, invalid("subscriber read cursor is outside the aligned ring")
		}
		lag := (w + b.capacity - r) % b.capacity
		if lag > maxLag {
			maxLag = lag
			limit = int(slot)
		}
	}
	if limit < 0 {
		return free, false, nil
	}
	slot := uint32(limit)
	b.pin = limit
	atomic.StoreUint32(b.slotFlag(slot), 1)
	recheck, err := b.scanFreeLocked()
	if err != nil {
		atomic.StoreUint32(b.slotFlag(slot), 0)
		b.pin = -1
		return 0, false, err
	}
	b.cachedFree = recheck
	b.cachedFreeValid = true
	if recheck >= needed {
		atomic.StoreUint32(b.slotFlag(slot), 0)
		b.pin = -1
		return recheck, false, nil
	}
	expected := atomic.LoadUint32(b.slotRead(slot))
	readPtr := b.slotRead(slot)
	b.mu.Unlock()
	var waitErr error
	if reg != nil {
		proceed, werr := reg.waitOn(readPtr, expected)
		waitErr = werr
		if waitErr == nil && !proceed {
			b.mu.Lock()
			atomic.StoreUint32(b.slotFlag(slot), 0)
			b.pin = -1
			b.finishRetiringLocked(slot)
			return 0, true, nil
		}
	} else {
		waitErr = waitWord(readPtr, expected)
	}
	b.mu.Lock()
	atomic.StoreUint32(b.slotFlag(slot), 0)
	b.pin = -1
	b.finishRetiringLocked(slot)
	if waitErr != nil {
		return 0, false, waitErr
	}
	if b.cachedFreeValid {
		return b.cachedFree, false, nil
	}
	free, err = b.scanFreeLocked()
	if err != nil {
		return 0, false, err
	}
	b.cachedFree = free
	b.cachedFreeValid = true
	return free, false, nil
}

// reserveSlotLocked reserves a free slot and a fresh generation. The caller
// must hold mu.
func (b *broadcastInner) reserveSlotLocked() (uint32, uint32, error) {
	if len(b.freeSlots) == 0 {
		return 0, 0, fmt.Errorf("no free subscriber slot: %w", errors.New("subscriber capacity exhausted"))
	}
	slot := b.freeSlots[len(b.freeSlots)-1]
	b.freeSlots = b.freeSlots[:len(b.freeSlots)-1]
	previous := atomic.LoadUint32(b.slotGeneration(slot))
	if previous == ^uint32(0) {
		b.freeSlots = append(b.freeSlots, slot)
		return 0, 0, invalid("subscriber slot generation exhausted")
	}
	generation := previous + 1
	atomic.StoreUint32(b.slotGeneration(slot), generation)
	atomic.StoreUint32(b.slotRead(slot), 0)
	atomic.StoreUint32(b.slotFlag(slot), 0)
	atomic.StoreUint32(b.slotState(slot), broadcastSlotReserved)
	b.pending++
	return slot, generation, nil
}

// releaseReservationLocked returns a slot whose handshake failed before
// activation. The caller must hold mu.
func (b *broadcastInner) releaseReservationLocked(slot uint32) {
	if atomic.LoadUint32(b.slotState(slot)) != broadcastSlotReserved {
		return
	}
	atomic.StoreUint32(b.slotState(slot), broadcastSlotFree)
	b.freeSlots = append(b.freeSlots, slot)
	if b.pending > 0 {
		b.pending--
	}
	b.cachedFreeValid = false
}

// activateSlotLocked initializes the read cursor at the current published write
// position and adds the slot to the dense active list. The caller must hold mu.
func (b *broadcastInner) activateSlotLocked(slot, generation uint32) (int, error) {
	if atomic.LoadUint32(b.slotState(slot)) != broadcastSlotReserved {
		return 0, invalid("subscriber slot is not reserved")
	}
	if atomic.LoadUint32(b.slotGeneration(slot)) != generation {
		return 0, invalid("subscriber slot generation mismatch")
	}
	w, err := b.loadWriteLocked()
	if err != nil {
		return 0, err
	}
	atomic.StoreUint32(b.slotRead(slot), uint32(w))
	atomic.StoreUint32(b.slotFlag(slot), 0)
	atomic.StoreUint32(b.slotState(slot), broadcastSlotActive)
	b.activeIndex[slot] = int32(len(b.activeSlots))
	b.activeSlots = append(b.activeSlots, slot)
	b.cachedFreeValid = false
	if b.pending > 0 {
		b.pending--
	}
	b.notifyChangedLocked()
	return w, nil
}

// retireSlotLocked removes an active subscriber. A pinned slot stays Retiring
// until the publisher clears its pin, preventing a cursor ABA. The caller must
// hold mu.
func (b *broadcastInner) retireSlotLocked(slot, generation uint32) error {
	if int(slot) >= b.maxSubscribers {
		return invalid("subscriber slot is out of range")
	}
	if atomic.LoadUint32(b.slotState(slot)) != broadcastSlotActive {
		return invalid("subscriber slot is not active")
	}
	if atomic.LoadUint32(b.slotGeneration(slot)) != generation {
		return invalid("subscriber slot generation mismatch")
	}
	position := b.activeIndex[slot]
	if position < 0 {
		return invalid("subscriber slot is missing from the active list")
	}
	last := len(b.activeSlots) - 1
	lastSlot := b.activeSlots[last]
	b.activeSlots[position] = lastSlot
	b.activeSlots = b.activeSlots[:last]
	b.activeIndex[slot] = -1
	if int(position) < len(b.activeSlots) {
		b.activeIndex[lastSlot] = position
	}
	b.cachedFreeValid = false
	atomic.StoreUint32(b.slotState(slot), broadcastSlotRetiring)
	if b.pin == int(slot) {
		w, err := b.loadWriteLocked()
		if err != nil {
			return err
		}
		atomic.StoreUint32(b.slotRead(slot), uint32(w))
		if err := wakeWord(b.slotRead(slot)); err != nil {
			return err
		}
	} else {
		b.finishRetiringLocked(slot)
	}
	return nil
}

// finishRetiringLocked frees a Retiring slot once no publisher pin references
// it. The caller must hold mu.
func (b *broadcastInner) finishRetiringLocked(slot uint32) {
	if atomic.LoadUint32(b.slotState(slot)) != broadcastSlotRetiring {
		return
	}
	atomic.StoreUint32(b.slotState(slot), broadcastSlotFree)
	b.freeSlots = append(b.freeSlots, slot)
	b.cachedFreeValid = false
}

func (b *broadcastInner) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.activeSlots)
}

// broadcastRecordSize returns the aligned record size or the rejection that
// makes it unstageable.
func broadcastRecordSize(payloadLen, capacity int) (int, Rejection) {
	if uint64(payloadLen)+recordHeader > uint64(^uint32(0)) {
		return 0, RejectionOversized
	}
	raw := recordHeader + payloadLen
	size := (raw + 7) &^ 7
	if size > capacity-gap {
		return 0, RejectionOversized
	}
	return size, RejectionNone
}

// writeBroadcastRecord writes a complete record (header, payload, zero padding)
// at cursor. The caller guarantees the aligned size fits before the physical
// ring end.
func writeBroadcastRecord(ring []byte, cursor int, record Record, size int) {
	at := ring[cursor : cursor+size]
	binary.LittleEndian.PutUint32(at[0:4], uint32(len(record.Payload)))
	binary.LittleEndian.PutUint32(at[4:8], record.Kind)
	copy(at[recordHeader:recordHeader+len(record.Payload)], record.Payload)
	clear(at[recordHeader+len(record.Payload):])
}

// waitChanged blocks until the membership channel is replaced or the context
// fires. It reports false when the context fired.
func waitChanged(ctx context.Context, changed <-chan struct{}) bool {
	if ctx == nil {
		<-changed
		return true
	}
	select {
	case <-changed:
		return true
	case <-ctx.Done():
		return false
	}
}

func checkCancelled(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}

// Subscription is one subscriber's independently mapped view of the ring.
type Subscription struct {
	shared       *mapping
	protocol     ProtocolDescriptor
	session      uint64
	slot         uint32
	generation   uint32
	capacity     int
	endpoint     SetupEndpoint
	setupTimeout time.Duration
	unsubscribed bool
}

// SessionID reports the session identifier.
func (s *Subscription) SessionID() uint64 { return s.session }

// SlotID reports the subscriber's slot index.
func (s *Subscription) SlotID() uint32 { return s.slot }

// Generation reports the slot generation assigned at activation.
func (s *Subscription) Generation() uint32 { return s.generation }

// Receive waits on the shared write cursor when the ring is empty, then returns
// the next record's type and an owned copy of its payload.
func (s *Subscription) Receive() (kind uint32, payload []byte, err error) {
	k, data, cancelled, err := s.receiveInner(nil, nil)
	if cancelled {
		return 0, nil, invalid("uncancelled broadcast receive stopped")
	}
	return k, data, err
}

// ReceiveContext receives one record, or returns an error wrapping ErrCancelled
// after local cancellation without reclaiming a queued record.
func (s *Subscription) ReceiveContext(ctx context.Context) (kind uint32, payload []byte, err error) {
	if ctx == nil {
		return s.Receive()
	}
	if cause := ctx.Err(); cause != nil {
		return 0, nil, cancelled(cause)
	}
	reg := newWaitRegistry()
	stop := watchContext(ctx, reg)
	defer stop()
	k, data, wasCancelled, err := s.receiveInner(ctx, reg)
	if wasCancelled {
		return 0, nil, cancelled(ctx.Err())
	}
	return k, data, err
}

func (s *Subscription) receiveInner(ctx context.Context, reg *waitRegistry) (uint32, []byte, bool, error) {
	ring := s.shared.ring()
	for {
		if reg != nil && reg.check() {
			return 0, nil, true, nil
		}
		w := int(atomic.LoadUint32(s.shared.word(broadcastWriteOffset)))
		if w >= s.capacity || w%8 != 0 {
			return 0, nil, false, invalid("broadcast write cursor is outside the aligned ring")
		}
		readPtr := s.readWord()
		r := int(atomic.LoadUint32(readPtr))
		if r >= s.capacity || r%8 != 0 {
			return 0, nil, false, invalid("subscriber read cursor is outside the aligned ring")
		}
		if r == w {
			if reg != nil {
				proceed, werr := reg.waitOnAll(s.shared.word(broadcastWriteOffset), uint32(w))
				if werr != nil {
					return 0, nil, false, werr
				}
				if !proceed {
					return 0, nil, true, nil
				}
			} else {
				if werr := waitWord(s.shared.word(broadcastWriteOffset), uint32(w)); werr != nil {
					return 0, nil, false, werr
				}
			}
			continue
		}
		span := w - r
		if w <= r {
			span = s.capacity - r
		}
		if span < recordHeader {
			return 0, nil, false, invalid("published span lacks record header")
		}
		header := ring[r : r+recordHeader]
		length := binary.LittleEndian.Uint32(header[0:4])
		recordKind := binary.LittleEndian.Uint32(header[4:8])
		if recordKind == 0 {
			if length != 0 || r == 0 || w >= r {
				return 0, nil, false, invalid("invalid wrap marker")
			}
			atomic.StoreUint32(readPtr, 0)
			continue
		}
		if !s.protocol.supports(recordKind) {
			return 0, nil, false, invalid("unknown record type")
		}
		size := (uint64(length) + recordHeader + 7) &^ 7
		if size > uint64(s.capacity-gap) || size > uint64(span) {
			return 0, nil, false, invalid("record exceeds published contiguous span")
		}
		data := make([]byte, length)
		copy(data, ring[r+recordHeader:r+recordHeader+int(length)])
		atomic.StoreUint32(readPtr, uint32((r+int(size))%s.capacity))
		// Advance first, then check the flag and wake one producer space
		// waiter. The producer's store-then-load pairs with this load.
		if atomic.LoadUint32(s.flagWord()) == 1 {
			if werr := wakeWord(readPtr); werr != nil {
				return 0, nil, false, werr
			}
		}
		return recordKind, data, false, nil
	}
}

func (s *Subscription) readWord() *uint32 {
	return s.shared.word(broadcastSlotsOffset + int(s.slot)*broadcastSlotStride + broadcastSlotReadCursor)
}

func (s *Subscription) flagWord() *uint32 {
	return s.shared.word(broadcastSlotsOffset + int(s.slot)*broadcastSlotStride + broadcastSlotProducerWaiting)
}
