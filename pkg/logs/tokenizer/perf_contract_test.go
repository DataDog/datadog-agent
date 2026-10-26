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

// The tests in this file are the hard half of the tokenizer's performance
// contract (see doc.go). The allocation tests count allocations, so they give
// the same answer on any machine and under the race detector and coverage. The
// timing tests compare two timings taken in the same process, and skip where
// the clock is too coarse to time a call (Windows). If one fails, the change
// made a hot path allocate differently, or grow worse than linearly; fix the
// change, or update the budget here and in doc.go if the new cost is intended.

// allocBudgets lists the exact number of heap allocations one call makes on a
// warm Tokenizer, for a line that produces at least one token; empty lines make
// none. A different count, lower or higher, fails: update it here and in doc.go.
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

// The sinks keep results alive, so the compiler can neither drop them nor keep
// them on the stack, and the gates count what a consumer that keeps them pays.
var (
	ownedSink    []Token
	indexSink    []int
	retainedSink BorrowedTokens
	stringSink   string
)

// contractLines adds the edge cases to perfLines: an empty line, and a line
// far longer than every consumer window, which also checks that allocations do
// not grow with the input.
func contractLines() [][]byte {
	lines := [][]byte{nil, bytes.Repeat(appLogLine, 256)}
	for _, l := range perfLines {
		lines = append(lines, l.line)
	}
	return lines
}

// contractWindows are the maxEvalBytes values the gates run at: every
// consumer's, plus 100 (the timestamp detector's formats) and 0 (no limit).
func contractWindows() []int {
	windows := []int{0, 100}
	for _, c := range consumers {
		windows = append(windows, c.maxEvalBytes)
	}
	return windows
}

// canarySink and newCanary detect a build without inlining (for example
// -gcflags=all=-N -l under Delve), where allocation counts mean nothing:
// inlined, newCanary's array stays on the stack.
var canarySink int

func newCanary() *[4]int { return &[4]int{1} }

func skipWithoutOptimizations(t *testing.T) {
	if testing.AllocsPerRun(1, func() { canarySink += newCanary()[0] }) > 0 {
		t.Skip("built without inlining, so allocation counts are not meaningful")
	}
}

// allocsFor returns the fewest allocations f made across three runs, each
// after a warm-up run. Every run of f reads the same input, so an allocation
// that happens only on input never seen before is TestNewInputNeverAllocates'
// job; here the minimum only discards allocations by other goroutines.
func allocsFor(f func()) int {
	fewest := testing.AllocsPerRun(1, f)
	for range 2 {
		fewest = min(fewest, testing.AllocsPerRun(1, f))
	}
	return int(fewest)
}

// mallocs returns how many heap allocations f made, with no warm-up run.
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

// shifted returns line with every ASCII letter and digit moved n places along
// its alphabet: the same token shape, but words and numbers not seen before.
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

// TestNewInputNeverAllocates checks that a warm Tokenizer does not allocate on
// lines it has never seen. A cache or intern table that allocates only for a
// new word would pass TestAllocationBudgets, which replays one corpus.
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

// TestSmallWindowNeverAllocates checks that a Tokenizer with a window of 10 to
// 64 bytes, like every Logs decoder's at the default 60, allocates nothing
// after NewTokenizer, not even on its first lines.
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
// growing, and stay within twice the longest input it reads.
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

// skipWithCoarseClock skips a timing test where time.Now moves in steps too
// coarse to time a call. Windows reads the clock-interrupt time, which moves
// every 0.5 to 15.6 ms.
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

// fastest returns the shortest time one call to f took, over seven samples of
// at least a millisecond each.
func fastest(f func()) time.Duration {
	reps := 1
	for {
		start := time.Now()
		for range reps {
			f()
		}
		if time.Since(start) >= time.Millisecond {
			break
		}
		reps *= 2
	}
	best := time.Duration(1<<63 - 1)
	for range 7 {
		start := time.Now()
		for range reps {
			f()
		}
		best = min(best, time.Since(start)/time.Duration(reps))
	}
	return best
}

// TestWorkIsLinearInInput guards the O(n) scan: per byte, a 256 KiB line must
// cost about what a 16 KiB line costs. Rescanning earlier tokens for each new
// one, or any other quadratic step, makes the large line about 16 times slower
// per byte. An extra linear pass keeps the ratio at 1; perf_compare.sh sees it.
func TestWorkIsLinearInInput(t *testing.T) {
	skipWithCoarseClock(t)
	for _, l := range []struct {
		name string
		line []byte
	}{{"applog", appLogLine}, {"dense", []byte("a1.")}} {
		small := bytes.Repeat(l.line, 16<<10/len(l.line))
		large := bytes.Repeat(l.line, 256<<10/len(l.line))
		tok := NewTokenizer(0)
		perByte := func(input []byte) float64 {
			return float64(fastest(func() { tok.TokenizeBorrowed(input) })) / float64(len(input))
		}
		ratio := perByte(large) / perByte(small)
		assert.Less(t, ratio, 4.0, "%s: per-byte cost of a 256 KiB line is %.1fx a 16 KiB line's", l.name, ratio)
	}
}

// TestWindowBoundsWork guards maxEvalBytes: past the window, a longer line must
// cost nothing more. Reading the whole line first (validating it, searching it)
// makes a 16 MiB line at least 25 times slower than one window's worth, even at
// the largest window.
func TestWindowBoundsWork(t *testing.T) {
	skipWithCoarseClock(t)
	long := bytes.Repeat(appLogLine, 16<<20/len(appLogLine))
	for _, c := range consumers {
		tok := NewTokenizer(c.maxEvalBytes)
		window := long[:c.maxEvalBytes]
		ratio := float64(fastest(func() { tok.TokenizeBorrowed(long) })) /
			float64(fastest(func() { tok.TokenizeBorrowed(window) }))
		assert.Less(t, ratio, 4.0, "%s: a 16 MiB line costs %.1fx a %d-byte line", c.name, ratio, c.maxEvalBytes)
	}
}
