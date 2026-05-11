// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	pkgslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// recordingSubmitter captures every ErrorLog routed through the chain.
// Tests use it as the registered Submitter to assert the chain forwards
// (or filters) records.
type recordingSubmitter struct {
	mu   sync.Mutex
	logs []errortracking.ErrorLog
}

func (r *recordingSubmitter) submit(e errortracking.ErrorLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, e)
}

func (r *recordingSubmitter) snapshot() []errortracking.ErrorLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]errortracking.ErrorLog, len(r.logs))
	copy(out, r.logs)
	return out
}

func resetErrortrackingSlot(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { RegisterErrortrackingSubmitter(nil) })
	RegisterErrortrackingSubmitter(nil)
}

// TestRegisterErrortrackingSubmitter_NilResets verifies the on/off contract:
// after registering and then clearing, the slot must return to a no-op
// state. Tests rely on this for cleanup between cases.
func TestRegisterErrortrackingSubmitter_NilResets(t *testing.T) {
	resetErrortrackingSlot(t)

	rec := &recordingSubmitter{}
	RegisterErrortrackingSubmitter(rec.submit)
	require.NotNil(t, loadErrortrackingSubmitter())

	RegisterErrortrackingSubmitter(nil)
	require.Nil(t, loadErrortrackingSubmitter())
}

// TestBuildSlogLogger_ForwardsErrorRecord asserts that the chain assembled
// by buildSlogLogger fans error records out to the registered Submitter
// while routing non-error records only to the formatted writer branch.
func TestBuildSlogLogger_ForwardsErrorRecord(t *testing.T) {
	resetErrortrackingSlot(t)

	rec := &recordingSubmitter{}
	RegisterErrortrackingSubmitter(rec.submit)

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
// built by buildSlogLogger remains a no-op for the errortracking branch
// when no Submitter has been registered (the common path before opt-in).
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

	// Should not panic; should produce nothing observable on the
	// errortracking side.
	sl.Error("error with nobody listening")
	logger.Flush()
}
