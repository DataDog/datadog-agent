// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventbuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// k builds a Key used across tests.
func k(goid uint64, entryKtime uint64) Key {
	return Key{Goid: goid, StackByteDepth: 100, ProbeID: 1, EntryKtime: entryKtime}
}

// ------------------------------------------------------------------
// Basic pairing (no drops).
// ------------------------------------------------------------------

// 1. Entry + return, both single-fragment.
func TestBuffer_EntryReturnSingleFragment(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)
	rm := newTestMessage(4)

	r, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.False(t, done, "entry alone is not final for paired probe")
	require.Nil(t, r.Entry)

	r, done = b.AddFragment(k(1, 1000), rm, Return, 0, true, false)
	require.True(t, done)
	require.NotNil(t, r.Entry)
	require.NotNil(t, r.Return)
	assert.False(t, r.EntryTruncated)
	assert.False(t, r.ReturnTruncated)
	assert.False(t, r.ReturnLost)
	r.Entry.Release()
	r.Return.Release()
	assert.True(t, em.released)
	assert.True(t, rm.released)
	assert.Equal(t, 0, b.Len())
}

// 2. Entry multi-fragment + return single.
func TestBuffer_EntryMultiReturnSingle(t *testing.T) {
	b := newTestBuffer()
	e0 := newTestMessage(8)
	e1 := newTestMessage(8)
	rm := newTestMessage(4)

	_, done := b.AddFragment(k(1, 1000), e0, Entry, 0, false, true)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), e1, Entry, 1, true, true)
	require.False(t, done, "entry assembled but return not yet here")

	r, done := b.AddFragment(k(1, 1000), rm, Return, 0, true, false)
	require.True(t, done)
	// Ready.Entry should have two messages.
	count := 0
	for range r.Entry.Fragments() {
		count++
	}
	assert.Equal(t, 2, count)
	r.Entry.Release()
	r.Return.Release()
}

// 3. Entry single + return multi-fragment.
func TestBuffer_EntrySingleReturnMulti(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)
	r0 := newTestMessage(4)
	r1 := newTestMessage(4)

	_, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), r0, Return, 0, false, false)
	require.False(t, done)
	r, done := b.AddFragment(k(1, 1000), r1, Return, 1, true, false)
	require.True(t, done)
	count := 0
	for range r.Return.Fragments() {
		count++
	}
	assert.Equal(t, 2, count)
	r.Entry.Release()
	r.Return.Release()
}

// 4. Both multi-fragment.
func TestBuffer_EntryReturnBothMulti(t *testing.T) {
	b := newTestBuffer()
	e0, e1 := newTestMessage(8), newTestMessage(8)
	r0, r1 := newTestMessage(4), newTestMessage(4)

	_, done := b.AddFragment(k(1, 1000), e0, Entry, 0, false, true)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), e1, Entry, 1, true, true)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), r0, Return, 0, false, false)
	require.False(t, done)
	r, done := b.AddFragment(k(1, 1000), r1, Return, 1, true, false)
	require.True(t, done)
	r.Entry.Release()
	r.Return.Release()
}

// 5. Standalone (no return) single fragment.
func TestBuffer_StandaloneSingle(t *testing.T) {
	b := newTestBuffer()
	m := newTestMessage(16)
	r, done := b.AddFragment(k(1, 1000), m, Entry, 0, true, false /*expectReturn*/)
	require.True(t, done)
	require.NotNil(t, r.Entry)
	assert.Nil(t, r.Return)
	r.Entry.Release()
}

// 6. Standalone multi-fragment.
func TestBuffer_StandaloneMulti(t *testing.T) {
	b := newTestBuffer()
	m0, m1 := newTestMessage(16), newTestMessage(16)
	_, done := b.AddFragment(k(1, 1000), m0, Entry, 0, false, false)
	require.False(t, done)
	r, done := b.AddFragment(k(1, 1000), m1, Entry, 1, true, false)
	require.True(t, done)
	r.Entry.Release()
}

// ------------------------------------------------------------------
// RETURN_LOST.
// ------------------------------------------------------------------

// 7. Entry complete; RETURN_LOST → emit entry alone.
func TestBuffer_ReturnLost_EntryComplete(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)

	_, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.False(t, done)

	r, done := b.NoteReturnLost(k(1, 1000))
	require.True(t, done)
	assert.NotNil(t, r.Entry)
	assert.Nil(t, r.Return)
	assert.True(t, r.ReturnLost)
	assert.False(t, r.EntryTruncated)
	r.Entry.Release()
}

