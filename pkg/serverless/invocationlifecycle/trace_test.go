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

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
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

func TestInjectContextWith128BitTraceID(t *testing.T) {
	currentExecutionInfo := newExecutionContextWithTime()
	httpHeaders := http.Header{}
	httpHeaders.Set("x-datadog-trace-id", "1234")
	httpHeaders.Set("x-datadog-parent-id", "5678")
	httpHeaders.Set("x-datadog-sampling-priority", "2")
	httpHeaders.Set(TraceTagsHeader, Upper64BitsTag+"=1234567890abcdef")
	InjectContext(currentExecutionInfo, httpHeaders)
	assert.Equal(t, uint64(1234), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(5678), currentExecutionInfo.parentID)
	assert.Equal(t, sampler.PriorityUserKeep, currentExecutionInfo.SamplingPriority)
	assert.Equal(t, "1234567890abcdef", currentExecutionInfo.TraceIDUpper64Hex)
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

func TestStartExecutionSpan(t *testing.T) {
	eventWithoutCtx := events.APIGatewayV2HTTPRequest{}
	eventWithCtx := events.APIGatewayV2HTTPRequest{
		Headers: map[string]string{
			"x-datadog-trace-id":          "1",
			"x-datadog-parent-id":         "1",
			"x-datadog-sampling-priority": "1",
			"traceparent":                 "00-00000000000000000000000000000004-0000000000000004-01",
		},
	}
	payloadWithoutCtx := []byte(`{"hello":"world"}`)
	payloadWithCtx := []byte(`{
		"hello": "world",
		"headers": {
			"x-datadog-trace-id": "2",
			"x-datadog-parent-id": "2",
			"x-datadog-sampling-priority": "2",
			"traceparent": "00-00000000000000000000000000000005-0000000000000005-01"
		}
	}`)
	reqHeadersWithoutCtx := http.Header{}
	reqHeadersWithCtx := http.Header{}
	reqHeadersWithCtx.Set("x-datadog-trace-id", "3")
	reqHeadersWithCtx.Set("x-datadog-parent-id", "3")
	reqHeadersWithCtx.Set("x-datadog-sampling-priority", "3")
	reqHeadersWithCtx.Set("traceparent", "00-00000000000000000000000000000006-0000000000000006-01")

	stepFunctionEvent := events.StepFunctionPayload{
		Execution: struct {
			ID           string
			RedriveCount uint16
		}{
			ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
			RedriveCount: 0,
		},
		State: struct {
			Name        string
			EnteredTime string
			RetryCount  uint16
		}{
			Name:        "agocsTest1",
			EnteredTime: "2024-07-30T20:46:20.824Z",
			RetryCount:  0,
		},
	}

	nestedStepFunctionEvent := events.NestedStepFunctionPayload{
		Payload: events.StepFunctionPayload{
			Execution: struct {
				ID           string
				RedriveCount uint16
			}{
				ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
				RedriveCount: 0,
			},
			State: struct {
				Name        string
				EnteredTime string
				RetryCount  uint16
			}{
				Name:        "agocsTest1",
				EnteredTime: "2024-07-30T20:46:20.824Z",
				RetryCount:  0,
			},
		},
		RootExecutionID:   "arn:aws:states:sa-east-1:425362996713:execution:invokeJavaLambda:4875aba4-ae31-4a4c-bf8a-63e9eee31dad",
		ServerlessVersion: "v1",
	}

	lambdaRootStepFunctionEvent := events.LambdaRootStepFunctionPayload{
		Payload: events.StepFunctionPayload{
			Execution: struct {
				ID           string
				RedriveCount uint16
			}{
				ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
				RedriveCount: 0,
			},
			State: struct {
				Name        string
				EnteredTime string
				RetryCount  uint16
			}{
				Name:        "agocsTest1",
				EnteredTime: "2024-07-30T20:46:20.824Z",
				RetryCount:  0,
			},
		},
		TraceID:           "5821803790426892636",
		TraceTags:         "_dd.p.dm=-0,_dd.p.tid=672a7cb100000000",
		ServerlessVersion: "v1",
	}

	testcases := []struct {
		name           string
		event          interface{}
		payload        []byte
		reqHeaders     http.Header
		infSpanEnabled bool
		propStyle      string
		expectCtx      *ExecutionStartInfo
	}{
		{
			name:       "eventWithoutCtx-payloadWithoutCtx-reqHeadersWithoutCtx-datadog",
			event:      eventWithoutCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          0,
				parentID:         0,
				SamplingPriority: sampler.PriorityNone,
			},
		},
		{
			name:       "eventWithCtx-payloadWithoutCtx-reqHeadersWithoutCtx-datadog",
			event:      eventWithCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          1,
				parentID:         1,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithoutCtx-payloadWithCtx-reqHeadersWithoutCtx-datadog",
			event:      eventWithoutCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          2,
				parentID:         2,
				SamplingPriority: sampler.SamplingPriority(2),
			},
		},
		{
			name:       "eventWithCtx-payloadWithCtx-reqHeadersWithoutCtx-datadog",
			event:      eventWithCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          1,
				parentID:         1,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithoutCtx-payloadWithoutCtx-reqHeadersWithCtx-datadog",
			event:      eventWithoutCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          3,
				parentID:         3,
				SamplingPriority: sampler.SamplingPriority(3),
			},
		},
		{
			name:       "eventWithCtx-payloadWithoutCtx-reqHeadersWithCtx-datadog",
			event:      eventWithCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          1,
				parentID:         1,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithoutCtx-payloadWithCtx-reqHeadersWithCtx-datadog",
			event:      eventWithoutCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          2,
				parentID:         2,
				SamplingPriority: sampler.SamplingPriority(2),
			},
		},
		{
			name:       "eventWithCtx-payloadWithCtx-reqHeadersWithCtx-datadog",
			event:      eventWithCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          1,
				parentID:         1,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithoutCtx-payloadWithoutCtx-reqHeadersWithoutCtx-tracecontext",
			event:      eventWithoutCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          0,
				parentID:         0,
				SamplingPriority: sampler.PriorityNone,
			},
		},
		{
			name:       "eventWithCtx-payloadWithoutCtx-reqHeadersWithoutCtx-tracecontext",
			event:      eventWithCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          4,
				parentID:         4,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithoutCtx-payloadWithCtx-reqHeadersWithoutCtx-tracecontext",
			event:      eventWithoutCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          5,
				parentID:         5,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithCtx-payloadWithCtx-reqHeadersWithoutCtx-tracecontext",
			event:      eventWithCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithoutCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          4,
				parentID:         4,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithoutCtx-payloadWithoutCtx-reqHeadersWithCtx-tracecontext",
			event:      eventWithoutCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          3,
				parentID:         3,
				SamplingPriority: sampler.SamplingPriority(3),
			},
		},
		{
			name:       "eventWithCtx-payloadWithoutCtx-reqHeadersWithCtx-tracecontext",
			event:      eventWithCtx,
			payload:    payloadWithoutCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          4,
				parentID:         4,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithoutCtx-payloadWithCtx-reqHeadersWithCtx-tracecontext",
			event:      eventWithoutCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          5,
				parentID:         5,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:       "eventWithCtx-payloadWithCtx-reqHeadersWithCtx-tracecontext",
			event:      eventWithCtx,
			payload:    payloadWithCtx,
			reqHeaders: reqHeadersWithCtx,
			propStyle:  "tracecontext",
			expectCtx: &ExecutionStartInfo{
				TraceID:          4,
				parentID:         4,
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:           "inferred-spans-enabled",
			event:          eventWithCtx,
			payload:        payloadWithCtx,
			reqHeaders:     reqHeadersWithCtx,
			infSpanEnabled: true,
			propStyle:      "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:          1,
				parentID:         123, // parent is inferred span
				SamplingPriority: sampler.SamplingPriority(1),
			},
		},
		{
			name:           "step function event",
			event:          stepFunctionEvent,
			payload:        payloadWithoutCtx,
			reqHeaders:     reqHeadersWithoutCtx,
			infSpanEnabled: false,
			propStyle:      "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:           5377636026938777059,
				TraceIDUpper64Hex: "6fb5c3a05c73dbfe",
				parentID:          8947638978974359093,
				SamplingPriority:  1,
			},
		},
		{
			name:           "nested step function event",
			event:          nestedStepFunctionEvent,
			payload:        payloadWithoutCtx,
			reqHeaders:     reqHeadersWithoutCtx,
			infSpanEnabled: false,
			propStyle:      "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:           1322229001489018110,
				TraceIDUpper64Hex: "579d19b3ee216ee9",
				parentID:          8947638978974359093,
				SamplingPriority:  1,
			},
		},
		{
			name:           "lambda root step function event",
			event:          lambdaRootStepFunctionEvent,
			payload:        payloadWithoutCtx,
			reqHeaders:     reqHeadersWithoutCtx,
			infSpanEnabled: false,
			propStyle:      "datadog",
			expectCtx: &ExecutionStartInfo{
				TraceID:           5821803790426892636,
				TraceIDUpper64Hex: "672a7cb100000000",
				parentID:          8947638978974359093,
				SamplingPriority:  1,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_TRACE_PROPAGATION_STYLE", tc.propStyle)
			startTime := time.Now()
			actualCtx := &ExecutionStartInfo{}
			inferredSpan := &inferredspan.InferredSpan{
				Span: &pb.Span{
					SpanID: 123,
					Start:  startTime.UnixNano() - 10,
				},
			}
			lp := &LifecycleProcessor{
				InferredSpansEnabled: tc.infSpanEnabled,
				requestHandler: &RequestHandler{
					executionInfo: actualCtx,
					inferredSpans: [2]*inferredspan.InferredSpan{inferredSpan},
					triggerTags:   make(map[string]string),
				},
			}
			startDetails := &InvocationStartDetails{
				StartTime:          startTime,
				InvokeEventHeaders: tc.reqHeaders,
			}

			lp.startExecutionSpan(tc.event, tc.payload, startDetails)

			assert := assert.New(t)
			assert.Equal(tc.payload, actualCtx.requestPayload)
			assert.Equal(startTime, actualCtx.startTime)

			// default values allow for assert.Equal comparison on the context
			actualCtx.requestPayload = nil
			actualCtx.startTime = time.Time{}
			assert.Equal(tc.expectCtx, actualCtx)

			if tc.infSpanEnabled {
				assert.Equal(tc.expectCtx.TraceID, inferredSpan.Span.TraceID)
				assert.Equal(tc.expectCtx.TraceID, inferredSpan.Span.ParentID)
			} else {
				assert.Equal(uint64(0), inferredSpan.Span.TraceID)
				assert.Equal(uint64(0), inferredSpan.Span.ParentID)
			}
		})
	}
}

