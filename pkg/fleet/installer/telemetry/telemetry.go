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

	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	envTraceID          = "DATADOG_TRACE_ID"
	envParentID         = "DATADOG_PARENT_ID"
	envService          = "DATADOG_SERVICE"
	envSamplingPriority = "DATADOG_SAMPLING_PRIORITY"
	telemetrySubdomain  = "instrumentation-telemetry-intake"
)

// Telemetry handles the telemetry for fleet components.
type Telemetry struct {
	telemetryClient *client
	done            chan struct{}
	flushed         chan struct{}

	env     string
	service string
}

// NewTelemetry creates a new telemetry instance
func NewTelemetry(client *http.Client, apiKey string, site string, service string) *Telemetry {
	t := newTelemetry(client, apiKey, site, service)
	t.Start()
	return t
}

func newTelemetry(client *http.Client, apiKey string, site string, service string) *Telemetry {
	if site == "" {
		site = "datadoghq.com"
	}
	e := &endpoint{
		Host:   fmt.Sprintf("https://%s.%s", telemetrySubdomain, strings.TrimSpace(site)),
		APIKey: apiKey,
	}
	env := "prod"
	if site == "datad0g.com" {
		env = "staging"
	}

	return &Telemetry{
		telemetryClient: newClient(client, []*endpoint{e}, service, site == "datad0g.com"),
		done:            make(chan struct{}),
		flushed:         make(chan struct{}),
		env:             env,
		service:         service,
	}
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

func (t *Telemetry) extractCompletedSpans() traces {
	spans := globalTracer.flushCompletedSpans()
	if len(spans) == 0 {
		return traces{}
	}
	ts := make(map[uint64][]*span)
	for _, span := range spans {
		if span.span.Service == "" {
			span.span.Service = t.service
		}
		span.span.Meta["env"] = t.env
		span.span.Meta["version"] = version.AgentVersion
		if _, ok := span.span.Metrics["_sampling_priority_v1"]; !ok {
			span.span.Metrics["_sampling_priority_v1"] = 2
		}
		ts[span.span.TraceID] = append(ts[span.span.TraceID], &span.span)
	}
	tracesArray := make([]trace, 0, len(ts))
	for _, t := range ts {
		tracesArray = append(tracesArray, trace(t))
	}
	return traces(tracesArray)
}

func (t *Telemetry) sendCompletedSpans() {
	tracesArray := t.extractCompletedSpans()
	if len(tracesArray) == 0 {
		return
	}
	t.telemetryClient.SendTraces(tracesArray)
}

// SpanFromContext returns the span from the context if available.
func SpanFromContext(ctx context.Context) (*Span, bool) {
	spanIDs, ok := getSpanIDsFromContext(ctx)
	if !ok {
		return nil, false
	}
	return globalTracer.getSpan(spanIDs.spanID)
}

// WithService sets the service name on the context. Spans created from this context
// (and child contexts) inherit the service unless overridden by another WithService call.
func WithService(ctx context.Context, service string) context.Context {
	return context.WithValue(ctx, serviceKey, service)
}

// WithSamplingPriority sets the sampling priority on the context. Spans created from
// this context (and child contexts) inherit the priority unless overridden by another
// WithSamplingPriority call. When unset, a default of 2 (FORCE_KEEP) is stamped at flush.
func WithSamplingPriority(ctx context.Context, priority int) context.Context {
	return context.WithValue(ctx, samplingPriorityKey, priority)
}

func getServiceFromContext(ctx context.Context) (string, bool) {
	service, ok := ctx.Value(serviceKey).(string)
	return service, ok
}

func getSamplingPriorityFromContext(ctx context.Context) (*int, bool) {
	priority, ok := ctx.Value(samplingPriorityKey).(int)
	if !ok {
		return nil, false
	}
	return &priority, true
}

// StartSpanFromEnv starts a span using the environment variables to find the parent span.
// It also reads DATADOG_SERVICE and DATADOG_SAMPLING_PRIORITY from the environment and
// applies them to the context so the created span (and its children) inherit them.
func StartSpanFromEnv(ctx context.Context, operationName string) (*Span, context.Context) {
	if service, ok := os.LookupEnv(envService); ok {
		ctx = WithService(ctx, service)
	}
	if priorityStr, ok := os.LookupEnv(envSamplingPriority); ok {
		if priority, err := strconv.Atoi(priorityStr); err == nil {
			ctx = WithSamplingPriority(ctx, priority)
		}
	}
	traceID, parentID := extractIDsFromEnv()
	return StartSpanFromIDs(ctx, operationName, traceID, parentID)
}

func extractIDsFromEnv() (string, string) {
	parentID, ok := os.LookupEnv(envParentID)
	if !ok {
		return "0", "0"
	}
	traceID, ok := os.LookupEnv(envTraceID)
	if !ok {
		return "0", "0"
	}
	return traceID, parentID
}

func converIDsToUint64(traceID, parentID string) (uint64, uint64) {
	traceIDInt, err := strconv.ParseUint(traceID, 10, 64)
	if err != nil {
		return 0, 0
	}
	parentIDInt, err := strconv.ParseUint(parentID, 10, 64)
	if err != nil {
		return 0, 0
	}
	return traceIDInt, parentIDInt
}

// StartSpanFromIDs starts a span using the trace and parent IDs provided as
// decimal strings. Malformed inputs yield a fresh top-level trace.
func StartSpanFromIDs(ctx context.Context, operationName, traceID, parentID string) (*Span, context.Context) {
	traceIDInt, parentIDInt := converIDsToUint64(traceID, parentID)
	return StartSpanFromUint64IDs(ctx, operationName, traceIDInt, parentIDInt)
}

// StartSpanFromUint64IDs starts a span using the trace and parent IDs provided
// as uint64. The created span is marked top-level: it is the entry point of
// this process's participation in the trace.
func StartSpanFromUint64IDs(ctx context.Context, operationName string, traceID, parentID uint64) (*Span, context.Context) {
	span, ctx := startSpanFromIDs(ctx, operationName, traceID, parentID)
	span.SetTopLevel()
	return span, ctx
}

func startSpanFromIDs(ctx context.Context, operationName string, traceID, parentID uint64) (*Span, context.Context) {
	service, _ := getServiceFromContext(ctx)
	samplingPriority, _ := getSamplingPriorityFromContext(ctx)
	s := newSpan(operationName, parentID, traceID, service, samplingPriority)
	ctx = setSpanIDsInContext(ctx, s)
	return s, ctx
}

// StartSpanFromContext starts a span using the context to find the parent span.
func StartSpanFromContext(ctx context.Context, operationName string) (*Span, context.Context) {
	spanIDs, _ := getSpanIDsFromContext(ctx)
	return startSpanFromIDs(ctx, operationName, spanIDs.traceID, spanIDs.spanID)
}

// EnvFromContext returns the environment variables for the context.
// Service and sampling priority are included when set on the context so child
// processes can inherit them via StartSpanFromEnv.
func EnvFromContext(ctx context.Context) []string {
	sIDs, ok := getSpanIDsFromContext(ctx)
	if !ok {
		return []string{}
	}
	env := []string{
		fmt.Sprintf("%s=%s", envTraceID, strconv.FormatUint(sIDs.traceID, 10)),
		fmt.Sprintf("%s=%s", envParentID, strconv.FormatUint(sIDs.spanID, 10)),
	}
	if service, ok := getServiceFromContext(ctx); ok && service != "" {
		env = append(env, fmt.Sprintf("%s=%s", envService, service))
	}
	if priority, ok := getSamplingPriorityFromContext(ctx); ok {
		env = append(env, fmt.Sprintf("%s=%d", envSamplingPriority, *priority))
	}
	return env
}
