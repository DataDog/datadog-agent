// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package slidingwindow

// --------------------------------------------------------------------------
// Test-suite design notes
// --------------------------------------------------------------------------
// Each test is labelled with the specific implementation mistake it catches.
// A naive/incorrect LLM-generated solution will fail one or more of these.
//
// Failure modes targeted:
//   FM-1  Using `>` instead of `>=` for the staleness boundary (off-by-one).
//   FM-2  Forgetting to zero a ring-buffer slot before reusing it (accumulation
//         across window cycles).
//   FM-3  Maintaining a running `total` that is never corrected for stale data
//         when Count() is called without a preceding Add() (phantom counts).
//         Sub-variants:
//           FM-3a  All slots stale — running total never decremented at all.
//           FM-3b  Partial expiry — some slots live, some stale; total only
//                  decremented when a slot is reused by Add, not when expired.
//           FM-3c  Lazy-clear-current-slot — Count() only subtracts the ONE
//                  slot mapped by now%windowSize, leaving other stale slots.
//   FM-4  Initialising slotTimes to 0 rather than a negative sentinel, so that
//         unwritten slots appear live when the fake clock starts at t=0.
//   FM-5  Failing to handle a time jump larger than the entire window (stale
//         slots from long ago are never cleared and pollute Count()).
//   FM-6  Data race: updating slots or running totals without holding the mutex.
//   FM-7  windowSize=1 edge-case: slot 0 is the only slot; the previous second
//         is always outside the window.
//   FM-8  Multiple Add calls in the same second must accumulate, not overwrite.
//   FM-9  Add(negative) is not a strict no-op: it updates slotTimes or
//         subtracts from slots, producing a corrupt state for the next Add.
//   FM-10 Count() has observable side effects so repeated calls return different
//         values (idempotency violation).
//   FM-11 windowSize=2 alternating-slot reuse: old slot data leaks into the
//         next window cycle when the ring wraps every other second.
// --------------------------------------------------------------------------

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers ---------------------------------------------------------------

// makeClock returns a closure that returns successive values from the provided
// sequence.  Calling it beyond the end of the sequence panics.
func makeClock(times ...int64) func() int64 {
	idx := 0
	return func() int64 {
		if idx >= len(times) {
			panic("makeClock: clock advanced past end of sequence")
		}
		t := times[idx]
		idx++
		return t
	}
}

// staticClock returns a closure that always returns the same value.
// Useful when a method is called multiple times at the "same" logical time.
func staticClock(t int64) func() int64 { return func() int64 { return t } }

// advancingClock returns a clock function backed by an atomic, so tests can
// bump the time from outside the counter.
func advancingClock(start int64) (*int64, func() int64) {
	v := new(int64)
	atomic.StoreInt64(v, start)
	return v, func() int64 { return atomic.LoadInt64(v) }
}

// ---- FM-1: boundary is exclusive at exactly windowSize seconds ago ----------

// If an event was recorded at second T, it must NOT appear in Count() when
// the current time is T+windowSize.  It MUST appear at T+windowSize-1.
func TestBoundaryExclusive(t *testing.T) {
	const W = 60
	// We need: Add at t=1000, Count at t=1059 (age 59 < 60 → included),
	// Count at t=1060 (age 60 >= 60 → excluded).
	clk := makeClock(
		1000, // Add
		1059, // Count — must see 1
		1060, // Count — must see 0
	)
	c := newCounter(W, clk)
	c.Add(1)

	assert.Equal(t, int64(1), c.Count(), "event 59 s old should be inside [now-60, now)")
	assert.Equal(t, int64(0), c.Count(), "event exactly 60 s old should be outside window")
}

// Same boundary check for a single-second window (FM-7 and FM-1 combined).
func TestBoundaryWindowSizeOne(t *testing.T) {
	clk := makeClock(
		100, // Add
		100, // Count at same second → inside window [99, 100)
		101, // Count one second later → event is 1 s old, age == windowSize → excluded
	)
	c := newCounter(1, clk)
	c.Add(5)

	assert.Equal(t, int64(5), c.Count(), "event in current second must be counted")
	assert.Equal(t, int64(0), c.Count(), "event from previous second is outside a window-size-1 counter")
}

