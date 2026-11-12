// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import (
	"bytes"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// These tests fail a change that makes the tokenizer allocate differently or
// read past its window (the rules are in doc.go). They run in every unit test
// job, so they can't depend on how fast the machine is:
//   - the allocation tests count allocations, which give the same number on
//     any machine, and under the race detector and coverage too;
//   - the window test compares two timings taken in the same process, and
//     skips on Windows, whose clock is too coarse to time one call.
//
// If one fails, either fix the change, or, if the new cost is on purpose,
// update the budget here and in doc.go in the same change.

// allocBudgets is how many heap allocations each call makes per line on a warm
// Tokenizer. An empty line makes none. The test wants this exact number: a
// count that goes down fails too, so the budget here and in doc.go always
// matches the code. Lower it in the change that saves the allocation.
var allocBudgets = []struct {
	name   string
	budget int
	call   func(tok *Tokenizer, line []byte, owned, next []Token)
}{
	{"TokenizeBorrowed", 0, func(tok *Tokenizer, line []byte, _, _ []Token) {
		benchSink += tok.TokenizeBorrowed(line).Len()
	}},
	{"TokenizeBorrowed+Limit", 0, func(tok *Tokenizer, line []byte, _, _ []Token) {
		benchSink += tok.TokenizeBorrowed(line).Limit(60).Len()
	}},
	{"Tokenize", 2, func(tok *Tokenizer, line []byte, _, _ []Token) {
		ownedSink, indexSink = tok.Tokenize(line)
	}},
	{"TokenizeBorrowed+Clone", 1, func(tok *Tokenizer, line []byte, _, _ []Token) {
		ownedSink = tok.TokenizeBorrowed(line).Clone()
	}},
	{"TokenizeBorrowed+Retained", 1, func(tok *Tokenizer, line []byte, _, _ []Token) {
		retainedSink = tok.TokenizeBorrowed(line).Retained()
	}},
	{"TokensToString", 1, func(_ *Tokenizer, _ []byte, owned, _ []Token) {
		stringSink = TokensToString(owned)
	}},
	{"IsImportant", 0, func(_ *Tokenizer, _ []byte, owned, _ []Token) {
		if IsImportant(owned) {
			benchSink++
		}
	}},
	{"IsMatch", 0, func(_ *Tokenizer, _ []byte, owned, next []Token) {
		if IsMatch(owned, next, 0.75) {
			benchSink++
		}
	}},
	{"IsMatch/full", 0, func(_ *Tokenizer, _ []byte, owned, _ []Token) {
		if IsMatch(owned, owned, 0.75) {
			benchSink++
		}
	}},
}

// The tests store results in these package variables, the way a consumer keeps
// them. Otherwise the compiler could drop the call or keep its result on the
// stack, and the count would be lower than what a real consumer pays.
var (
	ownedSink    []Token
	indexSink    []int
	retainedSink BorrowedTokens
	stringSink   string
)

// contractLines is perfLines plus two edge cases: an empty line, and a 134 KB
// line (appLogLine 256 times), far longer than any consumer's window. The long
// line checks that a bigger input doesn't mean more allocations.
func contractLines() [][]byte {
	lines := [][]byte{nil, bytes.Repeat(appLogLine, 256)}
	for _, l := range perfLines {
		lines = append(lines, l.line)
	}
	return lines
}

// contractWindows are the windows (maxEvalBytes) the tests run at: each
// consumer's (60, 2048, 4096), plus 100 for the timestamp detector and 0 for
// no limit.
func contractWindows() []int {
	windows := []int{0, 100}
	for _, c := range consumers {
		windows = append(windows, c.maxEvalBytes)
	}
	return windows
}

// canarySink and newCanary tell whether inlining is on. Under a debugger build
// (-gcflags=all=-N -l, as Delve uses) nothing is inlined, values that normally
// stay on the stack move to the heap, and allocation counts mean nothing. With
// inlining, newCanary's array stays on the stack => 0 allocations.
var canarySink int

func newCanary() *[4]int { return &[4]int{1} }

