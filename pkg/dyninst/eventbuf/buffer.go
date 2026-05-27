// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventbuf

import (
	"cmp"
	"sync"

	"github.com/google/btree"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Budget is an aggregate byte ceiling shared across many Buffers. It tracks
// bytes currently held as buffered fragments; each Buffer charges itself on
// AddFragment and credits itself on release. When a charge would take the
// total over the ceiling, the Buffer evicts its oldest entries (truncated-
// finalized) until enough bytes are freed.
//
// Safe for concurrent use by multiple Buffers.
type Budget struct {
	byteLimit int
	mu        struct {
		sync.Mutex
		used int
	}
}

// NewBudget returns a Budget with the given byte limit.
func NewBudget(byteLimit int) *Budget {
	return &Budget{byteLimit: byteLimit}
}

// Limit returns the configured byte ceiling.
func (b *Budget) Limit() int { return b.byteLimit }

// Used returns the current aggregate bytes held.
func (b *Budget) Used() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mu.used
}

// tryCharge attempts to reserve n bytes. Returns true if the charge fits
// under the limit; false otherwise. On false, no bytes are charged.
func (b *Budget) tryCharge(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.mu.used+n > b.byteLimit {
		return false
	}
	b.mu.used += n
	return true
}

// charge reserves n bytes unconditionally. The caller is responsible for
// having evicted enough bytes to make room (or accepting an over-limit
// condition). Used by the "one fragment is larger than the entire budget"
// degenerate path where rejection would lose data outright.
func (b *Budget) charge(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mu.used += n
}

// release credits n bytes back to the budget.
func (b *Budget) release(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.mu.used < n {
		log.Errorf("eventbuf: budget release underflow: used=%d release=%d", b.mu.used, n)
		b.mu.used = 0
		return
	}
	b.mu.used -= n
}

// Side identifies which side of an invocation a fragment or drop notification
// refers to.
type Side uint8

// Side values.
const (
	Entry Side = iota
	Return
)

// Key identifies a single invocation. Goid, StackByteDepth, ProbeID are
// unique across concurrent calls (enforced BPF-side via in_progress_calls).
// EntryKtime further disambiguates rapid sequential invocations sharing
// the same triple.
type Key struct {
	Goid           uint64
	StackByteDepth uint32
	ProbeID        uint32
	EntryKtime     uint64
}

func cmpKey(a, b Key) int {
	return cmp.Or(
		cmp.Compare(a.Goid, b.Goid),
		cmp.Compare(a.StackByteDepth, b.StackByteDepth),
		cmp.Compare(a.ProbeID, b.ProbeID),
		cmp.Compare(a.EntryKtime, b.EntryKtime),
	)
}

// Ready is the result of finalizing an invocation: the fragments to decode,
// plus truncation flags so the caller can render an "incomplete" marker.
type Ready struct {
	Key   Key
	Entry *MessageList
	// Return is nil when the invocation had no return (standalone / inlined /
	// no-body probes) or when the return was lost entirely.
	Return *MessageList
	// EntryTruncated is set when some entry-side fragments didn't arrive due
	// to a ringbuf-full drop.
	EntryTruncated bool
	// ReturnTruncated is set when some return-side fragments didn't arrive.
	ReturnTruncated bool
	// ReturnLost is set when a RETURN_LOST notification arrived: the return
	// probe fired but its signal couldn't reach userspace. The entry is
	// complete; Return is nil.
	ReturnLost bool
}

// bufferedEvent is an in-progress invocation.
//
// Fragments accumulate in entry / returnList. The invocation is ready to
// finalize (i.e. a Ready should be returned) when:
//
//	entryExpected > 0 && entry.length == entryExpected
//	    &&
//	(!expectReturn || returnLost || (returnExpected > 0 && returnList.length == returnExpected))
//
// entryExpected / returnExpected are set by one of two paths: a fragment
// arriving with HasMoreFragments=false records its seq+1; a PARTIAL_ENTRY /
// PARTIAL_RETURN notification records last_seq+1. Either path may run first
// and the other then tops up. entryExpected==0 means "still assembling,
// we don't know the total yet".
type bufferedEvent struct {
	key Key

	entry          *MessageList
	entryFragments uint16
	entryExpected  uint16
	entryTruncated bool

	returnList      *MessageList
	returnFragments uint16
	returnExpected  uint16
	returnTruncated bool

	// expectReturn is true when we've learned this invocation has a return
	// (either from an entry fragment's pairing expectation or from a
	// RETURN_LOST notification). When false, finalization doesn't wait for
	// a return.
	expectReturn bool
	// returnLost: a RETURN_LOST notification was received. Finalize as soon
	// as the entry is complete; do not wait for any return fragments.
	returnLost bool

	// touch is a monotonic counter used by the budget-driven eviction path
	// to find the longest-idle entry (smallest touch) to evict first when
	// the shared Budget is saturated. It is updated on every mutation.
	touch uint64

	// bytes is the sum of event byte lengths across entry + returnList.
	// Maintained incrementally on AddFragment / releaseLists so the Budget
	// can charge/credit without walking the fragment chain.
	bytes int
}

