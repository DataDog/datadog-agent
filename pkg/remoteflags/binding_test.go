// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteflags

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

const boundField = "feature.x.enabled"

// boundHandler is a stubHandler that binds its flag to a configuration field.
type boundHandler struct {
	*stubHandler
	field string
}

func newBoundHandler(flag FlagName, field string) *boundHandler {
	return &boundHandler{stubHandler: newStubHandler(flag), field: field}
}

func (h *boundHandler) ConfigurationField() string { return h.field }

func (f *fakeConfigSetter) setCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sets)
}

func (f *fakeConfigSetter) lastUnset() (string, model.Source, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.unsets) == 0 {
		return "", "", false
	}
	u := f.unsets[len(f.unsets)-1]
	return u.key, u.source, true
}

// A handler-bound field is mirrored under SourceRC even though the payload,
// like the backend's, only carries the name and the value.
func TestBinding_MirrorsBoundFieldWhenPayloadLacksIt(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, boundField)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	assert.True(t, bool(waitChan(t, h.onChangeCh)))
	setter.mu.Lock()
	defer setter.mu.Unlock()
	require.Len(t, setter.sets, 1)
	assert.Equal(t, boundField, setter.sets[0].key)
	assert.Equal(t, true, setter.sets[0].value)
	assert.Equal(t, model.SourceRC, setter.sets[0].source)
}

// A field declared by the payload is used as is.
func TestBinding_PayloadFieldIsKept(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, boundField)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, ConfigurationField: "other.enabled"})

	waitChan(t, h.onChangeCh)
	assert.Equal(t, model.SourceRC, setter.GetSource("other.enabled"))
	assert.Equal(t, model.SourceDefault, setter.GetSource(boundField))
}

// A flag without a bound field is not mirrored, as before.
func TestBinding_UnboundFlagIsNotMirrored(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	h := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	assert.True(t, bool(waitChan(t, h.onChangeCh)))
	assert.Zero(t, setter.setCount())
}

// A handler binding an empty field behaves like an unbound one.
func TestBinding_EmptyFieldIsNotMirrored(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, "")
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	waitChan(t, h.onChangeCh)
	assert.Zero(t, setter.setCount())
}

// Without a ConfigSetter a bound flag only reaches its handler.
func TestBinding_NoSetterStillNotifies(t *testing.T) {
	client := NewClient()
	h := newBoundHandler(testFlag1, boundField)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	assert.True(t, bool(waitChan(t, h.onChangeCh)))
}

// The backend never sends override_local; a bound flag ignores it if a
// producer does, so a local value is never overridden.
func TestBinding_OverrideLocalIsIgnored(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	setter.seedSource(boundField, model.SourceFile)
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, boundField)
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true, OverrideLocal: true})

	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, setter.setCount())
	assert.Equal(t, model.SourceFile, setter.GetSource(boundField))
	assert.Empty(t, h.onChangeCh)
}

// Which source currently resolves the key decides whether the flag may write.
// The Remote Config source outranks file, environment and fleet policies in
// the configuration, so the guard at mirror time is what keeps a local value.
func TestBinding_PrecedenceBySource(t *testing.T) {
	tests := []struct {
		name    string
		current model.Source
		applied bool
	}{
		{name: "unset (schema default)", current: model.SourceDefault, applied: true},
		{name: "unknown", current: model.SourceUnknown, applied: true},
		{name: "empty", current: "", applied: true},
		{name: "own earlier write", current: model.SourceRC, applied: true},
		{name: "local file", current: model.SourceFile, applied: false},
		{name: "environment", current: model.SourceEnvVar, applied: false},
		{name: "fleet policies", current: model.SourceFleetPolicies, applied: false},
		{name: "agent runtime", current: model.SourceAgentRuntime, applied: false},
		{name: "cli", current: model.SourceCLI, applied: false},
	}
	for _, tt := range tests {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s, flag %t", tt.name, enabled), func(t *testing.T) {
				client := NewClient()
				setter := newFakeConfigSetter()
				setter.seedSource(boundField, tt.current)
				client.WithConfigSetter(setter)
				h := newBoundHandler(testFlag1, boundField)
				require.NoError(t, client.SubscribeWithHandler(h))

				sendUpdate(client, Flag{Name: string(testFlag1), Enabled: FlagValue(enabled)})

				if tt.applied {
					assert.Equal(t, enabled, bool(waitChan(t, h.onChangeCh)))
					assert.Equal(t, 1, setter.setCount())
					assert.Equal(t, model.SourceRC, setter.GetSource(boundField))
					return
				}
				time.Sleep(50 * time.Millisecond)
				assert.Zero(t, setter.setCount(), "a local value must not be written over")
				assert.Equal(t, tt.current, setter.GetSource(boundField))
				assert.Empty(t, h.onChangeCh)
			})
		}
	}
}

