// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	rand "github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
)

// executionStartInfo is saved information from when an execution span was started
type executionStartInfo struct {
	startTime time.Time
	traceID   uint64
	spanID    uint64
	parentID  uint64
	// reference for nil checking
	samplingPriority *sampler.SamplingPriority
	requestPayload   string
}
type invocationPayload struct {
	Headers map[string]string `json:"headers"`
}

// currentExecutionInfo represents information from the start of the current execution span
var currentExecutionInfo executionStartInfo

// startExecutionSpan records information from the start of the invocation.
// It should be called at the start of the invocation.
func startExecutionSpan(startTime time.Time, rawPayload string, invokeEventHeaders LambdaInvokeEventHeaders, inferredSpansEnabled bool) {
	currentExecutionInfo.startTime = startTime
	currentExecutionInfo.traceID = rand.Random.Uint64()
	currentExecutionInfo.spanID = rand.Random.Uint64()
	currentExecutionInfo.parentID = 0

	payload := convertRawPayload(rawPayload)

	currentExecutionInfo.requestPayload = rawPayload

	if inferredSpansEnabled {
		currentExecutionInfo.traceID = inferredSpan.Span.TraceID
		currentExecutionInfo.parentID = inferredSpan.Span.SpanID
	}

	if payload.Headers != nil {

		traceID, err := strconv.ParseUint(payload.Headers[TraceIDHeader], 0, 64)
		if err != nil {
			log.Debug("Unable to parse traceID from payload headers")
		} else {
			currentExecutionInfo.traceID = traceID
			if inferredSpansEnabled {
				inferredSpan.Span.TraceID = traceID
			}
		}

		parentID, err := strconv.ParseUint(payload.Headers[ParentIDHeader], 0, 64)
		if err != nil {
			log.Debug("Unable to parse parentID from payload headers")
		} else {
			if inferredSpansEnabled {
				inferredSpan.Span.ParentID = parentID
			} else {
				currentExecutionInfo.parentID = parentID
			}
		}

		samplingPriority, err := strconv.ParseInt(payload.Headers[SamplingPriorityHeader], 0, 8)
		priority := sampler.SamplingPriority(int8(samplingPriority))
		if err != nil {
			log.Debug("Unable to parse samling priority from invokeEventHeaders")
		} else {
			currentExecutionInfo.samplingPriority = &priority
			if inferredSpansEnabled {
				inferredSpan.SamplingPriority = &priority
			}
		}

	} else if invokeEventHeaders.TraceID != "" { // trace context from a direct invocation
		traceID, err := strconv.ParseUint(invokeEventHeaders.TraceID, 0, 64)
		if err != nil {
			log.Debug("Unable to parse traceID from invokeEventHeaders")
		} else {
			currentExecutionInfo.traceID = traceID
		}

		parentID, err := strconv.ParseUint(invokeEventHeaders.ParentID, 0, 64)
		if err != nil {
			log.Debug("Unable to parse parentID from invokeEventHeaders")
		} else {
			currentExecutionInfo.parentID = parentID
		}
	}
}

// endExecutionSpan builds the function execution span and sends it to the intake.
// It should be called at the end of the invocation.
func endExecutionSpan(processTrace func(p *api.Payload), requestID string, endTime time.Time, isError bool, responsePayload []byte) {
	duration := endTime.UnixNano() - currentExecutionInfo.startTime.UnixNano()

	executionSpan := &pb.Span{
		Service:  "aws.lambda", // will be replaced by the span processor
		Name:     "aws.lambda",
		Resource: os.Getenv(functionNameEnvVar),
		Type:     "serverless",
		TraceID:  currentExecutionInfo.traceID,
		SpanID:   currentExecutionInfo.spanID,
		ParentID: currentExecutionInfo.parentID,
		Start:    currentExecutionInfo.startTime.UnixNano(),
		Duration: duration,
		Meta: map[string]string{
			"request_id": requestID,
		},
	}
	captureLambdaPayloadEnabled := config.Datadog.GetBool("capture_lambda_payload")
	if captureLambdaPayloadEnabled {
		executionSpan.Meta["function.request"] = currentExecutionInfo.requestPayload
		executionSpan.Meta["function.response"] = string(responsePayload)
	}

	if isError {
		executionSpan.Error = 1
	}

	traceChunk := &pb.TraceChunk{
		Spans: []*pb.Span{executionSpan},
	}

	if currentExecutionInfo.samplingPriority != nil {
		priority := *currentExecutionInfo.samplingPriority
		traceChunk.Priority = int32(priority)
	} else {
		traceChunk.Priority = int32(sampler.PriorityNone)
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	processTrace(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}

func convertRawPayload(rawPayload string) invocationPayload {
	//Need to remove unwanted text from the initial payload
	reg := regexp.MustCompile(`{(?:|(.*))*}`)
	subString := reg.FindString(rawPayload)

	payload := invocationPayload{}

	err := json.Unmarshal([]byte(subString), &payload)
	if err != nil {
		log.Debug("Could not unmarshal the invocation event payload")
	}

	return payload
}

// TraceID returns the current TraceID
func TraceID() uint64 {
	return currentExecutionInfo.traceID
}

// SpanID returns the current SpanID
func SpanID() uint64 {
	return currentExecutionInfo.spanID
}
