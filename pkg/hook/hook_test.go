// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hook

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// ----------------------------------------------------------------------------
// Unit tests
// ----------------------------------------------------------------------------

func TestPublishDelivers(t *testing.T) {
	h := NewHook[int]("deliver")
	received := make(chan int, 10)

	unsub := h.Subscribe("consumer", func(v int) { received <- v })
	defer unsub()

	h.Publish("producer", 42)

	select {
	case v := <-received:
		assert.Equal(t, 42, v)
	case <-time.After(time.Second):
		t.Fatal("timeout: payload not delivered")
	}
}

func TestPublishFanout(t *testing.T) {
	h := NewHook[int]("fanout")
	const nConsumers = 5
	received := make([]chan int, nConsumers)
	for i := range received {
		received[i] = make(chan int, 10)
	}

	for i := range nConsumers {
		ch := received[i]
		unsub := h.Subscribe(fmt.Sprintf("consumer-%d", i), func(v int) { ch <- v })
		defer unsub()
	}

	h.Publish("producer", 7)

	for i, ch := range received {
		select {
		case v := <-ch:
			assert.Equal(t, 7, v, "consumer %d got wrong value", i)
		case <-time.After(time.Second):
			t.Fatalf("timeout: consumer %d did not receive payload", i)
		}
	}
}

func TestPublishDropsOnFullChannel(t *testing.T) {
	// Build a hook with a pre-filled consumer channel directly (white-box).
	// No subscriber goroutine is started, so the channel stays full.
	h := &hook[int]{
		name:      "drop",
		consumers: make(map[string]consumer[int]),
		ctx:       context.Background(),
	}
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3 // channel is now at capacity
	h.consumers["consumer"] = consumer[int]{ch: ch, dropLabel: []string{"drop", "consumer"}}

	// None of these should block or panic.
	for i := range 10 {
		h.Publish("producer", i)
	}
	// Channel still holds exactly the original 3 items.
	assert.Equal(t, 3, len(ch))
}

// TestPublishIsNonBlocking asserts that Publish returns quickly even when every
// consumer channel is full. This is the core guarantee: the metric pipeline
// must not be slowed down by a slow consumer (e.g. a shared-memory writer).
func TestPublishIsNonBlocking(t *testing.T) {
	h := &hook[int]{
		name:      "non-blocking",
		consumers: make(map[string]consumer[int]),
		ctx:       context.Background(),
	}
	// Pre-fill 10 consumer channels to capacity.
	for i := range 10 {
		name := fmt.Sprintf("consumer-%d", i)
		ch := make(chan int, 100)
		for j := range 100 {
			ch <- j
		}
		h.consumers[name] = consumer[int]{ch: ch, dropLabel: []string{"non-blocking", name}}
	}

	// 1k publishes with 10 full channels. Without the race detector this takes
	// ~100µs; with the race detector (~50x slower) still <5s. A truly blocking
	// Publish would deadlock and time out the test entirely.
	const N = 1_000
	start := time.Now()
	for i := range N {
		h.Publish("producer", i)
	}
	elapsed := time.Since(start)

	require.Less(t, elapsed, 5*time.Second,
		"Publish blocked: %d drops took %s", N, elapsed)
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHook[int]("unsub")
	var count atomic.Int64

	unsub := h.Subscribe("consumer", func(_ int) { count.Add(1) })

	h.Publish("producer", 1)
	// Wait for the first delivery to be processed.
	require.Eventually(t, func() bool { return count.Load() == 1 }, time.Second, time.Millisecond)

	unsub()

	// Publishes after unsubscribe must not reach the callback.
	for i := range 10 {
		h.Publish("producer", i)
	}
	time.Sleep(20 * time.Millisecond) // give subscriber goroutine time to drain if it were still running
	assert.Equal(t, int64(1), count.Load(), "callback invoked after unsubscribe")
}

func TestUnsubscribeIdempotent(t *testing.T) {
	h := NewHook[int]("idempotent")
	unsub := h.Subscribe("consumer", func(_ int) {})
	require.NotPanics(t, func() {
		unsub()
		unsub() // second call must not panic
	})
}

// TestConcurrentPublish exercises the race detector: multiple goroutines
// publishing simultaneously while the hook has active subscribers.
func TestConcurrentPublish(t *testing.T) {
	h := NewHook[int]("concurrent-publish")
	var received atomic.Int64
	unsub := h.Subscribe("consumer", func(_ int) { received.Add(1) })
	defer unsub()

	var wg sync.WaitGroup
	const goroutines = 8
	const perGoroutine = 200
	for range goroutines {
		wg.Go(func() {
			for i := range perGoroutine {
				h.Publish("producer", i)
			}
		})
	}
	wg.Wait()
}