// Buffer is the userspace-side event reassembly and pairing buffer.
//
// A single tree keyed by Key holds every in-flight invocation. Fragments
// and drop notifications both mutate tree entries; entries are finalized
// when they have enough state to emit.
//
// Not safe for concurrent use; the caller (sink) serializes access.
type Buffer struct {
	tree    *btree.BTreeG[*bufferedEvent]
	touchCt uint64

	// budget is the shared byte ceiling. AddFragment charges the budget
	// for new fragment bytes; if the charge won't fit, this buffer evicts
	// its own oldest entries (recorded as truncated Readys in
	// pendingBudgetEvictions) until it frees enough bytes. The sink drains
	// pendingBudgetEvictions via TakePendingBudgetEvictions. Must be
	// non-nil; callers that don't want a real ceiling pass a very large
	// Budget via newTestBuffer in tests.
	budget *Budget

	// bytes is the sum of bytes held by this buffer's entries. Must equal
	// the sum of `be.bytes` across tree entries. Tracked separately so the
	// buffer knows its own footprint cheaply when making eviction decisions.
	bytes int

	// pendingBudgetEvictions holds Readys produced by budget-driven eviction
	// so the caller (sink) can process them after the current mutation.
	// Drained via TakePendingBudgetEvictions.
	pendingBudgetEvictions []Ready
}

// NewBuffer returns an empty buffer that charges the given shared Budget
// for fragment bytes. When the budget is saturated, AddFragment evicts this
// buffer's oldest entries as truncated Readys (surfaced via
// TakePendingBudgetEvictions) until enough bytes are freed.
//
// budget must not be nil; tests that don't care about a size ceiling
// should pass a very large Budget (e.g. NewBudget(math.MaxInt)).
func NewBuffer(budget *Budget) *Buffer {
	if budget == nil {
		panic("eventbuf.NewBuffer: budget is required (pass a large Budget to effectively disable)")
	}
	return &Buffer{
		tree: btree.NewG[*bufferedEvent](16, func(a, b *bufferedEvent) bool {
			return cmpKey(a.key, b.key) < 0
		}),
		budget: budget,
	}
}

// Bytes returns the current bytes held by this buffer's fragments.
func (b *Buffer) Bytes() int { return b.bytes }

// TakePendingBudgetEvictions returns the Readys produced by the most recent
// round of budget-driven evictions and clears the internal queue. Callers
// should drain this after each AddFragment call and emit the results.
func (b *Buffer) TakePendingBudgetEvictions() []Ready {
	if len(b.pendingBudgetEvictions) == 0 {
		return nil
	}
	out := b.pendingBudgetEvictions
	b.pendingBudgetEvictions = nil
	return out
}

// Len returns the number of in-flight invocations.
func (b *Buffer) Len() int {
	return b.tree.Len()
}

// AddFragment records a fragment belonging to the invocation identified by
// key. seq is the fragment's continuation_seq; isFinal is true when
// HasMoreFragments==false on the fragment's header; expectReturn is true
// when the fragment is an entry whose BPF header had ReturnPairingExpected.
// For return-side fragments, expectReturn is ignored.
//
// If this fragment completes the invocation, AddFragment returns (Ready,
// true) and the caller owns the MessageLists inside Ready. Otherwise it
// returns (Ready{}, false) and the Message is retained inside the buffer.
func (b *Buffer) AddFragment(
	key Key, msg Message, side Side, seq uint16, isFinal bool, expectReturn bool,
) (Ready, bool) {
	// Charge the budget for the new fragment's bytes, evicting this
	// buffer's oldest entries as needed to fit. The charge happens before
	// the fragment is attached to the tree so we don't count bytes we're
	// about to free.
	nbytes := len(msg.Event())
	b.chargeForFragment(key, nbytes)

	be := b.getOrCreate(key)
	b.touch(be)
	be.bytes += nbytes
	b.bytes += nbytes
	switch side {
	case Entry:
		if be.entry == nil {
			be.entry = NewMessageList(msg)
		} else {
			be.entry.Append(msg)
		}
		be.entryFragments++
		if isFinal && be.entryExpected == 0 {
			be.entryExpected = seq + 1
		}
		if expectReturn {
			be.expectReturn = true
		}
	case Return:
		if be.returnList == nil {
			be.returnList = NewMessageList(msg)
		} else {
			be.returnList.Append(msg)
		}
		be.returnFragments++
		if isFinal && be.returnExpected == 0 {
			be.returnExpected = seq + 1
		}
		be.expectReturn = true
	}
	return b.tryFinalize(be)
}

