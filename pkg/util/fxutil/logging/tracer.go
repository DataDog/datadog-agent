// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"fmt"
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
	serviceName     = "dd-agent-fx-init"
	rootSpanName    = "startup"
	constructorName = "constructor"
	onStartHookName = "onStartHook"
)

// Span represents a Datadog APM span in JSON format for the v0.3/traces endpoint.
// This structure is based on the Span proto message in the datadog/trace package.
// https://github.com/DataDog/datadog-agent/blob/5be2876e3fc36fb1b70c69304bf6876ec90ed016/pkg/proto/pbgo/trace/span.pb.go#L519-L569
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

// FxTracingLogger implements fxevent.Logger interface to capture Fx lifecycle events
// and send them as traces to Datadog.
type FxTracingLogger struct {
	flavor     string     // The Agent process name (e.g. "agent", "trace-agent", "process-agent", "system-probe"), this is used as the service name for the spans.
	mu         sync.Mutex // Mutex to protect concurrent access to the fields below
	traceID    uint64     // The trace ID for the spans
	rootSpanID uint64     // The root span ID for the spans
	startTime  time.Time  // Fx startup time

	// The following fields are subject to concurrent access.
	fxLogger       fxevent.Logger // The underlying fxlogger, fxevent are forwarded to this logger.
	debugLogger    io.Writer      // The logger used to write debug logs.
	traceAgentPort string         // The port of the trace agent
	spans          []*Span        // Buffer spans during startup, reset to nil when the Fx initialization is complete.
}

// withFxTracer wraps the fxlogger with a fxTracingLogger.
// Tracing is enabled when DD_FX_TRACING_ENABLED is set to true.
// When enabled, it instruments Fx lifecycle events including component construction
// and OnStart hooks, sending traces to the configured trace agent.
func withFxTracer(fxlogger fxevent.Logger, startTime time.Time, agentLogger io.Writer) fxevent.Logger {
	if os.Getenv("DD_FX_TRACING_ENABLED") != "true" {
		return fxlogger
	}
	return &FxTracingLogger{
		fxLogger:    fxlogger,
		debugLogger: agentLogger,
		flavor:      filepath.Base(os.Args[0]),
		rootSpanID:  rand.Uint64(),
		traceID:     rand.Uint64(),
		spans:       make([]*Span, 0),
		startTime:   startTime,
	}
}

// LogEvent implements the fxevent.Logger interface.
func (l *FxTracingLogger) LogEvent(event fxevent.Event) {
	// Forward the event to the original fxlogger first to keep the original behavior.
	go l.fxLogger.LogEvent(event)

	switch e := event.(type) {
	case *fxevent.Run:
		l.handleRun(e)

	case *fxevent.OnStartExecuted:
		l.handleOnStartExecuted(e)

	case *fxevent.Started:
		l.handleStarted(e)
	}
}

// UpdateInnerLoggers updates the inner loggers of the FxTracingLogger.
func (l *FxTracingLogger) UpdateInnerLoggers(logger fxevent.Logger, agentLogger io.Writer) {
	l.mu.Lock()
	l.fxLogger = logger
	l.debugLogger = agentLogger
	l.mu.Unlock()
}

// EnableSpansSending will allow the tracer to forward traces to the trace-agent when the Fx initialization is complete.
func (l *FxTracingLogger) EnableSpansSending(traceAgentPort string) {
	l.mu.Lock()
	l.traceAgentPort = traceAgentPort
	l.mu.Unlock()
}

// handleRun captures constructor/decorator execution.
func (l *FxTracingLogger) handleRun(e *fxevent.Run) {
	// Use the Runtime field from Fx event for duration
	startTime := time.Now().Add(-e.Runtime).UnixNano()

	name := extractShortPathFromFullPath(e.Name)
	if name == "reflect.makeFuncStub()" {
		// fallback to the module to at least know where this comes from
		name = fmt.Sprintf("%s(%s)", e.Kind, e.ModuleName) // eg. "provide(comp/core/ipc)"
	}
	span := &Span{
		Service:  serviceName,
		Name:     constructorName,
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
	// drop the span if the Fx initialization is complete
	if l.spans == nil {
		return
	}
	l.spans = append(l.spans, span)
	l.mu.Unlock()
}

// handleOnStartExecuted captures OnStart hook execution time.
func (l *FxTracingLogger) handleOnStartExecuted(e *fxevent.OnStartExecuted) {
	// Use the Runtime field from Fx event for duration
	startTime := time.Now().Add(-e.Runtime).UnixNano()
	name := extractShortPathFromFullPath(e.FunctionName)

	span := &Span{
		Service:  serviceName,
		Name:     onStartHookName,
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
	// drop the span if the Fx initialization is complete
	if l.spans == nil {
		return
	}
	l.spans = append(l.spans, span)
	l.mu.Unlock()
}

// handleStarted is called when Fx startup completes.
// Sends all buffered spans to Datadog asynchronously with retry.
func (l *FxTracingLogger) handleStarted(e *fxevent.Started) {
	endTime := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// drop the span if the Fx initialization is complete
	if l.spans == nil {
		return
	}

	// Check if traceSending has been enabled during the Fx initialization
	// If not cleanup the memory and return
	if l.traceAgentPort == "" {
		l.spans = nil
		return
	}

	// Create root span
	rootSpan := &Span{
		Service:  serviceName,
		Name:     rootSpanName,
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
	go sendSpansToDatadog(l.debugLogger, l.spans, l.traceAgentPort)
	// reset the spans buffer
	l.spans = nil
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
