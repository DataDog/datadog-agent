// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/propagation"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// LifecycleProcessor is a InvocationProcessor implementation
type LifecycleProcessor struct {
	ExtraTags            *serverlessLog.Tags
	ProcessTrace         func(p *api.Payload)
	Demux                aggregator.Demultiplexer
	DetectLambdaLibrary  func() bool
	InferredSpansEnabled bool
	SubProcessor         InvocationSubProcessor
	Extractor            propagation.Extractor

	requestHandler *RequestHandler
	serviceName    string
}

// RequestHandler is the struct that stores information about the trace,
// inferred span, and tags about the current invocation
// inferred spans may contain a secondary inferred span in certain cases like SNS from SQS
type RequestHandler struct {
	executionInfo  *ExecutionStartInfo
	event          interface{}
	inferredSpans  [2]*inferredspan.InferredSpan
	triggerTags    map[string]string
	triggerMetrics map[string]float64
}

// SetMetaTag sets a meta span tag. A meta tag is a tag whose value type is string.
func (r *RequestHandler) SetMetaTag(tag string, value string) {
	panic("not called")
}

// GetMetaTag returns the meta span tag value if it exists.
func (r *RequestHandler) GetMetaTag(tag string) (value string, exists bool) {
	panic("not called")
}

// SetMetricsTag sets a metrics span tag. A metrics tag is a tag whose value type is float64.
func (r *RequestHandler) SetMetricsTag(tag string, value float64) {
	panic("not called")
}

// Event returns the invocation event parsed by the LifecycleProcessor. It is nil if the event type is not supported
// yet. The actual event type can be figured out thanks to a Go type switch on the event types of the package
// github.com/aws/aws-lambda-go/events
func (r *RequestHandler) Event() interface{} {
	panic("not called")
}

// SetSamplingPriority sets the trace priority
func (r *RequestHandler) SetSamplingPriority(priority sampler.SamplingPriority) {
	panic("not called")
}

// OnInvokeStart is the hook triggered when an invocation has started
func (lp *LifecycleProcessor) OnInvokeStart(startDetails *InvocationStartDetails) {
	panic("not called")
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (lp *LifecycleProcessor) OnInvokeEnd(endDetails *InvocationEndDetails) {
	panic("not called")
}

// GetTags returns the tagset of the currently executing lambda function
func (lp *LifecycleProcessor) GetTags() map[string]string {
	panic("not called")
}

// GetExecutionInfo returns the trace and payload information of
// the currently executing lambda function
func (lp *LifecycleProcessor) GetExecutionInfo() *ExecutionStartInfo {
	panic("not called")
}

// GetInferredSpan returns the generated inferred span of the
// currently executing lambda function
func (lp *LifecycleProcessor) GetInferredSpan() *inferredspan.InferredSpan {
	panic("not called")
}

func (lp *LifecycleProcessor) getInferredSpanStart() time.Time {
	panic("not called")
}

// GetServiceName returns the value stored in the environment variable
// DD_SERVICE. Also assigned into `lp.serviceName` if not previously set
func (lp *LifecycleProcessor) GetServiceName() string {
	panic("not called")
}

// NewRequest initializes basic information about the current request
// on the LifecycleProcessor
func (lp *LifecycleProcessor) newRequest(lambdaPayloadString []byte, startTime time.Time) {
	panic("not called")
}

func (lp *LifecycleProcessor) addTags(tagSet map[string]string) {
	panic("not called")
}

func (lp *LifecycleProcessor) addTag(key string, value string) {
	panic("not called")
}

// Sets the parent and span IDs when multiple inferred spans are necessary.
// Inferred spans of index 1 are generally sent inside of inferred span index 0.
// Like an SNS event inside an SQS message, and the parenting order is essential.
func (lp *LifecycleProcessor) setParentIDForMultipleInferredSpans() {
	panic("not called")
}
