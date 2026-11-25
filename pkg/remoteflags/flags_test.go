// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// fakeConfigSetter records Set/Unset calls and reports a configurable source per key.
type fakeConfigSetter struct {
	mu      sync.Mutex
	values  map[string]any
	sources map[string]model.Source // pre-seeded source per key, defaults to SourceDefault
	sets    []struct {
		key    string
		value  any
		source model.Source
	}
	unsets []struct {
		key    string
		source model.Source
	}
}

func newFakeConfigSetter() *fakeConfigSetter {
	return &fakeConfigSetter{
		values:  make(map[string]any),
		sources: make(map[string]model.Source),
	}
}

func (f *fakeConfigSetter) Set(key string, value any, source model.Source) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = value
	f.sources[key] = source
	f.sets = append(f.sets, struct {
		key    string
		value  any
		source model.Source
	}{key, value, source})
}

func (f *fakeConfigSetter) UnsetForSource(key string, source model.Source) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sources[key] == source {
		delete(f.values, key)
		delete(f.sources, key)
	}
	f.unsets = append(f.unsets, struct {
		key    string
		source model.Source
	}{key, source})
}

func (f *fakeConfigSetter) GetSource(key string) model.Source {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.sources[key]; ok {
		return s
	}
	return model.SourceDefault
}

func (f *fakeConfigSetter) seedSource(key string, source model.Source) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sources[key] = source
}

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
		onChangeCh: make(chan FlagValue, 16),
		noConfigCh: make(chan struct{}, 16),
		recoverCh:  make(chan FlagValue, 16),
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

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

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
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
	waitChan(t, h.onChangeCh)

	// Same value again: should not trigger
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, h.onChangeCh)

	// Different value: should trigger
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false})
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
	sendUpdate(client, Flag{Name: string(testFlag2), Enabled: true})

	waitChan(t, h.noConfigCh)
}

// the RF client correctly notify multiple subs when necessary.
func TestOnUpdate_MultipleSubscribers(t *testing.T) {
	client := NewClient()
	h1 := newStubHandler(testFlag1)
	h2 := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h1))
	require.NoError(t, client.SubscribeWithHandler(h2))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	assert.True(t, bool(waitChan(t, h1.onChangeCh)))
	assert.True(t, bool(waitChan(t, h2.onChangeCh)))
}

func TestOnChange_ErrorTriggersSafeRecover(t *testing.T) {
	client := NewClient()

	h := newStubHandler(testFlag1)
	h.onChangeFn = func(FlagValue) error { return errors.New("apply failed") }
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	v := waitChan(t, h.recoverCh)
	assert.True(t, bool(v))
}

func TestSubscribeWithHandler(t *testing.T) {
	client := NewClient()

	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))
	require.Error(t, client.SubscribeWithHandler(nil))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
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
	client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
	defer client.Stop()

	h := newStubHandler(testFlag1)
	h.healthy.Store(false) // Start unhealthy to trigger SafeRecover
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:                             string(testFlag1),
		Enabled:                          true,
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

// Versioned flags: a flag with a version less than or equal to the last applied
// version must be ignored.
func TestOnUpdate_VersionDropsOlderOrEqual(t *testing.T) {
	client := NewClient()
	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	// Apply version 5 with value=true
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, Version: 5})
	assert.True(t, bool(waitChan(t, h.onChangeCh)))

	// Older version with a different value: must be dropped
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false, Version: 3})
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, h.onChangeCh)
	v, _ := client.GetCurrentValue(testFlag1)
	assert.True(t, bool(v), "current value must remain the one from the latest applied version")

	// Same version: also dropped
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false, Version: 5})
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, h.onChangeCh)
	v, _ = client.GetCurrentValue(testFlag1)
	assert.True(t, bool(v))

	// Strictly newer version: applied
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false, Version: 6})
	assert.False(t, bool(waitChan(t, h.onChangeCh)))
}

// Versioned flags: version 0 (omitted) bypasses the sequence check.
func TestOnUpdate_VersionZeroBypassesCheck(t *testing.T) {
	client := NewClient()
	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, Version: 10})
	assert.True(t, bool(waitChan(t, h.onChangeCh)))

	// Unversioned update with a different value: applied regardless of prior version
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false})
	assert.False(t, bool(waitChan(t, h.onChangeCh)))
}