func TestEndExecutionSpanWithEmptyObjectRequestResponse(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")
	currentExecutionInfo := &ExecutionStartInfo{}
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_CAPTURE_LAMBDA_PAYLOAD", "true")
	startTime := time.Now()
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}

	lp.startExecutionSpan(nil, []byte("[]"), startDetails)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte("{}"),
		ColdStart:          true,
		ProactiveInit:      false,
		Runtime:            "dotnet6",
	}

	executionSpan := lp.endExecutionSpan(endDetails)
	expectingResultMetaMap := map[string]string{
		"request_id":        "test-request-id",
		"cold_start":        "true",
		"function.request":  "[]", // []byte("{}") => empty list in JSON => "[]"
		"function.response": "{}", // []byte("{}") => empty map in JSON => "{}"
		"language":          "dotnet",
	}
	assert.Equal(t, executionSpan.Meta, expectingResultMetaMap)
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithNullRequestResponse(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")
	currentExecutionInfo := &ExecutionStartInfo{}
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_CAPTURE_LAMBDA_PAYLOAD", "true")
	startTime := time.Now()
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}

	lp.startExecutionSpan(nil, nil, startDetails)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(""),
		ColdStart:          true,
		ProactiveInit:      false,
		Runtime:            "dotnet6",
	}

	executionSpan := lp.endExecutionSpan(endDetails)
	expectingResultMetaMap := map[string]string{
		"request_id":        "test-request-id",
		"cold_start":        "true",
		"function.request":  "", // nil => null in JSON => ""
		"function.response": "", // []byte("") => null in JSON => ""
		"language":          "dotnet",
	}
	assert.Equal(t, executionSpan.Meta, expectingResultMetaMap)
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithNoError(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}
	configmock.SetDefaultConfigType(t, "yaml")
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_CAPTURE_LAMBDA_PAYLOAD", "true")
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := time.Now()

	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}
	lp.startExecutionSpan(nil, []byte(testString), startDetails)
	execInfo := lp.GetExecutionInfo()
	execInfo.TraceIDUpper64Hex = "1234567890abcdef"

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
		ColdStart:          true,
		ProactiveInit:      false,
		Runtime:            "dotnet6",
	}

	executionSpan := lp.endExecutionSpan(endDetails)
	expectingResultMetaMap := map[string]string{
		"request_id":                "test-request-id",
		"cold_start":                "true",
		"function.request.resource": "/users/create",
		"function.request.path":     "/users/create",
		"function.request.headers.x-datadog-parent-id": "1480558859903409531",
		"function.request.headers.x-datadog-trace-id":  "5736943178450432258",
		"_dd.p.tid": "1234567890abcdef",
		"function.request.headers.x-datadog-sampling-priority": "1",
		"function.request.httpMethod":                          "GET",
		"function.request.headers.Accept":                      "*/*",
		"function.request.headers.Accept-Encoding":             "gzip",
		"function.response.response":                           "test response payload",
		"language":                                             "dotnet",
	}
	assert.Equal(t, expectingResultMetaMap, executionSpan.Meta)
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanProactInit(t *testing.T) {
	currentExecutionInfo := &ExecutionStartInfo{}
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}
	configmock.SetDefaultConfigType(t, "yaml")
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_CAPTURE_LAMBDA_PAYLOAD", "true")
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := time.Now()

	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}
	lp.startExecutionSpan(nil, []byte(testString), startDetails)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
		ColdStart:          false,
		ProactiveInit:      true,
	}

	executionSpan := lp.endExecutionSpan(endDetails)
	expectingResultMetaMap := map[string]string{
		"request_id":                                           "test-request-id",
		"cold_start":                                           "false",
		"proactive_initialization":                             "true",
		"function.request.resource":                            "/users/create",
		"function.request.path":                                "/users/create",
		"function.request.headers.x-datadog-parent-id":         "1480558859903409531",
		"function.request.headers.x-datadog-trace-id":          "5736943178450432258",
		"function.request.headers.x-datadog-sampling-priority": "1",
		"function.request.httpMethod":                          "GET",
		"function.request.headers.Accept":                      "*/*",
		"function.request.headers.Accept-Encoding":             "gzip",
		"function.response.response":                           "test response payload",
	}
	assert.Equal(t, executionSpan.Meta, expectingResultMetaMap)
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithInvalidCaptureLambdaPayloadValue(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_CAPTURE_LAMBDA_PAYLOAD", "INVALID_INPUT")
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := time.Now()
	currentExecutionInfo := &ExecutionStartInfo{}
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}
	lp.startExecutionSpan(nil, []byte(testString), startDetails)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
	}

	executionSpan := lp.endExecutionSpan(endDetails)
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
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}
	t.Setenv(functionNameEnvVar, "TestFunction")
	testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}`
	startTime := time.Now()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}
	lp.startExecutionSpan(nil, []byte(testString), startDetails)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            true,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{}`),
	}
	executionSpan := lp.endExecutionSpan(endDetails)
	assert.Equal(t, executionSpan.Error, int32(1))
}

