// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags

import (
	"encoding/json"
	"fmt"
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

func TestNewClient(t *testing.T) {
	client := NewClient()
	assert.NotNil(t, client)
	assert.NotNil(t, client.subscriptions)
	assert.NotNil(t, client.currentValues)
}

func TestSubscribe_ValidParams(t *testing.T) {
	client := NewClient()

	onChangeCalled := false
	onChange := func(_ FlagValue) error {
		onChangeCalled = true
		return nil
	}

	onNoConfig := func() {}

	safeRecover := func(_ error, _ FlagValue) {}

	err := client.Subscribe(testFlag1, onChange, onNoConfig, safeRecover)
	assert.NoError(t, err)
	assert.False(t, onChangeCalled, "onChange should not be called if no value exists yet")
}

func TestSubscribeMustHaveSafeRecover(t *testing.T) {
	client := NewClient()

	err := client.Subscribe(
	    "random_first_flag",
    	func(FlagValue) error { return nil },
	    func() { fmt.Println("no data function") },
        func(error, FlagValue) { fmt.Println("safeRecover function") },
	)
    require.NoError(t, err)

	err = client.Subscribe(
	    "random_second_flag",
    	func(FlagValue) error { return nil },
	    func() { fmt.Println("no data function") },
    	nil,
	)
	require.Error(t, err)
}

func TestSubscribe_ImmediateCallbackIfValueExists(t *testing.T) {
	client := NewClient()

	// Set a value before subscribing
	client.mu.Lock()
	client.currentValues[testFlag1] = FlagValue(true)
	client.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)

	onChangeCalled := false
	receivedValue := FlagValue(false)
	onChange := func(value FlagValue) error {
		onChangeCalled = true
		receivedValue = value
		wg.Done()
		return nil
	}

	onNoConfig := func() {}

	safeRecover := func(_ error, _ FlagValue) {}

	err := client.Subscribe(testFlag1, onChange, onNoConfig, safeRecover)
	require.NoError(t, err)

	// Wait for callback (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, onChangeCalled, "onChange should be called immediately if value exists")
		assert.True(t, bool(receivedValue), "should receive the existing value")
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for onChange callback")
	}
}

func TestOnUpdate_ValidConfig(t *testing.T) {
	client := NewClient()

	var wg sync.WaitGroup
	wg.Add(1)

	receivedValue := FlagValue(false)
	onChange := func(value FlagValue) error {
		receivedValue = value
		wg.Done()
		return nil
	}

	onNoConfig := func() {}

	safeRecover := func(_ error, _ FlagValue) {
		t.Errorf("safeRecover should not be called for valid config")
	}

	err := client.Subscribe(testFlag1, onChange, onNoConfig, safeRecover)
	require.NoError(t, err)

	// Create a valid config update
	flagConfig := FlagConfig{
		Flags: []Flag{
			{Name: string(testFlag1), Value: true},
		},
	}
	configBytes, _ := json.Marshal(flagConfig)

	updates := map[string]state.RawConfig{
		"remote_flags_config": {
			Config: configBytes,
		},
	}

	applyStatusCalled := false
	applyStateCallback := func(path string, status state.ApplyStatus) {
		applyStatusCalled = true
		assert.Equal(t, "remote_flags_config", path)
		assert.Equal(t, state.ApplyStateAcknowledged, status.State)
		assert.Empty(t, status.Error)
	}

	client.OnUpdate(updates, applyStateCallback)

	// Wait for callback
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, bool(receivedValue))
		assert.True(t, applyStatusCalled)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for onChange callback")
	}

	// Verify current value is stored
	value, exists := client.GetCurrentValue(testFlag1)
	assert.True(t, exists)
	assert.True(t, bool(value))
}

