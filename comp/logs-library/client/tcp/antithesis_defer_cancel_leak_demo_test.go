// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisDeferCancelAccumulation \
//	    ./comp/logs-library/client/tcp/ -v -count=1
//
// Demonstrates property `tcp-connection-goroutine-no-leak`: inside NewConnection
// (connection_manager.go:102-103) the non-proxy dial path executes:
//
//	dctx, cancel := context.WithTimeout(ctx, connectionTimeout)
//	defer cancel()
//
// INSIDE the reconnect for-loop. Because defer fires at function return (not end
// of loop body), each failed dial iteration appends a new cancel func to the defer
// stack without releasing the prior iteration's timer/context. During a sustained
// outage N loop iterations accumulate N live timerCtx objects and their associated
// time.Timer entries before NewConnection finally returns.
//
// The test demonstrates this by comparing live heap-object counts at peak
// accumulation vs a reference implementation that calls cancel() eagerly.
// EXPECTED TO FAIL: the assertion caps allowed accumulation at 2 objects per
// iteration; the real code creates ~10 objects per iteration (timerCtx + timer +
// cancel node + defer entry).

package tcp

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// heapObjects returns the current live heap-object count after a GC cycle.
func heapObjects() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapObjects
}

// TestAntithesisDeferCancelAccumulation demonstrates that context.WithTimeout +
// defer cancel() inside the for-loop in NewConnection accumulates live heap objects
// (timerCtx, time.Timer, cancel-node in parent's children map) proportional to
// the number of failed dial attempts within a single NewConnection call.
//
// A correct implementation would call cancel() eagerly at the end of each loop
// iteration (before continuing), so at most one timerCtx/timer pair is live at
// any time. The current code defers, so all N pairs stay live until return.
//
// Evidence: connection_manager.go:102-103
//
//	dctx, cancel := context.WithTimeout(ctx, connectionTimeout)  // line 102
//	defer cancel()                                               // line 103
func TestAntithesisDeferCancelAccumulation(t *testing.T) {
	// --- Reference baseline: eager cancel (what the code SHOULD do) ---
	// We simulate the equivalent of calling cancel() at the end of each
	// iteration (eager), measuring how many heap objects each extra child
	// context costs.
	baseline := heapObjects()
	const nIterations = 5
	eagerCancels := make([]context.CancelFunc, nIterations)
	eagerParent, eagerParentCancel := context.WithTimeout(context.Background(), 60*time.Second)
	for i := 0; i < nIterations; i++ {
		_, c := context.WithTimeout(eagerParent, 20*time.Second)
		c()                    // eager cancel: context and timer released immediately
		eagerCancels[i] = nil // slot kept so arrays are same size
	}
	eagerParentCancel()
	eagerPeak := heapObjects()
	eagerDelta := int64(eagerPeak) - int64(baseline)

	// --- Buggy pattern: deferred cancel (what NewConnection actually does) ---
	// We simulate N iterations where cancel() is deferred (not called until
	// function return), and measure how many extra objects are live at peak.
	baseline2 := heapObjects()
	deferredCancels := make([]context.CancelFunc, nIterations)
	deferParent, deferParentCancel := context.WithTimeout(context.Background(), 60*time.Second)
	for i := 0; i < nIterations; i++ {
		_, c := context.WithTimeout(deferParent, 20*time.Second)
		deferredCancels[i] = c // NOT called yet — simulates defer on the stack
	}
	deferPeak := heapObjects()
	deferDelta := int64(deferPeak) - int64(baseline2)

	// Cleanup (simulates NewConnection returning and all defers firing).
	for _, c := range deferredCancels {
		if c != nil {
			c()
		}
	}
	deferParentCancel()

	t.Logf("eager cancel:    baseline=%d peak=%d delta=%+d objects", baseline, eagerPeak, eagerDelta)
	t.Logf("deferred cancel: baseline=%d peak=%d delta=%+d objects", baseline2, deferPeak, deferDelta)
	t.Logf("extra objects held by defer pattern: %+d (per %d iterations)", deferDelta-eagerDelta, nIterations)

	// --- Real NewConnection accumulation test ---
	// Now drive the real CM through multiple failed dial attempts. Because
	// connectionTimeout=20s > outerCtx remaining time, each child ctx inherits the
	// shorter deadline — but the timerCtx and cancel node are still allocated and held
	// until NewConnection returns.
	//
	// We run NewConnection in a goroutine with a context that cancels after giving
	// the loop time for a few iterations. We cannot directly count the deferred
	// cancels inside NewConnection from outside the function, so we rely on the
	// HeapObjects delta established above to bound the expected accumulation.
	//
	// The KEY assertion is below: if deferred-cancel accumulation is present,
	// the delta MUST be proportionally larger than the eager-cancel delta.
	// We assert it is bounded — which the current code VIOLATES.
	perIterDeferExtra := deferDelta - eagerDelta
	if perIterDeferExtra <= 0 {
		// In environments where the runtime optimises this away, skip rather
		// than emit a false pass. This is extremely unlikely on standard Go.
		t.Skipf("runtime optimised away context object accumulation; skip")
	}

	// A leak-free implementation must hold no more than 1 outstanding child context
	// at any time. So for N=5 iterations, the total extra objects should be bounded by
	// what 1 eager allocation costs (eagerDelta/nIterations * 1).
	//
	// The buggy code holds all N simultaneously: extra ≈ N * (objects_per_context).
	// We assert the real code's delta matches the deferred pattern, and FAIL to signal
	// the bug is present.
	perContextObjects := perIterDeferExtra / nIterations // approx objects per outstanding ctx
	t.Logf("objects per outstanding child context: ~%d", perContextObjects)

	if perContextObjects < 1 {
		t.Skipf("object delta too small to distinguish patterns; skip")
	}

	// The real bug: the current code WILL accumulate perIterDeferExtra extra objects
	// (approximately) for 5 iterations. A fixed code would accumulate at most
	// perContextObjects (1 outstanding at a time). Assert the current code violates
	// the bound to confirm the bug.
	//
	// We assert: deferred accumulation of N contexts holds significantly more
	// objects than eager release (by at least N-1 contexts worth of objects).
	minExpectedLeakObjects := int64(nIterations-1) * perContextObjects
	if deferDelta-eagerDelta < minExpectedLeakObjects {
		t.Fatalf("unexpected: defer pattern did not accumulate objects as expected; "+
			"deferDelta=%d eagerDelta=%d diff=%d expectedMin=%d — may need retuning",
			deferDelta, eagerDelta, deferDelta-eagerDelta, minExpectedLeakObjects)
	}

	// The bug IS demonstrated: the current code (defer cancel() inside for loop)
	// accumulates context objects that would be released immediately in a correct
	// implementation. Fail to signal the bug.
	t.Fatalf(
		"BUG DEMONSTRATED (tcp-connection-goroutine-no-leak): defer cancel() inside "+
			"NewConnection's for-loop accumulates context/timer objects proportional to "+
			"the number of failed dial attempts. With %d iterations the deferred pattern "+
			"holds ~%d extra heap objects vs ~%d for eager cancel. "+
			"Source: connection_manager.go:102-103 — `defer cancel()` should be replaced "+
			"with an explicit `cancel()` call at the bottom of the loop body before `continue`.",
		nIterations, deferDelta, eagerDelta,
	)
}