func TestEndExecutionSpanWithTimeout(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")
	currentExecutionInfo := &ExecutionStartInfo{}
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}

	startTime := time.Now()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}
	lp.startExecutionSpan(nil, []byte("[]"), startDetails)

	assert.Zero(t, currentExecutionInfo.TraceID)
	assert.Zero(t, currentExecutionInfo.SpanID)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            true,
		IsTimeout:          true,
		RequestID:          "test-request-id",
		ResponseRawPayload: nil,
		ColdStart:          true,
		ProactiveInit:      false,
		Runtime:            "dotnet6",
	}
	executionSpan := lp.endExecutionSpan(endDetails)
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, "dotnet", executionSpan.Meta["language"])
	assert.Equal(t, lp.requestHandler.executionInfo.TraceID, executionSpan.TraceID)
	assert.NotZero(t, executionSpan.TraceID)
	assert.NotZero(t, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
	assert.Equal(t, "Impending Timeout", executionSpan.Meta["error.type"])
	assert.Equal(t, "Datadog detected an Impending Timeout", executionSpan.Meta["error.msg"])
}

func TestEndExecutionSpanWithStepFunctions(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")
	currentExecutionInfo := &ExecutionStartInfo{}
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}

	lp.requestHandler.triggerTags["_dd.p.tid"] = "6fb5c3a05c73dbfe"

	startTime := time.Now()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}

	stepFunctionEvent := events.StepFunctionPayload{
		Execution: struct {
			ID           string
			RedriveCount uint16
		}{
			ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
			RedriveCount: 0,
		},
		State: struct {
			Name        string
			EnteredTime string
			RetryCount  uint16
		}{
			Name:        "agocsTest1",
			EnteredTime: "2024-07-30T20:46:20.824Z",
			RetryCount:  0,
		},
	}

	lp.startExecutionSpan(stepFunctionEvent, []byte("[]"), startDetails)

	assert.Equal(t, uint64(5377636026938777059), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(8947638978974359093), currentExecutionInfo.parentID)
	assert.Equal(t, "6fb5c3a05c73dbfe", lp.requestHandler.triggerTags["_dd.p.tid"])

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
		ColdStart:          true,
		ProactiveInit:      false,
		Runtime:            "dotnet6",
	}
	executionSpan := lp.endExecutionSpan(endDetails)

	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
	assert.Equal(t, "6fb5c3a05c73dbfe", executionSpan.Meta["_dd.p.tid"])

}

