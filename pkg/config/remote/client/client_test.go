// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package client

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// mockListener captures OnUpdate / OnStateChange calls and (optionally) injects
// latency or counts concurrency.
type mockListener struct {
	mu           sync.Mutex
	updates      []map[string]state.RawConfig
	stateChanges []bool

	onUpdateHook func()

	inflight    atomic.Int32
	maxInflight atomic.Int32
}

func (m *mockListener) OnUpdate(configs map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
	n := m.inflight.Add(1)
	for {
		prev := m.maxInflight.Load()
		if n <= prev || m.maxInflight.CompareAndSwap(prev, n) {
			break
		}
	}
	defer m.inflight.Add(-1)

	if m.onUpdateHook != nil {
		m.onUpdateHook()
	}

	m.mu.Lock()
	m.updates = append(m.updates, configs)
	m.mu.Unlock()
}

func (m *mockListener) OnStateChange(connected bool) {
	m.mu.Lock()
	m.stateChanges = append(m.stateChanges, connected)
	m.mu.Unlock()
}

func (*mockListener) ShouldIgnoreSignatureExpiration() bool { return true }

func (m *mockListener) updateCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.updates)
}

func (m *mockListener) stateChangeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.stateChanges)
}

// newTestEntry wires up a listenerEntry with a dispatcher goroutine that
// terminates when the test ends.
func newTestEntry(t *testing.T, l Listener) *listenerEntry {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	e := &listenerEntry{
		listener: l,
		wake:     make(chan struct{}, 1),
	}
	go e.run(ctx)
	return e
}

func TestListenerEntry_DeliversUpdate(t *testing.T) {
	l := &mockListener{}
	e := newTestEntry(t, l)

	snapshot := map[string]state.RawConfig{"path/a": {}}
	e.scheduleUpdate(snapshot, nil)

	require.Eventually(t, func() bool { return l.updateCount() == 1 },
		time.Second, 5*time.Millisecond)

	l.mu.Lock()
	defer l.mu.Unlock()
	assert.Equal(t, snapshot, l.updates[0])
}

func TestListenerEntry_DeliversStateChange(t *testing.T) {
	l := &mockListener{}
	e := newTestEntry(t, l)

	e.scheduleStateChange(false)

	require.Eventually(t, func() bool { return l.stateChangeCount() == 1 },
		time.Second, 5*time.Millisecond)

	l.mu.Lock()
	defer l.mu.Unlock()
	assert.Equal(t, []bool{false}, l.stateChanges)
}

// TestListenerEntry_CoalescesUpdates verifies that schedules arriving while the
// worker is busy collapse into a single delivery carrying the freshest
// snapshot.
func TestListenerEntry_CoalescesUpdates(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	firstCall := atomic.Bool{}

	l := &mockListener{
		onUpdateHook: func() {
			// Block the first OnUpdate so subsequent schedules pile up. Later
			// calls fall through immediately.
			if firstCall.CompareAndSwap(false, true) {
				close(started)
				<-release
			}
		},
	}
	e := newTestEntry(t, l)

	s1 := map[string]state.RawConfig{"a": {Config: []byte("1")}}
	s2 := map[string]state.RawConfig{"b": {Config: []byte("2")}}
	s3 := map[string]state.RawConfig{"c": {Config: []byte("3")}}

	e.scheduleUpdate(s1, nil)
	<-started // first OnUpdate is now blocked

	// While blocked, push two more schedules. Only the latest snapshot should
	// be visible when the worker resumes.
	e.scheduleUpdate(s2, nil)
	e.scheduleUpdate(s3, nil)

	close(release)

	require.Eventually(t, func() bool { return l.updateCount() == 2 },
		time.Second, 5*time.Millisecond)

	l.mu.Lock()
	defer l.mu.Unlock()
	assert.Equal(t, s1, l.updates[0])
	assert.Equal(t, s3, l.updates[1], "expected s2 to be coalesced away")
}

// TestListenerEntry_SerialDispatch asserts that the worker never invokes
// OnUpdate concurrently with itself for the same entry, even under a flood of
// schedules.
func TestListenerEntry_SerialDispatch(t *testing.T) {
	l := &mockListener{
		onUpdateHook: func() { time.Sleep(2 * time.Millisecond) },
	}
	e := newTestEntry(t, l)

	for i := 0; i < 100; i++ {
		e.scheduleUpdate(map[string]state.RawConfig{}, nil)
	}

	// Wait for the worker to drain whatever wasn't coalesced.
	require.Eventually(t, func() bool { return l.inflight.Load() == 0 && l.updateCount() >= 1 },
		2*time.Second, 5*time.Millisecond)

	assert.Equal(t, int32(1), l.maxInflight.Load(),
		"OnUpdate must never run concurrently for the same listener")
}

