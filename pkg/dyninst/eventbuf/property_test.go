// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Property tests for eventbuf.Buffer. Each test generates a randomized
// sequence of operations over a small key space and checks that the
// invariants (message-release accounting, truncation-flag coherence,
// at-most-one finalization per invocation) hold at every step.
//
// Shrinking is manual: on failure the test prints the operation trace and
// seed so a failing run can be replayed by invoking the property function
// directly with that seed.

package eventbuf

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"
)

// opKind identifies a single randomized operation.
type opKind uint8

const (
	opAddFragment opKind = iota
	opNoteReturnLost
	opNotePartial
	opDiscard
	opEvictOlderThan
	opClose
)

// op represents one operation in a randomized trace.
type op struct {
	kind opKind

	// For opAddFragment
	key          Key
	side         Side
	seq          uint16
	isFinal      bool
	expectReturn bool

	// For opNoteReturnLost / opNotePartial / opDiscard
	nkey Key

	// For opNotePartial
	npartialSide Side
	nlastSeq     uint16

	// For opEvictOlderThan
	cutoffKtime uint64
}

func (o op) String() string {
	switch o.kind {
	case opAddFragment:
		return fmt.Sprintf(
			"AddFragment(key=%+v, side=%d, seq=%d, isFinal=%v, expectReturn=%v)",
			o.key, o.side, o.seq, o.isFinal, o.expectReturn,
		)
	case opNoteReturnLost:
		return fmt.Sprintf("NoteReturnLost(key=%+v)", o.nkey)
	case opNotePartial:
		return fmt.Sprintf("NotePartial(key=%+v, side=%d, lastSeq=%d)",
			o.nkey, o.npartialSide, o.nlastSeq)
	case opDiscard:
		return fmt.Sprintf("Discard(key=%+v)", o.nkey)
	case opEvictOlderThan:
		return fmt.Sprintf("EvictOlderThan(%d)", o.cutoffKtime)
	case opClose:
		return "Close()"
	}
	return "?"
}

// invocationState tracks what has been fed into the buffer for one Key.
// Used to validate Readys emitted for that key match the input history.
type invocationState struct {
	// Messages (by identity) added to the entry / return sides, in order.
	entryMsgs  []*testMessage
	returnMsgs []*testMessage

	// Notification history for this key.
	returnLostNoted      bool
	partialEntryNoted    bool
	partialReturnNoted   bool
	partialEntryLastSeq  uint16
	partialReturnLastSeq uint16

	// Whether this logical invocation has been finalized (Ready emitted).
	finalized bool
	// Whether this logical invocation has been Discarded.
	discarded bool
}

