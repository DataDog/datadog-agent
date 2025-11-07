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
)

func TestDisabledHandlerEnabled(t *testing.T) {
	handler := NewDisabledHandler()

	// Should always return false for any level
	assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelError))
}

func TestDisabledHandlerHandle(t *testing.T) {
	handler := NewDisabledHandler()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := handler.Handle(context.Background(), record)

	// Should not return an error
	assert.NoError(t, err)
}

func TestDisabledHandlerWithAttrs(t *testing.T) {
	handler := NewDisabledHandler()

	attrs := []slog.Attr{
		{Key: "key1", Value: slog.StringValue("value1")},
		{Key: "key2", Value: slog.IntValue(42)},
	}

	newHandler := handler.WithAttrs(attrs)

	// Should return a disabled handler (itself)
	assert.NotNil(t, newHandler)
	assert.False(t, newHandler.Enabled(context.Background(), slog.LevelInfo))
}

func TestDisabledHandlerWithGroup(t *testing.T) {
	handler := NewDisabledHandler()

	newHandler := handler.WithGroup("testgroup")

	// Should return a disabled handler (itself)
	assert.NotNil(t, newHandler)
	assert.False(t, newHandler.Enabled(context.Background(), slog.LevelInfo))
}
