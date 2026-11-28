// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func TestShutdownTraceDisabled(t *testing.T) {
	require.Nil(t, StartShutdownTrace(fxevent.NopLogger))
	logger := &FxTracingLogger{flavor: "agent", fxLogger: fxevent.NopLogger}
	require.Nil(t, StartShutdownTrace(logger))
	var trace *StartupTrace
	trace.StartPhase("ignored").Finish(nil)
}

func TestShutdownIncludesExplicitTeardownAndFxHooks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := &FxTracingLogger{flavor: "system-probe", traceID: 123, fxLogger: fxevent.NopLogger, debugLogger: io.Discard}
		var delivered []*Span
		logger.EnableShutdownTracing("test-version", func(spans []*Span) { delivered = spans })
		app := fx.New(fx.WithLogger(func() fxevent.Logger { return logger }), fx.Invoke(func(lc fx.Lifecycle) {
			lc.Append(fx.Hook{OnStop: func(context.Context) error { time.Sleep(3 * time.Second); return errors.New("private hook error") }})
		}))
		require.NoError(t, app.Start(context.Background()))
		trace := StartShutdownTrace(logger)
		require.NotNil(t, trace)
		require.Same(t, trace, StartShutdownTrace(logger))
		phase := trace.StartPhase("module.Close")
		nested := phase.StartPhase("module.network_tracer.close")
		time.Sleep(7 * time.Second)
		nested.Finish(nil)
		phase.Finish(nil)
		require.Error(t, app.Stop(context.Background()))
		// Export has finished before app.Stop returns, with no local trace-agent.
		require.Len(t, delivered, 4)
		root := delivered[3]
		assert.Equal(t, "stopSystemProbe", root.Name)
		assert.Equal(t, int64(10*time.Second), root.Duration)
		assert.Equal(t, int32(1), root.Error)
		assert.Equal(t, int64(3*time.Second), delivered[2].Duration)
		assert.Equal(t, int32(1), delivered[2].Error)
		assert.Equal(t, delivered[0].SpanID, delivered[1].ParentID)
		for _, s := range delivered {
			assert.Equal(t, root.TraceID, s.TraceID)
			assert.Equal(t, "123", s.Meta["fx.trace_id"])
		}
		assert.Nil(t, StartShutdownTrace(logger))
		logger.handleOnStopExecuted(&fxevent.OnStopExecuted{FunctionName: "late hook"})
		logger.handleStopped(&fxevent.Stopped{})
		assert.Len(t, delivered, 4)
	})
}

func TestShutdownFxOnlyAndWindowsFlavor(t *testing.T) {
	for _, flavor := range []string{"agent.exe", "system-probe.exe"} {
		t.Run(flavor, func(t *testing.T) {
			logger := &FxTracingLogger{flavor: flavor, fxLogger: fxevent.NopLogger}
			var spans []*Span
			logger.EnableShutdownTracing("version", func(s []*Span) { spans = s })
			logger.LogEvent(&fxevent.OnStopExecuting{FunctionName: "hook"})
			logger.LogEvent(&fxevent.OnStopExecuted{FunctionName: "hook", Runtime: time.Second})
			logger.LogEvent(&fxevent.Stopped{})
			require.Len(t, spans, 2)
			assert.Contains(t, []string{"stopAgent", "stopSystemProbe"}, spans[1].Name)
			assert.Equal(t, spans[1].SpanID, spans[0].ParentID)
		})
	}
}

func TestShutdownTimeoutPreservesUnfinishedHook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := &FxTracingLogger{flavor: "agent", fxLogger: fxevent.NopLogger}
		var delivered []*Span
		calls := 0
		logger.EnableShutdownTracing("test", func(spans []*Span) { calls++; delivered = spans })
		release := make(chan struct{})
		app := fx.New(fx.WithLogger(func() fxevent.Logger { return logger }), fx.Invoke(func(lc fx.Lifecycle) {
			lc.Append(fx.Hook{OnStop: func(context.Context) error { <-release; return nil }})
		}))
		require.NoError(t, app.Start(context.Background()))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.ErrorIs(t, app.Stop(ctx), context.DeadlineExceeded)
		require.Len(t, delivered, 2)
		hook, root := delivered[0], delivered[1]
		assert.Equal(t, "true", hook.Meta["shutdown.incomplete"])
		assert.Equal(t, int64(5*time.Second), hook.Duration)
		assert.Equal(t, int64(5*time.Second), root.Duration)
		assert.Equal(t, int32(1), root.Error)
		assert.Equal(t, int32(1), hook.Error)
		assert.NotContains(t, root.Meta, "shutdown.incomplete")
		close(release)
		synctest.Wait()
		assert.Equal(t, 1, calls)
		assert.Equal(t, int64(5*time.Second), hook.Duration, "late hook completion cannot change the exported snapshot")
	})
}