// 9. RETURN_LOST arrives before entry; entry arrives later; finalize.
func TestBuffer_ReturnLost_BeforeEntry(t *testing.T) {
	b := newTestBuffer()
	_, done := b.NoteReturnLost(k(1, 1000))
	require.False(t, done, "no entry yet, can't finalize")

	em := newTestMessage(8)
	r, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.True(t, done)
	assert.True(t, r.ReturnLost)
	r.Entry.Release()
}

// ------------------------------------------------------------------
// PanicUnwoundRange (synthetic return from runtime.recovery probe).
// ------------------------------------------------------------------

// kAt returns a Key with the goid, stack-byte depth and a fixed
// (probeID, ktime) — convenient for range-scan tests where the
// depth is the discriminator.
func kAt(goid uint64, depth uint32) Key {
	return Key{Goid: goid, StackByteDepth: depth, ProbeID: 1, EntryKtime: 1000}
}

// Single in-flight frame on goid in the unwound range: the synthetic
// payload becomes the return-side fragment and the invocation
// finalizes immediately.
func TestBuffer_PanicUnwoundRange_SingleFrame(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)
	pm := newTestMessage(16)

	_, done := b.AddFragment(kAt(1, 200), em, Entry, 0, true, true)
	require.False(t, done, "entry alone is not final for paired probe")

	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	shared.ReleaseBase()
	require.Len(t, readys, 1)
	r := readys[0]
	assert.True(t, r.PanicUnwound)
	assert.False(t, r.ReturnLost)
	require.NotNil(t, r.Entry)
	require.NotNil(t, r.Return)
	// Return-side has exactly one fragment (the panic value).
	count := 0
	for range r.Return.Fragments() {
		count++
	}
	assert.Equal(t, 1, count)
	r.Entry.Release()
	r.Return.Release()
	assert.True(t, em.released)
	assert.True(t, pm.released, "shared release fires once after last handle")
	assert.Equal(t, 0, b.Len())
}

// Multiple frames on the same goid inside the unwound range share
// the panic-value payload via refcount; each Ready finalizes
// independently, and the underlying message is released exactly once
// after the last consumer drains.
func TestBuffer_PanicUnwoundRange_MultiFrameFanout(t *testing.T) {
	b := newTestBuffer()
	em1 := newTestMessage(8)
	em2 := newTestMessage(8)
	em3 := newTestMessage(8)
	pm := newTestMessage(16)

	_, done := b.AddFragment(kAt(1, 150), em1, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 250), em2, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 400), em3, Entry, 0, true, true)
	require.False(t, done)

	// lo=100 (exclusive), hi=300 (inclusive) → matches depths 150, 250.
	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	shared.ReleaseBase()
	require.Len(t, readys, 2)
	for _, r := range readys {
		assert.True(t, r.PanicUnwound)
		r.Entry.Release()
		// Releasing both Returns decrements the shared payload from
		// 2 → 1 → 0 and the underlying message is released once.
		r.Return.Release()
	}
	assert.True(t, pm.released, "shared released after last handle")
	assert.False(t, em3.released, "depth 400 outside range, still in-flight")
	assert.Equal(t, 1, b.Len())

	// Cleanup the out-of-range entry.
	b.Discard(kAt(1, 400))
}

// Under budget pressure, every match of a panic-unwound fanout is
// pinned against eviction for the duration of the per-match mutation
// loop, so the loop never observes an evicted *bufferedEvent. Set up
// three in-flight invocations with incomplete entry sides (so the
// fanout cannot finalize them and the handle bytes stay charged) and
// a budget that fits only the entries plus one handle; verify that
// admitting the fanout leaves all three matches in the tree and emits
// no budget evictions.
func TestBuffer_PanicUnwoundRange_NoEvictionOfMatches(t *testing.T) {
	const entryBytes = 8
	const handleBytes = 16
	// 3 entries + 3 handles = 72 bytes needed; budget = entries + one
	// handle = 40 bytes. The fanout's upfront charge admits all three
	// handles over-limit because each match is in the pin set.
	b := NewBuffer(NewBudget(3*entryBytes + handleBytes))
	em1 := newTestMessage(entryBytes)
	em2 := newTestMessage(entryBytes)
	em3 := newTestMessage(entryBytes)
	pm := newTestMessage(handleBytes)

	// isFinal=false → entry side incomplete; tryFinalize returns false
	// after the panic-unwound handle attaches, so bytes stay charged
	// across the fanout loop and the budget stays under pressure.
	_, done := b.AddFragment(kAt(1, 150), em1, Entry, 0, false, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 200), em2, Entry, 0, false, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 250), em3, Entry, 0, false, true)
	require.False(t, done)

	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	shared.ReleaseBase()

	// None finalize yet (entries are incomplete), all three matches
	// remain in the tree, and none surface as truncated budget evictions.
	require.Empty(t, readys, "matches deferred while entries are incomplete")
	assert.Equal(t, 3, b.Len(), "all three matches must still be in the tree")
	assert.Empty(t, b.TakePendingBudgetEvictions(),
		"matches must be protected from budget eviction during fanout")

	// Cleanup: discard each in-flight invocation. Each Discard releases
	// the entry fragment(s) and the panic-unwound handle. After the
	// third Discard, all three handles have drained → underlying pm
	// is released exactly once.
	b.Discard(kAt(1, 150))
	b.Discard(kAt(1, 200))
	b.Discard(kAt(1, 250))
	assert.True(t, pm.released, "shared released after all handles drain")
	assert.Equal(t, 0, b.Len())
}

