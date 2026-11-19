// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxevent"
)

func TestStartupTraceDisabled(t *testing.T) {
	for _, enabled := range []string{"", "false"} {
		t.Run(enabled, func(t *testing.T) {
			t.Setenv("DD_FX_TRACING_ENABLED", enabled)
			logger := withFxTracer(fxevent.NopLogger, time.Now(), io.Discard)
			trace := StartStartupTrace(logger, "startAgent", "test")
			require.Nil(t, trace)
			trace.StartPhase("hostname.Get").Finish(errors.New("ignored"))
			trace.StartPhase("api.StartServer").StartPhase("module.Register").Finish(nil)
			trace.Finish(nil)
		})
	}
}

func TestStartupTraceNestedPhases(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := &FxTracingLogger{flavor: "system-probe", traceID: 123, traceAgentPort: "8126"}
		trace := StartStartupTrace(logger, "startSystemProbe", "7.test")
		require.NotNil(t, trace)
		api := trace.StartPhase("api.StartServer")
		modules := api.StartPhase("module.Register")
		create := modules.StartPhase("module.network_tracer.create")
		time.Sleep(2 * time.Second)
		create.Finish(errors.New("module initialization failed"))
		assert.Nil(t, create.StartPhase("finished"))
		register := modules.StartPhase("module.discovery.register")
		time.Sleep(3 * time.Second)
		register.Finish(nil)
		modules.Finish(nil)
		api.Finish(nil)
		spans := trace.complete(nil)
		require.Len(t, spans, 5)
		root := spans[4]
		assert.Equal(t, root.SpanID, spans[0].ParentID)
		assert.Equal(t, spans[0].SpanID, spans[1].ParentID)
		assert.Equal(t, spans[1].SpanID, spans[2].ParentID)
		assert.Equal(t, spans[1].SpanID, spans[3].ParentID)
		assert.Equal(t, int64(2*time.Second), spans[2].Duration)
		assert.Equal(t, int64(3*time.Second), spans[3].Duration)
		assert.Equal(t, int64(5*time.Second), root.Duration)
		assert.Equal(t, int32(1), spans[2].Error)
		assert.Zero(t, root.Error, "a recoverable module error need not fail process startup")
		for _, span := range spans {
			assert.Equal(t, root.TraceID, span.TraceID)
			assert.Equal(t, "system-probe", span.Meta["process_name"])
		}
		assert.Nil(t, api.StartPhase("finished"))
	})
}

func TestStartupTraceRequiresTransport(t *testing.T) {
	t.Setenv("DD_FX_TRACING_ENABLED", "true")
	logger := withFxTracer(fxevent.NopLogger, time.Now(), io.Discard)
	require.Nil(t, StartStartupTrace(logger, "startAgent", "test"))
}

func TestStartupTraceWaterfall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The Fx trace has already been sent and its buffer cleared. Post-Fx
		// tracing must still work without reopening or extending that root.
		logger := &FxTracingLogger{
			flavor:         "agent",
			traceID:        123,
			debugLogger:    io.Discard,
			traceAgentPort: "8126",
		}
		trace := StartStartupTrace(logger, "startAgent", "7.test")
		require.NotNil(t, trace)
		rootStart := trace.root.Start

		phase := trace.StartPhase("hostname.Get")
		time.Sleep(5 * time.Second)
		phase.Finish(nil)
		phase.Finish(errors.New("must not overwrite the completed phase"))

		phase = trace.StartPhase("autodiscovery.LoadAndRun")
		time.Sleep(65 * time.Second)
		phase.Finish(nil)

		spans := trace.complete(nil)
		require.Len(t, spans, 3)
		root := spans[2]
		assert.Equal(t, "startAgent", root.Name)
		assert.Equal(t, "agent", root.Resource)
		assert.Zero(t, root.ParentID)
		assert.Equal(t, int64(70*time.Second), root.Duration)
		assert.Equal(t, rootStart, root.Start)
		assert.NotEqual(t, logger.traceID, root.TraceID)
		assert.Nil(t, logger.spans)
		assert.Equal(t, int64(5*time.Second), spans[0].Duration)
		assert.Equal(t, int64(65*time.Second), spans[1].Duration)
		assert.Equal(t, spans[0].Start+spans[0].Duration, spans[1].Start)
		assert.Equal(t, root.Start+root.Duration, spans[1].Start+spans[1].Duration)
		for _, s := range spans {
			assert.Equal(t, serviceName, s.Service)
			assert.Equal(t, root.TraceID, s.TraceID)
			assert.Equal(t, map[string]string{
				"fx.trace_id": "123", "process_name": "agent", "agent.version": "7.test",
			}, s.Meta)
			assert.Zero(t, s.Error)
		}
		for _, s := range spans[:2] {
			assert.Equal(t, root.SpanID, s.ParentID)
			assert.Equal(t, "startAgent.phase", s.Name)
		}
		assert.Nil(t, trace.complete(nil), "a trace must only be sent once")
		assert.Nil(t, trace.StartPhase("already finished"))
	})
}

func TestStartupTraceFailure(t *testing.T) {
	logger := &FxTracingLogger{flavor: "agent", traceID: 123, traceAgentPort: "8126"}
	trace := StartStartupTrace(logger, "startAgent", "7.test")
	require.NotNil(t, trace)
	err := errors.New("potentially sensitive error details")
	trace.StartPhase("hostname.Get").Finish(err)
	spans := trace.complete(err)
	require.Len(t, spans, 2)
	assert.Equal(t, int32(1), spans[0].Error)
	assert.Equal(t, int32(1), spans[1].Error)
	payload, marshalErr := json.Marshal(spans)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(payload), err.Error())
}

type startupRoundTripper func(*http.Request) (*http.Response, error)

func (f startupRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestStartupTraceDeliveryDoesNotBlockStartup(t *testing.T) {
	previousTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	synctest.Test(t, func(t *testing.T) {
		delivered := make(chan [][]*Span, 1)
		http.DefaultTransport = startupRoundTripper(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "http://localhost:8126/v0.3/traces", req.URL.String())
			assert.Equal(t, "1", req.Header.Get("X-Datadog-Trace-Count"))
			var traces [][]*Span
			if err := json.NewDecoder(req.Body).Decode(&traces); err != nil {
				t.Error(err)
			}
			delivered <- traces
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		logger := &FxTracingLogger{flavor: "agent", traceID: 123, traceAgentPort: "8126", debugLogger: io.Discard}
		trace := StartStartupTrace(logger, "startAgent", "7.test")
		require.NotNil(t, trace)
		trace.StartPhase("hostname.Get").Finish(nil)
		beforeFinish := time.Now()
		trace.Finish(nil)
		assert.Equal(t, beforeFinish, time.Now(), "startup must not wait for the transport retry ticker")
		trace.Finish(nil)
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		select {
		case traces := <-delivered:
			require.Len(t, traces, 1)
			require.Len(t, traces[0], 2)
			assert.Equal(t, traces[0][1].SpanID, traces[0][0].ParentID)
			assert.Equal(t, "startAgent", traces[0][1].Name)
		default:
			t.Fatal("startup trace was not delivered")
		}
	})
}
