// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventbuf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// reusableMessage is a Message whose Release is a no-op, so a single
// *reusableMessage can be used by every benchmark iteration without
// allocating. Benchmarks that want to measure Release+pool overhead use
// testMessage instead.
type reusableMessage struct {
	data output.Event
}

func (m *reusableMessage) Event() output.Event { return m.data }
func (m *reusableMessage) Release()            {}

func newReusableMessage(size int) *reusableMessage {
	return &reusableMessage{data: make([]byte, size)}
}

// benchKey builds a synthetic Key. We use goid as the unique dimension so
// benchmarks can easily create N distinct keys.
func benchKey(i int) Key {
	return Key{
		Goid:           uint64(i + 1),
		StackByteDepth: 100,
		ProbeID:        1,
		EntryKtime:     uint64(i+1) * 1000,
	}
}

// --------------------------------------------------------------------
// Happy-path single-event flow: entry + return, one fragment each.
// Simulates the dominant production case: a complete invocation flows
// through AddFragment twice and finalizes on the second call.
// --------------------------------------------------------------------

func BenchmarkBuffer_SingleFragmentPair(b *testing.B) {
	buf := newTestBuffer()
	msg := newReusableMessage(128)
	key := benchKey(0)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = buf.AddFragment(key, msg, Entry, 0, true, true)
		r, _ := buf.AddFragment(key, msg, Return, 0, true, false)
		if r.Entry != nil {
			r.Entry.Release()
		}
		if r.Return != nil {
			r.Return.Release()
		}
	}
}

// --------------------------------------------------------------------
// Multi-fragment entry: four entry fragments then a single-frag return.
// Tests the Append path on MessageList as continuations grow.
// --------------------------------------------------------------------

func BenchmarkBuffer_MultiFragmentPair(b *testing.B) {
	buf := newTestBuffer()
	msg := newReusableMessage(128)
	key := benchKey(0)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = buf.AddFragment(key, msg, Entry, 0, false, true)
		_, _ = buf.AddFragment(key, msg, Entry, 1, false, true)
		_, _ = buf.AddFragment(key, msg, Entry, 2, false, true)
		_, _ = buf.AddFragment(key, msg, Entry, 3, true, true)
		r, _ := buf.AddFragment(key, msg, Return, 0, true, false)
		if r.Entry != nil {
			r.Entry.Release()
		}
		if r.Return != nil {
			r.Return.Release()
		}
	}
}

// --------------------------------------------------------------------
// EvictOlderThan on a tree of N entries. Fed by the actuator's periodic
// poll; we measure the scan cost when no entries qualify (the common
// case — the actuator fires it only when BPF reported a drop-notify
// loss).
// --------------------------------------------------------------------

func BenchmarkBuffer_EvictOlderThan_Noop(b *testing.B) {
	for _, size := range []int{1, 16, 128, 1024} {
		b.Run(sizeName(size), func(b *testing.B) {
			buf := newTestBuffer()
			msg := newReusableMessage(64)
			// Seed the tree with `size` in-flight invocations.
			for i := 0; i < size; i++ {
				_, _ = buf.AddFragment(benchKey(i), msg, Entry, 0, false, true)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				// Cutoff of 0 means nothing qualifies (EntryKtime starts
				// at 1 in benchKey) — we're benchmarking the scan.
				_ = buf.EvictOlderThan(0)
			}
		})
	}
}

// --------------------------------------------------------------------
// Budget pressure driving eviction: tree stays at `size` entries, budget
// caps exactly at that total, new invocations evict the oldest. Measures
// the cost of eviction-per-insert under sustained pressure.
// --------------------------------------------------------------------

func BenchmarkBuffer_BudgetEviction(b *testing.B) {
	const perFrag = 64
	for _, size := range []int{16, 128, 1024} {
		b.Run(sizeName(size), func(b *testing.B) {
			// Budget holds exactly `size` entries' worth of bytes.
			budget := NewBudget(perFrag * size)
			buf := NewBuffer(budget)
			msg := newReusableMessage(perFrag)
			// Seed to capacity with entries we'll cycle through.
			for i := 0; i < size; i++ {
				_, _ = buf.AddFragment(benchKey(i), msg, Entry, 0, false, true)
			}
			b.ReportAllocs()
			b.ResetTimer()
			i := size
			for b.Loop() {
				// Adding a new key forces eviction of the oldest entry
				// because the budget is saturated.
				_, _ = buf.AddFragment(benchKey(i), msg, Entry, 0, false, true)
				for _, r := range buf.TakePendingBudgetEvictions() {
					if r.Entry != nil {
						r.Entry.Release()
					}
					if r.Return != nil {
						r.Return.Release()
					}
				}
				i++
			}
		})
	}
}

// --------------------------------------------------------------------
// Drop-notification finalize-path: entry is in-flight (from a
// per-iteration AddFragment); NoteReturnLost fires the Ready. Measures
// the combined AddFragment + NoteReturnLost + Release per completed
// event when the return is lost.
// --------------------------------------------------------------------

func BenchmarkBuffer_ReturnLostFinalize(b *testing.B) {
	buf := newTestBuffer()
	msg := newReusableMessage(128)
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		key := benchKey(i)
		_, _ = buf.AddFragment(key, msg, Entry, 0, true, true)
		r, _ := buf.NoteReturnLost(key)
		if r.Entry != nil {
			r.Entry.Release()
		}
		i++
	}
}

// --------------------------------------------------------------------
// MessageList iteration: simulates the decoder traversing a fragment
// list across varying fragment counts. Measures Fragments() iter cost
// and the per-fragment access pattern.
// --------------------------------------------------------------------

func BenchmarkMessageList_Iterate(b *testing.B) {
	for _, nfrag := range []int{1, 4, 16, 64} {
		b.Run(sizeName(nfrag), func(b *testing.B) {
			// Build once; don't count setup against iterations.
			msg := newReusableMessage(128)
			list := NewMessageList(msg)
			for i := 1; i < nfrag; i++ {
				list.Append(msg)
			}
			b.ReportAllocs()
			b.ResetTimer()
			var total int
			for b.Loop() {
				total = 0
				for ev := range list.Fragments() {
					total += len(ev)
				}
			}
			// Prevent the compiler from DCE-ing the loop body.
			_ = total
		})
	}
}

// sizeName formats a tree size as a Go-benchmark-friendly sub-benchmark
// name (e.g. "size=1024"). Short and stable across runs.
func sizeName(n int) string {
	return "size=" + itoa(n)
}

// itoa is a small formatting helper that avoids fmt (which allocates).
// For benchmark sub-names we don't strictly need this, but it keeps the
// benchmark file free of fmt imports.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