// chargeForFragment reserves nbytes in the shared budget, evicting this
// buffer's oldest entries (excluding excludeKey, the one about to be
// appended to) if the charge doesn't fit. If no budget is configured, this
// is a no-op. If even after evicting every other entry the charge still
// won't fit (a single fragment exceeds the budget limit), we admit it
// over-limit and log; the next AddFragment will then trigger further
// eviction.
func (b *Buffer) chargeForFragment(excludeKey Key, nbytes int) {
	if b.budget.tryCharge(nbytes) {
		return
	}
	// Evict oldest entries other than the one we're extending, until the
	// charge fits.
	for b.tree.Len() > 0 {
		// Find the oldest entry (smallest touch) other than excludeKey.
		var oldest *bufferedEvent
		b.tree.Ascend(func(be *bufferedEvent) bool {
			if be.key == excludeKey {
				return true
			}
			if oldest == nil || be.touch < oldest.touch {
				oldest = be
			}
			return true
		})
		if oldest == nil {
			break
		}
		ready := b.forcedFinalize(oldest)
		b.pendingBudgetEvictions = append(b.pendingBudgetEvictions, ready)
		if b.budget.tryCharge(nbytes) {
			return
		}
	}
	// Nothing more to evict and still doesn't fit. Either a single
	// fragment is larger than the entire budget or the budget is held by
	// other buffers. Admit over-limit; subsequent adds will keep pushing
	// the budget down.
	b.budget.charge(nbytes)
	log.Tracef(
		"eventbuf: admitting fragment over budget (nbytes=%d, used=%d/%d)",
		nbytes, b.budget.Used(), b.budget.Limit(),
	)
}

// NoteReturnLost records a RETURN_LOST drop notification: the return side
// had no fragments reach userspace. If the entry is already complete the
// invocation finalizes immediately.
func (b *Buffer) NoteReturnLost(key Key) (Ready, bool) {
	be := b.getOrCreate(key)
	b.touch(be)
	be.returnLost = true
	be.expectReturn = true
	return b.tryFinalize(be)
}

// Discard removes any buffered state for the key without emitting. Used for
// the "condition failed" signal: the return probe's BPF code sent an empty
// marker event indicating the return condition evaluated to false, so the
// buffered entry should be silently dropped.
//
// Any fragments (entry or return) held for the key are released. If the
// key has no buffered state, Discard is a no-op.
func (b *Buffer) Discard(key Key) {
	be, ok := b.tree.Get(&bufferedEvent{key: key})
	if !ok {
		return
	}
	b.tree.Delete(be)
	b.creditBytes(be.bytes)
	if be.entry != nil {
		be.entry.Release()
	}
	if be.returnList != nil {
		be.returnList.Release()
	}
}

// NotePartial records a PARTIAL_ENTRY or PARTIAL_RETURN drop notification.
// lastSeq is the continuation_seq of the last fragment BPF successfully
// submitted on the indicated side; userspace should expect lastSeq+1
// fragments on that side and treat the resulting event as truncated.
func (b *Buffer) NotePartial(key Key, side Side, lastSeq uint16) (Ready, bool) {
	be := b.getOrCreate(key)
	b.touch(be)
	switch side {
	case Entry:
		if be.entryExpected == 0 {
			be.entryExpected = lastSeq + 1
		}
		be.entryTruncated = true
	case Return:
		if be.returnExpected == 0 {
			be.returnExpected = lastSeq + 1
		}
		be.returnTruncated = true
		be.expectReturn = true
	}
	return b.tryFinalize(be)
}