// TestListenerEntry_StateChangeBeforeUpdate confirms that when both an
// OnStateChange and an OnUpdate are pending in a single drain, the state
// change is delivered first.
func TestListenerEntry_StateChangeBeforeUpdate(t *testing.T) {
	ordered := make(chan string, 4)
	l := &deliveryOrderListener{ch: ordered}

	// Spawn entry but DO NOT consume yet — we want both signals staged before
	// the worker drains. We do this by holding the worker via a hook on the
	// first iteration. Instead we just schedule both back-to-back; since
	// scheduleUpdate signals only after the field is set, and the worker won't
	// start draining instantly, both fields land before a single wake fires.
	//
	// To guarantee both land in the same drain, prime the worker via a busy
	// hook on a sentinel call first.
	gateOpen := make(chan struct{})
	l.gate = gateOpen

	e := newTestEntry(t, l)

	// Sentinel: enqueue an update that blocks the worker so the next two
	// schedules accumulate in pending fields.
	e.scheduleUpdate(map[string]state.RawConfig{"warm": {}}, nil)
	// Wait for the worker to be inside the sentinel call.
	require.Eventually(t, func() bool { return l.inGate() }, time.Second, 5*time.Millisecond)

	// Stage both fields while the worker is parked.
	e.scheduleStateChange(true)
	e.scheduleUpdate(map[string]state.RawConfig{"hot": {}}, nil)

	// Release the worker.
	close(gateOpen)

	// Expect: sentinel update, then state change, then hot update.
	require.Eventually(t, func() bool { return len(ordered) >= 3 },
		time.Second, 5*time.Millisecond)

	got := drainChan(ordered)
	assert.Equal(t, []string{"update", "state", "update"}, got)
}

func drainChan(ch chan string) []string {
	out := []string{}
	for {
		select {
		case v := <-ch:
			out = append(out, v)
		default:
			return out
		}
	}
}

// deliveryOrderListener records the sequence of OnUpdate vs OnStateChange calls
// and lets the test park the worker inside the first OnUpdate.
type deliveryOrderListener struct {
	ch chan string

	mu   sync.Mutex
	gate <-chan struct{}
	in   bool
}

func (d *deliveryOrderListener) inGate() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.in
}

func (d *deliveryOrderListener) OnUpdate(_ map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
	d.mu.Lock()
	gate := d.gate
	d.gate = nil
	if gate != nil {
		d.in = true
	}
	d.mu.Unlock()

	if gate != nil {
		<-gate
		d.mu.Lock()
		d.in = false
		d.mu.Unlock()
	}
	d.ch <- "update"
}

func (d *deliveryOrderListener) OnStateChange(_ bool) {
	d.ch <- "state"
}

func (*deliveryOrderListener) ShouldIgnoreSignatureExpiration() bool { return true }

// TestListenerEntry_ExitsOnContextCancel verifies the dispatcher goroutine
// exits when the client context is canceled.
func TestListenerEntry_ExitsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	e := &listenerEntry{
		listener: &mockListener{},
		wake:     make(chan struct{}, 1),
	}
	done := make(chan struct{})
	go func() {
		e.run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not exit after ctx cancel")
	}
}

// TestListenerEntry_SignalDoesNotBlock asserts that scheduleUpdate is
// non-blocking even when the worker is stuck inside OnUpdate. This is the
// load-bearing property: a slow listener cannot delay the poll loop.
func TestListenerEntry_SignalDoesNotBlock(t *testing.T) {
	stuck := make(chan struct{})
	l := &mockListener{
		onUpdateHook: func() { <-stuck },
	}
	e := newTestEntry(t, l)
	t.Cleanup(func() { close(stuck) })

	// Park the worker inside OnUpdate.
	e.scheduleUpdate(map[string]state.RawConfig{}, nil)
	require.Eventually(t, func() bool { return l.inflight.Load() == 1 },
		time.Second, 5*time.Millisecond)

	// Now flood scheduleUpdate calls — none should block on the stuck worker.
	deadline := time.Now().Add(100 * time.Millisecond)
	for i := 0; i < 10_000; i++ {
		e.scheduleUpdate(map[string]state.RawConfig{}, nil)
	}
	require.True(t, time.Now().Before(deadline),
		"scheduleUpdate blocked while worker was stuck — this defeats the whole design")
}