func TestEndExecutionSpanWithNestedStepFunctions(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")
	currentExecutionInfo := &ExecutionStartInfo{}
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}

	lp.requestHandler.triggerTags["_dd.p.tid"] = "579d19b3ee216ee9"

	startTime := time.Now()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}

	nestedStepFunctionEvent := events.NestedStepFunctionPayload{
		Payload: events.StepFunctionPayload{
			Execution: struct {
				ID           string
				RedriveCount uint16
			}{
				ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
				RedriveCount: 0,
			},
			State: struct {
				Name        string
				EnteredTime string
				RetryCount  uint16
			}{
				Name:        "agocsTest1",
				EnteredTime: "2024-07-30T20:46:20.824Z",
				RetryCount:  0,
			},
		},
		RootExecutionID:   "arn:aws:states:sa-east-1:425362996713:execution:invokeJavaLambda:4875aba4-ae31-4a4c-bf8a-63e9eee31dad",
		ServerlessVersion: "v1",
	}

	lp.startExecutionSpan(nestedStepFunctionEvent, []byte("[]"), startDetails)

	assert.Equal(t, uint64(1322229001489018110), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(8947638978974359093), currentExecutionInfo.parentID)
	assert.Equal(t, "579d19b3ee216ee9", lp.requestHandler.triggerTags["_dd.p.tid"])

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
		ColdStart:          true,
		ProactiveInit:      false,
		Runtime:            "dotnet6",
	}
	executionSpan := lp.endExecutionSpan(endDetails)

	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
	assert.Equal(t, "579d19b3ee216ee9", executionSpan.Meta["_dd.p.tid"])

}

