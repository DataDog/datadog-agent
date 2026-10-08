// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil/startup"
)

func TestStartupPhasesInFxTrace(t *testing.T) {
	t.Setenv("DD_FX_TRACING_ENABLED", "true")
	packets := make(chan []*Span, 1)
	var tracer *FxTracingLogger
	app := fx.New(DefaultFxLoggingOption(), fx.Invoke(func(lc fx.Lifecycle, recorder *startup.Recorder, logger fxevent.Logger) {
		tracer = logger.(*FxTracingLogger)
		tracer.EnableSpansSending("unused-test-port")
		tracer.sendSpans = func(_ io.Writer, spans []*Span, _ string) { packets <- spans }
		lc.Append(fx.Hook{OnStart: func(context.Context) error {
			phase := recorder.Start("autodiscovery.listeners.initialize", "listeners")
			phase.SetMetric("config_files.files_read", 12)
			phase.Start("autodiscovery.listener.initialize", "kubelet").Finish(errors.New("sensitive configuration"))
			phase.Finish(nil)
			return nil
		}})
	}))
	require.NoError(t, app.Err())
	require.NoError(t, app.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, app.Stop(context.Background())) })

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var spans []*Span
	select {
	case spans = <-packets:
	case <-ctx.Done():
		t.Fatal("startup trace was not sent")
	}
	byID := make(map[uint64]*Span)
	var parent, child, root *Span
	for _, span := range spans {
		byID[span.SpanID] = span
		switch span.Name {
		case "autodiscovery.listeners.initialize":
			parent = span
		case "autodiscovery.listener.initialize":
			child = span
		case rootSpanName:
			root = span
		}
	}
	require.NotNil(t, root)
	require.NotNil(t, parent)
	require.NotNil(t, child)
	require.Equal(t, parent.SpanID, child.ParentID)
	require.Equal(t, float64(12), parent.Metrics["config_files.files_read"])
	hook := byID[parent.ParentID]
	require.NotNil(t, hook)
	require.Equal(t, onStartHookName, hook.Name)
	require.Equal(t, root.SpanID, hook.ParentID)
	for _, span := range []*Span{hook, parent, child} {
		require.Equal(t, root.TraceID, span.TraceID)
		require.Equal(t, serviceName, span.Service)
		require.GreaterOrEqual(t, span.Duration, int64(0))
	}
	require.EqualValues(t, 1, child.Error)
	require.Zero(t, root.Error, "a handled phase error must not change Fx's outcome")
	payload, err := json.Marshal(spans)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "sensitive configuration")

	// Late Fx events used to return with the logger mutex still held. Check
	// multiple event types after the buffer has closed, without sleeping.
	done := make(chan struct{})
	go func() {
		tracer.LogEvent(&fxevent.Run{Name: "late"})
		tracer.LogEvent(&fxevent.OnStartExecuting{})
		tracer.LogEvent(&fxevent.OnStartExecuted{})
		tracer.LogEvent(&fxevent.Started{})
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("late Fx events deadlocked")
	}
}

func TestStartupRecorderDisabledInFx(t *testing.T) {
	t.Setenv("DD_FX_TRACING_ENABLED", "false")
	app := fx.New(DefaultFxLoggingOption(), fx.Invoke(func(lc fx.Lifecycle, recorder *startup.Recorder) {
		lc.Append(fx.Hook{OnStart: func(context.Context) error {
			if recorder.Start("autodiscovery.setup", "agent") != nil {
				return errors.New("disabled recorder created a phase")
			}
			return nil
		}})
	}))
	require.NoError(t, app.Start(t.Context()))
	require.NoError(t, app.Stop(t.Context()))
}

func TestStartupRecordersAreAppLocal(t *testing.T) {
	t.Setenv("DD_FX_TRACING_ENABLED", "true")
	options := DefaultFxLoggingOption()
	var recorders []*startup.Recorder
	for range 2 {
		app := fx.New(options, fx.Invoke(func(lc fx.Lifecycle, recorder *startup.Recorder) {
			recorders = append(recorders, recorder)
			lc.Append(fx.Hook{OnStart: func(context.Context) error {
				phase := recorder.Start("autodiscovery.setup", "agent")
				if phase == nil {
					return errors.New("recorder was shared with an already stopped app")
				}
				phase.Finish(nil)
				return nil
			}})
		}))
		require.NoError(t, app.Start(t.Context()))
		require.NoError(t, app.Stop(t.Context()))
	}
	require.NotSame(t, recorders[0], recorders[1])
	for _, recorder := range recorders {
		require.Nil(t, recorder.Start("after-startup", "agent"))
	}
}

func TestStartupTimeoutPreservesPhaseParents(t *testing.T) {
	t.Setenv("DD_FX_TRACING_ENABLED", "true")
	recorder := startup.NewRecorder(true)
	tracer := withFxTracer(fxevent.NopLogger, time.Now(), io.Discard, recorder).(*FxTracingLogger)
	packets := make(chan []*Span, 1)
	tracer.EnableSpansSending("unused-test-port")
	tracer.sendSpans = func(_ io.Writer, spans []*Span, _ string) { packets <- spans }
	tracer.LogEvent(&fxevent.OnStartExecuting{FunctionName: "blockedHook"})
	parent := recorder.Start("autodiscovery.listeners.initialize", "listeners")
	parent.SetMetric("config_files.files_read", 7)
	parent.Start("autodiscovery.listener.initialize", "kubelet").Finish(nil)
	// Model Fx timing out before the synchronous hook returns, without sleeps.
	tracer.LogEvent(&fxevent.Started{Err: context.DeadlineExceeded})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var spans []*Span
	select {
	case spans = <-packets:
	case <-ctx.Done():
		t.Fatal("timeout trace was not sent")
	}
	require.Len(t, spans, 4)
	byID := make(map[uint64]*Span)
	for _, span := range spans {
		byID[span.SpanID] = span
	}
	for _, span := range spans {
		if span.ParentID != 0 {
			require.Contains(t, byID, span.ParentID, "partial traces must not have orphan spans")
		}
		switch span.Name {
		case onStartHookName, "autodiscovery.listeners.initialize":
			require.Equal(t, float64(1), span.Metrics["startup.incomplete"])
		case rootSpanName:
			require.EqualValues(t, 1, span.Error)
		}
		if span.Name == "autodiscovery.listeners.initialize" {
			require.Equal(t, float64(7), span.Metrics["config_files.files_read"])
		}
	}
	before, err := json.Marshal(spans)
	require.NoError(t, err)
	parent.Finish(nil)
	tracer.LogEvent(&fxevent.OnStartExecuted{FunctionName: "blockedHook"})
	after, err := json.Marshal(spans)
	require.NoError(t, err)
	require.Equal(t, before, after, "late completion must not mutate the exported snapshot")
}
