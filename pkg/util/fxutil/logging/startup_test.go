// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logging

import (
	"testing"
	"time"

	"go.uber.org/fx/fxevent"

	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/startup"
)

func TestStartupTracingDisabled(t *testing.T) {
	ctx := t.Context()
	var logger *FxTracingLogger
	for _, tracer := range []*FxTracingLogger{logger, {}} {
		root := tracer.StartStartupTrace("autodiscovery")
		if root != nil || root.Context(ctx) != ctx {
			t.Fatal("disabled tracing allocated a diagnostic trace")
		}
	}
	childCtx, span := startup.StartSpan(ctx, "config.scan")
	if childCtx != ctx || span != nil {
		t.Fatal("disabled tracing changed context")
	}
	span.SetTag("provider", "file")
	span.AddMetric("config.files", 1)
	span.Measure("config.read_time_ns")()
	span.Finish(nil)
}

func TestFxCompletionDoesNotBlockDiagnostics(t *testing.T) {
	l := &FxTracingLogger{startTime: time.Now(), spans: []*Span{}}
	l.handleStarted(&fxevent.Started{})
	// Late events must release the logger's mutex even after its buffer is gone.
	l.handleRun(&fxevent.Run{})
	l.handleOnStartExecuted(&fxevent.OnStartExecuted{})
	l.EnableSpansSending("8126")
	root := l.StartStartupTrace("autodiscovery")
	if root == nil {
		t.Fatal("Fx Started disabled subsequent startup diagnostics")
	}
	// Creating post-Fx diagnostics must not depend on the flushed Fx buffer.
	// Do not finish this root: this unit test does not start a network sender.
	_, child := startup.StartSpan(root.Context(t.Context()), "preparation")
	child.Finish(nil)
}
