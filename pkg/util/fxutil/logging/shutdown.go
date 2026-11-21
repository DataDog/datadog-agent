// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"errors"
	"maps"
	"strings"
	"time"

	"go.uber.org/fx/fxevent"
)

// EnableShutdownTracing installs an independent, synchronous, bounded exporter.
// The exporter must not depend on any Fx component remaining running.
func (l *FxTracingLogger) EnableShutdownTracing(agentVersion string, send func([]*Span)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.shutdownVersion = agentVersion
	l.shutdownSend = send
}

// StartShutdownTrace begins the core Agent or system-probe teardown trace. Call
// this immediately before explicit teardown. Fx Stopped completes and exports
// the trace after component OnStop hooks; callers must not call Finish on it.
// Nil traces/phases remain safe when diagnostic tracing is disabled.
func StartShutdownTrace(logger fxevent.Logger) *StartupTrace {
	l, ok := logger.(*FxTracingLogger)
	if !ok {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.startShutdownTraceLocked()
}

func (l *FxTracingLogger) startShutdownTraceLocked() *StartupTrace {
	if l.shutdownSend == nil || l.shutdownFinished {
		return nil
	}
	if l.shutdownTrace != nil {
		return l.shutdownTrace
	}
	var operation string
	switch strings.TrimSuffix(l.flavor, ".exe") {
	case "agent":
		operation = "stopAgent"
	case "system-probe":
		operation = "stopSystemProbe"
	default:
		return nil
	}
	l.shutdownTrace = l.newLifecycleTrace(operation, l.shutdownVersion)
	return l.shutdownTrace
}

func (l *FxTracingLogger) handleOnStopExecuting(e *fxevent.OnStopExecuting) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.startShutdownTraceLocked(); t != nil {
		l.shutdownHook = t.StartPhase(extractShortPathFromFullPath(e.FunctionName))
	}
}

func (l *FxTracingLogger) handleOnStopExecuted(e *fxevent.OnStopExecuted) {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.startShutdownTraceLocked()
	if t == nil {
		return
	}
	phase := l.shutdownHook
	if phase == nil {
		phase = t.StartPhase(extractShortPathFromFullPath(e.FunctionName))
	}
	// Fx provides the hook runtime, excluding observer overhead.
	phase.span.Start = time.Now().Add(-e.Runtime).UnixNano()
	phase.span.Duration = int64(e.Runtime)
	phase.span.Error = errToCode(e.Err)
	phase.span = nil
	l.shutdownHook = nil
}

func (l *FxTracingLogger) handleStopped(e *fxevent.Stopped) {
	l.mu.Lock()
	if l.shutdownFinished || l.shutdownTrace == nil {
		l.mu.Unlock()
		return
	}
	// app.Stop may time out while a hook is still running. Later hook events
	// must not reopen or mutate the completed trace.
	l.shutdownFinished = true
	stopErr := e.Err
	if phase := l.shutdownHook; phase != nil {
		if stopErr == nil {
			stopErr = errors.New("stop hook did not finish")
		}
		phase.span.Meta = maps.Clone(phase.span.Meta)
		phase.span.Meta["shutdown.incomplete"] = "true"
		phase.Finish(stopErr)
		l.shutdownHook = nil
	}
	spans := l.shutdownTrace.complete(stopErr)
	send := l.shutdownSend
	l.mu.Unlock()
	// Do not launch a goroutine: process exit follows app.Stop immediately.
	send(spans)
}
