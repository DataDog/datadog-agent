// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkgslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// recordingHandler is a test-only slog.Handler that captures every record it
// is asked to handle. Tests use it as the inner handler behind the
// errortracking slot to assert the chain forwards (or filters) records.
type recordingHandler struct {
	mu       sync.Mutex
	enabled  func(context.Context, slog.Level) bool
	records  []slog.Record
	withCall int
}

func (h *recordingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	if h.enabled != nil {
		return h.enabled(ctx, l)
	}
	return true
}

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	h.withCall++
	return h
}

func (h *recordingHandler) WithGroup(_ string) slog.Handler {
	h.withCall++
	return h
}

func (h *recordingHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

func resetErrortrackingSlot(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { RegisterErrortrackingHandler(nil) })
	RegisterErrortrackingHandler(nil)
}

func TestErrortrackingSlot_DisabledWhenUnregistered(t *testing.T) {
	resetErrortrackingSlot(t)

	s := newErrortrackingSlot()
	require.False(t, s.Enabled(context.Background(), slog.LevelError))
	require.False(t, s.Enabled(context.Background(), slog.LevelInfo))

	// Handle on the empty slot must not panic and must return nil.
	r := slog.NewRecord(time.Now(), slog.LevelError, "ignored", 0)
	require.NoError(t, s.Handle(context.Background(), r))
}

func TestErrortrackingSlot_DelegatesAfterRegister(t *testing.T) {
	resetErrortrackingSlot(t)

	rec := &recordingHandler{}
	RegisterErrortrackingHandler(rec)

	s := newErrortrackingSlot()
	require.True(t, s.Enabled(context.Background(), slog.LevelError))

	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	require.NoError(t, s.Handle(context.Background(), r))

	got := rec.snapshot()
	require.Len(t, got, 1)
	require.Equal(t, "boom", got[0].Message)
}

func TestErrortrackingSlot_HonorsInnerEnabled(t *testing.T) {
	resetErrortrackingSlot(t)

	rec := &recordingHandler{
		enabled: func(_ context.Context, l slog.Level) bool { return l >= slog.LevelError },
	}
	RegisterErrortrackingHandler(rec)

	s := newErrortrackingSlot()
	require.False(t, s.Enabled(context.Background(), slog.LevelInfo))
	require.False(t, s.Enabled(context.Background(), slog.LevelWarn))
	require.True(t, s.Enabled(context.Background(), slog.LevelError))
}

func TestRegisterErrortrackingHandler_NilResets(t *testing.T) {
	resetErrortrackingSlot(t)

	rec := &recordingHandler{}
	RegisterErrortrackingHandler(rec)
	s := newErrortrackingSlot()
	require.True(t, s.Enabled(context.Background(), slog.LevelError))

	RegisterErrortrackingHandler(nil)
	require.False(t, s.Enabled(context.Background(), slog.LevelError))
}

// TestBuildSlogLogger_ForwardsErrorRecord asserts that the chain assembled by
// buildSlogLogger fans error records out to the registered errortracking
// handler while routing non-error records only to the formatted writer
// branch.
func TestBuildSlogLogger_ForwardsErrorRecord(t *testing.T) {
	resetErrortrackingSlot(t)

	rec := &recordingHandler{
		enabled: func(_ context.Context, l slog.Level) bool { return l >= slog.LevelError },
	}
	RegisterErrortrackingHandler(rec)

	dir := t.TempDir()
	ddCfg := pkgconfigsetup.Datadog()
	logger, levelVar, err := buildSlogLogger(
		log.DebugLvl,
		false,
		filepath.Join(dir, "test.log"), 1000, 2,
		"",
		commonFormatter("TEST", ddCfg), nil,
	)
	require.NoError(t, err)
	levelVar.Set(slog.LevelDebug)

	wrapper, ok := logger.(*pkgslog.Wrapper)
	require.True(t, ok, "expected *pkgslog.Wrapper, got %T", logger)
	sl := slog.New(wrapper.Handler())

	sl.Info("info message - should not reach errortracking")
	sl.Warn("warn message - should not reach errortracking")
	sl.Error("error message - should reach errortracking")
	logger.Flush()

	got := rec.snapshot()
	require.Len(t, got, 1)
	require.Equal(t, "error message - should reach errortracking", got[0].Message)
	require.Equal(t, slog.LevelError, got[0].Level)
}

// TestBuildSlogLogger_NoForwardingWhenUnregistered asserts that the chain
// built by buildSlogLogger remains a no-op for the errortracking branch when
// no Sender has been registered (the common path before opt-in).
func TestBuildSlogLogger_NoForwardingWhenUnregistered(t *testing.T) {
	resetErrortrackingSlot(t)

	dir := t.TempDir()
	ddCfg := pkgconfigsetup.Datadog()
	logger, levelVar, err := buildSlogLogger(
		log.DebugLvl,
		false,
		filepath.Join(dir, "test.log"), 1000, 2,
		"",
		commonFormatter("TEST", ddCfg), nil,
	)
	require.NoError(t, err)
	levelVar.Set(slog.LevelDebug)

	wrapper := logger.(*pkgslog.Wrapper)
	sl := slog.New(wrapper.Handler())

	// Should not panic, should produce nothing observable on the
	// errortracking side.
	sl.Error("error with nobody listening")
	logger.Flush()
}
