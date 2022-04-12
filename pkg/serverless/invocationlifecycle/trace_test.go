// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestStartExecutionSpanWithoutPayload(t *testing.T) {
	defer reset()
	startTime := time.Now()
	var span inferredspan.InferredSpan = inferredspan.GenerateInferredSpan(time.Now())
	startExecutionSpan(startTime, "", span)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.NotEqual(t, 0, currentExecutionInfo.traceID)
	assert.NotEqual(t, 0, currentExecutionInfo.spanID)
}

func TestStartExecutionSpanWithPayload(t *testing.T) {
	defer reset()
	testString := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`
	startTime := time.Now()
	var span inferredspan.InferredSpan = inferredspan.GenerateInferredSpan(time.Now())
	startExecutionSpan(startTime, testString, span)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(5736943178450432258), currentExecutionInfo.traceID)
	assert.Equal(t, uint64(1480558859903409531), currentExecutionInfo.parentID)
	assert.NotEqual(t, 0, currentExecutionInfo.spanID)
}

func TestStartExecutionSpanWithPayloadAndInvalidIDs(t *testing.T) {
	defer reset()
	invalidTestString := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"INVALID","x-datadog-sampling-priority":"1","x-datadog-trace-id":"INVALID"}}0`
	startTime := time.Now()
	var span inferredspan.InferredSpan = inferredspan.GenerateInferredSpan(time.Now())
	startExecutionSpan(startTime, invalidTestString, span)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.NotEqual(t, 9, currentExecutionInfo.traceID)
	assert.Equal(t, uint64(0), currentExecutionInfo.parentID)
	assert.NotEqual(t, 0, currentExecutionInfo.spanID)
}

func TestStartExecutionSpanWithNoHeadersAndInferredSpan(t *testing.T) {
	defer os.Unsetenv("DD_TRACE_ENABLED")
	os.Setenv("DD_TRACE_ENABLED", "true")
	defer reset()
	defer os.Unsetenv("DD_TRACE_ENABLED")
	os.Setenv("DD_TRACE_MANAGED_SERVICES", "true")
	defer reset()
	testString := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET"}0`
	startTime := time.Now()
	var span inferredspan.InferredSpan
	span.Span = &pb.Span{}
	span.Span.TraceID = 2350923428932752492
	span.Span.SpanID = 1304592378509342580
	startExecutionSpan(startTime, testString, span)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(2350923428932752492), currentExecutionInfo.traceID)
	assert.Equal(t, uint64(1304592378509342580), currentExecutionInfo.parentID)
	assert.NotEqual(t, 0, currentExecutionInfo.spanID)
}

func TestStartExecutionSpanWithHeadersAndInferredSpan(t *testing.T) {
	defer os.Unsetenv("DD_TRACE_ENABLED")
	os.Setenv("DD_TRACE_ENABLED", "true")
	defer reset()
	defer os.Unsetenv("DD_TRACE_ENABLED")
	os.Setenv("DD_TRACE_MANAGED_SERVICES", "true")
	defer reset()
	testString := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`
	startTime := time.Now()
	var span inferredspan.InferredSpan = inferredspan.GenerateInferredSpan(time.Now())
	span.Span.SpanID = 1304592378509342580
	startExecutionSpan(startTime, testString, span)
	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.Equal(t, uint64(5736943178450432258), currentExecutionInfo.traceID)
	assert.Equal(t, uint64(1304592378509342580), currentExecutionInfo.parentID)
	assert.Equal(t, uint64(5736943178450432258), span.Span.TraceID)
	assert.Equal(t, uint64(1480558859903409531), span.Span.ParentID)
	assert.NotEqual(t, 0, currentExecutionInfo.spanID)
}
func TestEndExecutionSpanWithNoError(t *testing.T) {
	defer os.Unsetenv(functionNameEnvVar)
	os.Setenv(functionNameEnvVar, "TestFunction")
	defer reset()
	testString := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`
	startTime := time.Now()
	var span inferredspan.InferredSpan = inferredspan.GenerateInferredSpan(time.Now())
	startExecutionSpan(startTime, testString, span)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	isError := false
	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	endExecutionSpan(mockProcessTrace, "test-request-id", endTime, isError)
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, "test-request-id", executionSpan.Meta["request_id"])
	assert.Equal(t, currentExecutionInfo.traceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.spanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)

}

func TestEndExecutionSpanWithError(t *testing.T) {
	defer os.Unsetenv(functionNameEnvVar)
	os.Setenv(functionNameEnvVar, "TestFunction")
	defer reset()
	testString := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`
	startTime := time.Now()
	var span inferredspan.InferredSpan = inferredspan.GenerateInferredSpan(time.Now())
	startExecutionSpan(startTime, testString, span)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	isError := true
	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	endExecutionSpan(mockProcessTrace, "test-request-id", endTime, isError)
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, executionSpan.Error, int32(1))
}

func TestConvertRawPayloadWithHeaders(t *testing.T) {

	s := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`

	expectedPayload := invocationPayload{}
	expectedPayload.Headers = map[string]string{"Accept": "*/*", "Accept-Encoding": "gzip", "x-datadog-parent-id": "1480558859903409531", "x-datadog-sampling-priority": "1", "x-datadog-trace-id": "5736943178450432258"}

	p := convertRawPayload(s)

	assert.Equal(t, p, expectedPayload)
}

func TestConvertRawPayloadWithOutHeaders(t *testing.T) {

	s := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET"}0`

	expectedPayload := invocationPayload{}

	p := convertRawPayload(s)

	assert.Equal(t, p, expectedPayload)
}

func TestConvertStrToUnit64WithValidInt(t *testing.T) {
	s := "1480558859903409531"
	num, err := convertStrToUint64(s)

	i, _ := strconv.ParseUint(s, 0, 64)
	assert.Equal(t, num, i)
	assert.Nil(t, err)
}

func TestConvertStrToUnit64WithInvalidInt(t *testing.T) {
	s := "INVALID"
	_, err := convertStrToUint64(s)

	assert.NotNil(t, err)
}

func reset() {
	currentExecutionInfo = executionStartInfo{}
}
