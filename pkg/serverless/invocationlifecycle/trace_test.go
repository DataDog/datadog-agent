// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

var timeNow = func() time.Time {
	return time.Date(2022, time.December, 27, 0, 0, 0, 0, time.UTC)
}

func newExecutionContextWithTime() *ExecutionStartInfo {
	return &ExecutionStartInfo{
		startTime: timeNow(),
	}
}

func TestConvertStrToUnit64Error(t *testing.T) {
	value, err := convertStrToUnit64("invalid")
	assert.NotNil(t, err)
	assert.Equal(t, uint64(0), value)
}

func TestConvertStrToUnit64Success(t *testing.T) {
	value, err := convertStrToUnit64("1234")
	assert.Nil(t, err)
	assert.Equal(t, uint64(1234), value)
}

func TestGetSamplingPriority(t *testing.T) {
	assert.Equal(t, sampler.PriorityNone, getSamplingPriority("xxx", "yyy"))
	assert.Equal(t, sampler.PriorityUserDrop, getSamplingPriority("-1", "yyy"))
	assert.Equal(t, sampler.PriorityAutoKeep, getSamplingPriority("1", "yyy"))
	assert.Equal(t, sampler.PriorityUserKeep, getSamplingPriority("2", "yyy"))
	assert.Equal(t, sampler.PriorityUserDrop, getSamplingPriority("-1", "1"))
	assert.Equal(t, sampler.PriorityAutoKeep, getSamplingPriority("1", "-1"))
	assert.Equal(t, sampler.PriorityUserKeep, getSamplingPriority("2", "1"))
	assert.Equal(t, sampler.PriorityUserDrop, getSamplingPriority("xxx", "-1"))
	assert.Equal(t, sampler.PriorityAutoKeep, getSamplingPriority("xxx", "1"))
	assert.Equal(t, sampler.PriorityUserKeep, getSamplingPriority("xxx", "2"))
}

func TestInjectContextNoContext(t *testing.T) {
	currentExecutionInfo := newExecutionContextWithTime()
	InjectContext(currentExecutionInfo, nil)
	assert.Equal(t, uint64(0), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(0), currentExecutionInfo.parentID)
	assert.Equal(t, sampler.SamplingPriority(0), currentExecutionInfo.SamplingPriority)
}

func TestInjectContextWithContext(t *testing.T) {
	currentExecutionInfo := newExecutionContextWithTime()
	httpHeaders := http.Header{}
	httpHeaders.Set("x-datadog-trace-id", "1234")
	httpHeaders.Set("x-datadog-parent-id", "5678")
	httpHeaders.Set("x-datadog-sampling-priority", "2")
	InjectContext(currentExecutionInfo, httpHeaders)
	assert.Equal(t, uint64(1234), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(5678), currentExecutionInfo.parentID)
	assert.Equal(t, sampler.PriorityUserKeep, currentExecutionInfo.SamplingPriority)
}

func TestInjectSpanIDNoContext(t *testing.T) {
	currentExecutionInfo := newExecutionContextWithTime()
	InjectSpanID(currentExecutionInfo, nil)
	assert.Equal(t, uint64(0), currentExecutionInfo.SpanID)
}

func TestInjectSpanIDWithContext(t *testing.T) {
	currentExecutionInfo := newExecutionContextWithTime()
	httpHeaders := http.Header{}
	httpHeaders.Set("x-datadog-span-id", "1234")
	InjectSpanID(currentExecutionInfo, httpHeaders)
	assert.Equal(t, uint64(1234), currentExecutionInfo.SpanID)
}

func TestStartExecutionSpanWithoutPayload(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	startDetails := &InvocationStartDetails{
		StartTime:          timeNow(),
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	startExecutionSpan(currentExecutionInfo, nil, []byte(""), startDetails, false)
	assert.Equal(t, currentExecutionInfo.startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(0), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(0), currentExecutionInfo.SpanID)
	assert.Equal(t, sampler.PriorityNone, currentExecutionInfo.SamplingPriority)
}

func TestStartExecutionSpanWithPayload(t *testing.T) {
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"-1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := timeNow()
	currentExecutionInfo := &ExecutionStartInfo{}
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	startExecutionSpan(currentExecutionInfo, nil, []byte(testString), startDetails, false)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(5736943178450432258), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(1480558859903409531), currentExecutionInfo.parentID)
	assert.Equal(t, sampler.PriorityUserDrop, currentExecutionInfo.SamplingPriority)
	assert.NotEqual(t, 0, currentExecutionInfo.SpanID)
}

func TestStartExecutionSpanWithPayloadAndLambdaContextHeaders(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET"}`
	lambdaInvokeContext := LambdaInvokeEventHeaders{
		TraceID:          "5736943178450432258",
		ParentID:         "1480558859903409531",
		SamplingPriority: "1",
	}

	startTime := timeNow()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: lambdaInvokeContext,
	}
	startExecutionSpan(currentExecutionInfo, nil, []byte(testString), startDetails, false)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(5736943178450432258), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(1480558859903409531), currentExecutionInfo.parentID)
	assert.Equal(t, sampler.PriorityAutoKeep, currentExecutionInfo.SamplingPriority)
	assert.NotEqual(t, 0, currentExecutionInfo.SpanID)
}

