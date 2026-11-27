// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

// orderHandler records its callbacks in order. OnChange called with one of the
// blocked values waits until release(value), to simulate a slow handler.
type orderHandler struct {
	name    FlagName
	blocked map[FlagValue]chan struct{}
	healthy atomic.Bool
	mu      sync.Mutex
	events  []string
}

func newOrderHandler(flag FlagName, blocked ...FlagValue) *orderHandler {
	h := &orderHandler{name: flag, blocked: map[FlagValue]chan struct{}{}}
	for _, v := range blocked {
		h.blocked[v] = make(chan struct{})
	}
	h.healthy.Store(true)
	return h
}

func (h *orderHandler) release(v FlagValue) { close(h.blocked[v]) }

func (h *orderHandler) record(e string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
}

func (h *orderHandler) recorded() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.events)
}

func (h *orderHandler) FlagName() FlagName { return h.name }
func (h *orderHandler) OnChange(v FlagValue) error {
	if ch, ok := h.blocked[v]; ok {
		<-ch
	}
	h.record(fmt.Sprintf("change:%v", v))
	return nil
}
func (h *orderHandler) OnNoConfig()                      { h.record("noconfig") }
func (h *orderHandler) SafeRecover(_ error, v FlagValue) { h.record(fmt.Sprintf("recover:%v", v)) }
func (h *orderHandler) IsHealthy() bool                  { return h.healthy.Load() }

func TestOnUpdate_DelayedOnChangeDoesNotOverrideNoConfig(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		defer func() {
			client.Stop()
			synctest.Wait()
		}()
		h := newOrderHandler("my_flag", true)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: "my_flag", Enabled: true})
		synctest.Wait()    // OnChange(true) is blocked
		sendUpdate(client) // the flag is removed while OnChange(true) is still running
		synctest.Wait()
		h.release(true)
		synctest.Wait()

		assert.Equal(t, []string{"change:true", "noconfig"}, h.recorded())
	})
}

func TestOnUpdate_OnChangeCallbacksKeepUpdateOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		defer func() {
			client.Stop()
			synctest.Wait()
		}()
		h := newOrderHandler("my_flag", true)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: "my_flag", Enabled: true})
		synctest.Wait() // OnChange(true) is blocked
		sendUpdate(client, Flag{Name: "my_flag", Enabled: false})
		synctest.Wait()
		h.release(true)
		synctest.Wait()

		assert.Equal(t, []string{"change:true", "change:false"}, h.recorded())
	})
}

func TestOnUpdate_RemovalCancelsHealthMonitorBehindSlowOnChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer func() {
			client.Stop()
			synctest.Wait()
		}()
		h := newOrderHandler("my_flag", false)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: "my_flag", Enabled: true, HealthCheckDurationSeconds: 10})
		synctest.Wait() // OnChange(true) applied, health monitor running
		sendUpdate(client, Flag{Name: "my_flag", Enabled: false})
		synctest.Wait()    // OnChange(false) is blocked
		sendUpdate(client) // the flag is removed while OnChange(false) is still running
		h.healthy.Store(false)

		// Health checks would fail during this time if the monitor of OnChange(true) were still running.
		time.Sleep(time.Second)
		synctest.Wait()
		h.release(false)
		synctest.Wait()

		assert.Equal(t, []string{"change:true", "change:false", "noconfig"}, h.recorded())
	})
}

func TestOnUpdate_SupersededEnableStartsNoHealthMonitor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer func() {
			client.Stop()
			synctest.Wait()
		}()
		h := newOrderHandler("my_flag", true, false)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: "my_flag", Enabled: true, HealthCheckDurationSeconds: 10})
		synctest.Wait() // OnChange(true) is blocked
		sendUpdate(client, Flag{Name: "my_flag", Enabled: false})
		sendUpdate(client) // the flag is removed: there is no health monitor to cancel yet
		h.healthy.Store(false)
		h.release(true)
		synctest.Wait() // OnChange(true) done, OnChange(false) is blocked

		// Health checks would fail during this time if OnChange(true) had started a monitor.
		time.Sleep(time.Second)
		synctest.Wait()
		h.release(false)
		synctest.Wait()

		assert.Equal(t, []string{"change:true", "change:false", "noconfig"}, h.recorded())
	})
}

func TestSubscribe_LateSubscriberKeepsHealthMonitorOfExistingOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient().WithHealthCheckInterval(100 * time.Millisecond)
		defer func() {
			client.Stop()
			synctest.Wait()
		}()
		a := newOrderHandler("my_flag", true)
		require.NoError(t, client.SubscribeWithHandler(a))

		sendUpdate(client, Flag{Name: "my_flag", Enabled: true, HealthCheckDurationSeconds: 10})
		synctest.Wait() // OnChange(true) of a is blocked
		b := newOrderHandler("my_flag")
		require.NoError(t, client.SubscribeWithHandler(b)) // replays the cached value
		a.release(true)
		synctest.Wait()
		a.healthy.Store(false)

		// The health monitor of a must run and recover it.
		time.Sleep(time.Second)
		synctest.Wait()

		assert.Equal(t, []string{"change:true", "recover:true"}, a.recorded())
		assert.Equal(t, []string{"change:true"}, b.recorded())
	})
}

func TestStop_DropsPendingCallbacks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient()
		h := newOrderHandler("my_flag", true)
		require.NoError(t, client.SubscribeWithHandler(h))

		sendUpdate(client, Flag{Name: "my_flag", Enabled: true})
		synctest.Wait()    // OnChange(true) is blocked
		sendUpdate(client) // OnNoConfig is queued behind it
		client.Stop()
		h.release(true)
		synctest.Wait()

		assert.Equal(t, []string{"change:true"}, h.recorded())
	})
}
