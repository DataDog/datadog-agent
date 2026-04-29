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

// TestBudget_UnderLimit_NoEvictions: a buffer that stays under the budget
// never produces any eviction-driven Readys.
func TestBudget_UnderLimit_NoEvictions(t *testing.T) {
	budget := NewBudget(1024)
	b := NewBuffer(budget)

	m0, m1 := newTestMessage(100), newTestMessage(100)
	_, done := b.AddFragment(k(1, 1000), m0, Entry, 0, false, false)
	require.False(t, done)
	assert.Empty(t, b.TakePendingBudgetEvictions())
	assert.Equal(t, 100, b.Bytes())
	assert.Equal(t, 100, budget.Used())

	_, done = b.AddFragment(k(1, 1000), m1, Entry, 1, true, false)
	require.True(t, done)
	assert.Empty(t, b.TakePendingBudgetEvictions())
	// On finalize (done=true), the Ready becomes the caller's responsibility.
	// The buffer has credited the budget back since the entry left the tree.
	assert.Equal(t, 0, b.Bytes())
	assert.Equal(t, 0, budget.Used())
}

// TestBudget_FinalizeCreditsBudget: when an invocation finalizes (Ready
// emitted), its bytes are credited back even before the Ready's lists are
// Released. The Ready carries the MessageLists which the caller owns.
func TestBudget_FinalizeCreditsBudget(t *testing.T) {
	budget := NewBudget(1024)
	b := NewBuffer(budget)

	m := newTestMessage(64)
	r, done := b.AddFragment(k(1, 1000), m, Entry, 0, true, false)
	require.True(t, done)
	assert.Equal(t, 0, b.Bytes())
	assert.Equal(t, 0, budget.Used())

	// Releasing the Ready's message doesn't double-credit.
	r.Entry.Release()
	assert.Equal(t, 0, b.Bytes())
	assert.Equal(t, 0, budget.Used())
}

// TestBudget_DiscardCreditsBudget: Discard releases the entry's bytes.
func TestBudget_DiscardCreditsBudget(t *testing.T) {
	budget := NewBudget(1024)
	b := NewBuffer(budget)

	m := newTestMessage(64)
	_, done := b.AddFragment(k(1, 1000), m, Entry, 0, false, true)
	require.False(t, done)
	assert.Equal(t, 64, budget.Used())

	b.Discard(k(1, 1000))
	assert.Equal(t, 0, b.Bytes())
	assert.Equal(t, 0, budget.Used())
	assert.True(t, m.released)
}

// TestBudget_EvictsOldestWhenPressured: when a new fragment would push the
// buffer over the budget, the oldest entry is evicted (as a truncated
// Ready in pendingBudgetEvictions) so the new fragment can fit.
func TestBudget_EvictsOldestWhenPressured(t *testing.T) {
	// Budget fits two 100-byte fragments but not three.
	budget := NewBudget(250)
	b := NewBuffer(budget)

	// Three separate invocations, each holding one fragment.
	// Budget after each: 100, 200, then the third triggers eviction.
	m1 := newTestMessage(100)
	m2 := newTestMessage(100)
	m3 := newTestMessage(100)

	_, _ = b.AddFragment(k(1, 1000), m1, Entry, 0, false, false)
	assert.Empty(t, b.TakePendingBudgetEvictions())
	assert.Equal(t, 100, budget.Used())

	_, _ = b.AddFragment(k(2, 2000), m2, Entry, 0, false, false)
	assert.Empty(t, b.TakePendingBudgetEvictions())
	assert.Equal(t, 200, budget.Used())

	// Third fragment: 200+100 > 250, so the oldest (key 1, touch=1) must
	// be evicted. After eviction: 100 (k2) + 100 (k3) = 200, fits.
	_, _ = b.AddFragment(k(3, 3000), m3, Entry, 0, false, false)
	evicted := b.TakePendingBudgetEvictions()
	require.Len(t, evicted, 1)
	assert.Equal(t, k(1, 1000), evicted[0].Key)
	assert.True(t, evicted[0].EntryTruncated,
		"evicted entry should be marked truncated")
	assert.Equal(t, 200, budget.Used())
	assert.Equal(t, 2, b.Len(), "k=2 and k=3 remain")

	// Release the evicted Ready's messages.
	evicted[0].Entry.Release()
}

// TestBudget_EvictsMultipleEntriesIfOneNotEnough: if evicting one entry
// doesn't free enough bytes, more get evicted.
func TestBudget_EvictsMultipleEntriesIfOneNotEnough(t *testing.T) {
	budget := NewBudget(100)
	b := NewBuffer(budget)

	// Two invocations at 40 bytes each → 80 used.
	m1 := newTestMessage(40)
	m2 := newTestMessage(40)
	_, _ = b.AddFragment(k(1, 1000), m1, Entry, 0, false, false)
	_, _ = b.AddFragment(k(2, 2000), m2, Entry, 0, false, false)
	assert.Equal(t, 80, budget.Used())

	// A 70-byte new fragment: 80+70 > 100. Evicting one 40-byte entry
	// leaves 40, and 40+70 > 100 still. Must evict both to free 80 for
	// the single 70-byte fragment.
	m3 := newTestMessage(70)
	_, _ = b.AddFragment(k(3, 3000), m3, Entry, 0, false, false)
	evicted := b.TakePendingBudgetEvictions()
	require.Len(t, evicted, 2)
	assert.Equal(t, 70, budget.Used())
	for _, r := range evicted {
		assert.True(t, r.EntryTruncated)
		r.Entry.Release()
	}
}