// runProperty generates and executes a random trace against a fresh Buffer,
// collecting every Ready emitted and verifying invariants along the way.
//
// Returns nil on success; on failure, returns an error whose message
// includes the full operation trace.
func runProperty(seed uint64, numOps int) error {
	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
	b := newTestBuffer()

	// Bounded key space so operations repeat and invocations get re-used.
	const numKeys = 3
	keys := make([]Key, numKeys)
	for i := range keys {
		keys[i] = Key{
			Goid:           uint64(i + 1),
			StackByteDepth: 100,
			ProbeID:        uint32(i + 1),
			EntryKtime:     uint64(i*1000 + 1000),
		}
	}

	// Per-key tracking. Resets on finalize/discard (new logical invocation).
	states := make(map[Key]*invocationState)
	getState := func(k Key) *invocationState {
		s, ok := states[k]
		if !ok || s.finalized || s.discarded {
			s = &invocationState{}
			states[k] = s
		}
		return s
	}

	// Collect every Message created so we can verify all are released at
	// the end.
	var allMsgs []*testMessage

	// Collect every Ready emitted, so we can check invariants at the end.
	var allReadys []Ready

	// Operation trace for error reporting.
	trace := make([]op, 0, numOps)

	// Apply a Ready: verify invariants, mark state as finalized, release
	// message lists (we still need to assert on them but then they're done).
	//
	// forced is true if this Ready was produced by Evict/Close rather than
	// by the normal finalize path. Forced finalizes may legitimately set
	// EntryTruncated / ReturnTruncated without matching notifications.
	applyReady := func(r Ready, forced bool, curTrace []op) error {
		if os.Getenv("EVENTBUF_PROPERTY_DEBUG") != "" {
			fmt.Printf("Ready at op %d (forced=%v): key=%+v EntryTruncated=%v ReturnTruncated=%v ReturnLost=%v Entry=%v Return=%v\n",
				len(curTrace)-1, forced, r.Key, r.EntryTruncated, r.ReturnTruncated, r.ReturnLost,
				r.Entry != nil, r.Return != nil)
		}
		allReadys = append(allReadys, r)
		s := states[r.Key]
		if s == nil {
			// Ready emitted for a key we didn't touch. That can happen when
			// EvictOlderThan / Close processes a zombie created by a
			// notification alone. We allow it; validate flags against
			// whatever state we have (none), which is vacuously fine.
			return nil
		}
		if s.finalized {
			return fmt.Errorf(
				"double-finalization of key %+v\ntrace:\n%s",
				r.Key, formatTrace(curTrace),
			)
		}
		s.finalized = true

		// Flag coherence: truncation / return-lost claims on Ready must be
		// justified by a corresponding notification.
		if r.EntryTruncated && !s.partialEntryNoted && !forced {
			return fmt.Errorf(
				"Ready.EntryTruncated=true for key %+v without matching PartialEntry\n"+
					"state: entryMsgs=%d returnMsgs=%d partialEntryNoted=%v partialReturnNoted=%v returnLostNoted=%v\n"+
					"trace:\n%s",
				r.Key, len(s.entryMsgs), len(s.returnMsgs), s.partialEntryNoted,
				s.partialReturnNoted, s.returnLostNoted,
				formatTrace(curTrace),
			)
		}
		if r.ReturnTruncated && !s.partialReturnNoted && !forced {
			return fmt.Errorf(
				"Ready.ReturnTruncated=true for key %+v without matching PartialReturn\ntrace:\n%s",
				r.Key, formatTrace(curTrace),
			)
		}
		if r.ReturnLost && !s.returnLostNoted {
			return fmt.Errorf(
				"Ready.ReturnLost=true for key %+v without matching ReturnLost\ntrace:\n%s",
				r.Key, formatTrace(curTrace),
			)
		}

		// Entry / Return list presence must match what we fed in. This
		// property is tight in normal finalize but loose for forced finalize
		// (EvictOlderThan/Close can emit a zombie entry with no fragments,
		// e.g. when only a notification arrived).
		if r.Entry != nil && len(s.entryMsgs) == 0 && !forced {
			return fmt.Errorf(
				"Ready.Entry != nil for key %+v with no entry fragments added\ntrace:\n%s",
				r.Key, formatTrace(curTrace),
			)
		}
		if r.Entry == nil && len(s.entryMsgs) > 0 && !forced {
			return fmt.Errorf(
				"Ready.Entry == nil for key %+v with %d entry fragments added\ntrace:\n%s",
				r.Key, len(s.entryMsgs), formatTrace(curTrace),
			)
		}
		if r.Return != nil && len(s.returnMsgs) == 0 && !forced {
			return fmt.Errorf(
				"Ready.Return != nil for key %+v with no return fragments added\ntrace:\n%s",
				r.Key, formatTrace(curTrace),
			)
		}
		if r.Return == nil && len(s.returnMsgs) > 0 && !r.ReturnLost && !forced {
			return fmt.Errorf(
				"Ready.Return == nil for key %+v with %d return fragments added (and not ReturnLost)\ntrace:\n%s",
				r.Key, len(s.returnMsgs), formatTrace(curTrace),
			)
		}

		// Counting: fragments seen in the Ready's lists must match what we
		// added — unless Evict / Close truncated them.
		if r.Entry != nil {
			got := 0
			for range r.Entry.Fragments() {
				got++
			}
			if got != len(s.entryMsgs) {
				return fmt.Errorf(
					"Ready.Entry has %d fragments but %d were added for key %+v\ntrace:\n%s",
					got, len(s.entryMsgs), r.Key, formatTrace(curTrace),
				)
			}
		}
		if r.Return != nil {
			got := 0
			for range r.Return.Fragments() {
				got++
			}
			if got != len(s.returnMsgs) {
				return fmt.Errorf(
					"Ready.Return has %d fragments but %d were added for key %+v\ntrace:\n%s",
					got, len(s.returnMsgs), r.Key, formatTrace(curTrace),
				)
			}
		}

		// Release the Ready's messages so the end-of-test accounting works.
		if r.Entry != nil {
			r.Entry.Release()
		}
		if r.Return != nil {
			r.Return.Release()
		}
		return nil
	}

	for i := 0; i < numOps; i++ {
		o := genOp(rng, keys, i == numOps-1)
		trace = append(trace, o)

		switch o.kind {
		case opAddFragment:
			msg := newTestMessage(8)
			allMsgs = append(allMsgs, msg)
			s := getState(o.key)
			if o.side == Entry {
				s.entryMsgs = append(s.entryMsgs, msg)
			} else {
				s.returnMsgs = append(s.returnMsgs, msg)
			}
			r, done := b.AddFragment(o.key, msg, o.side, o.seq, o.isFinal, o.expectReturn)
			if done {
				if err := applyReady(r, false /*forced*/, trace); err != nil {
					return err
				}
			}
		case opNoteReturnLost:
			s := getState(o.nkey)
			s.returnLostNoted = true
			r, done := b.NoteReturnLost(o.nkey)
			if done {
				if err := applyReady(r, false, trace); err != nil {
					return err
				}
			}
		case opNotePartial:
			s := getState(o.nkey)
			if o.npartialSide == Entry {
				s.partialEntryNoted = true
				s.partialEntryLastSeq = o.nlastSeq
			} else {
				s.partialReturnNoted = true
				s.partialReturnLastSeq = o.nlastSeq
			}
			r, done := b.NotePartial(o.nkey, o.npartialSide, o.nlastSeq)
			if done {
				if err := applyReady(r, false, trace); err != nil {
					return err
				}
			}
		case opDiscard:
			if s, ok := states[o.nkey]; ok {
				s.discarded = true
			}
			b.Discard(o.nkey)
		case opEvictOlderThan:
			for _, r := range b.EvictOlderThan(o.cutoffKtime) {
				if err := applyReady(r, true /*forced*/, trace); err != nil {
					return err
				}
			}
		case opClose:
			for _, r := range b.Close() {
				if err := applyReady(r, true /*forced*/, trace); err != nil {
					return err
				}
			}
			// After Close, the buffer is drained. Any remaining ops in the
			// trace are unreachable — return early.
			goto done
		}
	}
done:

	// If we never hit opClose, close now to drain.
	if b.Len() > 0 {
		for _, r := range b.Close() {
			if err := applyReady(r, true /*forced*/, trace); err != nil {
				return err
			}
		}
	}

	// Final invariant: every message is released.
	for i, m := range allMsgs {
		if !m.released {
			return fmt.Errorf(
				"message %d (of %d) was never released\ntrace:\n%s",
				i, len(allMsgs), formatTrace(trace),
			)
		}
	}

	_ = allReadys
	return nil
}

