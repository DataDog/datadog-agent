// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
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

// stubHandler is a FlagHandler for testing.
type stubHandler struct {
	name       FlagName
	onChangeCh chan FlagValue
	noConfigCh chan struct{}
	recoverCh  chan FlagValue
	healthy    atomic.Bool
	onChangeFn func(FlagValue) error
}

func newStubHandler(flag FlagName) *stubHandler {
	h := &stubHandler{
		name:       flag,
		onChangeCh: make(chan FlagValue, 1),
		noConfigCh: make(chan struct{}, 1),
		recoverCh:  make(chan FlagValue, 1),
	}
	h.healthy.Store(true)
	return h
}

func (h *stubHandler) FlagName() FlagName { return h.name }
func (h *stubHandler) OnChange(v FlagValue) error {
	if h.onChangeFn != nil {
		return h.onChangeFn(v)
	}
	h.onChangeCh <- v
	return nil
}
func (h *stubHandler) OnNoConfig()                      { h.noConfigCh <- struct{}{} }
func (h *stubHandler) SafeRecover(_ error, v FlagValue) { h.recoverCh <- v }
func (h *stubHandler) IsHealthy() bool                  { return h.healthy.Load() }

// sendUpdate is a test helper that sends a flag config update to the client.
func sendUpdate(client *Client, flags ...Flag) {
	cfg, _ := json.Marshal(FlagConfig{Flags: flags})
	client.OnUpdate(
		map[string]state.RawConfig{"config": {Config: cfg}},
		func(string, state.ApplyStatus) {},
	)
}

// waitChan waits for a value on a channel with a timeout, failing the test on timeout.
func waitChan[T any](t *testing.T, ch <-chan T, timeouts ...time.Duration) T {
	t.Helper()
	timeout := time.Second
	if len(timeouts) > 0 {
		timeout = timeouts[0]
	}
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatal("timeout waiting for channel")
		var zero T
		return zero
	}
}

// the RF client must enforce non-nil handler and non-empty flag name.
func TestSubscribeWithHandler_RejectsInvalid(t *testing.T) {
	client := NewClient()
	require.Error(t, client.SubscribeWithHandler(nil))
	require.Error(t, client.SubscribeWithHandler(newStubHandler("")))
}

// the RF client immediately calls the subscribers if a value's already existing.
func TestSubscribe_ImmediateCallbackIfValueExists(t *testing.T) {
	client := NewClient()
	client.mu.Lock()
	client.currentValues[testFlag1] = true
	client.mu.Unlock()

	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	v := waitChan(t, h.onChangeCh)
	assert.True(t, bool(v))
}

// the RF client properly notifies subscribers.
func TestOnUpdate_NotifiesSubscriber(t *testing.T) {
	client := NewClient()
	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})

	v := waitChan(t, h.onChangeCh)
	assert.True(t, bool(v))

	value, exists := client.GetCurrentValue(testFlag1)
	assert.True(t, exists)
	assert.True(t, bool(value))
}

func TestOnUpdate_DeduplicatesSameValue(t *testing.T) {
	client := NewClient()
	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	// First update
	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})
	waitChan(t, h.onChangeCh)

	// Same value again: should not trigger
	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, h.onChangeCh)

	// Different value: should trigger
	sendUpdate(client, Flag{Name: string(testFlag1), Value: false})
	v := waitChan(t, h.onChangeCh)
	assert.False(t, bool(v))
}

// the RF client must return an error on an invalid json received by RC
func TestOnUpdate_InvalidJSON(t *testing.T) {
	client := NewClient()
	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	var gotError bool
	client.OnUpdate(
		map[string]state.RawConfig{"config": {Config: []byte("{bad}")}},
		func(_ string, s state.ApplyStatus) { gotError = s.State == state.ApplyStateError },
	)

	assert.True(t, gotError)
	assert.Empty(t, h.onChangeCh, "onChange must not fire on invalid JSON")
}