// ---- FM-2: stale slot must be cleared before reuse -------------------------

// Scenario: windowSize=5.  Write 10 to second 0 (ring slot 0).  Advance past
// the window.  Write 3 to second 5 (also ring slot 0).  Count must be 3, not 13.
func TestRingBufferSlotClearedOnReuse(t *testing.T) {
	const W = 5
	clk := makeClock(
		0, // Add(10) → slot[0%5=0] = 10, slotTime[0] = 0
		5, // Add(3)  → slot[5%5=0]: stale (slotTime=0 != 5) → clear first, then = 3
		5, // Count at t=5: slot[0] age=0 OK, slots 1-4 stale → 3
	)
	c := newCounter(W, clk)
	c.Add(10)
	c.Add(3)

	assert.Equal(t, int64(3), c.Count(), "stale slot must be zeroed before reuse (FM-2)")
}

// Extend FM-2: after the ring wraps around fully, none of the old data survives.
func TestRingBufferFullRotation(t *testing.T) {
	const W = 3
	// Add 1 event per second for seconds 0..2, then advance to second 3.
	// At t=3: seconds 0 is now outside [3-3, 3) = [0, 3), so sec 0 is excluded.
	// Seconds 1 and 2 are included.
	addTimes := []int64{0, 1, 2}
	clk := makeClock(append(addTimes,
		3,  // Count: window = [0,3) → slots at sec1=1, sec2=2 → sum 2 (sec0 excluded)
		4,  // Count: window = [1,4) → slots at sec1=1, sec2=2 → sum 2 (sec0 still gone)
	)...)
	c := newCounter(W, clk)
	for i := 0; i < W; i++ {
		c.Add(int64(i + 1)) // adds 1, 2, 3 at t=0,1,2
	}

	// At t=3: sec0 (value=1) has age=3 >= W=3 → excluded. sec1(2)+sec2(3)=5.
	assert.Equal(t, int64(5), c.Count(), "FM-2 full rotation at t=3")
	// At t=4: sec1 (value=2) has age=3 >= W=3 → excluded. Only sec2(3)=3.
	assert.Equal(t, int64(3), c.Count(), "FM-2 full rotation at t=4")
}

// ---- FM-3: phantom counts when Count() is called without prior Add() --------

// If an implementation caches a running total and fails to subtract stale
// slots when Count() is called without an Add(), it returns a value that is
// too large.
func TestNoPhantomsOnCountWithoutAdd(t *testing.T) {
	const W = 10
	// Write some data, then advance past the window and count.
	addTime := int64(500)
	countTime := addTime + int64(W) // exactly at boundary → event is excluded

	c := newCounter(W, staticClock(addTime))
	c.Add(42)

	// Now advance to countTime (outside window) and count.
	c.nowFn = staticClock(countTime)
	assert.Equal(t, int64(0), c.Count(),
		"no Add() was called at countTime; running total must not include stale events (FM-3)")
}

// FM-3 variant: add events at t=0, skip many seconds, then count.
func TestNoPhantomsAfterLongIdle(t *testing.T) {
	const W = 5
	clk := makeClock(
		0,   // Add(99)
		100, // Count: all slots age >=100 → 0
	)
	c := newCounter(W, clk)
	c.Add(99)
	assert.Equal(t, int64(0), c.Count(), "all slots must be stale after 100s idle (FM-3/FM-5)")
}

// ---- FM-4: empty slots must not be counted when clock starts at t=0 --------

// If slotTimes are initialised to 0, then at t=0, every slot looks "just
// written" and Count() will include garbage from all windowSize slots.
func TestEmptySlotsNotCountedAtEpoch(t *testing.T) {
	c := newCounter(60, staticClock(0))
	assert.Equal(t, int64(0), c.Count(),
		"fresh counter must return 0 even when the clock reads exactly 0 (FM-4)")
}

func TestEmptySlotsNotCountedMidWindow(t *testing.T) {
	const W = 10
	// Add 1 at t=5; Count at t=5 — only that 1 slot should be live.
	clk := makeClock(5, 5)
	c := newCounter(W, clk)
	c.Add(1)
	assert.Equal(t, int64(1), c.Count(),
		"only the written slot should count; uninitialised slots must not appear (FM-4)")
}

