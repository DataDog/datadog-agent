// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
)

func TestGenerateEnhancedErrorMetricOnInvocationEnd(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return true }
	demux := createDemultiplexer(t)

	endInvocationTime := time.Now()
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: true}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
	}
	go testProcessor.OnInvokeEnd(&endDetails)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(1, 0, 250*time.Millisecond)

	assert.Len(t, timedMetrics, 0)
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       extraTags.Tags,
		SampleRate: 1,
		Timestamp:  float64(endInvocationTime.UnixNano()) / float64(time.Second),
	}})
}

func TestStartExecutionSpanNoLambdaLibrary(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := createDemultiplexer(t)
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return false }

	eventPayload := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`
	startInvocationTime := time.Now()
	startDetails := InvocationStartDetails{
		StartTime:             startInvocationTime,
		InvokeEventRawPayload: []byte(eventPayload),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
	}

	testProcessor.OnInvokeStart(&startDetails)

	assert.NotNil(t, testProcessor.GetExecutionInfo())

	assert.Equal(t, uint64(0), testProcessor.GetExecutionInfo().SpanID)
	assert.Equal(t, uint64(5736943178450432258), testProcessor.GetExecutionInfo().TraceID)
	assert.Equal(t, uint64(1480558859903409531), testProcessor.GetExecutionInfo().parentID)
	assert.Equal(t, sampler.SamplingPriority(1), testProcessor.GetExecutionInfo().SamplingPriority)
	assert.Equal(t, startInvocationTime, testProcessor.GetExecutionInfo().startTime)
}

func TestStartExecutionSpanWithLambdaLibrary(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := createDemultiplexer(t)
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return true }

	startInvocationTime := time.Now()
	startDetails := InvocationStartDetails{
		StartTime:          startInvocationTime,
		InvokedFunctionARN: "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
	}
	testProcessor.OnInvokeStart(&startDetails)

	assert.NotEqual(t, 0, testProcessor.GetExecutionInfo().SpanID)
	assert.NotEqual(t, 0, testProcessor.GetExecutionInfo().TraceID)
	assert.Equal(t, startInvocationTime, testProcessor.GetExecutionInfo().startTime)
}

func TestEndExecutionSpanNoLambdaLibrary(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := createDemultiplexer(t)
	mockDetectLambdaLibrary := func() bool { return false }

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: false}
	samplingPriority := sampler.SamplingPriority(1)

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				startTime:        startInvocationTime,
				TraceID:          123,
				SpanID:           1,
				parentID:         3,
				SamplingPriority: samplingPriority,
			},
			triggerTags: make(map[string]string),
		},
	}
	testProcessor.OnInvokeEnd(&endDetails)
	executionChunkPriority := tracePayload.TracerPayload.Chunks[0].Priority
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, testProcessor.GetExecutionInfo().TraceID, executionSpan.TraceID)
	assert.Equal(t, testProcessor.GetExecutionInfo().SpanID, executionSpan.SpanID)
	assert.Equal(t, testProcessor.GetExecutionInfo().parentID, executionSpan.ParentID)
	assert.Equal(t, int32(testProcessor.GetExecutionInfo().SamplingPriority), executionChunkPriority)
	assert.Equal(t, startInvocationTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithLambdaLibrary(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := createDemultiplexer(t)
	mockDetectLambdaLibrary := func() bool { return true }

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: false}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				startTime: startInvocationTime,
				TraceID:   123,
				SpanID:    1,
			},
			triggerTags: make(map[string]string),
		},
	}

	testProcessor.OnInvokeEnd(&endDetails)

	assert.Equal(t, (*api.Payload)(nil), tracePayload)
}

func TestEndExecutionSpanWithTraceMetadata(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := createDemultiplexer(t)
	mockDetectLambdaLibrary := func() bool { return false }

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	endDetails := InvocationEndDetails{
		EndTime:    endInvocationTime,
		IsError:    true,
		RequestID:  "test-request-id",
		ErrorMsg:   "custom exception",
		ErrorType:  "Exception",
		ErrorStack: "exception",
	}
	samplingPriority := sampler.SamplingPriority(1)

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				startTime:        startInvocationTime,
				TraceID:          123,
				SpanID:           1,
				parentID:         3,
				SamplingPriority: samplingPriority,
			},
			triggerTags: make(map[string]string),
		},
	}
	testProcessor.OnInvokeEnd(&endDetails)
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]

	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, int32(1), executionSpan.Error)
	assert.Equal(t, "custom exception", executionSpan.Meta["error.msg"])
	assert.Equal(t, "Exception", executionSpan.Meta["error.type"])
	assert.Equal(t, "exception", executionSpan.Meta["error.stack"])
}

func TestCompleteInferredSpanWithStartTime(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")
	t.Setenv("DD_SERVICE", "mock-lambda-service")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := createDemultiplexer(t)
	mockDetectLambdaLibrary := func() bool { return false }

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	startInferredSpan := time.Now()
	startInvocationTime := startInferredSpan.Add(250 * time.Millisecond)
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: false}
	samplingPriority := sampler.SamplingPriority(1)

	var inferredSpanSlice [2]*inferredspan.InferredSpan
	inferredSpanSlice[0] = &inferredspan.InferredSpan{
		CurrentInvocationStartTime: startInferredSpan,
		Span: &pb.Span{
			TraceID: 123,
			SpanID:  3,
			Start:   startInferredSpan.UnixNano(),
		},
	}

	testProcessor := LifecycleProcessor{
		ExtraTags:            extraTags,
		ProcessTrace:         mockProcessTrace,
		DetectLambdaLibrary:  mockDetectLambdaLibrary,
		Demux:                demux,
		InferredSpansEnabled: true,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				startTime:        startInvocationTime,
				TraceID:          123,
				SpanID:           1,
				parentID:         3,
				SamplingPriority: samplingPriority,
			},
			triggerTags:   make(map[string]string),
			inferredSpans: inferredSpanSlice,
		},
	}

	testProcessor.OnInvokeEnd(&endDetails)

	spans := tracePayload.TracerPayload.Chunks[0].Spans
	assert.Equal(t, 2, len(spans))
	completedInferredSpan := spans[1]
	httpStatusCode := testProcessor.GetInferredSpan().Span.GetMeta()["http.status_code"]
	peerService := testProcessor.GetInferredSpan().Span.GetMeta()["peer.service"]
	assert.NotNil(t, httpStatusCode)
	assert.Equal(t, peerService, "mock-lambda-service")
	assert.Equal(t, testProcessor.GetInferredSpan().Span.Start, completedInferredSpan.Start)
}

func TestCompleteInferredSpanWithOutStartTime(t *testing.T) {
	t.Setenv(functionNameEnvVar, "TestFunction")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := createDemultiplexer(t)
	mockDetectLambdaLibrary := func() bool { return false }

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: false}
	samplingPriority := sampler.SamplingPriority(1)

	var inferredSpanSlice [2]*inferredspan.InferredSpan
	inferredSpanSlice[0] = &inferredspan.InferredSpan{
		CurrentInvocationStartTime: time.Time{},
		Span: &pb.Span{
			TraceID: 123,
			SpanID:  3,
			Start:   0,
		},
	}

	testProcessor := LifecycleProcessor{
		ExtraTags:            extraTags,
		ProcessTrace:         mockProcessTrace,
		DetectLambdaLibrary:  mockDetectLambdaLibrary,
		Demux:                demux,
		InferredSpansEnabled: true,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				startTime:        startInvocationTime,
				TraceID:          123,
				SpanID:           1,
				parentID:         3,
				SamplingPriority: samplingPriority,
			},
			triggerTags:   make(map[string]string),
			inferredSpans: inferredSpanSlice,
		},
	}

	testProcessor.OnInvokeEnd(&endDetails)

	// If our logic is correct this will actually be the execution span
	// and the start time is expected to be the invocation start time,
	// not the inferred span start time.
	completedInferredSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, startInvocationTime.UnixNano(), completedInferredSpan.Start)
}
func TestTriggerTypesLifecycleEventForAPIGatewayRest(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("api-gateway.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
	}

	testProcessor.OnInvokeStart(startDetails)
	assert.Equal(t, map[string]string{
		"function_trigger.event_source_arn": "arn:aws:apigateway:us-east-1::/restapis/1234567890/stages/prod",
		"http.method":                       "POST",
		"http.route":                        "/{proxy+}",
		"http.url":                          "70ixmpl4fl.execute-api.us-east-2.amazonaws.com",
		"http.url_details.path":             "/prod/path/to/resource",
		"http.useragent":                    "Custom User Agent String",
		"function_trigger.event_source":     "api-gateway",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForAPIGateway5xxResponse(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("api-gateway.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	demux := createDemultiplexer(t)

	testProcessor := &LifecycleProcessor{
		ExtraTags:           extraTags,
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        mockProcessTrace,
		Demux:               demux,
	}
	testProcessor.OnInvokeStart(startDetails)

	endTime := timeNow()
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"statusCode": 500}`),
	})

	// assert http.status_code is 500
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:apigateway:us-east-1::/restapis/1234567890/stages/prod",
		"http.method":                       "POST",
		"http.route":                        "/{proxy+}",
		"http.url":                          "70ixmpl4fl.execute-api.us-east-2.amazonaws.com",
		"http.url_details.path":             "/prod/path/to/resource",
		"http.useragent":                    "Custom User Agent String",
		"http.status_code":                  "500",
		"function_trigger.event_source":     "api-gateway",
		"request_id":                        "test-request-id",
	}, testProcessor.GetTags())

	// assert error metrics equal
	generatedMetrics, lateMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)
	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       extraTags.Tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.Unix()),
	}})
	assert.Len(t, lateMetrics, 0)

	// assert span error set to 1
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, executionSpan.Error, int32(1))
}

