// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestInferredSpanCheck(t *testing.T) {
	inferredSpan := &pb.Span{
		Meta: map[string]string{"_inferred_span.tag_source": "self", "C": "D"},
	}
	lambdaSpanTagged := &pb.Span{
		Meta: map[string]string{"_inferred_span.tag_source": "lambda", "C": "D"},
	}
	lambdaSpanUntagged := &pb.Span{
		Meta: map[string]string{"foo": "bar", "C": "D"},
	}

	checkInferredSpan := CheckIsInferredSpan(inferredSpan)
	checkLambdaSpanTagged := CheckIsInferredSpan(lambdaSpanTagged)
	checkLambdaSpanUntagged := CheckIsInferredSpan(lambdaSpanUntagged)

	assert.True(t, checkInferredSpan)
	assert.False(t, checkLambdaSpanTagged)
	assert.False(t, checkLambdaSpanUntagged)
}

func TestFilterFunctionTags(t *testing.T) {
	tagsToFilter := map[string]string{
		"_inferred_span.tag_source": "self",
		"extra":                     "tag",
		"tag1":                      "value1",
		"functionname":              "test",
		"function_arn":              "test",
		"executedversion":           "test",
		"runtime":                   "test",
		"memorysize":                "test",
		"architecture":              "test",
		"env":                       "test",
		"version":                   "test",
		"service":                   "test",
		"region":                    "test",
		"account_id":                "test",
		"aws_account":               "test",
	}

	mockConfig := config.Mock()
	mockConfig.Set("tags", []string{"tag1:value1"})
	mockConfig.Set("extra_tags", []string{"extra:tag"})

	filteredTags := FilterFunctionTags(tagsToFilter)

	assert.Equal(t, filteredTags, map[string]string{
		"_inferred_span.tag_source": "self",
		"account_id":                "test",
		"aws_account":               "test",
		"region":                    "test",
	})

	// ensure DD_TAGS are filtered out
	assert.NotEqual(t, filteredTags["tag1"], "value1")

	// ensure DD_EXTRA_TAGS are filtered out
	assert.NotEqual(t, filteredTags["extra"], "tag")

	// ensure all function specific tags are filtered out
	assert.NotEqual(t, filteredTags["functionname"], "test")
	assert.NotEqual(t, filteredTags["function_arn"], "test")
	assert.NotEqual(t, filteredTags["executedversion"], "test")
	assert.NotEqual(t, filteredTags["runtime"], "test")
	assert.NotEqual(t, filteredTags["memorysize"], "test")
	assert.NotEqual(t, filteredTags["env"], "test")
	assert.NotEqual(t, filteredTags["version"], "test")
	assert.NotEqual(t, filteredTags["service"], "test")

	// ensure generic aws tags are not filtered out
	assert.Equal(t, filteredTags["region"], "test")
	assert.Equal(t, filteredTags["account_id"], "test")
	assert.Equal(t, filteredTags["aws_account"], "test")
}

func TestCompleteInferredSpanWithNoError(t *testing.T) {

	startTime := time.Now()

	inferredSpan := GenerateInferredSpan(time.Now())
	inferredSpan.Span.TraceID = 2350923428932752492
	inferredSpan.Span.SpanID = 1304592378509342580
	inferredSpan.Span.Start = startTime.UnixNano()
	inferredSpan.Span.Name = "aws.mock"
	inferredSpan.Span.Service = "aws.mock"
	inferredSpan.Span.Resource = "test-function"
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		Stage: "dev",
	}

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	isError := false
	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	CompleteInferredSpan(mockProcessTrace, endTime, isError, inferredSpan)
	span := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.mock", span.Name)
	assert.Equal(t, "aws.mock", span.Service)
	assert.Equal(t, "test-function", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "dev", span.Meta["stage"])
	assert.Equal(t, inferredSpan.Span.TraceID, span.TraceID)
	assert.Equal(t, inferredSpan.Span.SpanID, span.SpanID)
	assert.Equal(t, duration.Nanoseconds(), span.Duration)
	assert.Equal(t, int32(0), inferredSpan.Span.Error)
}

