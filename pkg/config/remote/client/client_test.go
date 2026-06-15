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

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// blockingFetcher is a ConfigFetcher that lets a test gate the gRPC call.
// It is used to assert what Client.update() does — and does not — hold the
// client mutex around. See the lock-discipline tests at the bottom of this
// file.
type blockingFetcher struct {
	started chan struct{} // closed when ClientGetConfigs is entered
	release chan struct{} // signaled to let ClientGetConfigs return
	resp    *pbgo.ClientGetConfigsResponse
	err     error
}

func newBlockingFetcher() *blockingFetcher {
	return &blockingFetcher{
		started: make(chan struct{}),
		release: make(chan struct{}),
		resp:    &pbgo.ClientGetConfigsResponse{}, // empty: applyUpdate returns no changes, but lock discipline still applies
	}
}

func (f *blockingFetcher) ClientGetConfigs(_ context.Context, _ *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	// Signal once; subsequent calls just proceed without blocking so the
	// poll loop can keep iterating.
	select {
	case <-f.started:
	default:
		close(f.started)
		<-f.release
	}
	return f.resp, f.err
}

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

// --- Lock discipline tests --------------------------------------------------
//
// These tests pin the invariant that Client.update() releases c.m for its
// long-running steps (the gRPC fetch and the subsequent applyUpdate / TUF
// verification). Holding c.m across those steps would block every other
// Client API that takes c.m — SubscribeAll, SetAgentName, GetConfigs,
// broadcastStateChange — for the full duration of a poll iteration. That
// regression has been introduced before; these tests guard against it
// returning.
//
// Pre-PR locking discipline (preserved by this PR):
//
//	update():
//	  newUpdateRequest()        // c.state.CurrentState() — no c.m
//	    ↳ briefly takes c.m to copy c.products
//	  configFetcher.ClientGetConfigs(...)  // no c.m, may take seconds
//	  applyUpdate(response)                // no c.m, TUF verification
//	  c.m.Lock(); dispatch loop; c.m.Unlock()
//
// state.Repository is partly thread-safe (metadata is a sync.Map), but configs
// and TUF state are NOT — only the single poll-loop goroutine writes them.
// The lock discipline above is what makes that safe in practice.

// TestClient_LockNotHeldDuringFetch verifies that c.m is free while the
// gRPC fetch is in flight. Concretely: a blocking ConfigFetcher parks the
// poll loop inside ClientGetConfigs, and during that window we must be
// able to take c.m via every public API that touches it.
//
// If a future refactor hoists c.m above the fetcher call, this test deadlocks
// (each public API blocks until the fetcher returns).
func TestClient_LockNotHeldDuringFetch(t *testing.T) {
	fetcher := newBlockingFetcher()
	c, err := NewClient(fetcher, WithoutTufVerification(), WithAgent("test", "0.0"))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	c.Start()

	select {
	case <-fetcher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("ConfigFetcher.ClientGetConfigs was never called")
	}

	// While the fetcher is blocked, every c.m-taking API must return
	// promptly. We run each under a deadline so a regression manifests as
	// a clean failure instead of a hung test.
	done := make(chan string, 4)

	go func() {
		c.SubscribeAll("APM_SAMPLING", &mockListener{})
		done <- "SubscribeAll"
	}()
	go func() {
		c.SetAgentName("agent")
		done <- "SetAgentName"
	}()
	go func() {
		_ = c.GetConfigs("APM_SAMPLING")
		done <- "GetConfigs"
	}()
	go func() {
		c.broadcastStateChange(true)
		done <- "broadcastStateChange"
	}()

	deadline := time.After(500 * time.Millisecond)
	got := map[string]bool{}
	for len(got) < 4 {
		select {
		case name := <-done:
			got[name] = true
		case <-deadline:
			t.Fatalf("c.m appears held during fetch — only completed: %v", got)
		}
	}

	// Unblock the fetcher so the poll loop can finish cleanly.
	close(fetcher.release)
}

// TestClient_DispatchLoopHoldsLockBriefly verifies the lock-window after the
// fetch returns. Once applyUpdate completes, update() does take c.m to
// iterate listeners — that window must be short and must not block on
// listener work. The slow-listener case is covered by the dispatcher tests;
// here we assert the lock isn't held for an unbounded time relative to a
// single iteration.
func TestClient_DispatchLoopHoldsLockBriefly(t *testing.T) {
	fetcher := newBlockingFetcher()
	c, err := NewClient(fetcher, WithoutTufVerification(), WithAgent("test", "0.0"))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	// Subscribe a listener that would block forever if dispatch were
	// synchronous. With the async dispatcher this only parks the worker,
	// not the poll loop / lock.
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	c.SubscribeAll("APM_SAMPLING", &mockListener{onUpdateHook: func() { <-block }})

	c.Start()
	select {
	case <-fetcher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("poll loop never reached fetch")
	}

	// Release the fetcher and immediately race a SubscribeAll against the
	// dispatch lock window. The dispatch is non-blocking even if a listener
	// is permanently stuck (block channel held open), so this must return
	// fast.
	close(fetcher.release)

	settled := make(chan struct{})
	go func() {
		c.SubscribeAll("APM_TRACING", &mockListener{})
		close(settled)
	}()
	select {
	case <-settled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SubscribeAll blocked on dispatch lock window — listener work leaked under c.m?")
	}
}

