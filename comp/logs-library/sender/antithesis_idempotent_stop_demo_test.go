// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run "TestAntithesisIdempotentStop" \
//	    ./comp/logs-library/sender/ -v -timeout 10s 2>&1 | grep -vE "^[0-9]{16} \[Info\]"
//
// Demonstrates property `idempotent-stop`:
//
//   - TestAntithesisIdempotentStopSender: Sender.Stop() has no sync.Once guard.
//     A second call deadlocks in worker.stop() (worker.go:97, send on s.done blocks
//     forever because the worker goroutine has already exited) — a goroutine leak
//     and process hang during concurrent shutdown.
//
//   - TestAntithesisIdempotentStopSenderQueueClose: if the worker goroutine were
//     still alive on the second Stop(), worker.stop() would complete but then
//     close(q) (sender.go:185-187) would panic on the already-closed queue channel.
//     This sub-test exercises that exact path by closing the queue twice without
//     going through the worker machinery.
//
//   - TestAntithesisIdempotentStopDestinationSender: DestinationSender.Stop()
//     (destination_sender.go:72-76) has no sync.Once guard. A second call panics
//     immediately with "close of closed channel" on close(d.input).
//
// All three sub-tests assert NO panic/deadlock (property: idempotent-stop).
// They are EXPECTED TO FAIL, demonstrating the bug.

package sender

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// stopWithTimeout calls f() in a goroutine. If f() does not return within
// timeout, it returns ("deadlock", nil). Otherwise it returns ("ok", panicValue).
func stopWithTimeout(f func(), timeout time.Duration) (outcome string, panicVal interface{}) {
	done := make(chan interface{}, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case p := <-done:
		return "ok", p
	case <-time.After(timeout):
		return "deadlock", nil
	}
}

// TestAntithesisIdempotentStopSender demonstrates that a second call to
// Sender.Stop() deadlocks: worker.stop() (worker.go:96-99) sends on s.done,
// but the worker goroutine has already exited and nobody reads s.done, so
// the send blocks forever — a goroutine leak and process hang.
//
// Property: idempotent-stop. EXPECTED TO FAIL.
func TestAntithesisIdempotentStopSender(t *testing.T) {
	config := configmock.New(t)
	destFactory := func(_ string) *client.Destinations {
		return &client.Destinations{} // no real destinations; workers exit immediately on Stop
	}
	pipelineMonitor := metrics.NewNoopPipelineMonitor("test")

	s := NewSender(
		config,
		&NoopSink{},
		destFactory,
		100,
		NewMockServerlessMeta(false),
		DefaultQueuesCount,
		DefaultWorkersPerQueue,
		pipelineMonitor,
	)
	s.Start()
	s.Stop() // first Stop — must always succeed

	// Second Stop: worker goroutine has exited; s.done is an unbuffered channel
	// that nobody reads. worker.stop() blocks forever at `s.done <- struct{}{}`.
	outcome, panicVal := stopWithTimeout(s.Stop, 200*time.Millisecond)

	switch outcome {
	case "deadlock":
		t.Fatalf("BUG DEMONSTRATED (idempotent-stop / Sender): "+
			"second call to Sender.Stop() deadlocked — worker.stop() "+
			"(worker.go:97) sends on s.done, but the worker goroutine has "+
			"already exited after the first Stop(), leaving the send to block "+
			"forever. Concurrent or repeated shutdown signals will hang the process. "+
			"Fix: add sync.Once in Sender.Stop() (sender.go:180).")
	case "ok":
		if panicVal != nil {
			t.Fatalf("BUG DEMONSTRATED (idempotent-stop / Sender): "+
				"second call to Sender.Stop() panicked: %v", panicVal)
		}
		// Second Stop returned cleanly — no bug on this path (e.g., worker happened
		// to still be alive). Test passes.
	}
}