// TestConcurrentSubscribePublish exercises the race detector: subscribing and
// unsubscribing concurrently with active publishers.
func TestConcurrentSubscribePublish(t *testing.T) {
	h := NewHook[int]("concurrent-sub")

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Publisher goroutine runs continuously until stop is closed.
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				h.Publish("producer", i)
			}
		}
	})

	// Subscribe and immediately unsubscribe in a tight loop.
	for i := range 20 {
		name := fmt.Sprintf("consumer-%d", i)
		unsub := h.Subscribe(name, func(_ int) {})
		time.Sleep(time.Duration(i%3) * time.Millisecond)
		unsub()
	}

	close(stop)
	wg.Wait()
}

// TestNoGoroutineLeak asserts that the subscriber goroutine exits after
// the unsubscribe function is called.
func TestNoGoroutineLeak(t *testing.T) {
	options := goleak.IgnoreCurrent()

	h := NewHook[int]("leak-test")
	unsub := h.Subscribe("consumer", func(_ int) {})

	for i := range 20 {
		h.Publish("producer", i)
	}

	unsub()
	goleak.VerifyNone(t, options)
}

// ----------------------------------------------------------------------------
// Benchmarks
//
// Goal: Publish must add near-zero overhead to the metric pipeline.
//
// Key results to look at:
//   ns/op     — latency per Publish call
//   allocs/op — must be 0 on the send path (channel has space)
// ----------------------------------------------------------------------------

// BenchmarkPublish_NoConsumers is the baseline: pure RLock/RUnlock over an
// empty map. This is the minimum possible overhead of the hook machinery.
func BenchmarkPublish_NoConsumers(b *testing.B) {
	h := NewHook[int]("bench")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.Publish("producer", 0)
	}
}

// BenchmarkPublish_OneConsumer_Send measures the send path: channel has space,
// payload is copied into the channel buffer. No allocation at the hook level.
func BenchmarkPublish_OneConsumer_Send(b *testing.B) {
	h := &hook[int]{
		name:      "bench",
		consumers: make(map[string]consumer[int]),
		ctx:       context.Background(),
	}
	// Allocate a channel exactly as large as b.N so every iteration finds space.
	h.consumers["consumer"] = consumer[int]{
		ch:        make(chan int, b.N+1),
		dropLabel: []string{"bench", "consumer"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.Publish("producer", 0)
	}
}

// BenchmarkPublish_OneConsumer_Drop measures the drop path: channel is full,
// Publish takes the default branch and increments the drop counter.
// This shows the worst-case cost when the shared-memory writer falls behind.
func BenchmarkPublish_OneConsumer_Drop(b *testing.B) {
	h := &hook[int]{
		name:      "bench",
		consumers: make(map[string]consumer[int]),
		ctx:       context.Background(),
	}
	ch := make(chan int, 100)
	for i := range 100 {
		ch <- i
	}
	h.consumers["consumer"] = consumer[int]{ch: ch, dropLabel: []string{"bench", "consumer"}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.Publish("producer", 0)
	}
}

// BenchmarkPublish_TenConsumers_Send mirrors the production scenario with
// several consumers (observer + potential future sinks) all keeping up.
func BenchmarkPublish_TenConsumers_Send(b *testing.B) {
	h := &hook[int]{
		name:      "bench",
		consumers: make(map[string]consumer[int]),
		ctx:       context.Background(),
	}
	for i := range 10 {
		name := fmt.Sprintf("consumer-%d", i)
		h.consumers[name] = consumer[int]{
			ch:        make(chan int, b.N+1),
			dropLabel: []string{"bench", name},
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.Publish("producer", 0)
	}
}

// BenchmarkPublish_TenConsumers_Drop worst-case: 10 slow consumers, all full.
// Publish must still return in O(N_consumers) time with no blocking.
func BenchmarkPublish_TenConsumers_Drop(b *testing.B) {
	h := &hook[int]{
		name:      "bench",
		consumers: make(map[string]consumer[int]),
		ctx:       context.Background(),
	}
	for i := range 10 {
		name := fmt.Sprintf("consumer-%d", i)
		ch := make(chan int, 100)
		for j := range 100 {
			ch <- j
		}
		h.consumers[name] = consumer[int]{ch: ch, dropLabel: []string{"bench", name}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.Publish("producer", 0)
	}
}

// BenchmarkPublish_Parallel measures RLock contention with multiple concurrent
// producers (simulates several TimeSampler/CheckSampler goroutines).
func BenchmarkPublish_Parallel(b *testing.B) {
	h := NewHook[int]("bench-parallel")
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.Publish("producer", 0)
		}
	})
}
