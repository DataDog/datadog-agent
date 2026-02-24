// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	apiRequests = telemetry.NewCounterWithOpts("", "api_requests",
		[]string{"handler", "status", "forwarded"}, "Counter of requests made to the cluster agent API.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	apiElapsed = telemetry.NewHistogramWithOpts("", "api_elapsed",
		[]string{"handler", "status", "forwarded"}, "Poll duration distribution by config provider (in seconds).",
		prometheus.DefBuckets,
		telemetry.Options{NoDoubleUnderscoreSep: true})
)

// TelemetryHandler provides a http handler and emits requests telemetry for it.
type TelemetryHandler struct {
	handlerName    string
	handler        func(w http.ResponseWriter, r *http.Request)
	tracingEnabled bool
}

// WithTelemetryWrapper returns a http handler function that emits telemetry.
func WithTelemetryWrapper(handlerName string, handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	th := TelemetryHandler{
		handlerName:    handlerName,
		handler:        handler,
		tracingEnabled: pkgconfigsetup.Datadog().GetBool("cluster_agent.tracing.enabled"),
	}
	return th.handle
}

func (t *TelemetryHandler) handle(w http.ResponseWriter, r *http.Request) {
	wrapper := &telemetryWriterWrapper{ResponseWriter: w, handlerName: t.handlerName, startTime: time.Now()}
	if t.tracingEnabled {
		span, ctx := tracer.StartSpanFromContext(r.Context(), "cluster_agent.api.request",
			tracer.ResourceName(t.handlerName),
			tracer.SpanType("web"),
			tracer.Tag("http.method", r.Method),
			tracer.Tag("http.url", r.URL.Path))
		wrapper.setSpanTags = func(statusCode int) {
			span.SetTag("http.status_code", statusCode)
			if statusCode >= 500 {
				span.SetTag("error", true)
			}
		}
		defer span.Finish()
		r = r.WithContext(ctx)
	}
	t.handler(wrapper, r)
}

// telemetryWriterWrapper wraps http.ResponseWriter to capture response codes for telemetry and tracing.
type telemetryWriterWrapper struct {
	http.ResponseWriter
	handlerName string
	startTime   time.Time
	setSpanTags func(int) // non-nil only when tracing is enabled
}

func (w *telemetryWriterWrapper) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	forwarded := w.Header().Get(respForwarded)
	if forwarded == "" {
		forwarded = "false"
	}

	apiElapsed.Observe(time.Since(w.startTime).Seconds(), w.handlerName, strconv.Itoa(statusCode), forwarded)
	apiRequests.Inc(w.handlerName, strconv.Itoa(statusCode), forwarded)

	if w.setSpanTags != nil {
		w.setSpanTags(statusCode)
	}
}
