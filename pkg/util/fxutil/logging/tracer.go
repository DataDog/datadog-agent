// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx/fxevent"
)

const (
	serviceName = "dd-agent-fx-init"
)

// Span represents a Datadog APM span in JSON format for the v0.3/traces endpoint.
type Span struct {
	Service  string `json:"service"`
	Name     string `json:"name"`
	Resource string `json:"resource"`
	TraceID  uint64 `json:"trace_id"`
	SpanID   uint64 `json:"span_id"`
	ParentID uint64 `json:"parent_id"`
	Start    int64  `json:"start"`
	Duration int64  `json:"duration"`
	Error    int32  `json:"error"`
	// Meta     map[string]string  `json:"meta,omitempty"`
	// Metrics  map[string]float64 `json:"metrics,omitempty"`
	Type string `json:"type,omitempty"`
}

// fxTracingLogger implements fxevent.fxTracingLogger interface to capture Fx lifecycle events
// and send them as traces to Datadog.
type fxTracingLogger struct {
	fxLogger            fxevent.Logger
	agentLogger         io.Writer
	flavor              string
	mu                  sync.Mutex
	rootSpanID          uint64
	traceID             uint64
	spans               []*Span   // Buffer spans during startup
	startTime           time.Time // Fx startup time
	spansSendingEnabled bool
	finished            bool // True when the Fx initialization is complete
}

// withFxTracer wraps the fxlogger with a fxTracingLogger.
// Tracing is enabled when DD_FX_TRACING_ENABLED is set to true.
// When enabled, it instruments Fx lifecycle events including component construction
// and OnStart hooks, sending traces to the configured trace agent.
func withFxTracer(fxlogger fxevent.Logger, startTime time.Time, agentLogger io.Writer) fxevent.Logger {
	if os.Getenv("DD_FX_TRACING_ENABLED") != "true" {
		return fxlogger
	}
	return &fxTracingLogger{
		fxLogger:    fxlogger,
		agentLogger: agentLogger,
		flavor:      filepath.Base(os.Args[0]),
		rootSpanID:  rand.Uint64(),
		traceID:     rand.Uint64(),
		spans:       make([]*Span, 0),
		startTime:   startTime,
	}
}

// LogEvent implements the fxevent.Logger interface.
func (l *fxTracingLogger) LogEvent(event fxevent.Event) {
	if !l.finished {
		switch e := event.(type) {

		case *fxevent.Run:
			l.handleRun(e)

		case *fxevent.OnStartExecuted:
			l.handleOnStartExecuted(e)

		case *fxevent.Started:
			l.handleStarted(e)
		}
	}

	// Log the event to the original fxlogger
	l.fxLogger.LogEvent(event)
}

func (l *fxTracingLogger) UpdateInnerLoggers(logger fxevent.Logger, agentLogger io.Writer) {
	l.mu.Lock()
	l.fxLogger = logger
	l.agentLogger = agentLogger
	l.mu.Unlock()
}

// EnableSpansSending will allow the tracer to forward traces to the trace-agent when the Fx initialization is complete.
func (l *fxTracingLogger) EnableSpansSending() {
	l.mu.Lock()
	l.spansSendingEnabled = true
	l.mu.Unlock()
}

// handleRun captures constructor/decorator execution.
func (l *fxTracingLogger) handleRun(e *fxevent.Run) {
	// Use the Runtime field from Fx event for duration
	startTime := time.Now().Add(-e.Runtime).UnixNano()

	name := extractShortPathFromFullPath(e.Name)

	span := &Span{
		Service:  serviceName,
		Name:     name,
		Resource: name,
		TraceID:  l.traceID,
		SpanID:   rand.Uint64(),
		ParentID: l.rootSpanID,
		Start:    startTime,
		Duration: int64(e.Runtime),
		Error:    errToCode(e.Err),
		Type:     "custom",
	}

	l.mu.Lock()
	l.spans = append(l.spans, span)
	l.mu.Unlock()
}

// handleOnStartExecuted captures OnStart hook execution time.
func (l *fxTracingLogger) handleOnStartExecuted(e *fxevent.OnStartExecuted) {
	// Use the Runtime field from Fx event for duration
	startTime := time.Now().Add(-e.Runtime).UnixNano()
	name := extractShortPathFromFullPath(e.FunctionName)

	span := &Span{
		Service:  serviceName,
		Name:     name,
		Resource: name,
		TraceID:  l.traceID,
		SpanID:   rand.Uint64(),
		ParentID: l.rootSpanID,
		Start:    startTime,
		Duration: int64(e.Runtime),
		Error:    errToCode(e.Err),
		Type:     "custom",
	}

	l.mu.Lock()
	l.spans = append(l.spans, span)
	l.mu.Unlock()
}

// handleStarted is called when Fx startup completes.
// Sends all buffered spans to Datadog asynchronously with retry.
func (l *fxTracingLogger) handleStarted(e *fxevent.Started) {
	endTime := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if traceSending has been enabled during the Fx initialization
	// If not cleanup the memory and return
	if !l.spansSendingEnabled {
		l.spans = nil
		l.finished = true
		return
	}

	// Create root span
	rootSpan := &Span{
		Service:  serviceName,
		Name:     l.flavor,
		Resource: l.flavor,
		TraceID:  l.traceID,
		SpanID:   l.rootSpanID,
		ParentID: 0,
		Start:    l.startTime.UnixNano(),
		Duration: endTime.Sub(l.startTime).Nanoseconds(),
		Error:    errToCode(e.Err),
		Type:     "custom",
	}

	l.spans = append(l.spans, rootSpan)

	// Send spans asynchronously (trace-agent might not be ready immediately)
	go sendSpansToDatadog(l.agentLogger, l.spans)
	l.finished = true
}

func errToCode(err error) int32 {
	if err != nil {
		return 1
	}
	return 0
}

func extractShortPathFromFullPath(fullPath string) string {
	shortPath := ""
	slices := strings.Split(fullPath, "github.com/DataDog/datadog-agent/")
	if len(slices) > 1 {
		// We want to trim the part containing the path of the project
		// ie DataDog/datadog-agent/ or DataDog/datadog-process-agent/
		shortPath = slices[len(slices)-1]
	} else {
		// For logging from dependencies, we want to log e.g.
		// "collector@v0.35.0/service/collector.go"
		slices := strings.Split(fullPath, "/")
		atSignIndex := len(slices) - 1
		for ; atSignIndex > 0; atSignIndex-- {
			if strings.Contains(slices[atSignIndex], "@") {
				break
			}
		}
		shortPath = strings.Join(slices[atSignIndex:], "/")
	}
	return shortPath
}