// Range scan that finds no matching entries: the caller is
// responsible for releasing the shared base (no Acquires happened).
func TestBuffer_PanicUnwoundRange_NoMatches(t *testing.T) {
	b := newTestBuffer()
	pm := newTestMessage(16)
	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	require.Empty(t, readys)
	// Caller releases the unused base; underlying message is released.
	shared.ReleaseBase()
	assert.True(t, pm.released)
}

// Range scan stops at the goid boundary: a frame on a different goid
// within the same depth range is not affected.
func TestBuffer_PanicUnwoundRange_StopsAtGoidBoundary(t *testing.T) {
	b := newTestBuffer()
	emA := newTestMessage(8)
	emB := newTestMessage(8)
	pm := newTestMessage(16)

	_, done := b.AddFragment(kAt(1, 200), emA, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(2, 200), emB, Entry, 0, true, true)
	require.False(t, done)

	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	shared.ReleaseBase()
	require.Len(t, readys, 1)
	assert.Equal(t, uint64(1), readys[0].Key.Goid)
	readys[0].Entry.Release()
	readys[0].Return.Release()
	assert.True(t, pm.released)
	assert.False(t, emB.released, "goid 2 entry untouched by goid 1 unwind")

	b.Discard(kAt(2, 200))
}

// Range scan skips an entry that already has a real return-side
// fragment buffered (the regular return-side pairing will finalize it
// independently). The shared payload is unreferenced from that entry;
// other matches still receive their handles.
func TestBuffer_PanicUnwoundRange_SkipsEntryWithReturn(t *testing.T) {
	b := newTestBuffer()
	em1 := newTestMessage(8)
	em2 := newTestMessage(8)
	r2 := newTestMessage(4)
	pm := newTestMessage(16)

	_, done := b.AddFragment(kAt(1, 150), em1, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 250), em2, Entry, 0, true, true)
	require.False(t, done)
	// A real return fragment landed for depth=250 first.
	_, done = b.AddFragment(kAt(1, 250), r2, Return, 0, false, false)
	require.False(t, done, "return not final yet")

	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	shared.ReleaseBase()
	// Only depth=150 finalizes; depth=250 keeps its real return.
	require.Len(t, readys, 1)
	assert.Equal(t, uint32(150), readys[0].Key.StackByteDepth)
	readys[0].Entry.Release()
	readys[0].Return.Release()
	assert.True(t, pm.released, "shared released after the one handle drains")
	assert.False(t, r2.released, "real return preserved for depth 250")
	assert.Equal(t, 1, b.Len())

	b.Discard(kAt(1, 250))
}

// A range scan that attaches a handle to an invocation whose entry side
// is still in flight does not finalize the invocation. ReleaseBase
// must defer the underlying release until the entry completes and the
// outstanding handle drains.
func TestBuffer_PanicUnwoundRange_DeferredFinalize(t *testing.T) {
	b := newTestBuffer()
	e0 := newTestMessage(8)
	e1 := newTestMessage(8)
	pm := newTestMessage(16)

	// First entry fragment, not final — entry side will not be ready
	// when NotePanicUnwoundRange runs.
	_, done := b.AddFragment(kAt(1, 200), e0, Entry, 0, false, true)
	require.False(t, done)

	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	require.Empty(t, readys, "entry incomplete; finalization deferred")

	// Caller signals end-of-Acquire phase. A handle is outstanding in
	// the bufferedEvent's returnList, so this must NOT release pm yet.
	shared.ReleaseBase()
	assert.False(t, pm.released, "handle outstanding; underlying must not release")

	// Final entry fragment arrives. The buffered event already has the
	// synthetic return-side fragment from NotePanicUnwoundRange, so this
	// finalizes immediately.
	ready, done := b.AddFragment(kAt(1, 200), e1, Entry, 1, true, true)
	require.True(t, done)
	assert.True(t, ready.PanicUnwound)
	ready.Entry.Release()
	ready.Return.Release()
	assert.True(t, pm.released, "released only after the last handle drains")
}