// TestListenerEntry_RecoversFromPanic verifies that a panicking listener does
// not kill the dispatcher goroutine — subsequent updates still get delivered.
func TestListenerEntry_RecoversFromPanic(t *testing.T) {
	calls := atomic.Int32{}
	l := &mockListener{
		onUpdateHook: func() {
			if calls.Add(1) == 1 {
				panic("boom")
			}
		},
	}
	e := newTestEntry(t, l)

	e.scheduleUpdate(map[string]state.RawConfig{"a": {}}, nil)
	// Wait for the panicking call to be observed by the listener counter.
	require.Eventually(t, func() bool { return calls.Load() >= 1 },
		time.Second, 5*time.Millisecond)

	// Schedule a second update; if the dispatcher survived the panic, it
	// should be delivered.
	e.scheduleUpdate(map[string]state.RawConfig{"b": {}}, nil)
	require.Eventually(t, func() bool { return calls.Load() >= 2 },
		time.Second, 5*time.Millisecond)
}

// TestClient_CloseWaitsForInflightCallback verifies that Close blocks until
// any in-flight OnUpdate finishes — callers can rely on "no callbacks after
// Close returns" when tearing down listener-owned resources.
func TestClient_CloseWaitsForInflightCallback(t *testing.T) {
	release := make(chan struct{})
	finished := atomic.Bool{}
	l := &mockListener{
		onUpdateHook: func() {
			<-release
			finished.Store(true)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		ctx:       ctx,
		closeFn:   cancel,
		listeners: make(map[string][]*listenerEntry),
	}

	entry := &listenerEntry{listener: l, wake: make(chan struct{}, 1)}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		entry.run(c.ctx)
	}()
	c.listeners["p"] = []*listenerEntry{entry}

	// Park the worker inside OnUpdate.
	entry.scheduleUpdate(map[string]state.RawConfig{}, nil)
	require.Eventually(t, func() bool { return l.inflight.Load() == 1 },
		time.Second, 5*time.Millisecond)

	// Close should not return until OnUpdate finishes.
	closeReturned := make(chan struct{})
	go func() {
		c.Close()
		close(closeReturned)
	}()

	select {
	case <-closeReturned:
		t.Fatal("Close returned while OnUpdate was still in flight")
	case <-time.After(50 * time.Millisecond):
		// expected: Close is blocked on wg.Wait
	}

	close(release)

	select {
	case <-closeReturned:
		// expected: Close unblocked after callback finished
	case <-time.After(time.Second):
		t.Fatal("Close did not return after callback finished")
	}
	assert.True(t, finished.Load(), "OnUpdate did not run to completion")
}

// TestClient_SubscribeAfterCloseIsNoop verifies that registering a listener
// after Close has run does not spawn a goroutine that survives the wg.Wait
// barrier — the subscription is silently dropped.
func TestClient_SubscribeAfterCloseIsNoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		ctx:       ctx,
		closeFn:   cancel,
		listeners: make(map[string][]*listenerEntry),
	}

	c.Close() // cancels ctx, Wait returns immediately (wg is empty)

	l := &mockListener{}
	c.SubscribeAll("p", l)

	// No worker should have been spawned, no entry registered.
	c.m.Lock()
	assert.Empty(t, c.listeners["p"], "post-Close SubscribeAll should not register a listener")
	c.m.Unlock()
}

// TestClient_SubscribeAllRegistersWorkerTrackedByWg verifies the
// SubscribeAll → worker-goroutine → wg path end-to-end: after a real
// SubscribeAll, Close() must drain the dispatcher (a future refactor that
// forgets to wg.Add on the worker would hang or race here).
func TestClient_SubscribeAllRegistersWorkerTrackedByWg(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		ctx:       ctx,
		closeFn:   cancel,
		listeners: make(map[string][]*listenerEntry),
	}

	l := &mockListener{}
	c.SubscribeAll("p", l)

	c.m.Lock()
	require.Len(t, c.listeners["p"], 1, "SubscribeAll should register exactly one entry")
	require.Contains(t, c.products, "p", "SubscribeAll should track the product")
	c.m.Unlock()

	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not return — worker goroutine is not tracked in wg")
	}
}

// TestClient_SubscribeAllRaceWithClose hammers concurrent SubscribeAll/Close
// to verify the wg.Add ↔ wg.Wait race documented in cancelUnderLock cannot
// trigger sync.WaitGroup's "Add concurrent with Wait" panic. Run with -race.
func TestClient_SubscribeAllRaceWithClose(t *testing.T) {
	for iter := 0; iter < 50; iter++ {
		ctx, cancel := context.WithCancel(context.Background())
		c := &Client{
			ctx:       ctx,
			closeFn:   cancel,
			listeners: make(map[string][]*listenerEntry),
		}

		var wg sync.WaitGroup
		// Fire several SubscribeAll calls in parallel with Close. Whichever
		// wins, none of them must panic.
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.SubscribeAll("p", &mockListener{})
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Close()
		}()
		wg.Wait()

		// Drain any worker that won the race.
		c.Close()
	}
}