// ---- FM-5: time jump larger than the full window ---------------------------

// After a jump of > windowSize seconds, every slot is stale.
// Implementations that only clear the "current" slot on the next Add/Count
// will return non-zero from Count().
func TestTimeJumpBeyondFullWindow(t *testing.T) {
	const W = 60
	clk := makeClock(
		1000,        // Add(7) fills slot 1000%60=40
		1000+W+1,    // Count: jump 61 s → slot 40 is now 61 s old → stale → 0
	)
	c := newCounter(W, clk)
	c.Add(7)
	assert.Equal(t, int64(0), c.Count(),
		"jump of >windowSize seconds must invalidate all slots (FM-5)")
}

// Jump of exactly 2×windowSize: all slots stale, then new data is clean.
func TestTimeJumpTwiceWindow(t *testing.T) {
	const W = 5
	clk := makeClock(
		0,      // Add(3)
		0+W*2,  // Add(9) at t=10: slot 0%5=0 is stale (slotTime=0 != 10) → cleared first
		0+W*2,  // Count at t=10: only slot for t=10 is live → 9
	)
	c := newCounter(W, clk)
	c.Add(3)
	c.Add(9)
	assert.Equal(t, int64(9), c.Count(),
		"after a 2×windowSize jump the old slot must have been cleared before accumulating (FM-5/FM-2)")
}

// ---- FM-6: concurrent safety (run with -race to detect data races) ----------

func TestConcurrentAddAndCount(t *testing.T) {
	const (
		W          = 10
		goroutines = 8
		addsEach   = 10_000
	)
	tv, clk := advancingClock(1000)
	c := newCounter(W, clk)

	var wg sync.WaitGroup
	// Half goroutines add, half call Count.
	for g := 0; g < goroutines/2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < addsEach; i++ {
				c.Add(1)
			}
		}()
	}
	for g := 0; g < goroutines/2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < addsEach; i++ {
				_ = c.Count()
			}
		}()
	}
	wg.Wait()

	// Advance the clock so all the t=1000 data is still live, and verify
	// the total is non-negative and does not exceed the maximum possible.
	atomic.StoreInt64(tv, 1000) // stay in same window
	got := c.Count()
	require.GreaterOrEqual(t, got, int64(0), "Count must be non-negative (FM-6)")
	require.LessOrEqual(t, got, int64(goroutines/2*addsEach),
		"Count must not exceed total events added (FM-6)")
}

func TestConcurrentAddOnly(t *testing.T) {
	const (
		W          = 30
		goroutines = 16
		addsEach   = 5_000
	)
	c := newCounter(W, staticClock(2000))

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < addsEach; i++ {
				c.Add(1)
			}
		}()
	}
	wg.Wait()

	got := c.Count()
	require.Equal(t, int64(goroutines*addsEach), got,
		"all concurrent adds must be visible in Count() (FM-6)")
}

// ---- FM-7: windowSize=1 (single-slot ring buffer) --------------------------

func TestWindowSizeOneBasic(t *testing.T) {
	clk := makeClock(
		42, // Add(5)
		42, // Count → 5 (same second)
		43, // Count → 0 (next second, age==1 == windowSize)
	)
	c := newCounter(1, clk)
	c.Add(5)
	assert.Equal(t, int64(5), c.Count(), "FM-7: same-second count")
	assert.Equal(t, int64(0), c.Count(), "FM-7: next-second count must be zero")
}

func TestWindowSizeOneReuse(t *testing.T) {
	// Write at t=10, read at t=11 (0), write again at t=11, read at t=11 (new value only).
	clk := makeClock(
		10, // Add(7)
		11, // Add(3): slot is stale (slotTime=10 != 11), must clear first
		11, // Count: only 3
	)
	c := newCounter(1, clk)
	c.Add(7)
	c.Add(3)
	assert.Equal(t, int64(3), c.Count(), "FM-7/FM-2: single-slot reuse must not accumulate across seconds")
}

// ---- FM-8: multiple Add() calls in the same second must accumulate ----------

func TestMultipleAddsInSameSecond(t *testing.T) {
	clk := staticClock(999)
	c := newCounter(60, clk)

	c.Add(3)
	c.Add(7)
	c.Add(10)

	assert.Equal(t, int64(20), c.Count(), "FM-8: same-second adds must accumulate")
}