func TestCompleteInferredSpanWithError(t *testing.T) {

	startTime := time.Now()

	inferredSpan := GenerateInferredSpan(time.Now())
	inferredSpan.Span.TraceID = 2350923428932752492
	inferredSpan.Span.SpanID = 1304592378509342580
	inferredSpan.Span.Start = startTime.UnixNano()
	inferredSpan.Span.Name = "aws.mock"
	inferredSpan.Span.Service = "aws.mock"
	inferredSpan.Span.Resource = "test-function"
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		Stage: "dev",
	}

	duration := 1 * time.Second
	endTime := startTime.Add(duration)
	isError := true
	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	CompleteInferredSpan(mockProcessTrace, endTime, isError, inferredSpan)
	span := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.mock", span.Name)
	assert.Equal(t, "aws.mock", span.Service)
	assert.Equal(t, "test-function", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "dev", span.Meta["stage"])
	assert.Equal(t, inferredSpan.Span.TraceID, span.TraceID)
	assert.Equal(t, inferredSpan.Span.SpanID, span.SpanID)
	assert.Equal(t, duration.Nanoseconds(), span.Duration)
	assert.Equal(t, int32(1), inferredSpan.Span.Error)
}

func TestCompleteInferredSpanWithAsync(t *testing.T) {
	// Start of inferred span
	startTime := time.Now()
	duration := 2 * time.Second
	// mock invocation end time
	lambdaInvocationStartTime := startTime.Add(duration)
	inferredSpan := GenerateInferredSpan(lambdaInvocationStartTime)
	inferredSpan.IsAsync = true
	inferredSpan.Span.TraceID = 2350923428932752492
	inferredSpan.Span.SpanID = 1304592378509342580
	inferredSpan.Span.Start = startTime.UnixNano()
	inferredSpan.Span.Name = "aws.mock"
	inferredSpan.Span.Service = "aws.mock"
	inferredSpan.Span.Resource = "test-function"
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		Stage: "dev",
	}
	isError := false
	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	CompleteInferredSpan(mockProcessTrace, time.Now(), isError, inferredSpan)
	span := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.mock", span.Name)
	assert.Equal(t, "aws.mock", span.Service)
	assert.Equal(t, "test-function", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "dev", span.Meta["stage"])
	assert.Equal(t, inferredSpan.Span.TraceID, span.TraceID)
	assert.Equal(t, inferredSpan.Span.SpanID, span.SpanID)
	assert.Equal(t, duration.Nanoseconds(), span.Duration)
	assert.Equal(t, int32(0), inferredSpan.Span.Error)
}

func TestIsInferredSpansEnabledWhileTrue(t *testing.T) {
	defer unsetEnvVars()
	setEnvVars("true", "True")
	isEnabled := IsInferredSpansEnabled()
	assert.True(t, isEnabled)
}
func TestIsInferredSpansEnabledWhileFalse(t *testing.T) {
	defer unsetEnvVars()
	setEnvVars("true", "false")
	isEnabled := IsInferredSpansEnabled()
	assert.False(t, isEnabled)
}

func TestIsInferredSpansEnabledWhileInvalid(t *testing.T) {
	defer unsetEnvVars()
	setEnvVars("true", "42")
	isEnabled := IsInferredSpansEnabled()
	assert.False(t, isEnabled)

}

func unsetEnvVars() {
	os.Unsetenv("DD_TRACE_ENABLED")
	os.Unsetenv("DD_TRACE_MANAGED_SERVICES")
}

func setEnvVars(trace string, managedServices string) {
	os.Setenv("DD_TRACE_ENABLED", trace)
	os.Setenv("DD_TRACE_MANAGED_SERVICES", managedServices)
}