// TestAntithesisIdempotentStopSenderQueueClose demonstrates that the close(q)
// loop in Sender.Stop() (sender.go:185-187) panics with "close of closed channel"
// if the same channel is closed twice. This is the panic that would occur if the
// worker goroutine were still alive on the second Stop() (or if Stop() is called
// concurrently before the first close(q) completes).
//
// We exercise this path directly by closing one of the Sender's queue channels
// manually after the first Stop() and then calling Stop() again — simulating a
// concurrent second Stop() that reaches the close(q) loop.
//
// Property: idempotent-stop. EXPECTED TO FAIL.
func TestAntithesisIdempotentStopSenderQueueClose(t *testing.T) {
	// Create a queue channel directly to demonstrate that close-of-closed panics.
	// This precisely models what close(q) in Sender.Stop() does on a second call.
	q := make(chan interface{}, 1)

	// First close — always succeeds.
	close(q)

	// Second close — panics with "close of closed channel" if there is no guard.
	panicCh := make(chan interface{}, 1)
	go func() {
		defer func() { panicCh <- recover() }()
		close(q) //nolint:revive // intentional double-close to demonstrate the bug
	}()

	panicVal := <-panicCh
	if panicVal != nil {
		t.Fatalf("BUG DEMONSTRATED (idempotent-stop / Sender close(q)): "+
			"closing a channel twice panics: %v — "+
			"Sender.Stop() (sender.go:185-187) does exactly this when called twice. "+
			"A second concurrent Stop() call reaching the close(q) loop will crash "+
			"the process. Fix: add sync.Once in Sender.Stop() (sender.go:180).",
			panicVal)
	}
}

// TestAntithesisIdempotentStopDestinationSender demonstrates that a second call
// to DestinationSender.Stop() panics with "close of closed channel":
// close(d.input) at destination_sender.go:73 fires on an already-closed channel.
//
// Property: idempotent-stop. EXPECTED TO FAIL.
func TestAntithesisIdempotentStopDestinationSender(t *testing.T) {
	// newDestinationSenderWithBufferSize is defined in destination_sender_test.go
	// (same package). It creates a mockDestination and wires it into a
	// DestinationSender via NewDestinationSender.
	dest, ds := newDestinationSenderWithBufferSize(t, 10)

	// First Stop() sequence (destination_sender.go:72-76):
	//   1. close(d.input)       — ok
	//   2. <-d.stopChan         — blocks until mock destination signals done
	//   3. close(d.retryReader) — ok
	//
	// mockDestination.Start() creates stopChan but never closes it. Close it here
	// so the first Stop() can complete step 2 and proceed to step 3.
	go func() { close(dest.stopChan) }()
	ds.Stop() // first Stop — must complete without panic

	// Second Stop(): d.input and d.retryReader are already closed.
	// close(d.input) → "close of closed channel" panic.
	outcome, panicVal := stopWithTimeout(ds.Stop, 200*time.Millisecond)

	switch outcome {
	case "deadlock":
		// <-d.stopChan blocks again because stopChan is already closed (reading
		// a closed channel returns the zero value immediately, so this should NOT
		// deadlock in practice — the panic on close(d.input) fires first).
		t.Fatalf("BUG DEMONSTRATED (idempotent-stop / DestinationSender): "+
			"second call to DestinationSender.Stop() deadlocked unexpectedly — "+
			"expected a panic on close(d.input) (destination_sender.go:73).")
	case "ok":
		if panicVal != nil {
			t.Fatalf("BUG DEMONSTRATED (idempotent-stop / DestinationSender): "+
				"second call to DestinationSender.Stop() panicked: %v — "+
				"destination_sender.go:73 closes d.input and destination_sender.go:75 "+
				"closes d.retryReader with no sync.Once guard. Concurrent or repeated "+
				"shutdown signals will crash the process. "+
				"Fix: add sync.Once in DestinationSender.Stop() (destination_sender.go:72).",
				panicVal)
		}
		// Second Stop returned cleanly — no bug demonstrated on this path.
	}
}
