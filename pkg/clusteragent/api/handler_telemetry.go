// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var apiRequests = telemetry.NewCounterWithOpts("", "api_requests",
	[]string{"handler", "status", "forwarded"}, "Counter of requests made to the cluster agent API.",
	telemetry.Options{NoDoubleUnderscoreSep: true})

type TelemetryHandler struct {
	handlerName string
	handler     func(w http.ResponseWriter, r *http.Request)
}

func WithTelemetryWrapper(handlerName string, handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	th := TelemetryHandler{
		handlerName: handlerName,
		handler:     handler,
	}
	return th.handle
}

func (t *TelemetryHandler) handle(w http.ResponseWriter, r *http.Request) {
	t.handler(&telemetryWriterWrapper{ResponseWriter: w, handlerName: t.handlerName}, r)
}

// Could be made generic, overwite http.ResponseWriter/WriteHeader
type telemetryWriterWrapper struct {
	http.ResponseWriter
	handlerName string
}

func (w *telemetryWriterWrapper) WriteHeader(statusCode int) {
	forwarded := w.Header().Get(respForwarded)
	if forwarded == "" {
		forwarded = "false"
	}

	apiRequests.Inc(w.handlerName, strconv.Itoa(statusCode), forwarded)
}