func TestOnUpdate_InvalidJSON(t *testing.T) {
	client := NewClient()

	onChangeCalled := false
	onChange := func(_ FlagValue) error {
		onChangeCalled = true
		return nil
	}

	onNoConfig := func() {}

	safeRecover := func(_ error, _ FlagValue) {
		t.Errorf("safeRecover should not be called for JSON parsing errors")
	}

	err := client.Subscribe(testFlag1, onChange, onNoConfig, safeRecover)
	require.NoError(t, err)

	// Create an invalid config update
	updates := map[string]state.RawConfig{
		"remote_flags_config": {
			Config: []byte("{invalid json}"),
		},
	}

	applyStatusCalled := false
	applyStateCallback := func(path string, status state.ApplyStatus) {
		applyStatusCalled = true
		assert.Equal(t, "remote_flags_config", path)
		assert.Equal(t, state.ApplyStateError, status.State)
		assert.Contains(t, status.Error, "JSON parsing error")
	}

	client.OnUpdate(updates, applyStateCallback)

	// Give time for any erroneous callbacks
	time.Sleep(100 * time.Millisecond)

	assert.False(t, onChangeCalled, "onChange should not be called for invalid JSON")
	assert.True(t, applyStatusCalled)
}

func TestOnUpdate_FlagValueChange(t *testing.T) {
	client := NewClient()

	var wg sync.WaitGroup
	callCount := 0
	var receivedValues []FlagValue
	var mu sync.Mutex

	onChange := func(value FlagValue) error {
		mu.Lock()
		callCount++
		receivedValues = append(receivedValues, value)
		wg.Done()
		mu.Unlock()
		return nil
	}

	onNoConfig := func() {}

	safeRecover := func(_ error, _ FlagValue) {
		t.Errorf("safeRecover should not be called")
	}

	err := client.Subscribe(testFlag1, onChange, onNoConfig, safeRecover)
	require.NoError(t, err)

	// First update: enabled = true
	wg.Add(1)
	flagConfig := FlagConfig{
		Flags: []Flag{
			{Name: string(testFlag1), Value: true},
		},
	}
	configBytes, _ := json.Marshal(flagConfig)
	updates := map[string]state.RawConfig{
		"remote_flags_config": {Config: configBytes},
	}
	client.OnUpdate(updates, func(string, state.ApplyStatus) {})

	// Wait for first callback
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for first callback")
	}

	// Second update with same value: should NOT trigger callback
	time.Sleep(100 * time.Millisecond) // Give time for any erroneous callbacks
	flagConfig = FlagConfig{
		Flags: []Flag{
			{Name: string(testFlag1), Value: true},
		},
	}
	configBytes, _ = json.Marshal(flagConfig)
	updates = map[string]state.RawConfig{
		"remote_flags_config": {Config: configBytes},
	}
	client.OnUpdate(updates, func(string, state.ApplyStatus) {})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "should only be called once for same value")
	mu.Unlock()

	// Third update: enabled = false (different value)
	wg.Add(1)
	flagConfig = FlagConfig{
		Flags: []Flag{
			{Name: string(testFlag1), Value: false},
		},
	}
	configBytes, _ = json.Marshal(flagConfig)
	updates = map[string]state.RawConfig{
		"remote_flags_config": {Config: configBytes},
	}
	client.OnUpdate(updates, func(string, state.ApplyStatus) {})

	done = make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for second callback")
	}

	mu.Lock()
	assert.Equal(t, 2, callCount, "should be called twice for two different values")
	assert.Equal(t, []FlagValue{FlagValue(true), FlagValue(false)}, receivedValues)
	mu.Unlock()
}

func TestOnUpdate_MultipleSubscribers(t *testing.T) {
	client := NewClient()

	var wg sync.WaitGroup
	wg.Add(2) // Two subscribers

	subscriber1Called := false
	subscriber2Called := false

	onChange1 := func(value FlagValue) error {
		subscriber1Called = true
		assert.True(t, bool(value))
		wg.Done()
		return nil
	}

	onChange2 := func(value FlagValue) error {
		subscriber2Called = true
		assert.True(t, bool(value))
		wg.Done()
		return nil
	}

	onNoConfig := func() {}

	safeRecover := func(_ error, _ FlagValue) {}

	err := client.Subscribe(testFlag1, onChange1, onNoConfig, safeRecover)
	require.NoError(t, err)

	err = client.Subscribe(testFlag1, onChange2, onNoConfig, safeRecover)
	require.NoError(t, err)

	// Send update
	flagConfig := FlagConfig{
		Flags: []Flag{
			{Name: string(testFlag1), Value: true},
		},
	}
	configBytes, _ := json.Marshal(flagConfig)
	updates := map[string]state.RawConfig{
		"remote_flags_config": {Config: configBytes},
	}
	client.OnUpdate(updates, func(string, state.ApplyStatus) {})

	// Wait for both callbacks
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, subscriber1Called)
		assert.True(t, subscriber2Called)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for subscriber callbacks")
	}
}

