// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"

	"github.com/stretchr/testify/assert"
)

func TestGenerateEnhancedErrorMetricOnInvocationEnd(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return true }
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)

	endInvocationTime := time.Now()
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: true}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
	}
	go testProcessor.OnInvokeEnd(&endDetails)

	generatedMetrics := demux.WaitForSamples(time.Millisecond * 250)

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
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return false }

	eventPayload := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`
	startInvocationTime := time.Now()
	startDetails := InvocationStartDetails{StartTime: startInvocationTime, InvokeEventRawPayload: eventPayload}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
	}

	testProcessor.OnInvokeStart(&startDetails)

	assert.NotNil(t, testProcessor.GetExecutionContext())

	assert.Equal(t, uint64(0), testProcessor.GetExecutionContext().SpanID)
	assert.Equal(t, uint64(5736943178450432258), testProcessor.GetExecutionContext().TraceID)
	assert.Equal(t, uint64(1480558859903409531), testProcessor.GetExecutionContext().parentID)
	assert.Equal(t, sampler.SamplingPriority(1), testProcessor.GetExecutionContext().SamplingPriority)
	assert.Equal(t, startInvocationTime, testProcessor.GetExecutionContext().startTime)
}

func TestStartExecutionSpanWithLambdaLibrary(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return true }

	startInvocationTime := time.Now()
	startDetails := InvocationStartDetails{StartTime: startInvocationTime}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		Demux:               demux,
	}
	testProcessor.OnInvokeStart(&startDetails)

	assert.NotEqual(t, 0, testProcessor.GetExecutionContext().SpanID)
	assert.NotEqual(t, 0, testProcessor.GetExecutionContext().TraceID)
	assert.Equal(t, startInvocationTime, testProcessor.GetExecutionContext().startTime)
}

func TestEndExecutionSpanNoLambdaLibrary(t *testing.T) {
	defer os.Unsetenv(functionNameEnvVar)
	os.Setenv(functionNameEnvVar, "TestFunction")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
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
			executionContext: &ExecutionStartInfo{
				startTime:        startInvocationTime,
				TraceID:          123,
				SpanID:           1,
				parentID:         3,
				SamplingPriority: samplingPriority,
			},
		},
	}
	testProcessor.OnInvokeEnd(&endDetails)
	executionChunkPriority := tracePayload.TracerPayload.Chunks[0].Priority
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, testProcessor.requestHandler.executionContext.TraceID, executionSpan.TraceID)
	assert.Equal(t, testProcessor.requestHandler.executionContext.SpanID, executionSpan.SpanID)
	assert.Equal(t, testProcessor.requestHandler.executionContext.parentID, executionSpan.ParentID)
	assert.Equal(t, int32(testProcessor.requestHandler.executionContext.SamplingPriority), executionChunkPriority)
	assert.Equal(t, startInvocationTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithLambdaLibrary(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
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
			executionContext: &ExecutionStartInfo{
				startTime: startInvocationTime,
				TraceID:   123,
				SpanID:    1,
			},
		},
	}

	testProcessor.OnInvokeEnd(&endDetails)

	assert.Equal(t, (*api.Payload)(nil), tracePayload)
}

func TestCompleteInferredSpanWithStartTime(t *testing.T) {
	defer os.Unsetenv(functionNameEnvVar)
	os.Setenv(functionNameEnvVar, "TestFunction")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
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

	testProcessor := LifecycleProcessor{
		ExtraTags:            extraTags,
		ProcessTrace:         mockProcessTrace,
		DetectLambdaLibrary:  mockDetectLambdaLibrary,
		Demux:                demux,
		InferredSpansEnabled: true,
		requestHandler: &RequestHandler{
			executionContext: &ExecutionStartInfo{
				startTime:        startInvocationTime,
				TraceID:          123,
				SpanID:           1,
				parentID:         3,
				SamplingPriority: samplingPriority,
			},
			triggerTags: make(map[string]string),
		},
	}

	testProcessor.requestHandler.CreateNewInferredSpan(startInferredSpan)
	testProcessor.requestHandler.inferredSpanContext.Span.TraceID = 123
	testProcessor.requestHandler.inferredSpanContext.Span.SpanID = 3
	testProcessor.requestHandler.inferredSpanContext.Span.Start = startInferredSpan.UnixNano()

	testProcessor.OnInvokeEnd(&endDetails)

	completedInferredSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, testProcessor.requestHandler.inferredSpanContext.Span.Start, completedInferredSpan.Start)
}

func TestCompleteInferredSpanWithOutStartTime(t *testing.T) {
	defer os.Unsetenv(functionNameEnvVar)
	os.Setenv(functionNameEnvVar, "TestFunction")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
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
		ExtraTags:            extraTags,
		ProcessTrace:         mockProcessTrace,
		DetectLambdaLibrary:  mockDetectLambdaLibrary,
		Demux:                demux,
		InferredSpansEnabled: true,
		requestHandler: &RequestHandler{
			executionContext: &ExecutionStartInfo{
				startTime:        startInvocationTime,
				TraceID:          123,
				SpanID:           1,
				parentID:         3,
				SamplingPriority: samplingPriority,
			},
		},
	}

	testProcessor.requestHandler.CreateNewInferredSpan(time.Time{})
	testProcessor.requestHandler.inferredSpanContext.Span.TraceID = 123
	testProcessor.requestHandler.inferredSpanContext.Span.SpanID = 3

	testProcessor.OnInvokeEnd(&endDetails)

	// If our logic is correct this will actually be the execution span
	// and the start time is expected to be the invocation start time,
	// not the inferred span start time.
	completedInferredSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, startInvocationTime.UnixNano(), completedInferredSpan.Start)
}
func TestTriggerTypesLifecycleEventForAPIGatewayRest(t *testing.T) {

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	extraTags, demux, mockDetectLambdaLibrary := triggerTestInitMocks()
	testProcessor := triggerTypesSetProcessorInit(extraTags, mockProcessTrace, demux, mockDetectLambdaLibrary)
	startTime, endTime := triggerTypesSetStartEndTimes()

	startDetails := setStartDetailsForTriggerTest(startTime, "api-gateway.json")
	endDetails := setEndDetailsForTriggerTest(endTime)

	testProcessor.OnInvokeStart(&startDetails)
	t.Logf("%+v", startDetails.InvokeEventRawPayload)
	t.Log(testProcessor.requestHandler.triggerTags)

	testProcessor.OnInvokeEnd(&endDetails)

	span := tracePayload.TracerPayload.Chunks[0].Spans[0]
	t.Log(span)
}

func getEventFromFile(filename string) []byte {
	event, err := os.ReadFile("../trace/testdata/event_samples/" + filename)
	if err != nil {
		panic(err)
	}
	return event
}

func triggerTestInitMocks() (*logs.Tags, func() bool, *aggregator.TestAgentDemultiplexer) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	mockDetectLambdaLibrary := func() bool { return false }

	return extraTags, mockDetectLambdaLibrary, demux
}

func triggerTypesSetProcessorInit(
	extraTags *logs.Tags,
	mockProcessTrace func(p *api.Payload),
	mockDetectLambdaLibrary func() bool,
	demux *aggregator.TestAgentDemultiplexer) *LifecycleProcessor {

	testProcessor := &LifecycleProcessor{
		ExtraTags:            extraTags,
		ProcessTrace:         mockProcessTrace,
		DetectLambdaLibrary:  mockDetectLambdaLibrary,
		Demux:                demux,
		InferredSpansEnabled: false,
	}
	return testProcessor

}

func triggerTypesSetStartEndTimes() (time.Time, time.Time) {
	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	return startInvocationTime, endInvocationTime
}

func setStartDetailsForTriggerTest(startTime time.Time, filename string) InvocationStartDetails {
	return InvocationStartDetails{
		StartTime:             startTime,
		InvokeEventRawPayload: string(getEventFromFile(filename)),
		InvokeEventHeaders: LambdaInvokeEventHeaders{
			TraceID:          "100",
			ParentID:         "50",
			SamplingPriority: "1",
		},
	}
}

func setEndDetailsForTriggerTest(endTime time.Time) InvocationEndDetails {
	return InvocationEndDetails{
		EndTime:   endTime,
		RequestID: "test-request-id",
	}
}
