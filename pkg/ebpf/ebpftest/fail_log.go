// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"testing"

	//nolint:depguard // creating a custom logger for testing

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FailLogLevel sets the logger level for this test only and only outputs if the test fails
func FailLogLevel(t testing.TB, level string) {
	t.Helper()
	inner := &failureTestLogger{TB: t}
	t.Cleanup(func() {
		t.Helper()
		log.SetupLogger(log.Default(), "off")
		inner.outputIfFailed()
	})

	logLevel, _ := log.LogLevelFromString(level)
	var lvl slog.LevelVar
	lvl.Set(slog.Level(logLevel))

	handler := &failureTestLogger{t, nil, &lvl}
	logger := log.NewSlogWrapper(slog.New(handler), 0, &lvl, nil, nil)
	log.SetupLogger(logger, level)
}

type failureTestLogger struct {
	testing.TB
	logData []byte
	level   slog.Leveler
}

// Handle implements logger.CustomReceiver
func (l *failureTestLogger) Handle(_ context.Context, r slog.Record) error {
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, _ := frames.Next()

	l.logData = append(l.logData, fmt.Sprintf("%s:%d: %s | %s | %s\n", frame.File, frame.Line, r.Time.Format("2006-01-02 15:04:05.000 MST"), log.LogLevel(r.Level).Uppercase(), r.Message)...)
	return nil
}

func (l *failureTestLogger) outputIfFailed() {
	l.Helper()
	if l.Failed() {
		l.TB.Logf("\n%s", l.logData)
	}
	l.logData = nil
}

// Enabled returns true if the logger is enabled for the given level
func (l *failureTestLogger) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= l.level.Level()
}

// WithAttrs is a no-op
func (l *failureTestLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	return l
}

// WithGroup is a no-op
func (l *failureTestLogger) WithGroup(name string) slog.Handler {
	return l
}
