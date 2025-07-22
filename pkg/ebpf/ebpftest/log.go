// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogLevel sets the logger level for this test only
func LogLevel(t testing.TB, level string) {
	t.Cleanup(func() {
		log.SetupLogger(log.Default(), "off")
	})

	logLevel, _ := log.LogLevelFromString(level)
	var lvl slog.LevelVar
	lvl.Set(slog.Level(logLevel))

	handler := testLogger{t, &lvl}
	logger := log.NewSlogWrapper(slog.New(handler), 0, &lvl, nil, nil)
	log.SetupLogger(logger, level)
}

type testLogger struct {
	t     testing.TB
	level slog.Leveler
}

// Handle logs the message
func (t testLogger) Handle(_ context.Context, r slog.Record) error {
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, _ := frames.Next()

	t.t.Logf("%s:%d: %s | %s | %s", frame.File, frame.Line, r.Time.Format("2006-01-02 15:04:05.000 MST"), strings.ToUpper(log.LogLevel(r.Level).String()), r.Message)
	return nil
}

// Enabled returns true if the logger is enabled for the given level
func (t testLogger) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= t.level.Level()
}

// WithAttrs is a no-op
func (t testLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	return t
}

// WithGroup is a no-op
func (t testLogger) WithGroup(name string) slog.Handler {
	return t
}