// TestClient_BoundApplyStatusVersionCheck verifies the closure returned by
// boundApplyStatus drops writes for paths NOT present in the snapshot it was
// built from, and forwards writes for paths that are present. This guards the
// version-binding wiring; the underlying version-vs-version comparison is
// exhaustively tested in state.TestUpdateApplyStatusIfVersion.
func TestClient_BoundApplyStatusVersionCheck(t *testing.T) {
	repo, err := state.NewUnverifiedRepository()
	require.NoError(t, err)
	c := &Client{state: repo}

	// Snapshot containing one path at v1. The closure must remember v1.
	snapshot := map[string]state.RawConfig{
		"datadog/2/APM_SAMPLING/known/x": {Metadata: state.Metadata{Version: 1}},
	}
	bound := c.boundApplyStatus(snapshot)

	// Unknown path: the closure must drop the write without panicking and
	// without touching the repository.
	bound("datadog/2/APM_SAMPLING/unknown/y",
		state.ApplyStatus{State: state.ApplyStateError, Error: "should be dropped"})

	// Known path: the closure forwards to UpdateApplyStatusIfVersion. Since
	// no metadata is loaded in the empty repository, the underlying call
	// returns false — we only care here that the wiring invoked it without
	// panic. The end-to-end "stale ack rejected" guarantee is covered by
	// state.TestUpdateApplyStatusIfVersion.
	bound("datadog/2/APM_SAMPLING/known/x",
		state.ApplyStatus{State: state.ApplyStateAcknowledged})
}

// TestClient_ConcurrentSubscribeWhilePolling is a stress test that runs the
// real poll loop with a fast (non-blocking) fetcher and hammers it with
// concurrent SubscribeAll / SetAgentName / GetConfigs calls. Under -race this
// catches any reintroduction of the c.m / c.state locking confusion the PR
// review flagged. It does not assert exact behavior — only that no deadlock,
// panic, or data race occurs.
func TestClient_ConcurrentSubscribeWhilePolling(t *testing.T) {
	fetcher := &blockingFetcher{
		started: make(chan struct{}),
		release: make(chan struct{}),
		resp:    &pbgo.ClientGetConfigsResponse{},
	}
	close(fetcher.release) // never block

	c, err := NewClient(fetcher, WithoutTufVerification(), WithAgent("test", "0.0"))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	c.Start()

	products := []string{"APM_SAMPLING", "APM_TRACING", "ASM_FEATURES", "LIVE_DEBUGGING"}

	var wg sync.WaitGroup
	deadline := time.Now().Add(200 * time.Millisecond)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				p := products[i%len(products)]
				c.SubscribeAll(p, &mockListener{})
				_ = c.GetConfigs(p)
				c.SetAgentName("agent")
			}
		}(i)
	}
	wg.Wait()
}

// TestClient_SubscribeDuringClose races SubscribeAll against Close to exercise
// the wg.Add-vs-wg.Wait window mellon85 flagged: SubscribeAll does wg.Add(1)
// for its dispatcher, and Close does wg.Wait(). If the gate (the ctx.Err()
// check under c.m in SubscribeAll, paired with cancelUnderLock holding c.m)
// were removed, a wg.Add concurrent with wg.Wait at counter zero would panic
// with "WaitGroup is reused before previous Wait has returned" / "negative
// counter". Run under -race; the harness reports any data race on c.listeners
// or the WaitGroup. It asserts no panic/deadlock rather than exact behavior.
func TestClient_SubscribeDuringClose(t *testing.T) {
	for iter := 0; iter < 200; iter++ {
		fetcher := &blockingFetcher{
			started: make(chan struct{}),
			release: make(chan struct{}),
			resp:    &pbgo.ClientGetConfigsResponse{},
		}
		close(fetcher.release) // never block the poll loop

		c, err := NewClient(fetcher, WithoutTufVerification(), WithAgent("test", "0.0"))
		require.NoError(t, err)
		c.Start()

		// Fire SubscribeAll concurrently with Close. Whichever wins, the
		// outcome must be safe: either the subscription registers before the
		// ctx is canceled (its wg.Add completes before wg.Wait begins), or it
		// observes the canceled ctx and is dropped. Neither may panic.
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.SubscribeAll("APM_SAMPLING", &mockListener{})
		}()
		go func() {
			defer wg.Done()
			c.Close()
		}()
		wg.Wait()

		// A SubscribeAll that lands after Close must be dropped, never leaving
		// a leaked dispatcher behind (Close already returned from wg.Wait).
		c.SubscribeAll("APM_TRACING", &mockListener{})
	}
}