// configuration_field: when set, the client mirrors the value into pkg/config under SourceRC.
func TestOnUpdate_ConfigurationFieldMirrorsValue(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)

	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:               string(testFlag1),
		Enabled:            true,
		ConfigurationField: "feature.x.enabled",
	})

	assert.True(t, bool(waitChan(t, h.onChangeCh)))

	setter.mu.Lock()
	defer setter.mu.Unlock()
	require.Len(t, setter.sets, 1)
	assert.Equal(t, "feature.x.enabled", setter.sets[0].key)
	assert.Equal(t, true, setter.sets[0].value)
	assert.Equal(t, model.SourceRC, setter.sets[0].source)
}

// override_local: with the default (false), the client must not clobber a value already
// set by a user-facing source like SourceFile, and must skip the OnChange notification.
func TestOnUpdate_OverrideLocalFalse_RespectsLocal(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	setter.seedSource("feature.x.enabled", model.SourceFile)
	client.WithConfigSetter(setter)

	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:               string(testFlag1),
		Enabled:            true,
		ConfigurationField: "feature.x.enabled",
		// OverrideLocal default false
	})

	time.Sleep(50 * time.Millisecond)

	setter.mu.Lock()
	defer setter.mu.Unlock()
	assert.Empty(t, setter.sets, "Set must not be called when local source has precedence")
	assert.Empty(t, h.onChangeCh, "OnChange must not fire when the flag is rejected")
}

// override_local: with override_local=true, even a user-facing source is overridden.
func TestOnUpdate_OverrideLocalTrue_Overrides(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	setter.seedSource("feature.x.enabled", model.SourceFile)
	client.WithConfigSetter(setter)

	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:               string(testFlag1),
		Enabled:            true,
		ConfigurationField: "feature.x.enabled",
		OverrideLocal:      true,
	})

	assert.True(t, bool(waitChan(t, h.onChangeCh)))

	setter.mu.Lock()
	defer setter.mu.Unlock()
	require.Len(t, setter.sets, 1)
}

// override_local: SourceAgentRuntime is treated as protected (deliberate agent decision).
func TestOnUpdate_OverrideLocalFalse_ProtectsAgentRuntime(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	setter.seedSource("feature.x.enabled", model.SourceAgentRuntime)
	client.WithConfigSetter(setter)

	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:               string(testFlag1),
		Enabled:            true,
		ConfigurationField: "feature.x.enabled",
	})

	time.Sleep(50 * time.Millisecond)

	setter.mu.Lock()
	defer setter.mu.Unlock()
	assert.Empty(t, setter.sets)
}

// recover: on OnChange error, the configuration field must be unset before SafeRecover.
func TestOnChange_ErrorUnsetsConfigurationField(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)

	h := newStubHandler(testFlag1)
	h.onChangeFn = func(FlagValue) error { return errors.New("apply failed") }
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{
		Name:               string(testFlag1),
		Enabled:            true,
		ConfigurationField: "feature.x.enabled",
	})

	waitChan(t, h.recoverCh)

	setter.mu.Lock()
	defer setter.mu.Unlock()
	require.Len(t, setter.unsets, 1)
	assert.Equal(t, "feature.x.enabled", setter.unsets[0].key)
	assert.Equal(t, model.SourceRC, setter.unsets[0].source)
}

// Recovery monitor logs warning when component stays unhealthy through the entire probe window.
func TestHealthMonitor_RecoveryProbeStaysUnhealthy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer func() {
			client.Stop()
			synctest.Wait()
		}()

		h := newStubHandler(testFlag1)
		h.healthy.Store(false) // Unhealthy throughout
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{
			Name:                             string(testFlag1),
			Enabled:                          true,
			HealthCheckDurationSeconds:       1, // Short window so test is fast
			HealthCheckFailuresBeforeRecover: 1,
		})

		// Wait for SafeRecover
		waitChan(t, h.recoverCh, 3*time.Second)

		// Advance virtual time past the recovery probe window (1s) so
		// the context deadline fires and the recovery goroutine exits.
		time.Sleep(1500 * time.Millisecond)
		synctest.Wait()

		// SafeRecover should only have been called once
		select {
		case <-h.recoverCh:
			t.Fatal("SafeRecover should not be called a second time during recovery probing")
		default:
			// expected: recovery monitor gave up gracefully
		}
	})
}

// recHandler is a FlagHandler that records every callback, in completion order,
// and lets tests block or fail individual calls through hooks.
type recHandler struct {
	name        FlagName
	mu          sync.Mutex
	events      []string
	healthy     atomic.Bool
	healthCalls atomic.Int32
	onChange    func(FlagValue) error
	onNoConfig  func()
	onRecover   func()
	isHealthy   func() bool // when set, overrides healthy
}