func TestStartExecutionSpanWithPayloadAndInvalidIDs(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	invalidTestString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"INVALID","x-datadog-sampling-priority":"-1","x-datadog-trace-id":"INVALID"}}`
	startTime := timeNow()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	startExecutionSpan(currentExecutionInfo, nil, []byte(invalidTestString), startDetails, false)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.NotEqual(t, 9, currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(0), currentExecutionInfo.parentID)
	assert.Equal(t, sampler.PriorityUserDrop, currentExecutionInfo.SamplingPriority)
	assert.NotEqual(t, 0, currentExecutionInfo.SpanID)
}

func TestStartExecutionSpanWithNoHeadersAndInferredSpan(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET"}`
	startTime := timeNow()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	inferredSpan := &inferredspan.InferredSpan{}

	inferredSpan.Span = &pb.Span{
		TraceID: 2350923428932752492,
		SpanID:  1304592378509342580,
		Start:   startTime.UnixNano(),
	}
	startExecutionSpan(currentExecutionInfo, inferredSpan, []byte(testString), startDetails, true)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(2350923428932752492), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(1304592378509342580), currentExecutionInfo.parentID)
	assert.NotEqual(t, 0, currentExecutionInfo.SpanID)
}

func TestStartExecutionSpanWithHeadersAndInferredSpan(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := timeNow()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	inferredSpan := &inferredspan.InferredSpan{}
	inferredSpan.Span = &pb.Span{
		SpanID: 1304592378509342580,
		Start:  startTime.UnixNano(),
	}
	startExecutionSpan(currentExecutionInfo, inferredSpan, []byte(testString), startDetails, true)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(5736943178450432258), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(1304592378509342580), currentExecutionInfo.parentID)
	assert.Equal(t, sampler.SamplingPriority(1), currentExecutionInfo.SamplingPriority)
	assert.Equal(t, uint64(5736943178450432258), inferredSpan.Span.TraceID)
	assert.Equal(t, uint64(1480558859903409531), inferredSpan.Span.ParentID)

	assert.NotEqual(t, 0, currentExecutionInfo.SpanID)
}
func TestEndExecutionSpanWithNoError(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_CAPTURE_LAMBDA_PAYLOAD", "true")
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := time.Now()

	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	startExecutionSpan(currentExecutionInfo, nil, []byte(testString), startDetails, false)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
	}

	endExecutionSpan(currentExecutionInfo, make(map[string]string), nil, mockProcessTrace, endDetails)
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, "test-request-id", executionSpan.Meta["request_id"])
	assert.Equal(t, testString, executionSpan.Meta["function.request"])
	assert.Equal(t, `{"response":"test response payload"}`, executionSpan.Meta["function.response"])
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithInvalidCaptureLambdaPayloadValue(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_CAPTURE_LAMBDA_PAYLOAD", "INVALID_INPUT")
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := time.Now()
	currentExecutionInfo := &ExecutionStartInfo{}
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	startExecutionSpan(currentExecutionInfo, nil, []byte(testString), startDetails, false)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
	}

	endExecutionSpan(currentExecutionInfo, make(map[string]string), nil, mockProcessTrace, endDetails)
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, "test-request-id", executionSpan.Meta["request_id"])
	assert.NotContains(t, executionSpan.Meta, "function.request")
	assert.NotContains(t, executionSpan.Meta, "function.response")
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithError(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	t.Setenv(functionNameEnvVar, "TestFunction")
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := time.Now()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: LambdaInvokeEventHeaders{},
	}
	startExecutionSpan(currentExecutionInfo, nil, []byte(testString), startDetails, false)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            true,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{}`),
	}
	endExecutionSpan(currentExecutionInfo, make(map[string]string), nil, mockProcessTrace, endDetails)
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, executionSpan.Error, int32(1))
}

func TestConvertRawPayloadWithHeaders(t *testing.T) {

	s := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`

	expectedPayload := invocationPayload{}
	expectedPayload.Headers = map[string]string{"Accept": "*/*", "Accept-Encoding": "gzip", "x-datadog-parent-id": "1480558859903409531", "x-datadog-sampling-priority": "1", "x-datadog-trace-id": "5736943178450432258"}

	p := convertRawPayload([]byte(s))

	assert.Equal(t, p, expectedPayload)
}

func TestConvertRawPayloadWithOutHeaders(t *testing.T) {

	s := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET"}`

	expectedPayload := invocationPayload{}

	p := convertRawPayload([]byte(s))

	assert.Equal(t, p, expectedPayload)
}

func TestParseLambdaPayload(t *testing.T) {
	assert.Equal(t, []byte(""), parseLambdaPayload([]byte("")))
	assert.Equal(t, []byte("{}"), parseLambdaPayload([]byte("{}")))
	assert.Equal(t, []byte("{}"), parseLambdaPayload([]byte("a{}a")))
	assert.Equal(t, []byte("{a}"), parseLambdaPayload([]byte("{a}a")))
	assert.Equal(t, []byte("{a}"), parseLambdaPayload([]byte("a{a}")))
	assert.Equal(t, []byte("{a}"), parseLambdaPayload([]byte("}{a}a{")))
	assert.Equal(t, []byte("{}{}"), parseLambdaPayload([]byte("{}{}")))
	assert.Equal(t, []byte("{a}"), parseLambdaPayload([]byte("a{a}a")))
	assert.Equal(t, []byte("{"), parseLambdaPayload([]byte("{")))
	assert.Equal(t, []byte("}"), parseLambdaPayload([]byte("}")))
}
