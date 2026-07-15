// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the agenttelemetry component.
package mock

import (
	"context"
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
	mu        sync.Mutex
	events    []Event
	errorLogs []errortracking.ErrorLog
}

var _ Mock = (*mock)(nil)

// SendEvent records the event and returns nil. The payload is copied so a
// caller reusing its buffer cannot corrupt what Events() later reports.
func (m *mock) SendEvent(eventType string, eventPayload []byte) error {
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

// New returns a new mock for the agenttelemetry component.
func New(_ testing.TB) Mock {
	return &mock{}
}
