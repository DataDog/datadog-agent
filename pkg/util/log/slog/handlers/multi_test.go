// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMultiHandler(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()

	handler := NewMulti(inner1, inner2)
	require.NotNil(t, handler)
}

func TestNewMultiHandlerSingleHandler(t *testing.T) {
	inner := newMockInnerHandler()

	// With a single handler, should return that handler directly
	handler := NewMulti(inner)
	require.NotNil(t, handler)

	// Should be the same handler
	assert.Equal(t, inner, handler)
}

func TestMultiHandlerHandle(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	handler := NewMulti(inner1, inner2)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Equal(t, 1, inner1.recordCount())
	assert.Equal(t, 1, inner2.recordCount())
	assert.Equal(t, "test message", inner1.lastMessage())
	assert.Equal(t, "test message", inner2.lastMessage())
}

func TestMultiHandlerHandleMultipleMessages(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	handler := NewMulti(inner1, inner2)

	for i := 0; i < 5; i++ {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
		handler.Handle(context.Background(), record)
	}

	assert.Equal(t, 5, inner1.recordCount())
	assert.Equal(t, 5, inner2.recordCount())
}

func TestMultiHandlerEnabled(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()

	inner1.enabled = true
	inner2.enabled = false

	handler := NewMulti(inner1, inner2)

	// Should be enabled if at least one handler is enabled
	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
}

func TestMultiHandlerEnabledAllDisabled(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()

	inner1.enabled = false
	inner2.enabled = false

	handler := NewMulti(inner1, inner2)

	// Should be disabled if all handlers are disabled
	assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
}

func TestMultiHandlerEnabledAllEnabled(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()

	inner1.enabled = true
	inner2.enabled = true

	handler := NewMulti(inner1, inner2)

	// Should be enabled if all handlers are enabled
	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
}

func TestMultiHandlerWithAttrs(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	handler := NewMulti(inner1, inner2)

	attrs := []slog.Attr{
		{Key: "key1", Value: slog.StringValue("value1")},
	}

	newHandler := handler.WithAttrs(attrs)
	require.NotNil(t, newHandler)

	// Should still work as a multi handler
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	newHandler.Handle(context.Background(), record)
}

func TestMultiHandlerWithGroup(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	handler := NewMulti(inner1, inner2)

	newHandler := handler.WithGroup("testgroup")
	require.NotNil(t, newHandler)

	// Should still work as a multi handler
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	newHandler.Handle(context.Background(), record)
}

func TestMultiHandlerErrorHandling(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner1.err = errors.New("error1")
	inner2 := newMockInnerHandler()
	inner2.err = errors.New("error2")
	handler := NewMulti(inner1, inner2)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := handler.Handle(context.Background(), record)

	// Should return combined errors
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error1")
	assert.Contains(t, err.Error(), "error2")
}

func TestMultiHandlerPartialError(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	inner2.err = errors.New("error2")
	handler := NewMulti(inner1, inner2)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := handler.Handle(context.Background(), record)

	// Should return error from failing handler
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error2")

	// But first handler should still have recorded the message
	assert.Equal(t, 1, inner1.recordCount())
}

func TestMultiHandlerNoErrors(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	handler := NewMulti(inner1, inner2)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := handler.Handle(context.Background(), record)

	// Should not return error when all handlers succeed
	assert.NoError(t, err)
}

func TestMultiHandlerThreeHandlers(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	inner3 := newMockInnerHandler()
	handler := NewMulti(inner1, inner2, inner3)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Equal(t, 1, inner1.recordCount())
	assert.Equal(t, 1, inner2.recordCount())
	assert.Equal(t, 1, inner3.recordCount())
}

func TestMultiHandlerMixedLevels(t *testing.T) {
	inner1 := NewLevel(slog.LevelInfo, newMockInnerHandler())
	inner2 := NewLevel(slog.LevelError, newMockInnerHandler())

	handler := NewMulti(inner1, inner2)

	// Info level should be enabled (first handler accepts it)
	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))

	// Error level should be enabled (both handlers accept it)
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))

	// Debug level should be disabled (no handler accepts it)
	assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
}

func TestMultiHandlerChaining(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner2 := newMockInnerHandler()
	handler := NewMulti(inner1, inner2)

	// Chain operations
	handler = handler.WithAttrs([]slog.Attr{{Key: "attr1", Value: slog.StringValue("value1")}})
	handler = handler.WithGroup("group1")

	// Should still work
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := handler.Handle(context.Background(), record)
	assert.NoError(t, err)
}

func TestMultiHandlerOnlyCallsEnabledHandlers(t *testing.T) {
	inner1 := newMockInnerHandler()
	inner1.enabled = true
	inner2 := newMockInnerHandler()
	inner2.enabled = false
	inner3 := newMockInnerHandler()
	inner3.enabled = true

	handler := NewMulti(inner1, inner2, inner3)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := handler.Handle(context.Background(), record)

	// Should not return error
	assert.NoError(t, err)

	// Only enabled handlers should have Handle called
	assert.Equal(t, 1, inner1.recordCount())
	assert.Equal(t, 0, inner2.recordCount())
	assert.Equal(t, 1, inner3.recordCount())
}