func skipWithoutOptimizations(t *testing.T) {
	if testing.AllocsPerRun(1, func() { canarySink += newCanary()[0] }) > 0 {
		t.Skip("built without inlining, so allocation counts are not meaningful")
	}
}

// allocsFor returns the fewest allocations f made over three runs, each after a
// warm-up run. Taking the minimum filters out allocations made by other
// goroutines at the same time. Every run reads the same input, so this can't
// see an allocation that only happens on new input: that's what
// TestNewInputNeverAllocates checks.
func allocsFor(f func()) int {
	fewest := testing.AllocsPerRun(1, f)
	for range 2 {
		fewest = min(fewest, testing.AllocsPerRun(1, f))
	}
	return int(fewest)
}

// mallocs returns how many heap allocations f made. Unlike allocsFor, it
// doesn't warm up first, so it also counts allocations on the first call.
func mallocs(f func()) uint64 {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.Mallocs - before.Mallocs
}

func TestAllocationBudgets(t *testing.T) {
	skipWithoutOptimizations(t)
	lines := contractLines()
	for _, window := range contractWindows() {
		tok := NewTokenizer(window)
		owned := make([][]Token, len(lines))
		nonEmpty := 0
		for i, line := range lines {
			owned[i], _ = tok.Tokenize(line)
			if len(owned[i]) > 0 {
				nonEmpty++
			}
		}
		for _, op := range allocBudgets {
			got := allocsFor(func() {
				for i, line := range lines {
					op.call(tok, line, owned[i], owned[(i+1)%len(owned)])
				}
			})
			assert.Equal(t, op.budget*nonEmpty, got,
				"%s at maxEvalBytes=%d: %d allocations over %d lines, budget %d per line",
				op.name, window, got, nonEmpty, op.budget)
		}
	}
}

// shifted moves every ASCII letter and digit in line n places along its
// alphabet: with n=1, "Error 42" becomes "Fssps 53". The tokens keep the same
// shape, but the words and numbers are new to the Tokenizer.
func shifted(line []byte, n int) []byte {
	out := make([]byte, len(line))
	for i, c := range line {
		switch {
		case c >= 'a' && c <= 'z':
			c = 'a' + (c-'a'+byte(n))%26
		case c >= 'A' && c <= 'Z':
			c = 'A' + (c-'A'+byte(n))%26
		case c >= '0' && c <= '9':
			c = '0' + (c-'0'+byte(n))%10
		}
		out[i] = c
	}
	return out
}

// TestNewInputNeverAllocates checks that a warm Tokenizer doesn't allocate on
// lines it has never seen. Say someone adds a word cache that allocates once
// per new word: TestAllocationBudgets replays the same lines, so after the
// warm-up every word is cached and it still sees 0. This test feeds new words
// (see shifted), so it catches that.
func TestNewInputNeverAllocates(t *testing.T) {
	skipWithoutOptimizations(t)
	lines := contractLines()
	for _, window := range contractWindows() {
		tok := NewTokenizer(window)
		for _, line := range lines {
			tok.TokenizeBorrowed(line)
		}
		fewest := ^uint64(0)
		for n := 1; n <= 3; n++ {
			fresh := make([][]byte, len(lines))
			for i, line := range lines {
				fresh[i] = shifted(line, n)
			}
			fewest = min(fewest, mallocs(func() {
				for _, line := range fresh {
					benchSink += tok.TokenizeBorrowed(line).Len()
				}
			}))
		}
		assert.Zero(t, fewest, "maxEvalBytes=%d: allocated on lines it had not seen", window)
	}
}

// TestSmallWindowNeverAllocates checks that NewTokenizer(60), the default for
// every Logs decoder, never allocates after it's created, not even on its
// first lines. NewTokenizer sizes its buffers to the window, and a 60-byte
// window can't produce more than 60 tokens.
func TestSmallWindowNeverAllocates(t *testing.T) {
	skipWithoutOptimizations(t)
	lines := contractLines()
	fewest := ^uint64(0)
	for range 3 {
		tok := NewTokenizer(60)
		fewest = min(fewest, mallocs(func() {
			for _, line := range lines {
				benchSink += tok.TokenizeBorrowed(line).Len()
			}
		}))
	}
	assert.Zero(t, fewest, "a new NewTokenizer(60) allocated on its first lines")
}