// EvictOlderThan finalizes every invocation whose Key.EntryKtime is less
// than or equal to cutoffKtimeNs, returning the resulting Ready values.
//
// Intended for use when the BPF side reports that a drop notification was
// itself lost: the caller, after waiting a grace window, concludes that
// any buffered entry whose invocation predates the observed fault can no
// longer make forward progress and should be emitted truncated.
//
// The returned slice may be empty. Callers own the MessageLists inside.
func (b *Buffer) EvictOlderThan(cutoffKtimeNs uint64) []Ready {
	if b.tree.Len() == 0 {
		return nil
	}
	var toEvict []*bufferedEvent
	b.tree.Ascend(func(be *bufferedEvent) bool {
		if be.key.EntryKtime <= cutoffKtimeNs {
			toEvict = append(toEvict, be)
		}
		return true
	})
	if len(toEvict) == 0 {
		return nil
	}
	out := make([]Ready, 0, len(toEvict))
	for _, be := range toEvict {
		out = append(out, b.forcedFinalize(be))
	}
	return out
}

// Close finalizes every remaining invocation and returns their Readys. The
// caller owns the returned MessageLists and must Release them (or decode
// them) before discarding. Pending fragments with no matching notification
// are emitted as truncated.
//
// After Close the Buffer is empty; further use is a programmer error.
func (b *Buffer) Close() []Ready {
	if b.tree.Len() == 0 {
		return nil
	}
	out := make([]Ready, 0, b.tree.Len())
	// Snapshot the entries to evict before mutating the tree (Ascend's
	// iterator doesn't like concurrent modification).
	var all []*bufferedEvent
	b.tree.Ascend(func(be *bufferedEvent) bool {
		all = append(all, be)
		return true
	})
	for _, be := range all {
		out = append(out, b.forcedFinalize(be))
	}
	return out
}

// forcedFinalize emits a Ready for an entry that is being removed before
// its normal finalization condition was met. Incomplete sides are marked
// truncated so the caller knows the output is partial.
func (b *Buffer) forcedFinalize(be *bufferedEvent) Ready {
	if be.entryExpected == 0 || be.entryFragments < be.entryExpected {
		be.entryTruncated = true
		if be.entryExpected == 0 {
			be.entryExpected = be.entryFragments
		}
	}
	if be.expectReturn && !be.returnLost {
		if be.returnExpected == 0 || be.returnFragments < be.returnExpected {
			be.returnTruncated = true
			if be.returnExpected == 0 {
				be.returnExpected = be.returnFragments
			}
		}
	}
	return b.finalize(be)
}

func (b *Buffer) getOrCreate(key Key) *bufferedEvent {
	if found, ok := b.tree.Get(&bufferedEvent{key: key}); ok {
		return found
	}
	be := &bufferedEvent{key: key}
	b.tree.ReplaceOrInsert(be)
	return be
}

func (b *Buffer) touch(be *bufferedEvent) {
	b.touchCt++
	be.touch = b.touchCt
}

// tryFinalize checks the finalization condition; if met, removes the entry
// from the tree and returns (Ready, true). Otherwise returns (Ready{}, false).
func (b *Buffer) tryFinalize(be *bufferedEvent) (Ready, bool) {
	entryReady := be.entryExpected > 0 && be.entryFragments == be.entryExpected
	if !entryReady {
		return Ready{}, false
	}
	if be.expectReturn && !be.returnLost {
		if be.returnExpected == 0 || be.returnFragments < be.returnExpected {
			return Ready{}, false
		}
	}
	return b.finalize(be), true
}

// finalize removes the entry from the tree and returns its Ready view.
// The message lists become the caller's responsibility (to decode and
// then Release); at that point the bytes leave this buffer's accounting.
func (b *Buffer) finalize(be *bufferedEvent) Ready {
	b.tree.Delete(be)
	b.creditBytes(be.bytes)
	return readyFrom(be)
}

func readyFrom(be *bufferedEvent) Ready {
	return Ready{
		Key:             be.key,
		Entry:           be.entry,
		Return:          be.returnList,
		EntryTruncated:  be.entryTruncated,
		ReturnTruncated: be.returnTruncated,
		ReturnLost:      be.returnLost,
	}
}

// creditBytes returns nbytes to both this buffer's accounting and (if set)
// the shared Budget.
func (b *Buffer) creditBytes(nbytes int) {
	b.bytes -= nbytes
	b.budget.release(nbytes)
}