// the RF makes sure the NoConfig callback is called properly.
func TestOnUpdate_MissingFlagCallsOnNoConfig(t *testing.T) {
	client := NewClient()
	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	// Send an update for a different flag
	sendUpdate(client, Flag{Name: string(testFlag2), Value: true})

	waitChan(t, h.noConfigCh)
}

// the RF client correctly notify multiple subs when necessary.
func TestOnUpdate_MultipleSubscribers(t *testing.T) {
	client := NewClient()
	h1 := newStubHandler(testFlag1)
	h2 := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h1))
	require.NoError(t, client.SubscribeWithHandler(h2))

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})

	assert.True(t, bool(waitChan(t, h1.onChangeCh)))
	assert.True(t, bool(waitChan(t, h2.onChangeCh)))
}

func TestOnChange_ErrorTriggersSafeRecover(t *testing.T) {
	client := NewClient()

	h := newStubHandler(testFlag1)
	h.onChangeFn = func(FlagValue) error { return errors.New("apply failed") }
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})

	v := waitChan(t, h.recoverCh)
	assert.True(t, bool(v))
}

func TestSubscribeWithHandler(t *testing.T) {
	client := NewClient()

	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))
	require.Error(t, client.SubscribeWithHandler(nil))

	sendUpdate(client, Flag{Name: string(testFlag1), Value: true})
	assert.True(t, bool(waitChan(t, h.onChangeCh)))
}

func TestConcurrentSubscriptions(t *testing.T) {
	client := NewClient()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := newStubHandler(testFlag1)
			_ = client.SubscribeWithHandler(h)
		}()
	}
	wg.Wait()
	client.mu.Lock()
	assert.Len(t, client.subscriptions[testFlag1], 10)
	client.mu.Unlock()
}

func TestGetCurrentValue_Unknown(t *testing.T) {
	client := NewClient()
	_, exists := client.GetCurrentValue(testFlag1)
	assert.False(t, exists)
}

// After SafeRecover triggers, a recovery monitor probes IsHealthy to validate recovery.
func TestHealthMonitor_RecoveryProbeConfirmsHealthy(t *testing.T) {
	origInterval := HealthCheckInterval
	HealthCheckInterval = 100 * time.Millisecond
	defer func() { HealthCheckInterval = origInterval }()

	client := NewClient()
	defer client.Stop()

	h := newStubHandler(testFlag1)
	h.healthy.Store(false) // Start unhealthy to trigger SafeRecover
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:                             string(testFlag1),
		Value:                            true,
		HealthCheckDurationSeconds:       5,
		HealthCheckFailuresBeforeRecover: 1,
	})

	// Wait for SafeRecover to be called
	waitChan(t, h.recoverCh, 3*time.Second)

	// Simulate recovery: component becomes healthy
	h.healthy.Store(true)

	// The recovery monitor should detect the healthy state.
	time.Sleep(300 * time.Millisecond)

	// Verify no second SafeRecover call was made.
	select {
	case <-h.recoverCh:
		t.Fatal("SafeRecover should not be called again during recovery probing")
	default:
		// expected
	}
}

// Recovery monitor logs warning when component stays unhealthy through the entire probe window.
func TestHealthMonitor_RecoveryProbeStaysUnhealthy(t *testing.T) {
	origInterval := HealthCheckInterval
	HealthCheckInterval = 100 * time.Millisecond
	defer func() { HealthCheckInterval = origInterval }()

	client := NewClient()
	defer client.Stop()

	h := newStubHandler(testFlag1)
	h.healthy.Store(false) // Unhealthy throughout
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:                             string(testFlag1),
		Value:                            true,
		HealthCheckDurationSeconds:       1, // Short window so test is fast
		HealthCheckFailuresBeforeRecover: 1,
	})

	// Wait for SafeRecover
	waitChan(t, h.recoverCh, 3*time.Second)

	// Wait for the recovery probe window to expire
	time.Sleep(1500 * time.Millisecond)

	// SafeRecover should only have been called once
	select {
	case <-h.recoverCh:
		t.Fatal("SafeRecover should not be called a second time during recovery probing")
	default:
		// expected: recovery monitor gave up gracefully
	}
}