func newRecHandler(flag FlagName) *recHandler {
	h := &recHandler{name: flag}
	h.healthy.Store(true)
	return h
}

func (h *recHandler) record(s string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, s)
}

func (h *recHandler) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.events...)
}

func (h *recHandler) count(ev string) int {
	n := 0
	for _, e := range h.snapshot() {
		if e == ev {
			n++
		}
	}
	return n
}

func (h *recHandler) FlagName() FlagName { return h.name }
func (h *recHandler) OnChange(v FlagValue) error {
	var err error
	if h.onChange != nil {
		err = h.onChange(v)
	}
	h.record(fmt.Sprintf("change(%v)", bool(v)))
	return err
}
func (h *recHandler) OnNoConfig() {
	if h.onNoConfig != nil {
		h.onNoConfig()
	}
	h.record("noconfig")
}
func (h *recHandler) SafeRecover(_ error, v FlagValue) {
	if h.onRecover != nil {
		h.onRecover()
	}
	h.record(fmt.Sprintf("recover(%v)", bool(v)))
}
func (h *recHandler) IsHealthy() bool {
	h.healthCalls.Add(1)
	if h.isHealthy != nil {
		return h.isHealthy()
	}
	return h.healthy.Load()
}

// sendRemoval sends an update that does not contain any subscribed flag.
func sendRemoval(client *Client) {
	sendUpdate(client, Flag{Name: "unrelated_flag", Enabled: true})
}

// stopAndWait stops the client and waits for every goroutine in the bubble to settle.
func stopAndWait(client *Client) {
	client.Stop()
	synctest.Wait()
}

// 1. A removal must not overtake an in-flight OnChange.
func TestOrdering_NoConfigDoesNotOvertakeInFlightChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		defer stopAndWait(client)

		gate := make(chan struct{})
		h := newRecHandler(testFlag1)
		h.onChange = func(FlagValue) error { <-gate; return nil }
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
		synctest.Wait() // OnChange(true) is now blocked

		sendRemoval(client)
		synctest.Wait()

		close(gate)
		synctest.Wait()

		assert.Equal(t, []string{"change(true)", "noconfig"}, h.snapshot())
	})
}

// 2. The flag comes back while the first OnChange is still running: the
// intermediate removal is coalesced, the final state is on, and OnChange(true)
// is delivered exactly once.
func TestOrdering_FlagReturnsAfterOverlappingRemoval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		defer stopAndWait(client)

		gate := make(chan struct{})
		h := newRecHandler(testFlag1)
		h.onChange = func(FlagValue) error { <-gate; return nil }
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
		synctest.Wait()
		sendRemoval(client)
		synctest.Wait()
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
		synctest.Wait()

		close(gate)
		synctest.Wait()

		assert.Equal(t, 1, h.count("change(true)"), "events: %v", h.snapshot())
		evs := h.snapshot()
		require.NotEmpty(t, evs)
		assert.NotEqual(t, "noconfig", evs[len(evs)-1], "final state must be on")
		v, ok := client.GetCurrentValue(testFlag1)
		assert.True(t, ok)
		assert.True(t, bool(v))
	})
}

// 3. OnChange(false) fails while a newer change(true) (which also Sets the
// configuration field) is pending: the true is delivered afterwards, and the
// recover of the failed false must not wipe the newer Set.
func TestOrdering_FailedChangeWithPendingChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		defer stopAndWait(client)
		setter := newFakeConfigSetter()
		client.WithConfigSetter(setter)

		const key = "feature.x.enabled"
		gate := make(chan struct{})
		var calls atomic.Int32
		h := newRecHandler(testFlag1)
		h.onChange = func(FlagValue) error {
			if calls.Add(1) == 2 {
				<-gate
				return errors.New("apply failed")
			}
			return nil
		}
		require.NoError(t, client.SubscribeWithHandler(h))

		flag := func(on bool) Flag {
			return Flag{Name: string(testFlag1), Enabled: FlagValue(on), ConfigurationField: key}
		}
		sendUpdate(client, flag(true))
		synctest.Wait()
		sendUpdate(client, flag(false))
		synctest.Wait() // OnChange(false) blocked
		sendUpdate(client, flag(true))
		synctest.Wait()

		close(gate)
		synctest.Wait()

		evs := h.snapshot()
		assert.Equal(t, []string{"change(true)", "change(false)", "recover(false)", "change(true)"}, evs)

		setter.mu.Lock()
		defer setter.mu.Unlock()
		assert.Equal(t, true, setter.values[key], "newer Set must not be wiped by the recover of the failed change")
		assert.Empty(t, setter.unsets, "UnsetForSource must be skipped when a newer Set happened")
	})
}