// TestClient_RecoveryStateChangeDeliveredBeforeUpdate covers the
// error→success transition that pollLoop fires: broadcastStateChange(true) is
// staged, then update() stages a fresh OnUpdate. When both land in a single
// drain, the listener must see OnStateChange first so it knows RC is back
// before processing the accompanying snapshot.
func TestClient_RecoveryStateChangeDeliveredBeforeUpdate(t *testing.T) {
	ordered := make(chan string, 4)
	l := &deliveryOrderListener{ch: ordered}

	gateOpen := make(chan struct{})
	l.gate = gateOpen

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &Client{
		ctx:       ctx,
		closeFn:   cancel,
		listeners: make(map[string][]*listenerEntry),
	}

	entry := &listenerEntry{listener: l, wake: make(chan struct{}, 1)}
	go entry.run(ctx)
	c.listeners["p"] = []*listenerEntry{entry}

	// Park the worker inside a sentinel OnUpdate so the recovery signals
	// accumulate in pending fields.
	entry.scheduleUpdate(map[string]state.RawConfig{"warm": {}}, nil)
	require.Eventually(t, l.inGate, time.Second, 5*time.Millisecond)

	// Recovery path: pollLoop calls broadcastStateChange(true) right before
	// the next successful update() dispatches its snapshot.
	c.broadcastStateChange(true)
	entry.scheduleUpdate(map[string]state.RawConfig{"recovered": {}}, nil)

	close(gateOpen)

	require.Eventually(t, func() bool { return len(ordered) >= 3 },
		time.Second, 5*time.Millisecond)
	got := drainChan(ordered)
	assert.Equal(t, []string{"update", "state", "update"}, got,
		"on recovery, OnStateChange(true) must precede the OnUpdate in the same drain")
}

// TestClient_CloseTimeout verifies that CloseTimeout returns false promptly
// when a listener is stuck, and true once it finishes. Either way the client
// context is canceled.
func TestClient_CloseTimeout(t *testing.T) {
	release := make(chan struct{})
	l := &mockListener{onUpdateHook: func() { <-release }}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		ctx:       ctx,
		closeFn:   cancel,
		listeners: make(map[string][]*listenerEntry),
	}
	entry := &listenerEntry{listener: l, wake: make(chan struct{}, 1)}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		entry.run(c.ctx)
	}()
	c.listeners["p"] = []*listenerEntry{entry}

	entry.scheduleUpdate(map[string]state.RawConfig{}, nil)
	require.Eventually(t, func() bool { return l.inflight.Load() == 1 },
		time.Second, 5*time.Millisecond)

	start := time.Now()
	ok := c.CloseTimeout(50 * time.Millisecond)
	elapsed := time.Since(start)
	assert.False(t, ok, "CloseTimeout should report failure when listener is stuck")
	assert.Less(t, elapsed, 200*time.Millisecond, "CloseTimeout overshot its deadline")
	assert.Error(t, c.ctx.Err(), "ctx should be canceled even on timeout")

	// Release the listener and verify a follow-up CloseTimeout drains cleanly.
	close(release)
	assert.True(t, c.CloseTimeout(time.Second),
		"CloseTimeout should report success once listeners finish")
}

// TestClient_BroadcastStateChange_NonBlocking confirms broadcastStateChange
// returns promptly even when a listener's worker is busy.
func TestClient_BroadcastStateChange_NonBlocking(t *testing.T) {
	stuck := make(chan struct{})
	slow := &mockListener{onUpdateHook: func() { <-stuck }}
	fast := &mockListener{}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		close(stuck)
		cancel()
	})

	c := &Client{
		ctx:       ctx,
		listeners: make(map[string][]*listenerEntry),
	}

	mkEntry := func(l Listener) *listenerEntry {
		e := &listenerEntry{listener: l, wake: make(chan struct{}, 1)}
		go e.run(ctx)
		return e
	}
	eSlow := mkEntry(slow)
	eFast := mkEntry(fast)
	c.listeners["slow"] = []*listenerEntry{eSlow}
	c.listeners["fast"] = []*listenerEntry{eFast}

	// Park the slow worker inside OnUpdate.
	eSlow.scheduleUpdate(map[string]state.RawConfig{}, nil)
	require.Eventually(t, func() bool { return slow.inflight.Load() == 1 },
		time.Second, 5*time.Millisecond)

	start := time.Now()
	c.broadcastStateChange(true)
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 50*time.Millisecond, "broadcastStateChange blocked on slow listener")

	// The fast listener should still observe the state change promptly.
	require.Eventually(t, func() bool { return fast.stateChangeCount() == 1 },
		time.Second, 5*time.Millisecond)
}