// Range scan matches one entry that's complete (finalizes immediately)
// and one whose entry is still in flight (handle stored, finalization
// deferred). The caller invokes ReleaseBase between the two finalization
// stages; the underlying must survive until both handles drain.
func TestBuffer_PanicUnwoundRange_MixedFinalize(t *testing.T) {
	b := newTestBuffer()
	e150 := newTestMessage(8)  // depth=150, single-fragment entry
	e250a := newTestMessage(8) // depth=250, fragment 0 (not final)
	e250b := newTestMessage(8) // depth=250, fragment 1 (final)
	pm := newTestMessage(16)

	_, done := b.AddFragment(kAt(1, 150), e150, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 250), e250a, Entry, 0, false, true)
	require.False(t, done)

	shared := NewSharedMessage(pm)
	readys := b.NotePanicUnwoundRange(1, 100, 300, shared)
	require.Len(t, readys, 1, "depth=150 finalizes, depth=250 deferred")
	assert.Equal(t, uint32(150), readys[0].Key.StackByteDepth)

	// Caller ends the Acquire phase. Two handles are alive: one in the
	// returned Ready, one in the deferred bufferedEvent. ReleaseBase
	// must not release pm yet.
	shared.ReleaseBase()
	assert.False(t, pm.released, "two handles outstanding")

	// Drain the immediate Ready. One handle drains; pm still has a
	// handle in the deferred bufferedEvent.
	readys[0].Entry.Release()
	readys[0].Return.Release()
	assert.False(t, pm.released, "one handle still outstanding")

	// Final entry fragment for depth=250 arrives → finalize.
	ready, done := b.AddFragment(kAt(1, 250), e250b, Entry, 1, true, true)
	require.True(t, done)
	ready.Entry.Release()
	ready.Return.Release()
	assert.True(t, pm.released, "released after the last handle drains")
}

// ------------------------------------------------------------------
// PANIC_UNWOUND_LOST (range drop notification).
// ------------------------------------------------------------------

// The BPF synthetic recovery event was dropped. For a single complete
// entry in the range, NotePanicUnwoundRangeLost finalizes it as a
// truncated panic-unwound capture (no return-side payload).
func TestBuffer_PanicUnwoundRangeLost_SingleFrame(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)

	_, done := b.AddFragment(kAt(1, 200), em, Entry, 0, true, true)
	require.False(t, done, "entry alone is not final for paired probe")

	readys := b.NotePanicUnwoundRangeLost(1, 100, 300)
	require.Len(t, readys, 1)
	r := readys[0]
	assert.True(t, r.PanicUnwound, "marked as panic-unwound")
	assert.True(t, r.ReturnLost, "no return payload survived")
	require.NotNil(t, r.Entry)
	assert.Nil(t, r.Return, "no return fragments — synthetic was dropped")
	r.Entry.Release()
	assert.True(t, em.released)
	assert.Equal(t, 0, b.Len())
}

// Multiple complete entries on the same goid in the unwound range all
// finalize as panic-unwound + return-lost from a single drop notification.
func TestBuffer_PanicUnwoundRangeLost_MultiFrame(t *testing.T) {
	b := newTestBuffer()
	em1 := newTestMessage(8)
	em2 := newTestMessage(8)
	em3 := newTestMessage(8)

	_, done := b.AddFragment(kAt(1, 150), em1, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 250), em2, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 400), em3, Entry, 0, true, true)
	require.False(t, done)

	readys := b.NotePanicUnwoundRangeLost(1, 100, 300)
	require.Len(t, readys, 2, "depths 150, 250 in range; 400 not")
	for _, r := range readys {
		assert.True(t, r.PanicUnwound)
		assert.True(t, r.ReturnLost)
		assert.Nil(t, r.Return)
		r.Entry.Release()
	}
	assert.True(t, em1.released)
	assert.True(t, em2.released)
	assert.False(t, em3.released, "depth 400 outside range, still in-flight")
	assert.Equal(t, 1, b.Len())

	b.Discard(kAt(1, 400))
}