// TestScratchBuffersStayBounded checks that a warm Tokenizer's buffers stop
// growing, and stay under twice the most it reads from one line (or 64, if
// that's bigger). A buffer that keeps growing is a memory leak in an Agent
// that runs for months.
func TestScratchBuffersStayBounded(t *testing.T) {
	lines := contractLines()
	longest := 0
	for _, line := range lines {
		longest = max(longest, len(line))
	}
	for _, window := range contractWindows() {
		tok := NewTokenizer(window)
		for _, line := range lines {
			tok.TokenizeBorrowed(line)
		}
		warm := [2]int{cap(tok.tsBuf), cap(tok.idxBuf)}
		for _, line := range lines {
			tok.TokenizeBorrowed(line)
		}
		assert.Equal(t, warm, [2]int{cap(tok.tsBuf), cap(tok.idxBuf)}, "maxEvalBytes=%d: buffers grew on a second pass", window)

		read := longest
		if window > 0 {
			read = min(read, window)
		}
		bound := max(64, 2*read)
		assert.LessOrEqual(t, warm[0], bound, "maxEvalBytes=%d: token buffer capacity", window)
		assert.LessOrEqual(t, warm[1], bound, "maxEvalBytes=%d: index buffer capacity", window)
	}
}

// skipWithCoarseClock skips a timing test when time.Now moves in steps bigger
// than 1 µs, too coarse to time one call. On Windows it moves every 0.5 to
// 15.6 ms.
func skipWithCoarseClock(t *testing.T) {
	step := time.Duration(1<<63 - 1)
	for range 5 {
		start := time.Now()
		next := time.Now()
		for next.Equal(start) {
			next = time.Now()
		}
		step = min(step, next.Sub(start))
	}
	if step > time.Microsecond {
		t.Skipf("time.Now moves in %v steps here, too coarse to time a call", step)
	}
}

// fastestPair returns the shortest time one call to a and one call to b took,
// over 15 interleaved rounds of at least 1 ms each. Interleaving matters: on a
// shared CI machine the core can slow down or change between two back-to-back
// measurements, which skews a ratio far more than noise within one round. Each
// round times both functions, alternating which goes first, so both see the
// same machine. The shortest sample is the one with the least noise.
func fastestPair(a, b func()) (time.Duration, time.Duration) {
	reps := 1
	for {
		start := time.Now()
		for range reps {
			a()
			b()
		}
		if time.Since(start) >= time.Millisecond {
			break
		}
		reps *= 2
	}
	sample := func(f func()) time.Duration {
		start := time.Now()
		for range reps {
			f()
		}
		return time.Since(start) / time.Duration(reps)
	}
	bestA, bestB := time.Duration(1<<63-1), time.Duration(1<<63-1)
	for i := range 15 {
		if i%2 == 0 {
			bestA = min(bestA, sample(a))
			bestB = min(bestB, sample(b))
		} else {
			bestB = min(bestB, sample(b))
			bestA = min(bestA, sample(a))
		}
	}
	return bestA, bestB
}

// TestWindowBoundsWork checks that bytes past the window cost nothing: a 16 MiB
// line must cost less than 4x a line exactly one window long. If the tokenizer
// read the whole line first, for example to validate or search it, the 16 MiB
// line would cost at least 25x more, even at the largest window (4096).
func TestWindowBoundsWork(t *testing.T) {
	skipWithCoarseClock(t)
	long := bytes.Repeat(appLogLine, 16<<20/len(appLogLine))
	for _, c := range consumers {
		tok := NewTokenizer(c.maxEvalBytes)
		window := long[:c.maxEvalBytes]
		longTime, windowTime := fastestPair(
			func() { tok.TokenizeBorrowed(long) },
			func() { tok.TokenizeBorrowed(window) },
		)
		ratio := float64(longTime) / float64(windowTime)
		assert.Less(t, ratio, 4.0, "%s: a 16 MiB line costs %.1fx a %d-byte line", c.name, ratio, c.maxEvalBytes)
	}
}
