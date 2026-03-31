// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"
	"testing"
)

func TestContextSet_IsKnown(t *testing.T) {
	cs := newContextSet(0)

	if cs.IsKnown(42) {
		t.Fatal("expected first insert to return false")
	}
	if !cs.IsKnown(42) {
		t.Fatal("expected second lookup to return true")
	}
}

func TestContextSet_NoFalseNegatives(t *testing.T) {
	cs := newContextSet(0)

	// Insert 10K keys.
	for i := uint64(0); i < 10_000; i++ {
		cs.IsKnown(i)
	}

	// Verify all are known.
	for i := uint64(0); i < 10_000; i++ {
		if !cs.IsKnown(i) {
			t.Fatalf("false negative for key %d", i)
		}
	}
}

func TestContextSet_Reset(t *testing.T) {
	cs := newContextSet(0)

	for i := uint64(0); i < 100; i++ {
		cs.IsKnown(i)
	}
	cs.Reset()
	// After reset, keys should be unknown again.
	if cs.IsKnown(42) {
		t.Fatal("expected key to be unknown after reset")
	}
}

func TestContextSet_RemoveIsNoOp(t *testing.T) {
	cs := newContextSet(0)

	cs.IsKnown(42)
	cs.Remove(42) // no-op for bloom filter
	// Key should still be known (bloom filters don't support deletion).
	if !cs.IsKnown(42) {
		t.Fatal("expected key to still be known after Remove (bloom filter)")
	}
}

func TestContextSet_CheckCapAlwaysFalse(t *testing.T) {
	cs := newContextSet(10)

	for i := uint64(0); i <= 100; i++ {
		cs.IsKnown(i)
	}
	// Bloom filter has fixed size — CheckCap never triggers.
	if cs.CheckCap() {
		t.Fatal("expected CheckCap to return false for bloom filter")
	}
}

func TestContextSet_Concurrent(t *testing.T) {
	cs := newContextSet(0)

	const goroutines = 16
	const keysPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(offset uint64) {
			defer wg.Done()
			for i := uint64(0); i < keysPerGoroutine; i++ {
				cs.IsKnown(offset*keysPerGoroutine + i)
			}
		}(uint64(g))
	}
	wg.Wait()

	// Verify all keys are known (no false negatives).
	for g := uint64(0); g < goroutines; g++ {
		for i := uint64(0); i < keysPerGoroutine; i++ {
			if !cs.IsKnown(g*keysPerGoroutine + i) {
				t.Fatalf("false negative for key %d after concurrent insert", g*keysPerGoroutine+i)
			}
		}
	}
}

func TestContextSet_FalsePositiveRate(t *testing.T) {
	cs := newContextSet(0)

	// Insert 100K keys.
	for i := uint64(0); i < 100_000; i++ {
		cs.IsKnown(i)
	}

	// Test 100K keys that were NOT inserted.
	fps := 0
	for i := uint64(1_000_000); i < 1_100_000; i++ {
		if cs.IsKnown(i) {
			fps++
		}
	}
	fpr := float64(fps) / 100_000.0
	// Allow up to 5% (generous margin for k=3).
	if fpr > 0.05 {
		t.Fatalf("false positive rate too high: %.2f%% (%d/100000)", fpr*100, fps)
	}
	t.Logf("FPR at 100K elements: %.2f%% (%d/100000)", fpr*100, fps)
}

func BenchmarkContextSet_IsKnown_Hit(b *testing.B) {
	cs := newContextSet(0)

	for i := uint64(0); i < 200_000; i++ {
		cs.IsKnown(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cs.IsKnown(uint64(i % 200_000))
	}
}

func BenchmarkContextSet_IsKnown_Miss(b *testing.B) {
	cs := newContextSet(0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cs.IsKnown(uint64(i))
	}
}