func TestTriggerTypesLifecycleEventForAPIGatewayNonProxy(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("api-gateway-non-proxy.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"statusCode": 200}`),
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:apigateway:us-east-1::/restapis/lgxbo6a518/stages/dev",
		"http.method":                       "GET",
		"http.route":                        "/http/get",
		"http.url":                          "lgxbo6a518.execute-api.sa-east-1.amazonaws.com",
		"http.url_details.path":             "/dev/http/get",
		"http.useragent":                    "curl/7.64.1",
		"request_id":                        "test-request-id",
		"http.status_code":                  "200",
		"function_trigger.event_source":     "api-gateway",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForAPIGatewayNonProxy5xxResponse(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("api-gateway-non-proxy.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	demux := createDemultiplexer(t)

	testProcessor := &LifecycleProcessor{
		ExtraTags:           extraTags,
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        mockProcessTrace,
		Demux:               demux,
	}
	testProcessor.OnInvokeStart(startDetails)

	endTime := timeNow()
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"statusCode": 500}`),
	})

	// assert http.status_code is 500
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:apigateway:us-east-1::/restapis/lgxbo6a518/stages/dev",
		"http.method":                       "GET",
		"http.route":                        "/http/get",
		"http.url":                          "lgxbo6a518.execute-api.sa-east-1.amazonaws.com",
		"http.url_details.path":             "/dev/http/get",
		"request_id":                        "test-request-id",
		"http.status_code":                  "500",
		"http.useragent":                    "curl/7.64.1",
		"function_trigger.event_source":     "api-gateway",
	}, testProcessor.GetTags())

	// assert error metrics equal
	generatedMetrics, lateMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)
	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       extraTags.Tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.Unix()),
	}})
	assert.Len(t, lateMetrics, 0)

	// assert span error set to 1
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, executionSpan.Error, int32(1))
}

func TestTriggerTypesLifecycleEventForAPIGatewayWebsocket(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("api-gateway-websocket-default.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"statusCode": 200}`),
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:apigateway:us-east-1::/restapis/p62c47itsb/stages/dev",
		"request_id":                        "test-request-id",
		"http.status_code":                  "200",
		"function_trigger.event_source":     "api-gateway",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForAPIGatewayWebsocket5xxResponse(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("api-gateway-websocket-default.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	demux := createDemultiplexer(t)

	testProcessor := &LifecycleProcessor{
		ExtraTags:           extraTags,
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        mockProcessTrace,
		Demux:               demux,
	}
	testProcessor.OnInvokeStart(startDetails)

	endTime := timeNow()
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"statusCode": 500}`),
	})

	// assert http.status_code is 500
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:apigateway:us-east-1::/restapis/p62c47itsb/stages/dev",
		"request_id":                        "test-request-id",
		"http.status_code":                  "500",
		"function_trigger.event_source":     "api-gateway",
	}, testProcessor.GetTags())

	// assert error metrics equal
	generatedMetrics, lateMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)
	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       extraTags.Tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.Unix()),
	}})
	assert.Len(t, lateMetrics, 0)

	// assert span error set to 1
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, executionSpan.Error, int32(1))
}

func TestTriggerTypesLifecycleEventForALB(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("application-load-balancer.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"statusCode": 200}`),
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/lambda-xyz/123abc",
		"request_id":                        "test-request-id",
		"http.status_code":                  "200",
		"http.method":                       "GET",
		"http.url_details.path":             "/lambda",
		"function_trigger.event_source":     "application-load-balancer",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForALB5xxResponse(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("application-load-balancer.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	demux := createDemultiplexer(t)

	testProcessor := &LifecycleProcessor{
		ExtraTags:           extraTags,
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        mockProcessTrace,
		Demux:               demux,
	}
	testProcessor.OnInvokeStart(startDetails)

	endTime := timeNow()
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		EndTime:            endTime,
		IsError:            false,
		RequestID:          "test-request-id",
		ResponseRawPayload: []byte(`{"statusCode": 500}`),
	})

	// assert http.status_code is 500
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/lambda-xyz/123abc",
		"request_id":                        "test-request-id",
		"http.status_code":                  "500",
		"http.method":                       "GET",
		"http.url_details.path":             "/lambda",
		"function_trigger.event_source":     "application-load-balancer",
	}, testProcessor.GetTags())

	// assert error metrics equal
	generatedMetrics, lateMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)
	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       extraTags.Tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.Unix()),
	}})
	assert.Len(t, lateMetrics, 0)

	// assert span error set to 1
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, executionSpan.Error, int32(1))
}

