// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"os"
	"sync"
	"time"

	"go.uber.org/fx/fxevent"
)

const (
	serviceName = "datadog-agent-fx"
)

type traceSpanType int

const (
	fxConstructorType traceSpanType = iota
	fxOnStartHookType
)

// TraceSpan represents a captured span from Fx events.
type TraceSpan struct {
	Type     traceSpanType
	Resource string
	Start    int64 // Unix timestamp in nanoseconds
	Duration int64 // Duration in nanoseconds
	Error    error
}

// fxTracingLogger implements fxevent.fxTracingLogger interface to capture Fx lifecycle events
// and send them as traces to Datadog.
type fxTracingLogger struct {
	fxLogger       fxevent.Logger
	agentLogger    Logger
	mu             sync.Mutex
	spans          []*TraceSpan // Buffer spans during startup
	startTime      time.Time    // Fx startup time
	lifecycleStart time.Time    // Fx lifecycle start time
	endTime        time.Time    // Fx completion time
	err            error
}

// withFxTracer wraps the fxlogger with a fxTracingLogger.
// Tracing is enabled when DD_FX_TRACING_ENABLED is set to true.
// When enabled, it instruments Fx lifecycle events including component construction
// and OnStart hooks, sending traces to the configured trace agent.
func withFxTracer(fxlogger fxevent.Logger, agentLogger Logger) fxevent.Logger {
	if os.Getenv("DD_FX_TRACING_ENABLED") == "true" {
		agentLogger.Infof("[Fx Tracing] Initialized - will capture component construction and OnStart hooks")
		return &fxTracingLogger{
			fxLogger:    fxlogger,
			agentLogger: agentLogger,
			spans:       make([]*TraceSpan, 0),
		}
	}

	return fxlogger
}

// LogEvent implements the fxevent.Logger interface.
func (l *fxTracingLogger) LogEvent(event fxevent.Event) {
	notificationTime := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	switch e := event.(type) {
	case *fxevent.LoggerInitialized:
		l.handleLoggerInitialized(e, notificationTime)

	case *fxevent.Run:
		l.handleRun(e, notificationTime)

	case *fxevent.OnStartExecuted:
		l.handleOnStartExecuted(e, notificationTime)

	case *fxevent.Started:
		l.handleStarted(e, notificationTime)
	}

	l.fxLogger.LogEvent(event)
}

// handleLoggerInitialized records the start time.
func (l *fxTracingLogger) handleLoggerInitialized(_ *fxevent.LoggerInitialized, notificationTime time.Time) {
	l.startTime = notificationTime
	l.agentLogger.Infof("[Fx Tracing] Started capturing Fx events")
}

// handleRun captures constructor/decorator execution.
func (l *fxTracingLogger) handleRun(e *fxevent.Run, notificationTime time.Time) {
	// Use the Runtime field from Fx event for duration
	startTime := notificationTime.Add(-e.Runtime).UnixNano()

	span := &TraceSpan{
		Type:     fxConstructorType,
		Resource: e.Name,
		Start:    startTime,
		Duration: int64(e.Runtime),
		Error:    e.Err,
	}

	l.spans = append(l.spans, span)
}

// handleOnStartExecuted captures OnStart hook execution time.
func (l *fxTracingLogger) handleOnStartExecuted(e *fxevent.OnStartExecuted, notificationTime time.Time) {
	// Use the Runtime field from Fx event for duration
	startTime := notificationTime.Add(-e.Runtime)

	// Record the lifecycle start time if it hasn't been set yet
	// Since the constructor and onstart are sequential, we can use the first one to represent the lifecycle start time
	if l.lifecycleStart.IsZero() {
		l.lifecycleStart = startTime
	}

	span := &TraceSpan{
		Type:     fxOnStartHookType,
		Resource: e.FunctionName,
		Start:    startTime.UnixNano(),
		Duration: int64(e.Runtime),
		Error:    e.Err,
	}

	l.spans = append(l.spans, span)
}

// handleStarted is called when Fx startup completes.
// Sends all buffered spans to Datadog asynchronously with retry.
func (l *fxTracingLogger) handleStarted(e *fxevent.Started, notificationTime time.Time) {
	l.endTime = notificationTime
	l.err = e.Err

	// Send spans asynchronously (trace-agent might not be ready immediately)
	if len(l.spans) > 0 {
		go l.sendSpansToDatadog()
	}
}
