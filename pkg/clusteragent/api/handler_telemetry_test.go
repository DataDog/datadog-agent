// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"
)

// TestWithTelemetryWrapper_NoopTracer verifies that the handler works correctly
// when the tracer has not been started (no-op spans, no panics).
func TestWithTelemetryWrapper_NoopTracer(t *testing.T) {
	// Intentionally no mocktracer.Start() — tracer returns NoopSpan.
	th := &TelemetryHandler{
		handlerName: "noopHandler",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWithTelemetryWrapper_SpanCreation(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	th := &TelemetryHandler{
		handlerName: "getCheckConfigs",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	req := httptest.NewRequest("GET", "/clusterchecks/configs/node1", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	assert.Equal(t, "cluster_agent.api.request", span.OperationName())
	assert.Equal(t, "getCheckConfigs", span.Tag("resource.name"))
	assert.Equal(t, "GET", span.Tag("http.method"))
	assert.Equal(t, "/clusterchecks/configs/node1", span.Tag("http.url"))
	assert.Equal(t, 200, span.Tag("http.status_code"))
	assert.Nil(t, span.Tag("error"))
}

func TestWithTelemetryWrapper_5xxSetsErrorTag(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	th := &TelemetryHandler{
		handlerName: "errorHandler",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, 500, spans[0].Tag("http.status_code"))
	_, ok := spans[0].Tag("error").(error)
	assert.True(t, ok, "error tag should be set for 5xx responses")
}

func TestWithTelemetryWrapper_4xxSetsErrorTag(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	th := &TelemetryHandler{
		handlerName: "notFoundHandler",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, 404, spans[0].Tag("http.status_code"))
	_, ok := spans[0].Tag("error").(error)
	assert.True(t, ok, "error tag should be set for 4xx responses")
}

func TestWithTelemetryWrapper_PanicCapturesErrorDetails(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	th := &TelemetryHandler{
		handlerName: "panicHandler",
		handler: func(_ http.ResponseWriter, _ *http.Request) {
			panic("something went wrong")
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	assert.Panics(t, func() {
		th.handle(rec, req)
	})

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	// mocktracer stores the error object in the "error" tag via WithError
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object from WithError")
	assert.Equal(t, "panic: something went wrong", err.Error())
}

type customError struct{ msg string }

func (e *customError) Error() string { return e.msg }

func TestWithTelemetryWrapper_PanicWithErrorPreservesType(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	panicErr := &customError{msg: "runtime failure"}
	th := &TelemetryHandler{
		handlerName: "panicErrorHandler",
		handler: func(_ http.ResponseWriter, _ *http.Request) {
			panic(panicErr)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	assert.Panics(t, func() {
		th.handle(rec, req)
	})

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error interface, got %T", span.Tag("error"))
	assert.Equal(t, "runtime failure", err.Error())
}

func TestWithTelemetryWrapper_SetSpanError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	testErr := errors.New("workloadmeta lookup failed")
	th := &TelemetryHandler{
		handlerName: "errorDetailHandler",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			if tw, ok := w.(*telemetryWriterWrapper); ok {
				tw.SetSpanError(testErr)
			}
			w.WriteHeader(http.StatusInternalServerError)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, 500, span.Tag("http.status_code"))
	// mocktracer stores the error object in the "error" tag via WithError
	err, ok := span.Tag("error").(error)
	require.True(t, ok, "error tag should be an error object from WithError")
	assert.Equal(t, "workloadmeta lookup failed", err.Error())
}

func TestWithTelemetryWrapper_ForwardedTag(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	th := &TelemetryHandler{
		handlerName: "forwardedHandler",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(respForwarded, "true")
			w.WriteHeader(http.StatusOK)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "true", spans[0].Tag("forwarded"))
}

func TestWithTelemetryWrapper_NotForwarded(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	th := &TelemetryHandler{
		handlerName: "localHandler",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	// forwarded tag should not be set when request is not forwarded
	assert.Nil(t, spans[0].Tag("forwarded"))
}

func TestWithTelemetryWrapper_NoErrorWhenNilCapturedErr(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	th := &TelemetryHandler{
		handlerName: "okHandler",
		handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	th.handle(rec, req)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	// When capturedErr is nil, WithError(nil) should not set error tags
	assert.Nil(t, spans[0].Tag("error"))
}