// Range scan stops at the goid boundary: an in-range frame on a
// different goid is not affected.
func TestBuffer_PanicUnwoundRangeLost_StopsAtGoidBoundary(t *testing.T) {
	b := newTestBuffer()
	emA := newTestMessage(8)
	emB := newTestMessage(8)

	_, done := b.AddFragment(kAt(1, 200), emA, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(2, 200), emB, Entry, 0, true, true)
	require.False(t, done)

	readys := b.NotePanicUnwoundRangeLost(1, 100, 300)
	require.Len(t, readys, 1)
	assert.Equal(t, uint64(1), readys[0].Key.Goid)
	readys[0].Entry.Release()
	assert.True(t, emA.released)
	assert.False(t, emB.released, "goid 2 entry untouched")

	b.Discard(kAt(2, 200))
}

// No matching invocations: empty Readys slice, no state change.
func TestBuffer_PanicUnwoundRangeLost_NoMatches(t *testing.T) {
	b := newTestBuffer()
	readys := b.NotePanicUnwoundRangeLost(1, 100, 300)
	require.Empty(t, readys)
	assert.Equal(t, 0, b.Len())
}

// A frame whose entry side is incomplete (multi-fragment, last fragment
// in flight) gets the panic-unwound + return-lost markers stored on the
// bufferedEvent but does NOT finalize yet — there are still pending
// entry fragments. The final entry fragment finalizes the truncated
// panic-unwound capture.
func TestBuffer_PanicUnwoundRangeLost_DeferredFinalize(t *testing.T) {
	b := newTestBuffer()
	e0 := newTestMessage(8)
	e1 := newTestMessage(8)

	_, done := b.AddFragment(kAt(1, 200), e0, Entry, 0, false, true)
	require.False(t, done)

	readys := b.NotePanicUnwoundRangeLost(1, 100, 300)
	require.Empty(t, readys, "entry incomplete; finalization deferred")

	ready, done := b.AddFragment(kAt(1, 200), e1, Entry, 1, true, true)
	require.True(t, done)
	assert.True(t, ready.PanicUnwound)
	assert.True(t, ready.ReturnLost)
	assert.Nil(t, ready.Return)
	ready.Entry.Release()
	assert.True(t, e0.released)
	assert.True(t, e1.released)
}

// A frame that already has a real return fragment buffered is skipped
// (invariant violation logged) so the regular pairing finalizes it.
func TestBuffer_PanicUnwoundRangeLost_SkipsEntryWithReturn(t *testing.T) {
	b := newTestBuffer()
	em1 := newTestMessage(8)
	em2 := newTestMessage(8)
	r2 := newTestMessage(4)

	_, done := b.AddFragment(kAt(1, 150), em1, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.AddFragment(kAt(1, 250), em2, Entry, 0, true, true)
	require.False(t, done)
	// A real return fragment landed for depth=250 first.
	_, done = b.AddFragment(kAt(1, 250), r2, Return, 0, false, false)
	require.False(t, done)

	readys := b.NotePanicUnwoundRangeLost(1, 100, 300)
	// Only depth=150 finalizes; depth=250 keeps its real return path.
	require.Len(t, readys, 1)
	assert.Equal(t, uint32(150), readys[0].Key.StackByteDepth)
	assert.True(t, readys[0].ReturnLost)
	readys[0].Entry.Release()
	assert.True(t, em1.released)
	assert.False(t, r2.released, "real return preserved for depth 250")

	b.Discard(kAt(1, 250))
}

// ------------------------------------------------------------------
// PARTIAL_ENTRY.
// ------------------------------------------------------------------

// 10. Fragment 0 (not marked final) + PARTIAL_ENTRY(last_seq=0) → emit truncated.
// This represents: fragment 0 was sent with HasMoreFragments=true, then BPF
// discovered it couldn't send more, so sent PARTIAL_ENTRY(last_seq=0).
func TestBuffer_PartialEntry_ImmediateAfterFragments(t *testing.T) {
	b := newTestBuffer()
	e0 := newTestMessage(8)

	_, done := b.AddFragment(k(1, 1000), e0, Entry, 0, false, false)
	require.False(t, done)

	r, done := b.NotePartial(k(1, 1000), Entry, 0)
	require.True(t, done)
	assert.True(t, r.EntryTruncated)
	assert.False(t, r.ReturnTruncated)
	assert.Nil(t, r.Return)
	r.Entry.Release()
}

// 11. Fragments 0, 1 + PARTIAL_ENTRY(last_seq=1); then return arrives → paired, entry truncated.
func TestBuffer_PartialEntry_ThenReturn(t *testing.T) {
	b := newTestBuffer()
	e0, e1 := newTestMessage(8), newTestMessage(8)

	_, done := b.AddFragment(k(1, 1000), e0, Entry, 0, false, true)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), e1, Entry, 1, false /*!isFinal*/, true)
	require.False(t, done)

	_, done = b.NotePartial(k(1, 1000), Entry, 1) // last_seq=1 -> expect 2 fragments
	require.False(t, done, "entry assembled as truncated but return still expected")

	rm := newTestMessage(4)
	r, done := b.AddFragment(k(1, 1000), rm, Return, 0, true, false)
	require.True(t, done)
	assert.True(t, r.EntryTruncated)
	assert.False(t, r.ReturnTruncated)
	r.Entry.Release()
	r.Return.Release()
}

