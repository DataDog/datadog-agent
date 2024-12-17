// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/internaltelemetry"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	envTraceID         = "DATADOG_TRACE_ID"
	envParentID        = "DATADOG_PARENT_ID"
	telemetrySubdomain = "instrumentation-telemetry-intake"
)

// Telemetry handles the telemetry for fleet components.
type Telemetry struct {
	telemetryClient internaltelemetry.Client
	done            chan struct{}
	flushed         chan struct{}

	env     string
	service string
}

// NewTelemetry creates a new telemetry instance
func NewTelemetry(client *http.Client, apiKey string, site string, service string) *Telemetry {
	endpoint := &internaltelemetry.Endpoint{
		Host:   fmt.Sprintf("https://%s.%s", telemetrySubdomain, strings.TrimSpace(site)),
		APIKey: apiKey,
	}
	env := "prod"
	if site == "datad0g.com" {
		env = "staging"
	}

	t := &Telemetry{
		telemetryClient: internaltelemetry.NewClient(client, []*internaltelemetry.Endpoint{endpoint}, service, site == "datad0g.com"),
		done:            make(chan struct{}),
		flushed:         make(chan struct{}),
		env:             env,
		service:         service,
	}
	t.Start()
	return t
}

// Start starts the telemetry
func (t *Telemetry) Start() {
	ticker := time.Tick(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker:
				t.sendCompletedSpans()
			case <-t.done:
				t.sendCompletedSpans()
				close(t.flushed)
				return
			}
		}
	}()
}

// Stop stops the telemetry
func (t *Telemetry) Stop() {
	close(t.done)
	<-t.flushed
}

func (t *Telemetry) sendCompletedSpans() {
	spans := globalTracer.flushCompletedSpans()
	if len(spans) == 0 {
		return
	}
	traces := make(map[uint64][]*internaltelemetry.Span)
	for _, span := range spans {
		span.span.Service = t.service
		span.span.Meta["env"] = t.env
		span.span.Meta["version"] = version.AgentVersion
		span.span.Metrics["_sampling_priority_v1"] = 2
		traces[span.span.TraceID] = append(traces[span.span.TraceID], &span.span)
	}
	tracesArray := make([]internaltelemetry.Trace, 0, len(traces))
	for _, trace := range traces {
		tracesArray = append(tracesArray, internaltelemetry.Trace(trace))
	}
	t.telemetryClient.SendTraces(internaltelemetry.Traces(tracesArray))
}

// SpanFromContext returns the span from the context if available.
func SpanFromContext(ctx context.Context) (*Span, bool) {
	spanIDs, ok := getSpanIDsFromContext(ctx)
	if !ok {
		return nil, false
	}
	return globalTracer.getSpan(spanIDs.spanID)
}

// StartSpanFromEnv starts a span using the environment variables to find the parent span.
func StartSpanFromEnv(ctx context.Context, operationName string) (*Span, context.Context) {
	traceID, ok := os.LookupEnv(envTraceID)
	if !ok {
		traceID = "0"
	}
	parentID, ok := os.LookupEnv(envParentID)
	if !ok {
		parentID = "0"
	}
	return StartSpanFromIDs(ctx, operationName, traceID, parentID)
}

// StartSpanFromIDs starts a span using the trace and parent
// IDs provided.
func StartSpanFromIDs(ctx context.Context, operationName, traceID, parentID string) (*Span, context.Context) {
	traceIDInt, err := strconv.ParseUint(traceID, 10, 64)
	if err != nil {
		traceIDInt = 0
	}
	parentIDInt, err := strconv.ParseUint(parentID, 10, 64)
	if err != nil {
		parentIDInt = 0
	}
	span, ctx := startSpanFromIDs(ctx, operationName, traceIDInt, parentIDInt)
	span.SetTopLevel()
	return span, ctx
}

func startSpanFromIDs(ctx context.Context, operationName string, traceID, parentID uint64) (*Span, context.Context) {
	s := newSpan(operationName, parentID, traceID)
	ctx = setSpanIDsInContext(ctx, s)
	return s, ctx
}

// StartSpanFromContext starts a span using the context to find the parent span.
func StartSpanFromContext(ctx context.Context, operationName string) (*Span, context.Context) {
	spanIDs, _ := getSpanIDsFromContext(ctx)
	return startSpanFromIDs(ctx, operationName, spanIDs.traceID, spanIDs.spanID)
}

// EnvFromContext returns the environment variables for the context.
func EnvFromContext(ctx context.Context) []string {
	sIDs, ok := getSpanIDsFromContext(ctx)
	if !ok {
		return []string{}
	}
	return []string{
		fmt.Sprintf("%s=%s", envTraceID, strconv.FormatUint(sIDs.traceID, 10)),
		fmt.Sprintf("%s=%s", envParentID, strconv.FormatUint(sIDs.spanID, 10)),
	}
}