func TestOnUpdate_MissingFlag(t *testing.T) {
	client := NewClient()

	var wg sync.WaitGroup
	wg.Add(1)

	noDataReceived := false
	onChange := func(_ FlagValue) error {
		t.Errorf("onChange should not be called when flag is missing")
		return nil
	}

	onNoConfig := func() {
		noDataReceived = true
		wg.Done()
	}

	safeRecover := func(_ error, _ FlagValue) {
		t.Errorf("safeRecover should not be called when flag is simply missing")
	}

	err := client.Subscribe(testFlag1, onChange, onNoConfig, safeRecover)
	require.NoError(t, err)

	// Send update for a different flag
	flagConfig := FlagConfig{
		Flags: []Flag{
			{Name: string(testFlag2), Value: true}, // Different flag
		},
	}
	configBytes, _ := json.Marshal(flagConfig)
	updates := map[string]state.RawConfig{
		"remote_flags_config": {Config: configBytes},
	}
	client.OnUpdate(updates, func(string, state.ApplyStatus) {})

	// Wait for noData callback
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, noDataReceived)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for noData callback")
	}
}

func TestGetCurrentValue(t *testing.T) {
	client := NewClient()

	// Flag doesn't exist
	value, exists := client.GetCurrentValue(testFlag1)
	assert.False(t, exists)
	assert.False(t, bool(value))

	// Set a value
	client.mu.Lock()
	client.currentValues[testFlag1] = FlagValue(true)
	client.mu.Unlock()

	// Flag exists
	value, exists = client.GetCurrentValue(testFlag1)
	assert.True(t, exists)
	assert.True(t, bool(value))
}

func TestConcurrentSubscriptions(t *testing.T) {
	client := NewClient()

	var wg sync.WaitGroup
	subscriberCount := 10

	for i := 0; i < subscriberCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := client.Subscribe(
				testFlag1,
				func(_ FlagValue) error { return nil },
				func() {},
				func(_ error, _ FlagValue) {},
			)
			assert.NoError(t, err)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		client.mu.RLock()
		assert.Len(t, client.subscriptions[testFlag1], subscriberCount)
		client.mu.RUnlock()
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent subscriptions")
	}
}

func TestOnChange_FailedPropagation(t *testing.T) {
	client := NewClient()

	var wg sync.WaitGroup
	wg.Add(1) // Only wait for safeRecover callback

	onChangeCalled := false
	safeRecoverCalled := false
	var receivedError error
	var receivedFailedValue FlagValue

	onChange := func(_ FlagValue) error {
		onChangeCalled = true
		// Simulate failed propagation
		return fmt.Errorf("simulated apply failure")
	}

	onNoConfig := func() {}

	safeRecover := func(err error, failedValue FlagValue) {
		safeRecoverCalled = true
		receivedError = err
		receivedFailedValue = failedValue
		wg.Done()
	}

	err := client.Subscribe(testFlag1, onChange, onNoConfig, safeRecover)
	require.NoError(t, err)

	// Create a valid config update
	flagConfig := FlagConfig{
		Flags: []Flag{
			{Name: string(testFlag1), Value: true},
		},
	}
	configBytes, _ := json.Marshal(flagConfig)

	updates := map[string]state.RawConfig{
		"remote_flags_config": {
			Config: configBytes,
		},
	}

	client.OnUpdate(updates, func(string, state.ApplyStatus) {})

	// Wait for callbacks
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, onChangeCalled, "onChange should have been called")
		assert.True(t, safeRecoverCalled, "safeRecover should have been called when onChange returns error")
		assert.NotNil(t, receivedError)
		assert.Contains(t, receivedError.Error(), "failed to apply configuration change")
		assert.True(t, bool(receivedFailedValue), "should receive the failed value (true)")
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for callbacks")
	}
}
