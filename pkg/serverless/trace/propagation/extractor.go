// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package propagation

import (
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	defaultSamplingPriority sampler.SamplingPriority = sampler.PriorityNone

	ddTraceIDHeader          = "x-datadog-trace-id"
	ddParentIDHeader         = "x-datadog-parent-id"
	ddSpanIDHeader           = "x-datadog-span-id"
	ddSamplingPriorityHeader = "x-datadog-sampling-priority"
	ddInvocationErrorHeader  = "x-datadog-invocation-error"
)

var (
	errorUnsupportedExtractionType = errors.New("Unsupported event type for trace context extraction")
	errorNoContextFound            = errors.New("No trace context found")
	errorNoSQSRecordFound          = errors.New("No sqs message records found for trace context extraction")
	errorNoSNSRecordFound          = errors.New("No sns message records found for trace context extraction")
	errorNoTraceIDFound            = errors.New("No trace ID found")
	errorNoParentIDFound           = errors.New("No parent ID found")
)

// Extractor inserts trace context into and extracts trace context out of
// different types.
type Extractor struct {
	propagator tracer.Propagator
}

// TraceContext stores the propagated trace context values.
type TraceContext struct {
	TraceID          uint64
	ParentID         uint64
	SamplingPriority sampler.SamplingPriority
}

// TraceContextExtended stores the propagated trace context values plus other
// non-standard header values.
type TraceContextExtended struct {
	*TraceContext
	SpanID          uint64
	InvocationError bool
}

// Extract looks in the given events one by one and returns once a proper trace
// context is found.
func (e Extractor) Extract(events ...interface{}) (*TraceContext, error) {
	panic("not called")
}

// extract uses dd-trace-go's Propagator type to extract trace context from the
// given event.
func (e Extractor) extract(event interface{}) (*TraceContext, error) {
	panic("not called")
}

// ExtractFromLayer is used for extracting context from the request headers
// sent from a tracing layer. Currently, only datadog style headers are
// extracted. If a trace id or parent id are not found, then the embedded
// *TraceContext will be nil.
func (e Extractor) ExtractFromLayer(hdr http.Header) *TraceContextExtended {
	panic("not called")
}

func (e Extractor) extractTraceContextFromLayer(hdr http.Header) (*TraceContext, error) {
	panic("not called")
}

// InjectToLayer is used for injecting context into the response headers sent
// to a tracing layer. Currently, only datadog style headers are injected.
func (e Extractor) InjectToLayer(tc *TraceContext, hdr http.Header) {
	panic("not called")
}

// getSamplingPriority searches the given ddtrace.SpanContext for sampling
// priority. Note that not all versions of ddtrace export the SamplingPriority
// method, therefore the interface check is required.
func getSamplingPriority(sc ddtrace.SpanContext) (priority sampler.SamplingPriority) {
	panic("not called")
}

// convertStrToUint64 converts a given string to uint64 optionally returning an
// error.
func convertStrToUint64(s string) (uint64, error) {
	panic("not called")
}
