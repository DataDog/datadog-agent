// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"encoding/json"
	"net/http"
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

func (esi *executionStartInfo) reset(startTime time.Time) {
	esi.startTime = startTime
	esi.traceID = 0
	esi.spanID = 0
	esi.parentID = 0
	esi.requestPayload = ""
	esi.samplingPriority = sampler.PriorityNone
}

// currentExecutionInfo represents information from the start of the current execution span
var currentExecutionInfo executionStartInfo

// startExecutionSpan records information from the start of the invocation.
// It should be called at the start of the invocation.
func startExecutionSpan(startTime time.Time, rawPayload string, invokeEventHeaders LambdaInvokeEventHeaders, inferredSpansEnabled bool) {
	currentExecutionInfo.reset(startTime)
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
	currentExecutionInfo.samplingPriority = getSamplingPriority(payload.Headers[SamplingPriorityHeader], invokeEventHeaders.SamplingPriority)
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
		log.Debugf("Error while converting %s, failing with : %s", s, err)
	}
	return num, err
}

func getSamplingPriority(header string, directInvokeHeader string) sampler.SamplingPriority {
	// default priority if nothing is found from headers or direct invocation payload
	samplingPriority := sampler.PriorityNone
	if v, err := strconv.ParseInt(header, 10, 8); err == nil {
		// if the current lambda invocation is not the head of the trace, we need to propagate the sampling decision
		samplingPriority = sampler.SamplingPriority(v)
	} else {
		// try to look for direction invocation headers
		if v, err := strconv.ParseInt(directInvokeHeader, 10, 8); err == nil {
			samplingPriority = sampler.SamplingPriority(v)
		}
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

// InjectContext injects the context
func InjectContext(headers http.Header) {
	if value, err := convertStrToUnit64(headers.Get(TraceIDHeader)); err == nil {
		log.Debug("injecting traceID = %v", value)
		currentExecutionInfo.traceID = value
	}
	if value, err := convertStrToUnit64(headers.Get(ParentIDHeader)); err == nil {
		log.Debug("injecting parentId = %v", value)
		currentExecutionInfo.parentID = value
	}
	if value, err := strconv.ParseInt(headers.Get(SamplingPriorityHeader), 10, 8); err == nil {
		log.Debug("injecting samplingPriority = %v", value)
		currentExecutionInfo.samplingPriority = sampler.SamplingPriority(value)
	}
}

// InjectSpanID injects the spanId
func InjectSpanID(headers http.Header) {
	if value, err := strconv.ParseUint(headers.Get(SpanIDHeader), 10, 64); err == nil {
		log.Debug("injecting spanID = %v", value)
		currentExecutionInfo.spanID = value
	}
}
