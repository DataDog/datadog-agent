// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the agenttelemetry component.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// Event captures a single SendEvent call.
type Event struct {
	Type    string
	Payload []byte
}

// Mock is the mocked agenttelemetry component. It implements the public
// Component interface and records what was submitted so tests can assert on it.
type Mock interface {
	// Component methods are included in Mock.
	agenttelemetry.Component

	// Events returns the events captured by SendEvent, in call order.
	Events() []Event
	// ErrorLogs returns the error logs captured by SubmitErrorLog, in call order.
	ErrorLogs() []errortracking.ErrorLog
}

type mock struct {
	mu               sync.Mutex
	events           []Event
	errorLogs        []errortracking.ErrorLog
	registeredEvents map[string]struct{}
}

var _ Mock = (*mock)(nil)

// SendEvent validates eventType and eventPayload the same way the real
// component does -- rejecting event types that weren't registered and
// payloads that aren't valid JSON -- before recording the event. The payload
// is copied so a caller reusing its buffer cannot corrupt what Events() later
// reports.
func (m *mock) SendEvent(eventType string, eventPayload []byte) error {
	if _, ok := m.registeredEvents[eventType]; !ok {
		return fmt.Errorf("payload type %q is not registered", eventType)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(eventPayload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, Event{Type: eventType, Payload: append([]byte(nil), eventPayload...)})
	return nil
}

// SubmitErrorLog records the error log.
func (m *mock) SubmitErrorLog(log errortracking.ErrorLog) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorLogs = append(m.errorLogs, log)
}

// StartStartupSpan returns a fully-initialized span (rooted at a background
// context) so consumers can safely call span.Finish(err), including on the
// error path. A zero-value &Span{} would panic in Finish on a nil Meta map.
func (m *mock) StartStartupSpan(operationName string) (*installertelemetry.Span, context.Context) {
	return installertelemetry.StartSpanFromContext(context.Background(), operationName)
}

// Events returns a copy of the events captured by SendEvent, in call order.
func (m *mock) Events() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event(nil), m.events...)
}

// ErrorLogs returns a copy of the error logs captured by SubmitErrorLog, in call order.
func (m *mock) ErrorLogs() []errortracking.ErrorLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]errortracking.ErrorLog(nil), m.errorLogs...)
}

// New returns a new mock for the agenttelemetry component. registeredEvents
// lists the event types SendEvent will accept, mirroring the event names a
// real deployment declares under profiles[].events in its agent_telemetry
// config -- SendEvent rejects any other eventType, just as the real
// component does for events that were never registered.
func New(_ testing.TB, registeredEvents ...string) Mock {
	m := &mock{registeredEvents: make(map[string]struct{}, len(registeredEvents))}
	for _, e := range registeredEvents {
		m.registeredEvents[e] = struct{}{}
	}
	return m
}
