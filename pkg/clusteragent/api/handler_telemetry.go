// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

var (
	apiRequests = telemetryimpl.GetCompatComponent().NewCounterWithOpts("", "api_requests",
		[]string{"handler", "status", "forwarded"}, "Counter of requests made to the cluster agent API.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	apiElapsed = telemetryimpl.GetCompatComponent().NewHistogramWithOpts("", "api_elapsed",
		[]string{"handler", "status", "forwarded"}, "Poll duration distribution by config provider (in seconds).",
		prometheus.DefBuckets,
		telemetry.Options{NoDoubleUnderscoreSep: true})
)

// TelemetryHandler provides a http handler and emits requests telemetry for it.
type TelemetryHandler struct {
	handlerName string
	handler     func(w http.ResponseWriter, r *http.Request)
}

// WithTelemetryWrapper returns a http handler function that emits telemetry.
func WithTelemetryWrapper(handlerName string, handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	th := TelemetryHandler{
		handlerName: handlerName,
		handler:     handler,
	}
	return th.handle
}

func (t *TelemetryHandler) handle(w http.ResponseWriter, r *http.Request) {
	wrapper := &telemetryWriterWrapper{ResponseWriter: w, handlerName: t.handlerName, startTime: time.Now()}
	span, ctx := tracer.StartSpanFromContext(r.Context(), "cluster_agent.api.request",
		tracer.ResourceName(t.handlerName),
		tracer.SpanType("web"),
		tracer.Tag("http.method", r.Method),
		tracer.Tag("http.url", r.URL.Path))
	wrapper.setSpanTags = func(statusCode int) {
		span.SetTag("http.status_code", statusCode)
		if statusCode >= 400 {
			span.SetTag("error", true)
		}
	}
	defer func() {
		if p := recover(); p != nil {
			if !wrapper.wroteHeader {
				wrapper.setSpanTags(http.StatusInternalServerError)
			}
			var panicErr error
			if e, ok := p.(error); ok {
				panicErr = e
			} else {
				panicErr = fmt.Errorf("panic: %v", p)
			}
			span.Finish(tracer.WithError(panicErr))
			panic(p)
		}
		if !wrapper.wroteHeader {
			wrapper.setSpanTags(http.StatusOK)
		}
		if wrapper.forwarded {
			span.SetTag("forwarded", true)
		}
		span.Finish(tracer.WithError(wrapper.capturedErr))
	}()
	r = r.WithContext(ctx)
	t.handler(wrapper, r)
}

// telemetryWriterWrapper wraps http.ResponseWriter to capture response codes for telemetry and tracing.
type telemetryWriterWrapper struct {
	http.ResponseWriter
	handlerName string
	startTime   time.Time
	wroteHeader bool
	forwarded   bool
	capturedErr error
	setSpanTags func(int)
}

// SetSpanError propagates an error to the telemetry span if the writer is a telemetryWriterWrapper.
// This populates error.message, error.type, and error.stack on the span via tracer.WithError.
func SetSpanError(w http.ResponseWriter, err error) {
	if tw, ok := w.(*telemetryWriterWrapper); ok {
		tw.capturedErr = err
	}
}

func (w *telemetryWriterWrapper) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
	forwarded := w.Header().Get(respForwarded)
	if forwarded == "" {
		forwarded = "false"
	} else {
		w.forwarded = forwarded == "true"
	}

	apiElapsed.Observe(time.Since(w.startTime).Seconds(), w.handlerName, strconv.Itoa(statusCode), forwarded)
	apiRequests.Inc(w.handlerName, strconv.Itoa(statusCode), forwarded)

	w.setSpanTags(statusCode)
}
