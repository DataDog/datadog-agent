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
	if cs.Len() != 1 {
		t.Fatalf("expected Len=1, got %d", cs.Len())
	}
}

func TestContextSet_Remove(t *testing.T) {
	cs := newContextSet(0)
	cs.IsKnown(42)
	cs.IsKnown(43)
	if cs.Len() != 2 {
		t.Fatalf("expected Len=2, got %d", cs.Len())
	}
	cs.Remove(42)
	if cs.Len() != 1 {
		t.Fatalf("expected Len=1 after Remove, got %d", cs.Len())
	}
	// Removed key should be unknown again.
	if cs.IsKnown(42) {
		t.Fatal("expected key 42 to be unknown after Remove")
	}
	if cs.Len() != 2 {
		t.Fatalf("expected Len=2 after re-insert, got %d", cs.Len())
	}
	// Remove of non-existent key is a no-op.
	cs.Remove(999)
	if cs.Len() != 2 {
		t.Fatalf("expected Len=2 after no-op Remove, got %d", cs.Len())
	}
}

func TestContextSet_Reset(t *testing.T) {
	cs := newContextSet(0)
	for i := uint64(0); i < 100; i++ {
		cs.IsKnown(i)
	}
	if cs.Len() != 100 {
		t.Fatalf("expected Len=100, got %d", cs.Len())
	}
	cs.Reset()
	if cs.Len() != 0 {
		t.Fatalf("expected Len=0 after reset, got %d", cs.Len())
	}
	// After reset, keys should be unknown again.
	if cs.IsKnown(42) {
		t.Fatal("expected key to be unknown after reset")
	}
}

func TestContextSet_CheckCap(t *testing.T) {
	cs := newContextSet(10)
	for i := uint64(0); i <= 10; i++ {
		cs.IsKnown(i)
	}
	if cs.Len() != 11 {
		t.Fatalf("expected Len=11, got %d", cs.Len())
	}
	if !cs.CheckCap() {
		t.Fatal("expected CheckCap to return true when over cap")
	}
	if cs.Len() != 0 {
		t.Fatalf("expected Len=0 after CheckCap reset, got %d", cs.Len())
	}
}

func TestContextSet_CheckCap_NoCap(t *testing.T) {
	cs := newContextSet(0)
	for i := uint64(0); i < 100; i++ {
		cs.IsKnown(i)
	}
	if cs.CheckCap() {
		t.Fatal("expected CheckCap to return false when cap=0")
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

	expected := int64(goroutines * keysPerGoroutine)
	if cs.Len() != expected {
		t.Fatalf("expected Len=%d, got %d", expected, cs.Len())
	}
}

func TestContextSet_ConcurrentOverlapping(t *testing.T) {
	cs := newContextSet(0)
	const goroutines = 16
	const keys = 1000

	// All goroutines insert the same key range — each key should be counted once.
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := uint64(0); i < keys; i++ {
				cs.IsKnown(i)
			}
		}()
	}
	wg.Wait()

	if cs.Len() != keys {
		t.Fatalf("expected Len=%d, got %d", keys, cs.Len())
	}
}

func BenchmarkContextSet_IsKnown_Hit(b *testing.B) {
	cs := newContextSet(0)
	// Pre-populate
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

func BenchmarkSyncMap_LoadOrStore_Hit(b *testing.B) {
	var m sync.Map
	for i := uint64(0); i < 200_000; i++ {
		m.LoadOrStore(i, struct{}{})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.LoadOrStore(uint64(i%200_000), struct{}{})
	}
}

func BenchmarkSyncMap_LoadOrStore_Miss(b *testing.B) {
	var m sync.Map
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.LoadOrStore(uint64(i), struct{}{})
	}
}