// 12. PARTIAL_ENTRY arrives before any fragments; fragments arrive later; finalize.
func TestBuffer_PartialEntry_BeforeFragments(t *testing.T) {
	b := newTestBuffer()
	_, done := b.NotePartial(k(1, 1000), Entry, 1)
	require.False(t, done)

	e0 := newTestMessage(8)
	_, done = b.AddFragment(k(1, 1000), e0, Entry, 0, false, false)
	require.False(t, done, "expected 2 fragments, only have 1")

	e1 := newTestMessage(8)
	r, done := b.AddFragment(k(1, 1000), e1, Entry, 1, false, false)
	require.True(t, done, "2 fragments received, matches expected")
	assert.True(t, r.EntryTruncated)
	r.Entry.Release()
}

//  13. PARTIAL_ENTRY after fragments 0, 1 already present (both non-final) →
//     expected = last_seq+1 = 2, so immediate finalize.
func TestBuffer_PartialEntry_AfterFragmentsAlreadyPresent(t *testing.T) {
	b := newTestBuffer()
	e0, e1 := newTestMessage(8), newTestMessage(8)
	_, done := b.AddFragment(k(1, 1000), e0, Entry, 0, false, false)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), e1, Entry, 1, false, false)
	require.False(t, done)

	r, done := b.NotePartial(k(1, 1000), Entry, 1)
	require.True(t, done)
	assert.True(t, r.EntryTruncated)
	r.Entry.Release()
}

// 14. PARTIAL_ENTRY(last_seq=2) with only fragments 0, 1 present → wait for seq 2.
func TestBuffer_PartialEntry_WaitForMoreFragments(t *testing.T) {
	b := newTestBuffer()
	e0, e1 := newTestMessage(8), newTestMessage(8)
	_, done := b.AddFragment(k(1, 1000), e0, Entry, 0, false, false)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), e1, Entry, 1, false, false)
	require.False(t, done)

	_, done = b.NotePartial(k(1, 1000), Entry, 2)
	require.False(t, done, "only 2 of 3 fragments received")

	e2 := newTestMessage(8)
	r, done := b.AddFragment(k(1, 1000), e2, Entry, 2, false, false)
	require.True(t, done)
	assert.True(t, r.EntryTruncated)
	r.Entry.Release()
}

// ------------------------------------------------------------------
// PARTIAL_RETURN.
// ------------------------------------------------------------------

// 15. Entry complete + return fragments + PARTIAL_RETURN → emit paired, truncated.
func TestBuffer_PartialReturn_AfterFragments(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)
	_, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.False(t, done)
	r0, r1 := newTestMessage(4), newTestMessage(4)
	_, done = b.AddFragment(k(1, 1000), r0, Return, 0, false, false)
	require.False(t, done)
	_, done = b.AddFragment(k(1, 1000), r1, Return, 1, false, false)
	require.False(t, done)

	r, done := b.NotePartial(k(1, 1000), Return, 1)
	require.True(t, done)
	assert.True(t, r.ReturnTruncated)
	assert.False(t, r.EntryTruncated)
	r.Entry.Release()
	r.Return.Release()
}

// 16. PARTIAL_RETURN before return fragments → fragments arrive → finalize.
func TestBuffer_PartialReturn_BeforeFragments(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)
	_, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.False(t, done)
	_, done = b.NotePartial(k(1, 1000), Return, 0)
	require.False(t, done)
	rm := newTestMessage(4)
	r, done := b.AddFragment(k(1, 1000), rm, Return, 0, false, false)
	require.True(t, done)
	assert.True(t, r.ReturnTruncated)
	r.Entry.Release()
	r.Return.Release()
}

