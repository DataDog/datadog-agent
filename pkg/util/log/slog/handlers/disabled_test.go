// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledHandlerEnabled(t *testing.T) {
	handler := NewDisabled()

	// Should always return false for any level
	assert.False(t, handler.Enabled(t.Context(), slog.LevelDebug))
	assert.False(t, handler.Enabled(t.Context(), slog.LevelInfo))
	assert.False(t, handler.Enabled(t.Context(), slog.LevelWarn))
	assert.False(t, handler.Enabled(t.Context(), slog.LevelError))
}

func TestDisabledHandlerHandle(t *testing.T) {
	handler := NewDisabled()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := handler.Handle(t.Context(), record)
	require.NoError(t, err)
}
