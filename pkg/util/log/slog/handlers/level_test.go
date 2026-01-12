// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLevel(t *testing.T) {
	inner := newMockInnerHandler()
	level := slog.LevelInfo

	handler := NewLevel(level, inner)
	require.NotNil(t, handler)
}

func TestLevelHandlerEnabled(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelInfo, inner)

	// Messages at or above the level should be enabled
	assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
}

func TestLevelHandlerHandle(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelInfo, inner)

	// Message at the level should be handled
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info message", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Equal(t, 1, inner.recordCount())
	assert.Equal(t, "info message", inner.lastMessage())
}

func TestLevelHandlerHandleBelowLevel(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelInfo, inner)

	// Message below the level should be filtered
	record := slog.NewRecord(time.Now(), slog.LevelDebug, "debug message", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Equal(t, 0, inner.recordCount())
}

func TestLevelHandlerHandleAboveLevel(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelInfo, inner)

	// Message above the level should be handled
	record := slog.NewRecord(time.Now(), slog.LevelError, "error message", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Equal(t, 1, inner.recordCount())
	assert.Equal(t, "error message", inner.lastMessage())
}

func TestLevelHandlerMultipleMessages(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelWarn, inner)

	messages := []struct {
		level    slog.Level
		msg      string
		expected bool
	}{
		{slog.LevelDebug, "debug", false},
		{slog.LevelInfo, "info", false},
		{slog.LevelWarn, "warn", true},
		{slog.LevelError, "error", true},
	}

	for _, m := range messages {
		record := slog.NewRecord(time.Now(), m.level, m.msg, 0)
		handler.Handle(context.Background(), record)
	}

	// Only warn and error should be logged
	assert.Equal(t, 2, inner.recordCount())
}

func TestLevelHandlerWithAttrs(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelInfo, inner)

	attrs := []slog.Attr{
		{Key: "key1", Value: slog.StringValue("value1")},
	}

	newHandler := handler.WithAttrs(attrs)
	require.NotNil(t, newHandler)

	// Should still be a level handler
	assert.True(t, newHandler.Enabled(context.Background(), slog.LevelInfo))
	assert.False(t, newHandler.Enabled(context.Background(), slog.LevelDebug))
}

func TestLevelHandlerWithGroup(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelInfo, inner)

	newHandler := handler.WithGroup("testgroup")
	require.NotNil(t, newHandler)

	// Should still be a level handler
	assert.True(t, newHandler.Enabled(context.Background(), slog.LevelInfo))
	assert.False(t, newHandler.Enabled(context.Background(), slog.LevelDebug))
}

func TestLevelHandlerDebugLevel(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelDebug, inner)

	// All standard levels should be enabled when debug level is set
	assert.True(t, handler.Enabled(context.Background(), slog.LevelDebug))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
}

func TestLevelHandlerErrorLevel(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelError, inner)

	// Only error should be enabled
	assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
}

func TestLevelHandlerCustomLevel(t *testing.T) {
	inner := newMockInnerHandler()
	customLevel := slog.Level(5) // Custom level between info and warn
	handler := NewLevel(customLevel, inner)

	// Test custom level filtering
	assert.False(t, handler.Enabled(context.Background(), slog.Level(4)))
	assert.True(t, handler.Enabled(context.Background(), slog.Level(5)))
	assert.True(t, handler.Enabled(context.Background(), slog.Level(6)))
}

type customLeveler struct {
	level slog.Level
}

func (c customLeveler) Level() slog.Level {
	return c.level
}

func TestLevelHandlerWithCustomLeveler(t *testing.T) {
	inner := newMockInnerHandler()
	leveler := customLeveler{level: slog.LevelWarn}
	handler := NewLevel(leveler, inner)

	assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
}

func TestLevelHandlerChaining(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(slog.LevelInfo, inner)

	// Chain multiple operations
	handler = handler.WithAttrs([]slog.Attr{{Key: "attr1", Value: slog.StringValue("value1")}})
	handler = handler.WithGroup("group1")

	// Should still filter by level
	record := slog.NewRecord(time.Now(), slog.LevelDebug, "debug", 0)
	handler.Handle(context.Background(), record)
	assert.Equal(t, 0, inner.recordCount())

	record = slog.NewRecord(time.Now(), slog.LevelInfo, "info", 0)
	handler.Handle(context.Background(), record)
	assert.Equal(t, 1, inner.recordCount())
}