// A flag that disappears gives the key back to the local layers, once.
func TestBinding_RemovedFlagUnsetsRCSource(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, boundField)
	require.NoError(t, client.SubscribeWithHandler(h))
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
	waitChan(t, h.onChangeCh)

	sendUpdate(client, Flag{Name: string(testFlag2), Enabled: true})

	waitChan(t, h.noConfigCh)
	key, source, ok := setter.lastUnset()
	require.True(t, ok)
	assert.Equal(t, boundField, key)
	assert.Equal(t, model.SourceRC, source)
	assert.Equal(t, model.SourceDefault, setter.GetSource(boundField))

	// Absent again: nothing left to give back.
	sendUpdate(client, Flag{Name: string(testFlag2), Enabled: true})
	waitChan(t, h.noConfigCh)
	setter.mu.Lock()
	defer setter.mu.Unlock()
	assert.Len(t, setter.unsets, 1)
}

// A flag that was never applied (a local value won) is not unset when absent.
func TestBinding_AbsentFlagNeverMirroredIsNotUnset(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	setter.seedSource(boundField, model.SourceFile)
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, boundField)
	require.NoError(t, client.SubscribeWithHandler(h))
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	sendUpdate(client, Flag{Name: string(testFlag2), Enabled: true})

	waitChan(t, h.noConfigCh)
	_, _, ok := setter.lastUnset()
	assert.False(t, ok)
}

// An apply failure gives the key back before SafeRecover runs.
func TestBinding_RecoverUnsetsBoundField(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, boundField)
	h.onChangeFn = func(FlagValue) error { return errors.New("cannot start") }
	require.NoError(t, client.SubscribeWithHandler(h))

	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})

	waitChan(t, h.recoverCh)
	key, source, ok := setter.lastUnset()
	require.True(t, ok)
	assert.Equal(t, boundField, key)
	assert.Equal(t, model.SourceRC, source)
}

// A failed health probe recovers a bound flag the same way.
func TestBinding_UnhealthyRecoversBoundField(t *testing.T) {
	client := NewClient().WithHealthCheckInterval(10 * time.Millisecond)
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	h := newBoundHandler(testFlag1, boundField)
	require.NoError(t, client.SubscribeWithHandler(h))
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
	waitChan(t, h.onChangeCh)

	h.healthy.Store(false)

	waitChan(t, h.recoverCh)
	_, source, ok := setter.lastUnset()
	require.True(t, ok)
	assert.Equal(t, model.SourceRC, source)
}

// A handler subscribed after the value is known is notified with the binding
// too, so its recovery also gives the key back.
func TestBinding_LateSubscriberKeepsBinding(t *testing.T) {
	client := NewClient()
	setter := newFakeConfigSetter()
	client.WithConfigSetter(setter)
	early := newStubHandler(testFlag1)
	require.NoError(t, client.SubscribeWithHandler(early))
	sendUpdate(client, Flag{Name: string(testFlag1), Enabled: true})
	waitChan(t, early.onChangeCh)

	late := newBoundHandler(testFlag1, boundField)
	late.onChangeFn = func(FlagValue) error { return errors.New("cannot start") }
	require.NoError(t, client.SubscribeWithHandler(late))

	waitChan(t, late.recoverCh)
	_, source, ok := setter.lastUnset()
	require.True(t, ok)
	assert.Equal(t, model.SourceRC, source)
}
