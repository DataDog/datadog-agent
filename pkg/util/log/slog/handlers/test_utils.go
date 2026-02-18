// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package handlers

import (
	"context"
	"log/slog"
	"sync"
)

type mockInnerHandler struct {
	mu      sync.Mutex
	records []slog.Record
	enabled bool
	err     error
}

func newMockInnerHandler() *mockInnerHandler {
	return &mockInnerHandler{
		records: make([]slog.Record, 0),
		enabled: true,
	}
}

func (m *mockInnerHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return m.enabled
}

func (m *mockInnerHandler) Handle(_ context.Context, record slog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, record)
	return m.err
}

func (m *mockInnerHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return m
}

func (m *mockInnerHandler) WithGroup(_ string) slog.Handler {
	return m
}

func (m *mockInnerHandler) recordCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

func (m *mockInnerHandler) lastMessage() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) == 0 {
		return ""
	}
	return m.records[len(m.records)-1].Message
}
