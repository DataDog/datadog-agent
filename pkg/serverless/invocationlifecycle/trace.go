// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"net/http"
	"regexp"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
)

var /* const */ runtimeRegex = regexp.MustCompile(`^(dotnet|go|java|ruby)(\d+(\.\d+)*|\d+(\.x))$`)

// ExecutionStartInfo is saved information from when an execution span was started
type ExecutionStartInfo struct {
	startTime        time.Time
	TraceID          uint64
	SpanID           uint64
	parentID         uint64
	requestPayload   []byte
	SamplingPriority sampler.SamplingPriority
}

// startExecutionSpan records information from the start of the invocation.
// It should be called at the start of the invocation.
func (lp *LifecycleProcessor) startExecutionSpan(event interface{}, rawPayload []byte, startDetails *InvocationStartDetails) {
	panic("not called")
}

// endExecutionSpan builds the function execution span and sends it to the intake.
// It should be called at the end of the invocation.
func (lp *LifecycleProcessor) endExecutionSpan(endDetails *InvocationEndDetails) *pb.Span {
	panic("not called")
}

// completeInferredSpan finishes the inferred span and passes it
// as an API payload to be processed by the trace agent
func (lp *LifecycleProcessor) completeInferredSpan(inferredSpan *inferredspan.InferredSpan, endTime time.Time, isError bool) *pb.Span {
	panic("not called")
}

func (lp *LifecycleProcessor) processTrace(spans []*pb.Span) {
	panic("not called")
}

// ParseLambdaPayload removes extra data sent by the proxy that surrounds
// a JSON payload. For example, for `a5a{"event":"aws_lambda"...}0` it would remove
// a5a at the front and 0 at the end, and just leave a correct JSON payload.
func ParseLambdaPayload(rawPayload []byte) []byte {
	panic("not called")
}

func convertStrToUnit64(s string) (uint64, error) {
	panic("not called")
}

// InjectContext injects the context
func InjectContext(executionContext *ExecutionStartInfo, headers http.Header) {
	panic("not called")
}

// InjectSpanID injects the spanId
func InjectSpanID(executionContext *ExecutionStartInfo, headers http.Header) {
	panic("not called")
}

func capturePayloadAsTags(value interface{}, targetSpan *pb.Span, key string, depth int, maxDepth int) {
	panic("not called")
}

func convertJSONToString(payloadJSON interface{}) string {
	panic("not called")
}