// 17. PARTIAL_RETURN before entry; entry arrives; return arrives; finalize truncated.
func TestBuffer_PartialReturn_BeforeEntry(t *testing.T) {
	b := newTestBuffer()
	_, done := b.NotePartial(k(1, 1000), Return, 0)
	require.False(t, done)

	em := newTestMessage(8)
	_, done = b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.False(t, done)

	rm := newTestMessage(4)
	r, done := b.AddFragment(k(1, 1000), rm, Return, 0, false, false)
	require.True(t, done)
	assert.True(t, r.ReturnTruncated)
	r.Entry.Release()
	r.Return.Release()
}

// ------------------------------------------------------------------
// Rapid re-invocation.
// ------------------------------------------------------------------

// 18. Call N entry + RETURN_LOST(N) + call N+1 entry + normal N+1 return.
func TestBuffer_RapidReinvocation_DifferentEntryKtime(t *testing.T) {
	b := newTestBuffer()
	em1 := newTestMessage(8)
	em2 := newTestMessage(8)

	_, done := b.AddFragment(k(1, 1000), em1, Entry, 0, true, true)
	require.False(t, done)
	r1, done := b.NoteReturnLost(k(1, 1000))
	require.True(t, done, "RETURN_LOST on complete entry finalizes")
	assert.True(t, r1.ReturnLost)
	r1.Entry.Release()

	_, done = b.AddFragment(k(1, 2000), em2, Entry, 0, true, true)
	require.False(t, done)
	rm2 := newTestMessage(4)
	r2, done := b.AddFragment(k(1, 2000), rm2, Return, 0, true, false)
	require.True(t, done)
	assert.False(t, r2.ReturnLost)
	r2.Entry.Release()
	r2.Return.Release()
}

// 19. Interleaved N and N+1 fragments in the buffer simultaneously.
func TestBuffer_RapidReinvocation_SimultaneousInTree(t *testing.T) {
	b := newTestBuffer()
	e1a, e1b := newTestMessage(8), newTestMessage(8)
	e2a, e2b := newTestMessage(8), newTestMessage(8)

	// Call N (EntryKtime=1000): fragment 0.
	_, done := b.AddFragment(k(1, 1000), e1a, Entry, 0, false, false)
	require.False(t, done)
	// Call N+1 (EntryKtime=2000): fragment 0.
	_, done = b.AddFragment(k(1, 2000), e2a, Entry, 0, false, false)
	require.False(t, done)
	assert.Equal(t, 2, b.Len(), "two independent invocations in the tree")

	// Finish call N.
	r1, done := b.AddFragment(k(1, 1000), e1b, Entry, 1, true, false)
	require.True(t, done)
	r1.Entry.Release()

	// Call N+1 still present.
	assert.Equal(t, 1, b.Len())

	// Finish call N+1.
	r2, done := b.AddFragment(k(1, 2000), e2b, Entry, 1, true, false)
	require.True(t, done)
	r2.Entry.Release()
	assert.Equal(t, 0, b.Len())
}

//  20. Stale notification for N arrives after N has finalized; creates a
//     zombie entry; EvictOlderThan with a later ktime cleans it up.
func TestBuffer_StaleNotification_GCedByEvictOlderThan(t *testing.T) {
	b := newTestBuffer()
	// Complete call N (entryKtime=1000).
	em := newTestMessage(8)
	_, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, false)
	require.True(t, done)

	// Stale NoteReturnLost for N: creates a new entry with no fragments.
	_, done = b.NoteReturnLost(k(1, 1000))
	require.False(t, done, "no fragments means can't finalize")
	assert.Equal(t, 1, b.Len(), "zombie entry persists")

	// Evict entries whose EntryKtime <= 1000: the zombie has key
	// EntryKtime=1000, so it qualifies.
	evicted := b.EvictOlderThan(1000)
	require.NotEmpty(t, evicted)
	found := false
	for _, r := range evicted {
		if r.Key == k(1, 1000) {
			found = true
			assert.True(t, r.ReturnLost)
			// No entry fragments, so Entry should be nil.
			assert.Nil(t, r.Entry)
		}
	}
	assert.True(t, found, "zombie entry should have been evicted")
	assert.Equal(t, 0, b.Len())
}

// ------------------------------------------------------------------
// Edge cases.
// ------------------------------------------------------------------

// 21. Single-fragment event (HasMoreFragments=false, seq=0).
func TestBuffer_SingleFragmentStandalone(t *testing.T) {
	b := newTestBuffer()
	m := newTestMessage(8)
	r, done := b.AddFragment(k(1, 1000), m, Entry, 0, true, false)
	require.True(t, done)
	r.Entry.Release()
}

