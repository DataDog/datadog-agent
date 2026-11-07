// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logging

import (
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/startup"
	"strconv"
)

// StartStartupTrace starts diagnostics that can outlive Fx Started. The
// fx.trace_id attribute links the separate trace to the original Fx trace.
// A nil or disabled Fx tracer returns a nil, no-op span.
func (l *FxTracingLogger) StartStartupTrace(resource string) *startup.Span {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	port, writer, flavor, fxTraceID := l.traceAgentPort, l.debugLogger, l.flavor, l.traceID
	l.mu.Unlock()
	if port == "" {
		return nil
	}
	root := startup.NewTrace(serviceName, resource, func(spans []*Span) {
		go sendSpansToDatadog(writer, spans, port)
	})
	root.SetTag("process_name", flavor)
	root.SetTag("fx.trace_id", strconv.FormatUint(fxTraceID, 10))
	return root
}
