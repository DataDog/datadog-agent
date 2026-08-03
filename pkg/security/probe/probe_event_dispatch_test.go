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

// TestDispatchCustomEvent verifies that a registered handler receives all dispatched custom events.
func TestDispatchCustomEvent(t *testing.T) {
	p := &Probe{}

	wildcardHandler := &mockCustomEventHandler{}
	err := p.AddCustomEventHandler(wildcardHandler)
	require.NoError(t, err)

	customEvent := events.NewCustomEvent(model.ExecEventType, &mockEventMarshaler{})
	p.DispatchCustomEvent(&rules.Rule{}, customEvent)

	assert.Equal(t, 1, wildcardHandler.handleCount, "wildcard handler should have been called once")
}
