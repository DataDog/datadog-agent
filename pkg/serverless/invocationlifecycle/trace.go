// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
)

// ExecutionStartInfo is saved information from when an execution span was started
type ExecutionStartInfo struct {
	startTime        time.Time
	TraceID          uint64
	SpanID           uint64
	parentID         uint64
	requestPayload   []byte
	SamplingPriority sampler.SamplingPriority
}

type invocationPayload struct {
	Headers map[string]string `json:"headers"`
}

// startExecutionSpan records information from the start of the invocation.
// It should be called at the start of the invocation.
func startExecutionSpan(executionContext *ExecutionStartInfo, inferredSpan *inferredspan.InferredSpan, rawPayload []byte, startDetails *InvocationStartDetails, inferredSpansEnabled bool) {
	payload := convertRawPayload(rawPayload)
	executionContext.requestPayload = rawPayload
	executionContext.startTime = startDetails.StartTime

	if inferredSpansEnabled && inferredSpan.Span.Start != 0 {
		executionContext.TraceID = inferredSpan.Span.TraceID
		executionContext.parentID = inferredSpan.Span.SpanID
	}

	if payload.Headers != nil {

		traceID, err := strconv.ParseUint(payload.Headers[TraceIDHeader], 0, 64)
		if err != nil {
			log.Debug("Unable to parse traceID from payload headers")
		} else {
			executionContext.TraceID = traceID
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
				executionContext.parentID = parentID
			}
		}
	} else if startDetails.InvokeEventHeaders.TraceID != "" { // trace context from a direct invocation
		traceID, err := strconv.ParseUint(startDetails.InvokeEventHeaders.TraceID, 0, 64)
		if err != nil {
			log.Debug("Unable to parse traceID from invokeEventHeaders")
		} else {
			executionContext.TraceID = traceID
		}

		parentID, err := strconv.ParseUint(startDetails.InvokeEventHeaders.ParentID, 0, 64)
		if err != nil {
			log.Debug("Unable to parse parentID from invokeEventHeaders")
		} else {
			executionContext.parentID = parentID
		}
	}
	executionContext.SamplingPriority = getSamplingPriority(payload.Headers[SamplingPriorityHeader], startDetails.InvokeEventHeaders.SamplingPriority)
}

// endExecutionSpan builds the function execution span and sends it to the intake.
// It should be called at the end of the invocation.
func endExecutionSpan(executionContext *ExecutionStartInfo, triggerTags map[string]string, triggerMetrics map[string]float64, processTrace func(p *api.Payload), endDetails *InvocationEndDetails) {
	duration := endDetails.EndTime.UnixNano() - executionContext.startTime.UnixNano()

	executionSpan := &pb.Span{
		Service:  "aws.lambda", // will be replaced by the span processor
		Name:     "aws.lambda",
		Resource: os.Getenv(functionNameEnvVar),
		Type:     "serverless",
		TraceID:  executionContext.TraceID,
		SpanID:   executionContext.SpanID,
		ParentID: executionContext.parentID,
		Start:    executionContext.startTime.UnixNano(),
		Duration: duration,
		Meta:     triggerTags,
		Metrics:  triggerMetrics,
	}
	executionSpan.Meta["request_id"] = endDetails.RequestID

	captureLambdaPayloadEnabled := config.Datadog.GetBool("capture_lambda_payload")
	if captureLambdaPayloadEnabled {
		executionSpan.Meta["function.request"] = string(executionContext.requestPayload)
		executionSpan.Meta["function.response"] = string(endDetails.ResponseRawPayload)
	}

	if endDetails.IsError {
		executionSpan.Error = 1
	}

	traceChunk := &pb.TraceChunk{
		Priority: int32(executionContext.SamplingPriority),
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

// parseLambdaFunction removes extra data sent by the proxy that surrounds
// a JSON payload. For example, for `a5a{"event":"aws_lambda"...}0` it would remove
// a5a at the front and 0 at the end, and just leave a correct JSON payload.
func parseLambdaPayload(rawPayload []byte) []byte {
	leftIndex := bytes.Index(rawPayload, []byte("{"))
	rightIndex := bytes.LastIndex(rawPayload, []byte("}"))
	if leftIndex == -1 || rightIndex == -1 {
		return rawPayload
	}
	return rawPayload[leftIndex : rightIndex+1]
}

func convertRawPayload(payloadString []byte) invocationPayload {
	payload := invocationPayload{}

	err := json.Unmarshal(payloadString, &payload)
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

// InjectContext injects the context
func InjectContext(executionContext *ExecutionStartInfo, headers http.Header) {
	if value, err := convertStrToUnit64(headers.Get(TraceIDHeader)); err == nil {
		log.Debugf("injecting traceID = %v", value)
		executionContext.TraceID = value
	}
	if value, err := convertStrToUnit64(headers.Get(ParentIDHeader)); err == nil {
		log.Debugf("injecting parentId = %v", value)
		executionContext.parentID = value
	}
	if value, err := strconv.ParseInt(headers.Get(SamplingPriorityHeader), 10, 8); err == nil {
		log.Debugf("injecting samplingPriority = %v", value)
		executionContext.SamplingPriority = sampler.SamplingPriority(value)
	}
}

// InjectSpanID injects the spanId
func InjectSpanID(executionContext *ExecutionStartInfo, headers http.Header) {
	if value, err := strconv.ParseUint(headers.Get(SpanIDHeader), 10, 64); err == nil {
		log.Debugf("injecting spanID = %v", value)
		executionContext.SpanID = value
	}
}