func TestEndExecutionSpanWithLambdaRootStepFunctions(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")
	currentExecutionInfo := &ExecutionStartInfo{}
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: currentExecutionInfo,
			triggerTags:   make(map[string]string),
		},
	}

	lp.requestHandler.triggerTags["_dd.p.tid"] = "672a7cb100000000"

	startTime := time.Now()
	startDetails := &InvocationStartDetails{
		StartTime:          startTime,
		InvokeEventHeaders: http.Header{},
	}

	lambdaRootStepFunctionEvent := events.LambdaRootStepFunctionPayload{
		Payload: events.StepFunctionPayload{
			Execution: struct {
				ID           string
				RedriveCount uint16
			}{
				ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
				RedriveCount: 0,
			},
			State: struct {
				Name        string
				EnteredTime string
				RetryCount  uint16
			}{
				Name:        "agocsTest1",
				EnteredTime: "2024-07-30T20:46:20.824Z",
				RetryCount:  0,
			},
		},
		TraceID:           "5821803790426892636",
		TraceTags:         "_dd.p.dm=-0,_dd.p.tid=672a7cb100000000",
		ServerlessVersion: "v1",
	}

	lp.startExecutionSpan(lambdaRootStepFunctionEvent, []byte("[]"), startDetails)

	assert.Equal(t, uint64(5821803790426892636), currentExecutionInfo.TraceID)
	assert.Equal(t, uint64(8947638978974359093), currentExecutionInfo.parentID)
	assert.Equal(t, "672a7cb100000000", lp.requestHandler.triggerTags["_dd.p.tid"])

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	endDetails := &InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"response":"test response payload"}`),
		ColdStart:          true,
		ProactiveInit:      false,
		Runtime:            "dotnet6",
	}
	executionSpan := lp.endExecutionSpan(endDetails)

	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
	assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
	assert.Equal(t, "672a7cb100000000", executionSpan.Meta["_dd.p.tid"])

}

func TestParseLambdaPayload(t *testing.T) {
	assert.Equal(t, []byte(""), ParseLambdaPayload([]byte("")))
	assert.Equal(t, []byte("{}"), ParseLambdaPayload([]byte("{}")))
	assert.Equal(t, []byte("{}"), ParseLambdaPayload([]byte("a{}a")))
	assert.Equal(t, []byte("{a}"), ParseLambdaPayload([]byte("{a}a")))
	assert.Equal(t, []byte("{a}"), ParseLambdaPayload([]byte("a{a}")))
	assert.Equal(t, []byte("{a}"), ParseLambdaPayload([]byte("}{a}a{")))
	assert.Equal(t, []byte("{}{}"), ParseLambdaPayload([]byte("{}{}")))
	assert.Equal(t, []byte("{a}"), ParseLambdaPayload([]byte("a{a}a")))
	assert.Equal(t, []byte("{"), ParseLambdaPayload([]byte("{")))
	assert.Equal(t, []byte("}"), ParseLambdaPayload([]byte("}")))
}

func TestLanguageTag(t *testing.T) {
	testCases := []struct {
		runtime     string
		expectedTag string
	}{
		{runtime: "dotnet6", expectedTag: "dotnet"},
		{runtime: "java11", expectedTag: "java"},
		{runtime: "ruby2.7", expectedTag: "ruby"},
		{runtime: "go1.x", expectedTag: "go"},
	}

	for _, tc := range testCases {
		currentExecutionInfo := &ExecutionStartInfo{}
		lp := &LifecycleProcessor{
			requestHandler: &RequestHandler{
				executionInfo: currentExecutionInfo,
				triggerTags:   make(map[string]string),
			},
		}
		t.Setenv(functionNameEnvVar, "TestFunction")
		testString := `{"resource":"/users/create","path":"/users/create","httpMethod":"GET"}`

		startTime := time.Now()
		startDetails := &InvocationStartDetails{
			StartTime:          startTime,
			InvokeEventHeaders: http.Header{},
		}
		lp.startExecutionSpan(nil, []byte(testString), startDetails)

		duration := 1 * time.Second
		endTime := startTime.Add(duration)

		endDetails := &InvocationEndDetails{
			EndTime:            endTime,
			IsError:            false,
			RequestID:          "test-request-id",
			ResponseRawPayload: []byte(`{"response":"test response payload"}`),
			ColdStart:          true,
			ProactiveInit:      false,
			Runtime:            tc.runtime, // add runtime
		}

		executionSpan := lp.endExecutionSpan(endDetails)
		assert.Equal(t, "aws.lambda", executionSpan.Name)
		assert.Equal(t, "aws.lambda", executionSpan.Service)
		assert.Equal(t, "TestFunction", executionSpan.Resource)
		assert.Equal(t, "serverless", executionSpan.Type)
		assert.Equal(t, "test-request-id", executionSpan.Meta["request_id"])
		assert.Equal(t, currentExecutionInfo.TraceID, executionSpan.TraceID)
		assert.Equal(t, currentExecutionInfo.SpanID, executionSpan.SpanID)
		assert.Equal(t, startTime.UnixNano(), executionSpan.Start)
		assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)

		assert.Equal(t, tc.expectedTag, executionSpan.Meta["language"]) // expected tag from runtime
	}
}

func TestCapturePayloadAsTags(t *testing.T) {
	nestedMap := map[string]interface{}{
		"key1": "value1",
		"key2": map[string]interface{}{
			"key3":    3,
			"key4":    true,
			"keylist": []interface{}{1, 2, 3, "four", 5.5, `{"keyInsideSlice":"val7","age":84}`},
		},
		"innerJSONString": `{"key5":"value5","age":42}`,
		"innerJSONBytes":  []byte(`{"key6":"value6","age":21}`),
	}
	expectingResultMap := map[string]string{
		"test.key1":                          "value1",
		"test.key2.key3":                     "3",
		"test.key2.key4":                     "true",
		"test.key2.keylist.0":                "1",
		"test.key2.keylist.1":                "2",
		"test.key2.keylist.2":                "3",
		"test.key2.keylist.3":                "four",
		"test.key2.keylist.4":                "5.5",
		"test.key2.keylist.5.keyInsideSlice": "val7",
		"test.key2.keylist.5.age":            "84",
		"test.innerJSONString.key5":          "value5",
		"test.innerJSONString.age":           "42",
		"test.innerJSONBytes.key6":           "value6",
		"test.innerJSONBytes.age":            "21",
	}
	metaMap := make(map[string]string)
	executionSpan := &pb.Span{
		Meta: metaMap,
	}
	capturePayloadAsTags(nestedMap, executionSpan, "test", 0, 10)
	assert.Equal(t, executionSpan.Meta, expectingResultMap)
}

func TestCapturePayloadAsTagsMaxDepth(t *testing.T) {
	nestedMap := map[string]interface{}{
		"key1": "value1",
		"key2": map[string]interface{}{
			"key3": map[string]interface{}{
				"nestedKey": "nestedVal",
			},
			"key4": true,
		},
		"key5": "value5",
	}
	expectingResultMap := map[string]string{
		"test.key1":      "value1",
		"test.key2.key3": "{\"nestedKey\":\"nestedVal\"}",
		"test.key2.key4": "true",
		"test.key5":      "value5",
	}
	metaMap := make(map[string]string)
	executionSpan := &pb.Span{
		Meta: metaMap,
	}
	capturePayloadAsTags(nestedMap, executionSpan, "test", 0, 2)
	assert.Equal(t, executionSpan.Meta, expectingResultMap)
}

func TestCapturePayloadAsTagsNilCases(t *testing.T) {
	testMap := map[string]interface{}{
		"key1": nil,
		"key2": map[string]interface{}{
			"key3": nil,
			"key4": true,
		},
		"emptyMap":  map[string]interface{}{},
		"emptyList": []interface{}{},
	}
	expectingResultMap := map[string]string{
		"test.key1":      "",
		"test.key2.key3": "",
		"test.key2.key4": "true",
		"test.emptyMap":  "{}",
		"test.emptyList": "[]",
	}
	metaMap := make(map[string]string)
	executionSpan := &pb.Span{
		Meta: metaMap,
	}
	capturePayloadAsTags(testMap, executionSpan, "test", 0, 10)
	assert.Equal(t, executionSpan.Meta, expectingResultMap)
}

func TestCompleteInferredSpanWithNoError(t *testing.T) {
	inferredSpan := new(inferredspan.InferredSpan)
	startTime := time.Now()

	inferredSpan.GenerateInferredSpan(time.Now())
	inferredSpan.Span.TraceID = 2350923428932752492
	inferredSpan.Span.SpanID = 1304592378509342580
	inferredSpan.Span.Start = startTime.UnixNano()
	inferredSpan.Span.Name = "aws.mock"
	inferredSpan.Span.Service = "aws.mock"
	inferredSpan.Span.Resource = "test-function"
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		"stage": "dev",
	}

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	isError := false
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{TraceID: 1234},
		},
	}

	span := lp.completeInferredSpan(inferredSpan, endTime, isError)
	assert.Equal(t, "aws.mock", span.Name)
	assert.Equal(t, "aws.mock", span.Service)
	assert.Equal(t, "test-function", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "dev", span.Meta["stage"])
	assert.Equal(t, uint64(1234), span.TraceID)
	assert.Equal(t, inferredSpan.Span.SpanID, span.SpanID)
	assert.Equal(t, duration.Nanoseconds(), span.Duration)
	assert.Equal(t, int32(0), inferredSpan.Span.Error)
}

func TestCompleteInferredSpanWithError(t *testing.T) {
	inferredSpan := new(inferredspan.InferredSpan)
	startTime := time.Now()

	inferredSpan.GenerateInferredSpan(time.Now())
	inferredSpan.Span.TraceID = 2350923428932752492
	inferredSpan.Span.SpanID = 1304592378509342580
	inferredSpan.Span.Start = startTime.UnixNano()
	inferredSpan.Span.Name = "aws.mock"
	inferredSpan.Span.Service = "aws.mock"
	inferredSpan.Span.Resource = "test-function"
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		"stage": "dev",
	}

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	isError := true
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{TraceID: 1234},
		},
	}

	span := lp.completeInferredSpan(inferredSpan, endTime, isError)
	assert.Equal(t, "aws.mock", span.Name)
	assert.Equal(t, "aws.mock", span.Service)
	assert.Equal(t, "test-function", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "dev", span.Meta["stage"])
	assert.Equal(t, uint64(1234), span.TraceID)
	assert.Equal(t, inferredSpan.Span.SpanID, span.SpanID)
	assert.Equal(t, duration.Nanoseconds(), span.Duration)
	assert.Equal(t, int32(1), inferredSpan.Span.Error)
}

func TestCompleteInferredSpanWithAsync(t *testing.T) {
	inferredSpan := new(inferredspan.InferredSpan)
	// Start of inferred span
	startTime := time.Now()
	duration := 2 * time.Second
	// mock invocation end time
	lambdaInvocationStartTime := startTime.Add(duration)
	inferredSpan.GenerateInferredSpan(lambdaInvocationStartTime)
	inferredSpan.IsAsync = true
	inferredSpan.Span.TraceID = 2350923428932752492
	inferredSpan.Span.SpanID = 1304592378509342580
	inferredSpan.Span.Start = startTime.UnixNano()
	inferredSpan.Span.Name = "aws.mock"
	inferredSpan.Span.Service = "aws.mock"
	inferredSpan.Span.Resource = "test-function"
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		"stage": "dev",
	}
	endTime := time.Now()
	isError := false
	lp := &LifecycleProcessor{
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{TraceID: 1234},
		},
	}

	span := lp.completeInferredSpan(inferredSpan, endTime, isError)
	assert.Equal(t, "aws.mock", span.Name)
	assert.Equal(t, "aws.mock", span.Service)
	assert.Equal(t, "test-function", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "dev", span.Meta["stage"])
	assert.Equal(t, uint64(1234), span.TraceID)
	assert.Equal(t, inferredSpan.Span.SpanID, span.SpanID)
	assert.Equal(t, duration.Nanoseconds(), span.Duration)
	assert.Equal(t, int32(0), inferredSpan.Span.Error)
}

func Test_getUpper64Hex(t *testing.T) {
	tests := []struct {
		name string
		tags string
		want string
	}{
		{
			name: "just a trace tag",
			tags: "_dd.p.tid=1234567890abcdef",
			want: "1234567890abcdef",
		},
		{
			name: "nothing",
			tags: "",
			want: "",
		},
		{
			name: "multiple tags 1",
			tags: "some=tag,_dd.p.tid=1234567890abcdef",
			want: "1234567890abcdef",
		},
		{
			name: "multiple tags 2",
			tags: "_dd.p.tid=1234567890abcdef,some=tag",
			want: "1234567890abcdef",
		},
		{
			name: "multiple tags 3",
			tags: "some=tag,_dd.p.tid=1234567890abcdef,another=tag",
			want: "1234567890abcdef",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, getUpper64Hex(tt.tags), "getUpper64Hex(%v)", tt.tags)

		})
	}
}