// ---- basic sanity -----------------------------------------------------------

func TestZeroCountOnFreshCounter(t *testing.T) {
	c := newCounter(30, staticClock(5000))
	assert.Equal(t, int64(0), c.Count())
}

func TestReset(t *testing.T) {
	clk := staticClock(1234)
	c := newCounter(10, clk)
	c.Add(42)
	require.Equal(t, int64(42), c.Count())
	c.Reset()
	assert.Equal(t, int64(0), c.Count(), "Reset must clear all events")
}

func TestResetThenAdd(t *testing.T) {
	clk := makeClock(
		100, // Add(50)
		100, // Count → 50
		100, // Count after Reset → 0  (Reset does not call nowFn)
		101, // Add(7)
		101, // Count → 7
	)
	c := newCounter(10, clk)
	c.Add(50)
	assert.Equal(t, int64(50), c.Count())

	// Reset doesn't consume clock ticks (it doesn't call nowFn).
	c.Reset()
	assert.Equal(t, int64(0), c.Count())

	c.Add(7)
	assert.Equal(t, int64(7), c.Count())
}

// Panic on bad windowSize.
func TestInvalidWindowSizePanics(t *testing.T) {
	assert.Panics(t, func() { _ = New(0) })
	assert.Panics(t, func() { _ = New(-1) })
}

// Add(0) is a no-op and must not modify slotTimes for an empty slot.
func TestAddZeroIsNoop(t *testing.T) {
	c := newCounter(10, staticClock(500))
	c.Add(0)
	assert.Equal(t, int64(0), c.Count(), "Add(0) must be a no-op")
}

// ---- comprehensive scenario -------------------------------------------------

// Reproduces a full lifecycle: fill window, let it expire, refill, advance
// partially, verify that only the live portion is counted.
func TestFullLifecycle(t *testing.T) {
	const W = 5
	// Seconds:  0  1  2  3  4  |  5  6  7  |  count queries
	// Events:   1  2  3  4  5  |        6  |
	// At t=7: window is [2,7). Sec 0 (age=7>4) and sec 1 (age=6>4) excluded.
	// Sec 2(3), sec 3(4), sec 4(5) included. Plus sec 6(6). Total = 3+4+5+6 = 18.
	// But we won't add anything at sec 5; sec 6 slot is 6%5=1 which was 2 at t=1 → cleared.
	clk := makeClock(
		0, 1, 2, 3, 4, // Adds at seconds 0-4
		6,             // Add(6) at t=6
		7,             // Count at t=7
	)
	c := newCounter(W, clk)
	for i := int64(1); i <= 5; i++ {
		c.Add(i)
	}
	c.Add(6)

	// At t=7, window [2,7):
	//   slot 2 (sec=2, age=5 >= W=5) → EXCLUDED  (sec 2 has age exactly W)
	//   slot 3 (sec=3, age=4 < 5)    → 4
	//   slot 4 (sec=4, age=3 < 5)    → 5
	//   slot 1 (sec=6, age=1 < 5)    → 6  (was reused from sec=1 which had 2; that was cleared)
	//   slot 0 (sec=0, age=7 >= 5)   → excluded
	// Expected: 4+5+6 = 15
	assert.Equal(t, int64(15), c.Count(), "full lifecycle: only live slots within window")
}

// ---- FM-3b: partial expiry — some slots live, some stale, no Add between ---
//
// The canonical running-total bug: total is decremented only when Add() reuses
// a stale slot, never when Count() is called and time has moved forward.  Tests
// below call Count() after several slots have expired but without any Add() that
// would trigger the lazy-clear path.

// Three distinct slots populated; at Count time only the newest is still inside
// the window.  A running-total implementation returns 10+30+70=110 instead of 70.
func TestPartialExpiryMultipleStaleSlots(t *testing.T) {
	const W = 10
	// Ring indices: 100%10=0, 103%10=3, 107%10=7.
	// At t=115 window=[105,115): t=100(age=15)→out, t=103(age=12)→out, t=107(age=8)→in.
	clk := makeClock(
		100, // Add(10) → slot 0
		103, // Add(30) → slot 3
		107, // Add(70) → slot 7
		115, // Count → 70
	)
	c := newCounter(W, clk)
	c.Add(10)
	c.Add(30)
	c.Add(70)

	assert.Equal(t, int64(70), c.Count(),
		"two stale slots must not appear when Count() is called without intervening Add() (FM-3b)")
}

