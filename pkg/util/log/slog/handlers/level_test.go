// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"log/slog"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

func TestNewLevel(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.InfoLvl), inner)
	require.NotNil(t, handler)
}

func TestLevelHandlerEnabled(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.InfoLvl), inner)

	assert.False(t, handler.Enabled(t.Context(), slog.LevelDebug))
	assert.True(t, handler.Enabled(t.Context(), slog.LevelInfo))
	assert.True(t, handler.Enabled(t.Context(), slog.LevelWarn))
	assert.True(t, handler.Enabled(t.Context(), slog.LevelError))
}

func TestLevelHandlerHandle(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.InfoLvl), inner)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info message", 0)
	err := handler.Handle(t.Context(), record)

	assert.NoError(t, err)
	assert.Equal(t, 1, inner.recordCount())
	assert.Equal(t, "info message", inner.lastMessage())
}

func TestLevelHandlerMultipleMessages(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.WarnLvl), inner)

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

	expectedCount := 0
	for _, m := range messages {
		record := slog.NewRecord(time.Now(), m.level, m.msg, 0)
		handler.Handle(t.Context(), record)
		if m.expected {
			expectedCount++
		}
	}

	assert.Equal(t, expectedCount, inner.recordCount())
}

func TestLevelHandlerWithAttrs(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.InfoLvl), inner)

	attrs := []slog.Attr{{Key: "key1", Value: slog.StringValue("value1")}}
	newHandler := handler.WithAttrs(attrs)
	require.NotNil(t, newHandler)

	assert.True(t, newHandler.Enabled(t.Context(), slog.LevelInfo))
	assert.False(t, newHandler.Enabled(t.Context(), slog.LevelDebug))
}

func TestLevelHandlerWithGroup(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.InfoLvl), inner)

	newHandler := handler.WithGroup("testgroup")
	require.NotNil(t, newHandler)

	assert.True(t, newHandler.Enabled(t.Context(), slog.LevelInfo))
	assert.False(t, newHandler.Enabled(t.Context(), slog.LevelDebug))
}

func TestLevelHandlerDebugLevel(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.DebugLvl), inner)

	assert.True(t, handler.Enabled(t.Context(), slog.LevelDebug))
	assert.True(t, handler.Enabled(t.Context(), slog.LevelInfo))
	assert.True(t, handler.Enabled(t.Context(), slog.LevelWarn))
	assert.True(t, handler.Enabled(t.Context(), slog.LevelError))
}

func TestLevelHandlerErrorLevel(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.ErrorLvl), inner)

	assert.False(t, handler.Enabled(t.Context(), slog.LevelDebug))
	assert.False(t, handler.Enabled(t.Context(), slog.LevelInfo))
	assert.False(t, handler.Enabled(t.Context(), slog.LevelWarn))
	assert.True(t, handler.Enabled(t.Context(), slog.LevelError))
}

func callerPC(t *testing.T) uintptr {
	t.Helper()
	var pcs [1]uintptr
	n := runtime.Callers(2, pcs[:])
	require.Equal(t, 1, n)
	return pcs[0]
}

func TestLevelHandlerPackageRules(t *testing.T) {
	inner := newMockInnerHandler()
	pc := callerPC(t)

	rules, err := types.ParseLevelRules(
		"error,github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers/...=debug",
		"github.com/DataDog/datadog-agent",
	)
	require.NoError(t, err)
	handler := NewLevel(types.NewRulesSync(rules), inner)

	record := slog.NewRecord(time.Now(), slog.LevelDebug, "debug message", pc)
	err = handler.Handle(t.Context(), record)

	require.NoError(t, err)
	assert.Equal(t, 1, inner.recordCount())
}

func TestLevelHandlerChaining(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewLevel(types.NewRulesSyncFromLevel(types.InfoLvl), inner)

	handler = handler.WithAttrs([]slog.Attr{{Key: "attr1", Value: slog.StringValue("value1")}})
	handler = handler.WithGroup("group1")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info", 0)
	handler.Handle(t.Context(), record)
	assert.Equal(t, 1, inner.recordCount())
}