func TestTriggerTypesLifecycleEventForCloudwatch(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("cloudwatch-events.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:events:us-east-1:123456789012:rule/ExampleRule",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "cloudwatch-events",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForCloudwatchLogs(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("cloudwatch-logs.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:logs:us-east-1:123456789012:log-group:testLogGroup",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "cloudwatch-logs",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForDynamoDB(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("dynamodb.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:dynamodb:us-east-1:123456789012:table/ExampleTableWithStream/stream/2015-06-27T00:48:05.899",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "dynamodb",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForKinesis(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("kinesis-batch.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:kinesis:sa-east-1:425362996713:stream/kinesisStream",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "kinesis",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForS3(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("s3.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "aws:s3:sample:event:source",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "s3",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForSNS(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("sns-batch.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:sns:sa-east-1:425362996713:serverlessTracingTopicPy",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "sns",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForSQS(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("sqs-batch.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "arn:aws:sqs:sa-east-1:425362996713:InferredSpansQueueNode",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "sqs",
	}, testProcessor.GetTags())
}

func TestTriggerTypesLifecycleEventForSNSSQS(t *testing.T) {

	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)

	var tracePayload *api.Payload

	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("snssqs.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		StartTime:             startInvocationTime,
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary:  func() bool { return false },
		ProcessTrace:         func(payload *api.Payload) { tracePayload = payload },
		InferredSpansEnabled: true,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				TraceID:          123,
				SamplingPriority: 1,
			},
		},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
		EndTime:   endInvocationTime,
		IsError:   false,
	})

	spans := tracePayload.TracerPayload.Chunks[0].Spans
	assert.Equal(t, 3, len(spans))
	snsSpan, sqsSpan := spans[1], spans[2]
	// These IDs are B64 decoded from the snssqs.json event sample's _datadog MessageAttribute
	expectedTraceID := uint64(1728904347387697031)
	expectedParentID := uint64(353722510835624345)

	assert.Equal(t, expectedTraceID, snsSpan.TraceID)
	assert.Equal(t, expectedParentID, snsSpan.ParentID)
	assert.Equal(t, snsSpan.SpanID, sqsSpan.ParentID)
}

func TestTriggerTypesLifecycleEventForSNSSQSNoDdContext(t *testing.T) {

	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)

	var tracePayload *api.Payload

	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("snssqs_no_dd_context.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		StartTime:             startInvocationTime,
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary:  func() bool { return false },
		ProcessTrace:         func(payload *api.Payload) { tracePayload = payload },
		InferredSpansEnabled: true,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				TraceID:          123,
				SamplingPriority: 1,
			},
		},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
		EndTime:   endInvocationTime,
		IsError:   false,
	})

	spans := tracePayload.TracerPayload.Chunks[0].Spans
	assert.Equal(t, 3, len(spans))
	snsSpan, sqsSpan := spans[1], spans[2]
	expectedTraceID := uint64(0)
	expectedParentID := uint64(0)

	assert.Equal(t, expectedTraceID, snsSpan.TraceID)
	assert.Equal(t, expectedParentID, snsSpan.ParentID)
	assert.Equal(t, snsSpan.SpanID, sqsSpan.ParentID)
}

