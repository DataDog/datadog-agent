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
	startTime        time.Time
	traceID          uint64
	spanID           uint64
	parentID         uint64
	requestPayload   string
	samplingPriority sampler.SamplingPriority
}
type invocationPayload struct {
	Headers map[string]string `json:"headers"`
}

// currentExecutionInfo represents information from the start of the current execution span
var currentExecutionInfo executionStartInfo

// startExecutionSpan records information from the start of the invocation.
// It should be called at the start of the invocation.
func startExecutionSpan(startTime time.Time, rawPayload string, invokeEventHeaders LambdaInvokeEventHeaders, samplingRate float64) {
	currentExecutionInfo.startTime = startTime
	currentExecutionInfo.traceID = random.Uint64()
	currentExecutionInfo.spanID = random.Uint64()
	currentExecutionInfo.parentID = 0

	payload := convertRawPayload(rawPayload)

	currentExecutionInfo.requestPayload = rawPayload

	var traceID, parentID uint64
	var e1, e2 error

	if payload.Headers != nil {
		traceID, e1 = convertStrToUnit64(payload.Headers[TraceIDHeader])
		parentID, e2 = convertStrToUnit64(payload.Headers[ParentIDHeader])
	} else if invokeEventHeaders.TraceID != "" { // trace context from a direct invocation
		traceID, e1 = convertStrToUnit64(invokeEventHeaders.TraceID)
		parentID, e2 = convertStrToUnit64(invokeEventHeaders.ParentID)
	}

	if e1 == nil && traceID != 0 {
		currentExecutionInfo.traceID = traceID
	}

	if e2 == nil && parentID != 0 {
		currentExecutionInfo.parentID = parentID
	}

	currentExecutionInfo.samplingPriority = computeSamplingPriority(currentExecutionInfo.traceID, samplingRate, payload.Headers[SamplingPriorityHeader], invokeEventHeaders.SamplingPriority)
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
		Priority: int32(currentExecutionInfo.samplingPriority),
		Spans:    []*pb.Span{executionSpan},
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

func convertStrToUnit64(s string) (uint64, error) {
	num, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		log.Debug("Error with string conversion of trace or parent ID")
	}

	return num, err
}

func computeSamplingPriority(traceID uint64, samplingRate float64, header string, directInvokeHeader string) sampler.SamplingPriority {
	var samplingPriority sampler.SamplingPriority
	if v, err := strconv.ParseInt(header, 10, 8); err == nil {
		// if the current lambda invocation is not the head of the trace, we need to propagate the sampling decision
		samplingPriority = sampler.SamplingPriority(v)
	} else {
		// try to look for direction invocation headers
		if v, err := strconv.ParseInt(directInvokeHeader, 10, 8); err == nil {
			samplingPriority = sampler.SamplingPriority(v)
		} else {
			// could no find sampling priority, computing a new one
			samplingPriority = generateSamplingPriority(traceID, samplingRate)
		}
	}
	return samplingPriority
}

func generateSamplingPriority(traceID uint64, samplingRate float64) sampler.SamplingPriority {
	samplingPriority := sampler.PriorityAutoKeep
	if !sampler.SampleByRate(traceID, samplingRate) {
		samplingPriority = sampler.PriorityUserDrop
	}
	return samplingPriority
}

// TraceID returns the current TraceID
func TraceID() uint64 {
	return currentExecutionInfo.traceID
}

// SpanID returns the current SpanID
func SpanID() uint64 {
	return currentExecutionInfo.spanID
}

// SamplingPriority returns the current samplingPriority
func SamplingPriority() sampler.SamplingPriority {
	return currentExecutionInfo.samplingPriority
}
