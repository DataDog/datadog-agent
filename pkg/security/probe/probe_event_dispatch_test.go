// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// mockCustomEventHandler is a test double for CustomEventHandler
type mockCustomEventHandler struct {
	handleCount int
}

func (m *mockCustomEventHandler) HandleCustomEvent(_ *rules.Rule, _ *events.CustomEvent) {
	m.handleCount++
}

// mockEventMarshaler is a no-op EventMarshaler for test CustomEvents
type mockEventMarshaler struct{}

func (m *mockEventMarshaler) ToJSON() ([]byte, error) {
	return []byte("{}"), nil
}

// TestEventHandlerArraySizes verifies that the handler arrays are sized
// MaxAllEventType+1, so that MaxAllEventType is a valid index.
func TestEventHandlerArraySizes(t *testing.T) {
	p := &Probe{}

	assert.Equal(t, int(model.MaxAllEventType)+1, len(p.eventHandlers),
		"eventHandlers array must have length MaxAllEventType+1")
	assert.Equal(t, int(model.MaxAllEventType)+1, len(p.eventConsumers),
		"eventConsumers array must have length MaxAllEventType+1")
	assert.Equal(t, int(model.MaxAllEventType)+1, len(p.customEventHandlers),
		"customEventHandlers array must have length MaxAllEventType+1")
}

// TestEventHandlerArrayMaxIndexAccessible verifies that MaxAllEventType can be
// used as an index without an out-of-bounds panic (regression for the off-by-one fix).
func TestEventHandlerArrayMaxIndexAccessible(t *testing.T) {
	p := &Probe{}

	require.NotPanics(t, func() {
		_ = p.eventHandlers[model.MaxAllEventType]
		_ = p.eventConsumers[model.MaxAllEventType]
		_ = p.customEventHandlers[model.MaxAllEventType]
	})
}

// TestSendEventToConsumersOutOfRange verifies that sendEventToConsumers does not
// panic when given an event whose type exceeds the array bounds.
func TestSendEventToConsumersOutOfRange(t *testing.T) {
	p := &Probe{}

	event := &model.Event{}
	event.Type = uint32(model.MaxAllEventType) + 1

	require.NotPanics(t, func() {
		p.sendEventToConsumers(event)
	})
}

// TestDispatchCustomEventOutOfRange verifies that DispatchCustomEvent does not
// panic when given an event whose type exceeds the array bounds.
func TestDispatchCustomEventOutOfRange(t *testing.T) {
	p := &Probe{}

	outOfRangeType := model.EventType(model.MaxAllEventType) + 1
	customEvent := events.NewCustomEvent(outOfRangeType, &mockEventMarshaler{})

	require.NotPanics(t, func() {
		p.DispatchCustomEvent(&rules.Rule{}, customEvent)
	})
}

// TestDispatchCustomEventMaxAllEventType verifies that DispatchCustomEvent
// correctly dispatches to a handler registered at MaxAllEventType-1 (valid boundary).
func TestDispatchCustomEventMaxAllEventType(t *testing.T) {
	p := &Probe{}

	handler := &mockCustomEventHandler{}
	validType := model.EventType(model.MaxAllEventType - 1)

	err := p.AddCustomEventHandler(validType, handler)
	require.NoError(t, err)

	customEvent := events.NewCustomEvent(validType, &mockEventMarshaler{})
	p.DispatchCustomEvent(&rules.Rule{}, customEvent)

	assert.Equal(t, 1, handler.handleCount, "handler should have been called once")
}

// TestDispatchCustomEventWildcard verifies that a wildcard handler registered at
// UnknownEventType receives all dispatched custom events.
func TestDispatchCustomEventWildcard(t *testing.T) {
	p := &Probe{}

	wildcardHandler := &mockCustomEventHandler{}
	err := p.AddCustomEventHandler(model.UnknownEventType, wildcardHandler)
	require.NoError(t, err)

	customEvent := events.NewCustomEvent(model.ExecEventType, &mockEventMarshaler{})
	p.DispatchCustomEvent(&rules.Rule{}, customEvent)

	assert.Equal(t, 1, wildcardHandler.handleCount, "wildcard handler should have been called once")
}
