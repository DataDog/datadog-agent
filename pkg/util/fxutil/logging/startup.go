// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"io"
	"math/rand"
	"strconv"
	"time"

	"go.uber.org/fx/fxevent"
)

// StartupTrace records synchronous lifecycle work, including post-Fx startup and shutdown.
// A trace and its phases must be used from the same goroutine. Nil traces and
// phases are safe to use when Fx tracing is disabled.
type StartupTrace struct {
	root           *Span
	startedAt      time.Time
	spans          []*Span
	debugLogger    io.Writer
	traceAgentPort string
}

// StartupPhase measures one synchronous call or group of calls in a StartupTrace.
type StartupPhase struct {
	span      *Span
	startedAt time.Time
	trace     *StartupTrace
}

// StartStartupTrace starts an independent diagnostic trace using the opt-in Fx
// tracing transport. It does not extend the existing Fx startup root, whose end
// remains Fx Started. fx.trace_id links the two traces.
// operationName must be a fixed name, not configuration or user input.
func StartStartupTrace(logger fxevent.Logger, operationName, agentVersion string) *StartupTrace {
	l, ok := logger.(*FxTracingLogger)
	if !ok {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.traceAgentPort == "" {
		return nil
	}
	return l.newLifecycleTrace(operationName, agentVersion)
}

// newLifecycleTrace requires l.mu to be held.
func (l *FxTracingLogger) newLifecycleTrace(operationName, agentVersion string) *StartupTrace {
	startedAt := time.Now()
	return &StartupTrace{
		root: &Span{
			Service:  serviceName,
			Name:     operationName,
			Resource: l.flavor,
			TraceID:  rand.Uint64(),
			SpanID:   rand.Uint64(),
			Start:    startedAt.UnixNano(),
			Type:     "custom",
			Meta: map[string]string{
				"fx.trace_id":   strconv.FormatUint(l.traceID, 10),
				"process_name":  l.flavor,
				"agent.version": agentVersion,
			},
		},
		startedAt:      startedAt,
		debugLogger:    l.debugLogger,
		traceAgentPort: l.traceAgentPort,
	}
}

// StartPhase starts a child span. resource must identify a fixed startup call,
// never a filename or configuration value. Finish the phase before starting the
// next one so the waterfall shows elapsed time spent in each synchronous call.
func (t *StartupTrace) StartPhase(resource string) *StartupPhase {
	if t == nil || t.root == nil {
		return nil
	}
	return t.startPhase(t.root.SpanID, resource)
}

// StartPhase starts a nested phase for synchronous work within this phase.
// Finish all children before finishing their parent. The same naming and
// goroutine restrictions as StartupTrace.StartPhase apply.
func (p *StartupPhase) StartPhase(resource string) *StartupPhase {
	if p == nil || p.span == nil || p.trace.root == nil {
		return nil
	}
	return p.trace.startPhase(p.span.SpanID, resource)
}

func (t *StartupTrace) startPhase(parentID uint64, resource string) *StartupPhase {
	startedAt := time.Now()
	s := &Span{
		Service:  t.root.Service,
		Name:     t.root.Name + ".phase",
		Resource: resource,
		TraceID:  t.root.TraceID,
		SpanID:   rand.Uint64(),
		ParentID: parentID,
		Start:    startedAt.UnixNano(),
		Type:     "custom",
		Meta:     t.root.Meta,
	}
	t.spans = append(t.spans, s)
	return &StartupPhase{span: s, startedAt: startedAt, trace: t}
}

// Finish ends the phase, recording only the error flag, not the error text.
func (p *StartupPhase) Finish(err error) {
	if p == nil || p.span == nil {
		return
	}
	p.span.Duration = time.Since(p.startedAt).Nanoseconds()
	p.span.Error = errToCode(err)
	p.span = nil
}

// Finish ends the startup trace and sends it asynchronously. Delivery failures
// cannot delay the startup caller; an exiting process may lose the trace.
func (t *StartupTrace) Finish(err error) {
	if spans := t.complete(err); len(spans) != 0 {
		go sendSpansToDatadog(t.debugLogger, spans, t.traceAgentPort)
	}
}

func (t *StartupTrace) complete(err error) []*Span {
	if t == nil || t.root == nil {
		return nil
	}
	t.root.Duration = time.Since(t.startedAt).Nanoseconds()
	t.root.Error = errToCode(err)
	spans := append(t.spans, t.root)
	t.root = nil
	t.spans = nil
	return spans
}