func TestTriggerTypesLifecycleEventForSQSNoDdContext(t *testing.T) {

	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)

	var tracePayload *api.Payload

	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("sqs_no_dd_context.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		StartTime:             startInvocationTime,
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary:  func() bool { return false },
		ProcessTrace:         func(payload *api.Payload) { tracePayload = payload },
		InferredSpansEnabled: true,
		requestHandler: &RequestHandler{
			executionInfo: &ExecutionStartInfo{
				TraceID:          123,
				SamplingPriority: 1,
			},
		},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
		EndTime:   endInvocationTime,
		IsError:   false,
	})

	spans := tracePayload.TracerPayload.Chunks[0].Spans
	assert.Equal(t, 2, len(spans))
	sqsSpan := spans[1]
	expectedTraceID := uint64(0)
	expectedParentID := uint64(0)

	assert.Equal(t, expectedTraceID, sqsSpan.TraceID)
	assert.Equal(t, expectedParentID, sqsSpan.ParentID)
}

func TestTriggerTypesLifecycleEventForEventBridge(t *testing.T) {
	startDetails := &InvocationStartDetails{
		InvokeEventRawPayload: getEventFromFile("eventbridge-custom.json"),
		InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}

	testProcessor := &LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace:        func(*api.Payload) {},
	}

	testProcessor.OnInvokeStart(startDetails)
	testProcessor.OnInvokeEnd(&InvocationEndDetails{
		RequestID: "test-request-id",
	})
	assert.Equal(t, map[string]string{
		"cold_start":                        "false",
		"function_trigger.event_source_arn": "eventbridge.custom.event.sender",
		"request_id":                        "test-request-id",
		"function_trigger.event_source":     "eventbridge",
	}, testProcessor.GetTags())
}

// Helper function for reading test file
func getEventFromFile(filename string) []byte {
	event, err := os.ReadFile("../trace/testdata/event_samples/" + filename)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	buf.WriteString("a5a")
	buf.Write(event)
	buf.WriteString("0")
	return buf.Bytes()
}

func createDemultiplexer(t *testing.T) demultiplexer.FakeSamplerMock {
	return fxutil.Test[demultiplexer.FakeSamplerMock](t, logimpl.MockModule(), compressionimpl.MockModule(), demultiplexerimpl.FakeSamplerMockModule(), hostnameimpl.MockModule())
}
