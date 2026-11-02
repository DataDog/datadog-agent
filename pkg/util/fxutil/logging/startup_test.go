// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logging

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/fx/fxevent"
)

func TestStartupTraceConcurrentPhases(t *testing.T) {
	var sent []*Span
	sends := 0
	root := newStartupTrace("autodiscovery", func(spans []*Span) { sent = spans; sends++ })
	root.SetTag("agent.version", "test-build")
	ctx, cancel := context.WithCancel(root.Context(t.Context()))
	cancel()
	prepCtx, prep := StartStartupSpan(ctx, "preparation")
	if !errors.Is(prepCtx.Err(), context.Canceled) {
		t.Fatal("diagnostic context lost cancellation")
	}
	_, scan := StartStartupSpan(prepCtx, "config.scan")
	scan.SetTag("config.with_formats", "false")
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			scan.AddMetric("config.files", 1)
			scan.AddMetric("config.bytes", 10)
		})
	}
	wg.Wait()
	scan.Finish(nil)
	prep.Finish(nil)
	_, wait := StartStartupSpan(root.Context(t.Context()), "preparation.wait")
	wait.Finish(context.Canceled)
	if sends != 0 {
		t.Fatal("trace sent before its startup barrier completed")
	}
	root.Finish(nil)
	root.Finish(errors.New("already finished"))
	if sends != 1 || len(sent) != 4 {
		t.Fatalf("got %d sends and %d spans", sends, len(sent))
	}
	if sent[1].ParentID != sent[0].SpanID || sent[2].ParentID != sent[1].SpanID || sent[3].ParentID != sent[0].SpanID {
		t.Fatal("phase hierarchy was not preserved")
	}
	if sent[2].Metrics["config.files"] != 100 || sent[2].Metrics["config.bytes"] != 1000 {
		t.Fatalf("lost concurrent worker measurements: %v", sent[2].Metrics)
	}
	if sent[2].Meta["config.with_formats"] != "false" || sent[3].Error != 1 {
		t.Fatal("lost phase metadata or cancellation status")
	}
	for _, span := range sent {
		if span.Meta["agent.version"] != "test-build" {
			t.Fatal("phase lost build metadata needed for grouping")
		}
		if span.TraceID != root.span.TraceID || span.Duration < 0 {
			t.Fatalf("invalid span: %+v", span)
		}
		if span.Start+span.Duration > root.span.Start+root.span.Duration {
			t.Fatal("child extends beyond diagnostic startup root")
		}
	}
	if _, late := StartStartupSpan(root.Context(t.Context()), "late"); late != nil {
		t.Fatal("finished startup trace recorded a runtime operation")
	}
	// The payload handed to the sender is immutable after root completion.
	scan.AddMetric("config.files", 1)
	if sent[2].Metrics["config.files"] != 100 {
		t.Fatal("finished payload was mutated")
	}
	if _, err := json.Marshal([][]*Span{sent}); err != nil {
		t.Fatal(err)
	}
}

func TestStartupTracingDisabled(t *testing.T) {
	ctx := t.Context()
	var logger *FxTracingLogger
	for _, tracer := range []*FxTracingLogger{logger, {}} {
		root := tracer.StartStartupTrace("autodiscovery")
		if root != nil || root.Context(ctx) != ctx {
			t.Fatal("disabled tracing allocated a diagnostic trace")
		}
	}
	childCtx, span := StartStartupSpan(ctx, "config.scan")
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
	if root == nil || root.span.Meta["fx.trace_id"] != "0" {
		t.Fatal("Fx Started disabled subsequent startup diagnostics")
	}
	// Exercise completion without starting a network sender.
	root.trace.send = func([]*Span) {}
	_, child := StartStartupSpan(root.Context(t.Context()), "preparation")
	child.Finish(nil)
	root.Finish(nil)
}
