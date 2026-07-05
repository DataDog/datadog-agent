// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	pkgslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

func BenchmarkSlogParallel(b *testing.B) {
	b.StopTimer()

	logger, levelVar := initLogger(b)
	levelVar.Set(slog.LevelDebug)
	log.SetupLoggerWithLevelVar(logger, levelVar)

	runLogParallel(b)
}

func runLogParallel(b *testing.B) {
	b.StartTimer()
	wg := sync.WaitGroup{}
	wg.Add(b.N)
	for range b.N {
		go func() {
			defer wg.Done()
			for range 1000 {
				log.Info("Hello I am a log")
			}
		}()
	}
	wg.Wait()
	log.Flush()
}

func BenchmarkSlogLogger(b *testing.B) {
	b.StopTimer()

	logger, levelVar := initLogger(b)
	levelVar.Set(slog.LevelDebug)
	log.SetupLoggerWithLevelVar(logger, levelVar)

	runLog(b)
}

func initLogger(b *testing.B) (log.LoggerInterface, *slog.LevelVar) {
	b.Helper()
	dir := b.TempDir()

	ddCfg := pkgconfigsetup.Datadog()
	logger, levelVar, err := buildSlogLogger(
		log.DebugLvl,
		false,
		filepath.Join(dir, "test.log"), 1000, 2,
		"",
		commonFormatter("TEST", ddCfg), nil,
	)
	require.NoError(b, err)
	return logger, levelVar
}

func runLog(b *testing.B) {
	b.StartTimer()
	for range b.N {
		log.Info("Hello I am a log")
	}
	log.Flush()
}

// --- Errortracking handler-chain tests ---------------------------------
//
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
	t.Cleanup(func() {
		RegisterErrortrackingSubmitter(nil)
		RegisterErrortrackingBouncer(nil)
	})
	RegisterErrortrackingSubmitter(nil)
	RegisterErrortrackingBouncer(nil)
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
	// A Bouncer must be registered alongside the Submitter: when loadBouncer
	// is set but returns nil the handler drops the record (safe default for
	// the Fx startup window). The production wiring registers both together.
	RegisterErrortrackingBouncer(errortracking.NewBouncer(15*time.Minute, 0))

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
	t.Cleanup(logger.Close)
	levelVar.Set(slog.LevelDebug)

	wrapper, ok := logger.(*pkgslog.Wrapper)
	require.True(t, ok, "expected *pkgslog.Wrapper, got %T", logger)
	sl := slog.New(wrapper.Handler())

	sl.Info("info message - should not reach errortracking")
	sl.Warn("warn message - should not reach errortracking")
	sl.Error("error message - should reach errortracking")
	logger.Flush()

	got := rec.snapshot()
	require.Len(t, got, 1, "exactly one Error record must reach the registered Submitter")
	require.NotZero(t, got[0].PC, "captured record must carry a call-site PC")
	require.Greater(t, got[0].PCsLen, 0, "captured record must carry stack PCs")
	require.Equal(t, uint32(1), got[0].Count, "first sighting always has Count=1")
}

func emitFromHelper(sl *slog.Logger, msg string) {
	sl.Error(msg)
}

func renderCapturedStackTrace(e errortracking.ErrorLog) string {
	if e.PCsLen == 0 {
		return ""
	}
	var out string
	frames := runtime.CallersFrames(e.PCs[:e.PCsLen])
	for {
		frame, more := frames.Next()
		if frame.File != "" {
			offset := uintptr(0)
			if frame.PC >= frame.Entry {
				offset = frame.PC - frame.Entry
			}
			if out != "" {
				out += "\n"
			}
			out += fmt.Sprintf("%s\n\t%s:%d +0x%x", frame.Function, frame.File, frame.Line, offset)
		}
		if !more {
			break
		}
	}
	return out
}

// TestBuildSlogLogger_HelperStackAndNoMessage exercises the real logger chain
// through a helper function and locks the privacy contract: the captured stack
// identifies the helper call site, but the log message itself is not present in
// the rendered stack trace.
func TestBuildSlogLogger_HelperStackAndNoMessage(t *testing.T) {
	resetErrortrackingSlot(t)

	rec := &recordingSubmitter{}
	RegisterErrortrackingSubmitter(rec.submit)
	RegisterErrortrackingBouncer(errortracking.NewBouncer(0, 0))

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
	t.Cleanup(logger.Close)
	levelVar.Set(slog.LevelDebug)

	wrapper := logger.(*pkgslog.Wrapper)
	sl := slog.New(wrapper.Handler())

	const msg = "privacy-test-message-should-not-appear-in-stacktrace"
	emitFromHelper(sl, msg)
	logger.Flush()

	got := rec.snapshot()
	require.Len(t, got, 1)

	stackTrace := renderCapturedStackTrace(got[0])
	assert.Contains(t, stackTrace, "emitFromHelper")
	assert.NotContains(t, stackTrace, msg)
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
	t.Cleanup(logger.Close)
	levelVar.Set(slog.LevelDebug)

	wrapper := logger.(*pkgslog.Wrapper)
	sl := slog.New(wrapper.Handler())

	// Should not panic; should produce nothing observable on the
	// errortracking side.
	sl.Error("error with nobody listening")
	logger.Flush()
}