// Two slots both expire, with no Add() in between Count() calls.  The count
// must decay correctly at each step purely from Count()'s own inspection.
func TestMultipleCountsAsTimeAdvancesWithoutAdd(t *testing.T) {
	const W = 5
	// Add 10 at t=0. Count at t=3 (live), t=4 (live), t=5 (boundary→gone), t=6 (gone).
	clk := makeClock(
		0, // Add(10)
		3, // Count → 10 (age=3 < 5)
		4, // Count → 10 (age=4 < 5)
		5, // Count →  0 (age=5 >= 5)
		6, // Count →  0 (age=6 >= 5)
	)
	c := newCounter(W, clk)
	c.Add(10)

	assert.Equal(t, int64(10), c.Count(), "at t=3 (age=3): event must be live")
	assert.Equal(t, int64(10), c.Count(), "at t=4 (age=4): event must be live")
	assert.Equal(t, int64(0), c.Count(), "at t=5 (age=5=windowSize): event must be stale (FM-1+FM-3)")
	assert.Equal(t, int64(0), c.Count(), "at t=6 (age=6>windowSize): event must remain stale")
}

// ---- FM-3c: lazy-clear-current-slot — Count() only clears the slot mapped
//      by now%windowSize, missing stale data in all other slots.
//
// Scenario: stale data lives at ring indices that are NOT the current index.
// A "clear current slot only" Count() touches an unrelated (empty) slot and
// leaves the stale data untouched, so total stays wrong.

func TestTwoStaleSlotsNeitherIsCurrentRingSlot(t *testing.T) {
	const W = 5
	// Add at ring slot 0 (t=0) and slot 3 (t=3).
	// Count at t=9: current ring slot = 9%5 = 4 (has no data).
	// window=[4,9): t=0(age=9≥5)→out, t=3(age=6≥5)→out → expected 0.
	// Lazy-clear-current-slot impl clears empty slot 4, leaves slots 0 and 3 in total → 300.
	clk := makeClock(
		0, // Add(100) → slot 0
		3, // Add(200) → slot 3
		9, // Count at slot 4 (neither stale slot) → 0
	)
	c := newCounter(W, clk)
	c.Add(100)
	c.Add(200)

	assert.Equal(t, int64(0), c.Count(),
		"stale slots not at the current ring index must still be excluded (FM-3c)")
}

// ---- FM-3b+FM-1 combined: second-by-second decay ---------------------------
//
// Steps through a full window decay one second at a time.  Any off-by-one in
// the boundary or any phantom-count bug causes an incorrect value at some step.

func TestSecondBySecondWindowDecay(t *testing.T) {
	const W = 3
	// Add 10 at t=0, 20 at t=1, 30 at t=2.
	// Count at t=2: window=[-1,2) → ages 2,1,0 all < 3 → 10+20+30 = 60
	// Count at t=3: window=[0,3) → t=0 age=3≥3 gone  → 20+30 = 50
	// Count at t=4: window=[1,4) → t=1 age=3≥3 gone  → 30
	// Count at t=5: window=[2,5) → t=2 age=3≥3 gone  → 0
	clk := makeClock(
		0, 1, 2,    // Add(10), Add(20), Add(30)
		2, 3, 4, 5, // Count queries
	)
	c := newCounter(W, clk)
	c.Add(10)
	c.Add(20)
	c.Add(30)

	cases := []struct {
		at   int64
		want int64
	}{
		{2, 60},
		{3, 50},
		{4, 30},
		{5, 0},
	}
	for _, tc := range cases {
		got := c.Count()
		assert.Equal(t, tc.want, got, "at t=%d", tc.at)
	}
}

// ---- FM-9: Add(negative) must be a strict no-op ----------------------------
//
// If addLocked is called for negative n it may write slotTimes[idx]=now and
// subtract |n| from slots[idx], producing a negative count and corrupting the
// slot state for the next real Add.