// TestBudget_DoesNotEvictTheEntryWeAreExtending: if the key being appended
// to is already in the tree, it's excluded from eviction candidates.
func TestBudget_DoesNotEvictTheEntryWeAreExtending(t *testing.T) {
	budget := NewBudget(100)
	b := NewBuffer(budget)

	// Fill the budget with one large invocation.
	m1 := newTestMessage(80)
	_, _ = b.AddFragment(k(1, 1000), m1, Entry, 0, false, false)
	assert.Equal(t, 80, budget.Used())

	// Extend the same invocation with a 30-byte fragment. 80+30 > 100 —
	// but the eviction loop excludes k=1 (the key we're extending), and
	// there's nothing else to evict, so we over-commit.
	m2 := newTestMessage(30)
	_, _ = b.AddFragment(k(1, 1000), m2, Entry, 1, false, false)
	// No eviction Readys because the only candidate is the one we're
	// extending.
	assert.Empty(t, b.TakePendingBudgetEvictions())
	// Used goes over the limit; this is the documented degenerate case.
	assert.Equal(t, 110, budget.Used())
	assert.Equal(t, 1, b.Len())
}

// TestBudget_SharedAcrossBuffers: two buffers share one Budget; one
// buffer's pressure causes eviction in that same buffer, not the other.
func TestBudget_SharedAcrossBuffers(t *testing.T) {
	budget := NewBudget(200)
	b1 := NewBuffer(budget)
	b2 := NewBuffer(budget)

	m1 := newTestMessage(100)
	m2 := newTestMessage(100)
	_, _ = b1.AddFragment(k(1, 1000), m1, Entry, 0, false, false)
	_, _ = b2.AddFragment(k(2, 2000), m2, Entry, 0, false, false)
	assert.Equal(t, 200, budget.Used())
	assert.Equal(t, 1, b1.Len())
	assert.Equal(t, 1, b2.Len())

	// A 50-byte fragment arriving at b1 must evict b1's own entry (k=1).
	// b2's entry (k=2) is untouched — we never evict another buffer's
	// entries, even though they're consuming shared budget.
	m3 := newTestMessage(50)
	_, _ = b1.AddFragment(k(3, 3000), m3, Entry, 0, false, false)
	evicted := b1.TakePendingBudgetEvictions()
	require.Len(t, evicted, 1)
	assert.Equal(t, k(1, 1000), evicted[0].Key)
	evicted[0].Entry.Release()

	// After eviction b1 holds 50 (k3), b2 holds 100 (k2); total 150.
	assert.Equal(t, 150, budget.Used())
	assert.Equal(t, 1, b1.Len())
	assert.Equal(t, 1, b2.Len())
}

// TestBudget_FragmentLargerThanLimit: if a single fragment exceeds the
// entire budget, we admit it over-limit rather than rejecting.
func TestBudget_FragmentLargerThanLimit(t *testing.T) {
	budget := NewBudget(100)
	b := NewBuffer(budget)

	big := newTestMessage(500)
	_, _ = b.AddFragment(k(1, 1000), big, Entry, 0, false, false)
	assert.Equal(t, 500, budget.Used(),
		"fragments larger than the budget are admitted over-limit")
	assert.Empty(t, b.TakePendingBudgetEvictions())
	assert.Equal(t, 1, b.Len())

	// Another small fragment on a different key: we'll try to evict the
	// big one to fit, and succeed.
	small := newTestMessage(50)
	_, _ = b.AddFragment(k(2, 2000), small, Entry, 0, false, false)
	evicted := b.TakePendingBudgetEvictions()
	require.Len(t, evicted, 1)
	assert.Equal(t, k(1, 1000), evicted[0].Key)
	evicted[0].Entry.Release()
	assert.Equal(t, 50, budget.Used())
}

// TestBudget_CloseReleasesAll: closing a buffer credits back all its
// bytes to the shared budget.
func TestBudget_CloseReleasesAll(t *testing.T) {
	budget := NewBudget(1024)
	b := NewBuffer(budget)

	_, _ = b.AddFragment(k(1, 1000), newTestMessage(30), Entry, 0, false, false)
	_, _ = b.AddFragment(k(2, 2000), newTestMessage(40), Entry, 0, false, false)
	_, _ = b.AddFragment(k(3, 3000), newTestMessage(50), Entry, 0, false, false)
	assert.Equal(t, 120, budget.Used())

	readys := b.Close()
	require.Len(t, readys, 3)
	for _, r := range readys {
		r.Entry.Release()
	}
	assert.Equal(t, 0, budget.Used())
	assert.Equal(t, 0, b.Bytes())
}

// TestBudget_VeryLargeBudgetNeverEvicts: a buffer whose budget is so
// large the workload cannot exhaust it never produces eviction Readys
// even though the budget is active.
func TestBudget_VeryLargeBudgetNeverEvicts(t *testing.T) {
	budget := NewBudget(1 << 30)
	b := NewBuffer(budget)

	for i := uint64(1); i < 10; i++ {
		_, _ = b.AddFragment(k(i, i*1000), newTestMessage(1000), Entry, 0, false, false)
	}
	assert.Equal(t, 9000, b.Bytes())
	assert.Equal(t, 9000, budget.Used())
	assert.Equal(t, 9, b.Len())
	assert.Empty(t, b.TakePendingBudgetEvictions())
	for _, r := range b.Close() {
		r.Entry.Release()
	}
	assert.Equal(t, 0, b.Bytes())
	assert.Equal(t, 0, budget.Used())
}