// 4. The health monitor threshold calls SafeRecover, then a recovery monitor probes.
func TestHealthMonitor_RecoverThenProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		h := newRecHandler(testFlag1)
		h.healthy.Store(false)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{
			Name:                             string(testFlag1),
			Enabled:                          true,
			HealthCheckDurationSeconds:       5,
			HealthCheckFailuresBeforeRecover: 1,
		})
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()

		require.Equal(t, 1, h.count("recover(true)"), "events: %v", h.snapshot())
		callsAtRecover := h.healthCalls.Load()

		time.Sleep(500 * time.Millisecond)
		synctest.Wait()

		assert.Greater(t, h.healthCalls.Load(), callsAtRecover, "recovery monitor must probe IsHealthy after SafeRecover")
		assert.Equal(t, 1, h.count("recover(true)"), "SafeRecover must not be called again while probing")
	})
}

// 5a. After the flag is removed no health monitor SafeRecover may fire.
func TestHealthMonitor_NoRecoverAfterRemoval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		h := newRecHandler(testFlag1)
		require.NoError(t, client.SubscribeWithHandler(h))
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, HealthCheckDurationSeconds: 10})
		synctest.Wait()

		sendRemoval(client)
		synctest.Wait()
		h.healthy.Store(false)

		time.Sleep(2 * time.Second)
		synctest.Wait()

		assert.Zero(t, h.count("recover(true)"), "events: %v", h.snapshot())
	})
}

// 5b. A monitor of an older application must not outlive or override the monitor
// of a newer one: true (threshold 1), false, true (threshold 3). Only the newest
// monitor may trigger SafeRecover, and only after its own threshold.
func TestHealthMonitor_StaleMonitorDoesNotOverrideNewer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		h := newRecHandler(testFlag1)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, HealthCheckDurationSeconds: 10, HealthCheckFailuresBeforeRecover: 1})
		synctest.Wait()
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false})
		synctest.Wait()
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, HealthCheckDurationSeconds: 10, HealthCheckFailuresBeforeRecover: 3})
		synctest.Wait()

		h.healthy.Store(false)
		time.Sleep(250 * time.Millisecond) // two failures, newest threshold is three
		synctest.Wait()
		assert.Zero(t, h.count("recover(true)"), "stale monitor fired; events: %v", h.snapshot())

		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, 1, h.count("recover(true)"), "events: %v", h.snapshot())
	})
}

// 6. A true to false transition cancels the monitor started for the true.
func TestHealthMonitor_TrueToFalseCancelsMonitor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		h := newRecHandler(testFlag1)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, HealthCheckDurationSeconds: 10})
		synctest.Wait()
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false})
		synctest.Wait()
		h.healthy.Store(false)

		time.Sleep(2 * time.Second)
		synctest.Wait()

		assert.Zero(t, h.count("recover(true)"), "monitor of the true value must be cancelled; events: %v", h.snapshot())
	})
}

// 7a. A late subscriber receives the full Flag metadata: the ConfigurationField
// is known, so a failing OnChange unsets it.
func TestSubscribe_LateSubscriberGetsConfigurationField(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		defer stopAndWait(client)
		setter := newFakeConfigSetter()
		client.WithConfigSetter(setter)

		const key = "feature.x.enabled"
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, ConfigurationField: key})
		synctest.Wait()

		h := newRecHandler(testFlag1)
		h.onChange = func(FlagValue) error { return errors.New("apply failed") }
		require.NoError(t, client.SubscribeWithHandler(h))
		synctest.Wait()

		assert.Equal(t, []string{"change(true)", "recover(true)"}, h.snapshot())
		setter.mu.Lock()
		defer setter.mu.Unlock()
		require.Len(t, setter.unsets, 1, "late subscriber must see ConfigurationField")
		assert.Equal(t, key, setter.unsets[0].key)
		_, stillSet := setter.values[key]
		assert.False(t, stillSet)
	})
}

// 7b. A late subscriber also receives the health check settings.
func TestSubscribe_LateSubscriberGetsHealthSettings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, HealthCheckDurationSeconds: 10, HealthCheckFailuresBeforeRecover: 3})
		synctest.Wait()

		h := newRecHandler(testFlag1)
		h.healthy.Store(false)
		require.NoError(t, client.SubscribeWithHandler(h))
		synctest.Wait()

		time.Sleep(250 * time.Millisecond) // two failures of three
		synctest.Wait()
		assert.Zero(t, h.count("recover(true)"), "events: %v", h.snapshot())

		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, 1, h.count("recover(true)"), "events: %v", h.snapshot())
	})
}

