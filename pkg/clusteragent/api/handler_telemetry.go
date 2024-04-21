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

	// TLSHandshakeErrors counts the number of requests dropped due to TLS handshake errors
	TLSHandshakeErrors = telemetry.NewCounterWithOpts("", "api_server_tls_handshake_errors",
		[]string{}, "Number of tls handshake errors from cluster-agent http api server.",
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
	t.handler(&telemetryWriterWrapper{ResponseWriter: w, handlerName: t.handlerName, startTime: time.Now()}, r)
}

// Could be made generic, overwite http.ResponseWriter/WriteHeader
type telemetryWriterWrapper struct {
	http.ResponseWriter
	handlerName string
	startTime   time.Time
}

func (w *telemetryWriterWrapper) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	forwarded := w.Header().Get(respForwarded)
	if forwarded == "" {
		forwarded = "false"
	}

	apiElapsed.Observe(time.Since(w.startTime).Seconds(), w.handlerName, strconv.Itoa(statusCode), forwarded)
	apiRequests.Inc(w.handlerName, strconv.Itoa(statusCode), forwarded)
}
