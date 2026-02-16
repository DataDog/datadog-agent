// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	testFlag1 FlagName = "test_feature_1"
	testFlag2 FlagName = "test_feature_2"
)

// testSub holds callback channels used to synchronize and assert in tests.
type testSub struct {
	onChange   chan FlagValue
	onNoConfig chan struct{}
	recover    chan FlagValue
}

func newTestSub() *testSub {
	return &testSub{
		onChange:   make(chan FlagValue, 1),
		onNoConfig: make(chan struct{}, 1),
		recover:    make(chan FlagValue, 1),
	}
}

func (ts *testSub) subscribe(t *testing.T, client *Client, flag FlagName) {
	t.Helper()
	err := client.Subscribe(
		flag,
		func(v FlagValue) error { ts.onChange <- v; return nil },
		func() { ts.onNoConfig <- struct{}{} },
		func(_ error, v FlagValue) { ts.recover <- v },
		func() bool { return true },
	)
	require.NoError(t, err)
}

// sendUpdate is a test helper that sends a flag config update to the client.
func sendUpdate(client *Client, flags ...Flag) {
	cfg, _ := json.Marshal(FlagConfig{Flags: flags})
	client.OnUpdate(
		map[string]state.RawConfig{"config": {Config: cfg}},
		func(string, state.ApplyStatus) {},
	)
}

// waitChan waits for a value on a channel with a 1s timeout, failing the test on timeout.
func waitChan[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel")
		var zero T
		return zero
	}
}

// the RF client must enforce non-nil callbacks.
// this unit test is here to validate it's still enforced in the future,
// or used as a strong signal that you're about to change something
// critical (if this test start failing on your change)
func TestSubscribe_RejectsNilCallbacks(t *testing.T) {
	client := NewClient()
	noop := func(FlagValue) error { return nil }
	noopNoConfig := func() {}
	noopRecover := func(error, FlagValue) {}
	noopHealthy := func() bool { return true }

	require.Error(t, client.Subscribe("f", nil, noopNoConfig, noopRecover, noopHealthy))
	require.Error(t, client.Subscribe("f", noop, nil, noopRecover, noopHealthy))
	require.Error(t, client.Subscribe("f", noop, noopNoConfig, nil, noopHealthy))
	require.Error(t, client.Subscribe("f", noop, noopNoConfig, noopRecover, nil))
	require.Error(t, client.Subscribe("", noop, noopNoConfig, noopRecover, noopHealthy))
}

// the RF client immediately calls the subscribers if a value's already existing.
func TestSubscribe_ImmediateCallbackIfValueExists(t *testing.T) {
	client := NewClient()
	client.mu.Lock()
	client.currentValues[testFlag1] = true
	client.mu.Unlock()

	sub := newTestSub()
	sub.subscribe(t, client, testFlag1)

	v := waitChan(t, sub.onChange)
	assert.True(t, bool(v))
}

// the RF client properly notifies subscribers.
func TestOnUpdate_NotifiesSubscriber(t *testing.T) {
	client := NewClient()
	sub := newTestSub()
	sub.subscribe(t, client, testFlag1)

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})

	v := waitChan(t, sub.onChange)
	assert.True(t, bool(v))

	value, exists := client.GetCurrentValue(testFlag1)
	assert.True(t, exists)
	assert.True(t, bool(value))
}

func TestOnUpdate_DeduplicatesSameValue(t *testing.T) {
	client := NewClient()
	sub := newTestSub()
	sub.subscribe(t, client, testFlag1)

	// First update
	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})
	waitChan(t, sub.onChange)

	// Same value again: should not trigger
	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, sub.onChange)

	// Different value: should trigger
	sendUpdate(client, Flag{Name: string(testFlag1), Value: false})
	v := waitChan(t, sub.onChange)
	assert.False(t, bool(v))
}

// the RF client must return an error on an invalid json received by RC
func TestOnUpdate_InvalidJSON(t *testing.T) {
	client := NewClient()
	sub := newTestSub()
	sub.subscribe(t, client, testFlag1)

	var gotError bool
	client.OnUpdate(
		map[string]state.RawConfig{"config": {Config: []byte("{bad}")}},
		func(_ string, s state.ApplyStatus) { gotError = s.State == state.ApplyStateError },
	)

	assert.True(t, gotError)
	assert.Empty(t, sub.onChange, "onChange must not fire on invalid JSON")
}

// the RF makes sure the NoConfig callback is called properly.
func TestOnUpdate_MissingFlagCallsOnNoConfig(t *testing.T) {
	client := NewClient()
	sub := newTestSub()
	sub.subscribe(t, client, testFlag1)

	// Send an update for a different flag
	sendUpdate(client, Flag{Name: string(testFlag2), Value: true})

	waitChan(t, sub.onNoConfig)
}

// the RF client correctly notify multiple subs when necessary.
func TestOnUpdate_MultipleSubscribers(t *testing.T) {
	client := NewClient()
	sub1 := newTestSub()
	sub2 := newTestSub()
	sub1.subscribe(t, client, testFlag1)
	sub2.subscribe(t, client, testFlag1)

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})

	assert.True(t, bool(waitChan(t, sub1.onChange)))
	assert.True(t, bool(waitChan(t, sub2.onChange)))
}

func TestOnChange_ErrorTriggersSafeRecover(t *testing.T) {
	client := NewClient()

	recoverCh := make(chan FlagValue, 1)
	err := client.Subscribe(
		testFlag1,
		func(FlagValue) error { return errors.New("apply failed") },
		func() {},
		func(_ error, v FlagValue) { recoverCh <- v },
		func() bool { return true },
	)
	require.NoError(t, err)

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})

	v := waitChan(t, recoverCh)
	assert.True(t, bool(v))
}

func TestSubscribeWithHandler(t *testing.T) {
	client := NewClient()

	h := &stubHandler{
		name:     testFlag1,
		onChange: make(chan FlagValue, 1),
		noConfig: make(chan struct{}, 1),
	}
	require.NoError(t, client.SubscribeWithHandler(h))
	require.Error(t, client.SubscribeWithHandler(nil))

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})
	assert.True(t, bool(waitChan(t, h.onChange)))
}

func TestConcurrentSubscriptions(t *testing.T) {
	client := NewClient()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := newTestSub()
			sub.subscribe(t, client, testFlag1)
		}()
	}
	wg.Wait()
	client.mu.RLock()
	assert.Len(t, client.subscriptions[testFlag1], 10)
	client.mu.RUnlock()
}

func TestGetCurrentValue_Unknown(t *testing.T) {
	client := NewClient()
	_, exists := client.GetCurrentValue(testFlag1)
	assert.False(t, exists)
}

// stubHandler is a minimal FlagHandler for testing SubscribeWithHandler.
type stubHandler struct {
	name     FlagName
	onChange chan FlagValue
	noConfig chan struct{}
}

func (h *stubHandler) FlagName() FlagName               { return h.name }
func (h *stubHandler) OnChange(v FlagValue) error       { h.onChange <- v; return nil }
func (h *stubHandler) OnNoConfig()                      { h.noConfig <- struct{}{} }
func (h *stubHandler) SafeRecover(_ error, _ FlagValue) {}
func (h *stubHandler) IsHealthy() bool                  { return true }