// 8. A handler may call back into the client from OnNoConfig. Not a synctest:
// a mutex deadlock is not a durable block there and would hang the bubble.
func TestOnNoConfig_HandlerCanCallGetCurrentValue(t *testing.T) {
	client := NewClient()
	defer client.Stop()

	called := make(chan struct{})
	h := newRecHandler(testFlag1)
	h.onNoConfig = func() {
		client.GetCurrentValue(testFlag1)
		close(called)
	}
	require.NoError(t, client.SubscribeWithHandler(h))

	done := make(chan struct{})
	go func() {
		defer close(done)
		sendRemoval(client)
	}()

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: OnNoConfig calling GetCurrentValue never returned")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnUpdate never returned")
	}
}

// 9. Stop with a pending event leaves no goroutine behind (synctest fails the
// test if any goroutine of the bubble is still blocked), and the queued event is
// not delivered after Stop.
func TestStop_WithPendingEventLeavesNoGoroutine(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()

		gate := make(chan struct{})
		h := newRecHandler(testFlag1)
		h.onChange = func(FlagValue) error { <-gate; return nil }
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
		synctest.Wait()
		sendRemoval(client) // pending behind the in-flight OnChange
		synctest.Wait()

		client.Stop()
		close(gate)
		synctest.Wait()

		assert.Equal(t, []string{"change(true)"}, h.snapshot(), "queued event must not be delivered after Stop")
	})
}

// A deduped change (same value, newer version) must leave the healthy monitor alone.
func TestHealthMonitor_DedupedChangeKeepsMonitor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		h := newRecHandler(testFlag1)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, Version: 1, HealthCheckDurationSeconds: 10})
		synctest.Wait()
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, Version: 2, HealthCheckDurationSeconds: 10})
		synctest.Wait()
		assert.Equal(t, 1, h.count("change(true)"))

		h.healthy.Store(false)
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, 1, h.count("recover(true)"), "monitor must survive the deduped change; events: %v", h.snapshot())
	})
}

// A removal followed by the same value, both queued while SafeRecover is still
// running, collapses into one change(true). It must be re-applied after the
// recover (the recover resets the last applied value), and monitored again.
func TestHealthMonitor_RecoverThenCoalescedSameValueReenables(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		h := newRecHandler(testFlag1)
		inRecover := make(chan struct{})
		releaseRecover := make(chan struct{})
		var once sync.Once
		h.onRecover = func() {
			once.Do(func() {
				close(inRecover)
				<-releaseRecover
			})
		}
		require.NoError(t, client.SubscribeWithHandler(h))

		on := Flag{Name: string(testFlag1), Enabled: true, HealthCheckDurationSeconds: 10}
		sendUpdate(client, on)
		synctest.Wait()
		require.Equal(t, 1, h.count("change(true)"))

		h.healthy.Store(false)
		time.Sleep(150 * time.Millisecond)
		<-inRecover // the worker is now blocked inside SafeRecover

		// Queued behind the recover: the removal is overwritten by the same value.
		sendRemoval(client)
		sendUpdate(client, on)
		close(releaseRecover)
		synctest.Wait()

		assert.Equal(t, 2, h.count("change(true)"), "same value must be re-applied after the recover; events: %v", h.snapshot())
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, 2, h.count("recover(true)"), "re-applied flag must be monitored again; events: %v", h.snapshot())
	})
}

// After a removal, nothing stale is stored and a late subscriber gets no change.
func TestSubscribe_NoStaleChangeAfterRemoval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		defer stopAndWait(client)
		setter := newFakeConfigSetter()
		client.WithConfigSetter(setter)

		// An early subscriber makes the flag tracked, so its removal is processed.
		early := newRecHandler(testFlag1)
		require.NoError(t, client.SubscribeWithHandler(early))
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, ConfigurationField: "feature.x.enabled"})
		synctest.Wait()
		sendRemoval(client)
		synctest.Wait()

		_, ok := client.GetCurrentValue(testFlag1)
		assert.False(t, ok)
		client.mu.Lock()
		_, stored := client.currentFlags[testFlag1]
		client.mu.Unlock()
		assert.False(t, stored, "the stored flag must be dropped together with its value")

		h := newRecHandler(testFlag1)
		require.NoError(t, client.SubscribeWithHandler(h))
		synctest.Wait()
		assert.Empty(t, h.snapshot(), "late subscriber must not get a change for a removed flag")
	})
}