func genOp(rng *rand.Rand, keys []Key, forceClose bool) op {
	if forceClose {
		return op{kind: opClose}
	}
	// Weight operations so AddFragment dominates; notifications and
	// eviction/close are sprinkled in.
	x := rng.IntN(100)
	k := keys[rng.IntN(len(keys))]
	switch {
	case x < 60:
		// AddFragment
		return op{
			kind:         opAddFragment,
			key:          k,
			side:         Side(rng.IntN(2)),
			seq:          uint16(rng.IntN(4)),
			isFinal:      rng.IntN(2) == 0,
			expectReturn: rng.IntN(2) == 0,
		}
	case x < 70:
		return op{kind: opNoteReturnLost, nkey: k}
	case x < 80:
		return op{
			kind:         opNotePartial,
			nkey:         k,
			npartialSide: Side(rng.IntN(2)),
			nlastSeq:     uint16(rng.IntN(4)),
		}
	case x < 85:
		return op{kind: opDiscard, nkey: k}
	case x < 95:
		// Pick a cutoff in the range of the key EntryKtimes (1000..N*1000)
		// so both "some evicted" and "none evicted" paths get coverage.
		return op{
			kind:        opEvictOlderThan,
			cutoffKtime: uint64(rng.IntN(len(keys)+2)) * 1000,
		}
	default:
		return op{kind: opClose}
	}
}

func formatTrace(ops []op) string {
	var sb strings.Builder
	for i, o := range ops {
		fmt.Fprintf(&sb, "  [%d] %s\n", i, o)
	}
	return sb.String()
}

// TestBuffer_PropertyRandom runs many short randomized traces and asserts
// the invariants hold. Seeds are deterministic so a failure is easy to
// reproduce (use EVENTBUF_PROPERTY_SEED to pin a single seed for
// debugging).
func TestBuffer_PropertyRandom(t *testing.T) {
	iters := 500
	opsPerIter := 64
	if s := os.Getenv("EVENTBUF_PROPERTY_ITERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			iters = n
		}
	}
	if s := os.Getenv("EVENTBUF_PROPERTY_SEED"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 64); err == nil {
			// Pinned seed: run once, verbose.
			if err := runProperty(n, opsPerIter); err != nil {
				t.Fatalf("seed=%d: %v", n, err)
			}
			return
		}
	}
	for i := 0; i < iters; i++ {
		seed := uint64(i) + 1
		if err := runProperty(seed, opsPerIter); err != nil {
			t.Fatalf("seed=%d: %v", seed, err)
		}
	}
}

// TestBuffer_PropertyLongerTraces runs fewer iterations with longer traces.
// Useful for catching invariant violations that only surface after many ops.
func TestBuffer_PropertyLongerTraces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long traces in -short mode")
	}
	iters := 100
	opsPerIter := 1024
	for i := 0; i < iters; i++ {
		seed := uint64(i)*7 + 13
		if err := runProperty(seed, opsPerIter); err != nil {
			t.Fatalf("seed=%d: %v", seed, err)
		}
	}
}