// 22. EvictOlderThan: entries whose EntryKtime <= cutoff are evicted.
func TestBuffer_EvictOlderThan_Cutoff(t *testing.T) {
	b := newTestBuffer()
	// Five entries at entryKtime 1000, 2000, 3000, 4000, 5000.
	for i := uint64(1); i <= 5; i++ {
		mm := newTestMessage(4)
		_, _ = b.AddFragment(k(i, i*1000), mm, Entry, 0, false, false)
	}

	// Cutoff 3500: expect entries at 1000, 2000, 3000 evicted; 4000
	// and 5000 remain.
	evicted := b.EvictOlderThan(3500)
	keys := make(map[Key]bool)
	for _, r := range evicted {
		keys[r.Key] = true
		if r.Entry != nil {
			r.Entry.Release()
		}
	}
	assert.True(t, keys[k(1, 1000)])
	assert.True(t, keys[k(2, 2000)])
	assert.True(t, keys[k(3, 3000)])
	assert.False(t, keys[k(4, 4000)])
	assert.False(t, keys[k(5, 5000)])
	assert.Equal(t, 2, b.Len(), "entries 4 and 5 remain")
}

// EvictOlderThan with an empty buffer is a no-op.
func TestBuffer_EvictOlderThan_Empty(t *testing.T) {
	b := newTestBuffer()
	assert.Nil(t, b.EvictOlderThan(1000))
}

// EvictOlderThan when all entries are newer than the cutoff returns nil.
func TestBuffer_EvictOlderThan_AllNewer(t *testing.T) {
	b := newTestBuffer()
	m := newTestMessage(8)
	_, _ = b.AddFragment(k(1, 5000), m, Entry, 0, false, false)
	assert.Nil(t, b.EvictOlderThan(1000))
	assert.Equal(t, 1, b.Len())
}

// EvictOlderThan is inclusive: an entry with EntryKtime == cutoff is
// evicted.
func TestBuffer_EvictOlderThan_BoundaryInclusive(t *testing.T) {
	b := newTestBuffer()
	m := newTestMessage(8)
	_, _ = b.AddFragment(k(1, 1000), m, Entry, 0, false, false)
	ev := b.EvictOlderThan(1000)
	require.Len(t, ev, 1)
	if ev[0].Entry != nil {
		ev[0].Entry.Release()
	}
	assert.Equal(t, 0, b.Len())
}

// Repeated EvictOlderThan calls with increasing cutoffs evict in waves
// without double-emitting.
func TestBuffer_EvictOlderThan_RepeatedWaves(t *testing.T) {
	b := newTestBuffer()
	for i := uint64(1); i <= 5; i++ {
		mm := newTestMessage(4)
		_, _ = b.AddFragment(k(i, i*1000), mm, Entry, 0, false, false)
	}
	first := b.EvictOlderThan(2500)
	for _, r := range first {
		if r.Entry != nil {
			r.Entry.Release()
		}
	}
	assert.Len(t, first, 2)

	second := b.EvictOlderThan(4500)
	for _, r := range second {
		if r.Entry != nil {
			r.Entry.Release()
		}
	}
	assert.Len(t, second, 2)
	assert.Equal(t, 1, b.Len(), "only the k=5,5000 entry remains")
}

// TestBuffer_Discard: the condition-failed signal path. An entry is stored
// and then Discarded; no Ready is emitted and the message is released.
func TestBuffer_Discard(t *testing.T) {
	b := newTestBuffer()
	em := newTestMessage(8)
	_, done := b.AddFragment(k(1, 1000), em, Entry, 0, true, true)
	require.False(t, done)
	require.Equal(t, 1, b.Len())

	b.Discard(k(1, 1000))
	assert.Equal(t, 0, b.Len())
	assert.True(t, em.released)

	// Discard on an empty key is a no-op.
	b.Discard(k(42, 42))
}

// 23. Close() releases everything and returns Readys for in-flight entries.
func TestBuffer_Close(t *testing.T) {
	b := newTestBuffer()
	var msgs []*testMessage
	for i := uint64(1); i < 5; i++ {
		m := newTestMessage(8)
		msgs = append(msgs, m)
		_, _ = b.AddFragment(k(i, i*1000), m, Entry, 0, false, false)
	}

	readys := b.Close()
	require.Len(t, readys, 4)
	for _, r := range readys {
		assert.True(t, r.EntryTruncated, "incomplete entries should be flagged truncated")
		r.Entry.Release()
	}
	assert.Equal(t, 0, b.Len())
	for i, m := range msgs {
		assert.Truef(t, m.released, "message %d should be released", i)
	}
}