// A same-value version bump re-applies the configuration field but queues no
// event, so a later health recover must still roll the field back.
func TestHealthMonitor_RecoverUnsetsFieldAfterSameValueVersionBump(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)
		setter := newFakeConfigSetter()
		client.WithConfigSetter(setter)

		h := newRecHandler(testFlag1)
		require.NoError(t, client.SubscribeWithHandler(h))

		flag := Flag{Name: string(testFlag1), Enabled: true, ConfigurationField: "feature.x.enabled", HealthCheckDurationSeconds: 10}
		flag.Version = 1
		sendUpdate(client, flag)
		synctest.Wait()
		flag.Version = 2
		sendUpdate(client, flag)
		synctest.Wait()
		require.Equal(t, 1, h.count("change(true)"))

		h.healthy.Store(false)
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()

		require.Equal(t, 1, h.count("recover(true)"), "events: %v", h.snapshot())
		setter.mu.Lock()
		defer setter.mu.Unlock()
		require.Len(t, setter.unsets, 1, "the rollback of the field must not be lost")
		assert.Equal(t, "feature.x.enabled", setter.unsets[0].key)
	})
}

// Updates false then true arrive while OnChange(true) runs: the final true is
// deduplicated against the applied value, but the monitor started for the first
// true must still roll the field back, since the field was Set again meanwhile.
func TestHealthMonitor_CoalescedBackToAppliedValueKeepsRollback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)
		setter := newFakeConfigSetter()
		client.WithConfigSetter(setter)

		gate := make(chan struct{})
		h := newRecHandler(testFlag1)
		h.onChange = func(FlagValue) error { <-gate; return nil }
		require.NoError(t, client.SubscribeWithHandler(h))

		flag := Flag{Name: string(testFlag1), Enabled: true, ConfigurationField: "feature.x.enabled", HealthCheckDurationSeconds: 10}
		sendUpdate(client, flag)
		synctest.Wait() // OnChange(true) is blocked
		flag.Enabled = false
		sendUpdate(client, flag)
		flag.Enabled = true
		sendUpdate(client, flag)
		synctest.Wait()

		close(gate)
		synctest.Wait()
		require.Equal(t, []string{"change(true)"}, h.snapshot())

		h.healthy.Store(false)
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()

		require.Equal(t, 1, h.count("recover(true)"), "events: %v", h.snapshot())
		setter.mu.Lock()
		defer setter.mu.Unlock()
		require.Len(t, setter.unsets, 1, "the rollback of the field must not be lost")
		assert.Equal(t, "feature.x.enabled", setter.unsets[0].key)
	})
}

// A cancelled monitor whose health probe finishes late must not replace the
// recover event of the current monitor while it is still waiting to be delivered.
func TestHealthMonitor_StaleMonitorCannotOverwriteCurrentRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer stopAndWait(client)

		gate := make(chan struct{})
		var calls atomic.Int32
		h := newRecHandler(testFlag1)
		h.isHealthy = func() bool {
			if calls.Add(1) == 1 {
				<-gate // the first monitor's probe hangs
			}
			return false
		}
		require.NoError(t, client.SubscribeWithHandler(h))

		on := Flag{Name: string(testFlag1), Enabled: true, HealthCheckDurationSeconds: 10, HealthCheckFailuresBeforeRecover: 1}
		sendUpdate(client, on)
		synctest.Wait()
		time.Sleep(100 * time.Millisecond)
		synctest.Wait() // first monitor is blocked inside IsHealthy
		sendUpdate(client, Flag{Name: string(testFlag1), Enabled: false})
		synctest.Wait()
		sendUpdate(client, on)
		synctest.Wait() // second monitor is installed

		client.mu.Lock()
		sub := client.subscriptions[testFlag1][0]
		client.mu.Unlock()
		// Hold delivery so the recover event waits in its slot.
		sub.qmu.Lock()
		sub.running = true
		sub.qmu.Unlock()

		time.Sleep(100 * time.Millisecond)
		synctest.Wait() // second monitor queued its recover
		close(gate)
		synctest.Wait() // first monitor returns from its probe

		go client.runWorker(sub)
		synctest.Wait()

		assert.Equal(t, 1, h.count("recover(true)"), "recovery of the current monitor was lost; events: %v", h.snapshot())
	})
}