func TestNegativeAddHasNoSideEffects(t *testing.T) {
	clk := staticClock(500)
	c := newCounter(10, clk)

	c.Add(-5) // must be a complete no-op
	c.Add(3)  // must behave as if the slot is fresh

	got := c.Count()
	assert.Equal(t, int64(3), got,
		"Add(-5) must not subtract from slots or update slotTimes; subsequent Add(3) must see a clean slot (FM-9)")
	assert.GreaterOrEqual(t, got, int64(0), "count must never be negative after Add(-n)+Add(m)")
}

func TestNegativeAddAloneYieldsZero(t *testing.T) {
	c := newCounter(10, staticClock(200))
	c.Add(-100)
	assert.Equal(t, int64(0), c.Count(),
		"Add(-100) alone must leave count at 0, not produce a negative value (FM-9)")
}

// ---- FM-10: Count() must be idempotent -------------------------------------
//
// Some implementations mutate internal state inside Count() (e.g. updating
// slotTimes or clearing slots).  Repeated calls at the same instant must
// return the identical value.

func TestCountIsIdempotent(t *testing.T) {
	clk := staticClock(5000)
	c := newCounter(60, clk)
	c.Add(42)

	for i := 0; i < 5; i++ {
		assert.Equal(t, int64(42), c.Count(),
			"Count() call #%d must return same value as first call (FM-10)", i+1)
	}
}

func TestCountIdempotentAfterExpiry(t *testing.T) {
	// After data expires, repeated Count() must consistently return 0.
	c := newCounter(5, staticClock(100))
	c.Add(7)

	c.nowFn = staticClock(106) // all data expired (age=6 >=5)
	for i := 0; i < 4; i++ {
		assert.Equal(t, int64(0), c.Count(),
			"Count() call #%d after expiry must return 0 (FM-10+FM-3)", i+1)
	}
}

// ---- FM-11: windowSize=2 alternating-slot reuse ----------------------------
//
// With windowSize=2, even seconds map to slot 0 and odd seconds to slot 1.
// Every other second the same ring slot is reused; the old value must be
// cleared before the new one is written.

func TestWindowSizeTwoAlternatingSlots(t *testing.T) {
	// t=10(even→slot0)=5, t=11(odd→slot1)=7, t=12(even→slot0): slot0 stale → clear.
	// At t=12 window=[10,12): t=10(age=2≥2)→excluded, t=11(age=1)→7, t=12(age=0)→3. Sum=10.
	clk := makeClock(
		10, // Add(5)  → slot 0, slotTime=10
		11, // Add(7)  → slot 1, slotTime=11
		12, // Add(3)  → slot 0: slotTime=10≠12 → clear 5, write 3
		12, // Count   → slot1(7) + slot0(3) = 10
	)
	c := newCounter(2, clk)
	c.Add(5)
	c.Add(7)
	c.Add(3)

	assert.Equal(t, int64(10), c.Count(),
		"windowSize=2: reused slot-0 must lose its t=10 value of 5 before accumulating t=12 value of 3 (FM-11/FM-2)")
}

func TestWindowSizeTwoExpiry(t *testing.T) {
	// windowSize=2: at t=12, t=10 (age=2) is excluded.
	clk := makeClock(
		10, // Add(5)  → slot 0
		12, // Count: window=[10,12) → t=10 age=2≥2 → excluded → 0
	)
	c := newCounter(2, clk)
	c.Add(5)

	assert.Equal(t, int64(0), c.Count(),
		"windowSize=2: event exactly 2 s old must be excluded (FM-1+FM-11)")
}

// ---- FM-6 extension: concurrent Reset() + Add() must not race -------------

func TestConcurrentResetAndAdd(t *testing.T) {
	const iters = 200
	c := newCounter(10, staticClock(1000))

	var wg sync.WaitGroup
	for i := 0; i < iters; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.Add(1)
		}()
		go func() {
			defer wg.Done()
			c.Reset()
		}()
	}
	wg.Wait()

	got := c.Count()
	assert.GreaterOrEqual(t, got, int64(0),
		"concurrent Reset+Add must never produce a negative count (FM-6)")
	assert.LessOrEqual(t, got, int64(iters),
		"concurrent Reset+Add must not exceed number of adds (FM-6)")
}
