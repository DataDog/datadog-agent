// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"context"
	"log/slog"
	"sync"
)

// Logger is a logger that sends logs to the telemetry system.
type Logger struct {
	parent slog.Handler
	logs   *logEntries
}

// NewLogger creates a new logger that sends logs to the telemetry system.
func NewLogger(parent slog.Handler) *Logger {
	return &Logger{
		parent: parent,
		logs:   &logEntries{},
	}
}

type logEntries struct {
	mu   sync.Mutex
	logs []logEntry
}

type logEntry struct {
	spanCtx *spanIDs
	log     slog.Record
}

func (l *Logger) flush() []logEntry {
	l.logs.mu.Lock()
	defer l.logs.mu.Unlock()
	logs := l.logs.logs
	l.logs.logs = []logEntry{}
	return logs
}

// Enabled checks if the logger is enabled for the given level.
func (l *Logger) Enabled(ctx context.Context, level slog.Level) bool {
	return l.parent.Enabled(context.Background(), slog.LevelError)
}

// Handle handles the log entry.
func (l *Logger) Handle(ctx context.Context, record slog.Record) error {
	var spanCtx *spanIDs
	spanIDs, ok := getSpanIDsFromContext(ctx)
	if ok {
		spanCtx = &spanIDs
	}
	log := logEntry{
		spanCtx: spanCtx,
		log:     record.Clone(),
	}
	l.logs.mu.Lock()
	l.logs.logs = append(l.logs.logs, log)
	l.logs.mu.Unlock()
	return l.parent.Handle(ctx, record)
}

// WithAttrs wraps the handler with the given attributes.
func (l *Logger) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Logger{
		parent: l.parent.WithAttrs(attrs),
		logs:   l.logs,
	}
}

// WithGroup wraps the handler with the given group.
func (l *Logger) WithGroup(name string) slog.Handler {
	return &Logger{
		parent: l.parent.WithGroup(name),
		logs:   l.logs,
	}
}
