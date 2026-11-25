// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrub

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockInnerHandler struct {
	records []slog.Record
	enabled bool
}

func newMockInnerHandler() *mockInnerHandler {
	return &mockInnerHandler{enabled: true}
}

func (m *mockInnerHandler) Enabled(context.Context, slog.Level) bool { return m.enabled }

func (m *mockInnerHandler) Handle(_ context.Context, r slog.Record) error {
	m.records = append(m.records, r)
	return nil
}

func (m *mockInnerHandler) WithAttrs([]slog.Attr) slog.Handler { return m }
func (m *mockInnerHandler) WithGroup(string) slog.Handler      { return m }

func TestScrubHandler(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewHandler(inner)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "password=apassword", 0)
	require.NoError(t, handler.Handle(context.Background(), record))

	require.Len(t, inner.records, 1)
	assert.Equal(t, "password=********", inner.records[0].Message)
}

func TestScrubHandlerFailsClosed(t *testing.T) {
	inner := newMockInnerHandler()
	handler := &scrubHandler{
		innerHandler: inner,
		scrubString: func(string) (string, error) {
			return "", errors.New("scrub failure")
		},
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "password=apassword", 0)
	require.NoError(t, handler.Handle(context.Background(), record))

	require.Len(t, inner.records, 1)
	assert.Equal(t, "[REDACTED] - failure to clean the message", inner.records[0].Message)
}

func TestScrubHandlerEnabled(t *testing.T) {
	inner := newMockInnerHandler()
	inner.enabled = false
	handler := NewHandler(inner)

	assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))

	inner.enabled = true
	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
}

func TestScrubHandlerWithAttrsAndGroup(t *testing.T) {
	inner := newMockInnerHandler()
	scrubHandler := NewHandler(inner)
	handler := scrubHandler.WithAttrs([]slog.Attr{{Key: "key1", Value: slog.StringValue("value1")}})
	handler = handler.WithGroup("group1")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "password=apassword", 0)
	require.NoError(t, handler.Handle(context.Background(), record))

	require.Len(t, inner.records, 1)
	assert.Equal(t, "password=********", inner.records[0].Message)
}
