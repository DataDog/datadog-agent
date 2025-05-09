// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	json "github.com/json-iterator/go"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
)

var /* const */ runtimeRegex = regexp.MustCompile(`^(dotnet|go|java|ruby)(\d+(\.\d+)*|\d+(\.x))$`)

// ExecutionStartInfo is saved information from when an execution span was started
type ExecutionStartInfo struct {
	startTime         time.Time
	TraceID           uint64
	TraceIDUpper64Hex string
	SpanID            uint64
	parentID          uint64
	requestPayload    []byte
	SamplingPriority  sampler.SamplingPriority
}

// startExecutionSpan records information from the start of the invocation.
// It should be called at the start of the invocation.
func (lp *LifecycleProcessor) startExecutionSpan(event interface{}, rawPayload []byte, startDetails *InvocationStartDetails) {
	inferredSpan := lp.GetInferredSpan()
	executionContext := lp.GetExecutionInfo()
	executionContext.requestPayload = rawPayload
	executionContext.startTime = startDetails.StartTime

	traceContext, err := lp.Extractor.Extract(event, rawPayload)
	if err != nil {
		traceContext = lp.Extractor.ExtractFromLayer(startDetails.InvokeEventHeaders).TraceContext
	}

	if traceContext != nil {
		executionContext.TraceID = traceContext.TraceID
		executionContext.parentID = traceContext.ParentID
		executionContext.SamplingPriority = traceContext.SamplingPriority
		if lp.InferredSpansEnabled && inferredSpan.Span.Start != 0 {
			inferredSpan.Span.TraceID = traceContext.TraceID
			inferredSpan.Span.ParentID = traceContext.ParentID
		}
		if traceContext.TraceIDUpper64Hex != "" {
			executionContext.TraceIDUpper64Hex = traceContext.TraceIDUpper64Hex
			lp.requestHandler.SetMetaTag(Upper64BitsTag, traceContext.TraceIDUpper64Hex)
		} else {
			delete(lp.requestHandler.triggerTags, Upper64BitsTag)
		}
	} else {
		executionContext.TraceID = 0
		executionContext.parentID = 0
		executionContext.SamplingPriority = sampler.PriorityNone
	}
	if lp.InferredSpansEnabled && inferredSpan.Span.Start != 0 {
		executionContext.parentID = inferredSpan.Span.SpanID
	}
}

// endExecutionSpan builds the function execution span and sends it to the intake.
// It should be called at the end of the invocation.
func (lp *LifecycleProcessor) endExecutionSpan(endDetails *InvocationEndDetails) *pb.Span {
	executionContext := lp.GetExecutionInfo()
	start := executionContext.startTime.UnixNano()

	traceID := executionContext.TraceID
	spanID := executionContext.SpanID
	// If we fail to receive the trace and span IDs from the tracer during a timeout we create it ourselves
	if endDetails.IsTimeout && traceID == 0 {
		traceID = random.Random.Uint64()
		lp.requestHandler.executionInfo.TraceID = traceID
	}
	if endDetails.IsTimeout && spanID == 0 {
		spanID = random.Random.Uint64()
	}

	executionSpan := &pb.Span{
		Service:  "aws.lambda", // will be replaced by the span processor
		Name:     "aws.lambda",
		Resource: os.Getenv(functionNameEnvVar),
		Type:     "serverless",
		TraceID:  traceID,
		SpanID:   spanID,
		ParentID: executionContext.parentID,
		Start:    start,
		Duration: endDetails.EndTime.UnixNano() - start,
		Meta:     lp.requestHandler.triggerTags,
		Metrics:  lp.requestHandler.triggerMetrics,
	}
	if executionContext.TraceIDUpper64Hex != "" {
		executionSpan.Meta[Upper64BitsTag] = executionContext.TraceIDUpper64Hex
	}
	executionSpan.Meta["request_id"] = endDetails.RequestID
	executionSpan.Meta["cold_start"] = fmt.Sprintf("%t", endDetails.ColdStart)
	if endDetails.ProactiveInit {
		executionSpan.Meta["proactive_initialization"] = fmt.Sprintf("%t", endDetails.ProactiveInit)
	}
	langMatches := runtimeRegex.FindStringSubmatch(endDetails.Runtime)
	if len(langMatches) >= 2 {
		executionSpan.Meta["language"] = langMatches[1]
	}
	captureLambdaPayloadEnabled := pkgconfigsetup.Datadog().GetBool("capture_lambda_payload")
	if captureLambdaPayloadEnabled {
		capturePayloadMaxDepth := pkgconfigsetup.Datadog().GetInt("capture_lambda_payload_max_depth")
		requestPayloadJSON := make(map[string]interface{})
		if err := json.Unmarshal(executionContext.requestPayload, &requestPayloadJSON); err != nil {
			log.Debugf("[lifecycle] Failed to parse request payload: %v", err)
			executionSpan.Meta["function.request"] = string(executionContext.requestPayload)
		} else {
			capturePayloadAsTags(requestPayloadJSON, executionSpan, "function.request", 0, capturePayloadMaxDepth)
		}
		if endDetails.ResponseRawPayload != nil {
			responsePayloadJSON := make(map[string]interface{})
			if err := json.Unmarshal(endDetails.ResponseRawPayload, &responsePayloadJSON); err != nil {
				log.Debugf("[lifecycle] Failed to parse response payload: %v", err)
				executionSpan.Meta["function.response"] = string(endDetails.ResponseRawPayload)
			} else {
				capturePayloadAsTags(responsePayloadJSON, executionSpan, "function.response", 0, capturePayloadMaxDepth)
			}
		}
	}
	if endDetails.IsError {
		executionSpan.Error = 1

		if len(endDetails.ErrorMsg) > 0 {
			executionSpan.Meta["error.msg"] = endDetails.ErrorMsg
		}
		if len(endDetails.ErrorType) > 0 {
			executionSpan.Meta["error.type"] = endDetails.ErrorType
		}
		if len(endDetails.ErrorStack) > 0 {
			executionSpan.Meta["error.stack"] = endDetails.ErrorStack
		}

		if endDetails.IsTimeout {
			executionSpan.Meta["error.type"] = "Impending Timeout"
			executionSpan.Meta["error.msg"] = "Datadog detected an Impending Timeout"
		}
	}

	return executionSpan
}

// completeInferredSpan finishes the inferred span and passes it
// as an API payload to be processed by the trace agent
func (lp *LifecycleProcessor) completeInferredSpan(inferredSpan *inferredspan.InferredSpan, endTime time.Time, isError bool) *pb.Span {
	durationIsSet := inferredSpan.Span.Duration != 0
	if inferredSpan.IsAsync {
		// SNSSQS span duration is set in invocationlifecycle/init.go
		if !durationIsSet {
			inferredSpan.Span.Duration = inferredSpan.CurrentInvocationStartTime.UnixNano() - inferredSpan.Span.Start
		}
	} else {
		inferredSpan.Span.Duration = endTime.UnixNano() - inferredSpan.Span.Start
	}
	if isError {
		inferredSpan.Span.Error = 1
	}

	inferredSpan.Span.TraceID = lp.GetExecutionInfo().TraceID
	if lp.GetExecutionInfo().TraceIDUpper64Hex != "" {
		inferredSpan.Span.Meta[Upper64BitsTag] = lp.GetExecutionInfo().TraceIDUpper64Hex
	}

	return inferredSpan.Span
}

func (lp *LifecycleProcessor) processTrace(spans []*pb.Span) {
	traceChunk := &pb.TraceChunk{
		Origin:   "lambda",
		Spans:    spans,
		Priority: int32(lp.GetExecutionInfo().SamplingPriority),
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	lp.ProcessTrace(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}

// ParseLambdaPayload removes extra data sent by the proxy that surrounds
// a JSON payload. For example, for `a5a{"event":"aws_lambda"...}0` it would remove
// a5a at the front and 0 at the end, and just leave a correct JSON payload.
func ParseLambdaPayload(rawPayload []byte) []byte {
	leftIndex := bytes.Index(rawPayload, []byte("{"))
	rightIndex := bytes.LastIndex(rawPayload, []byte("}"))
	if leftIndex == -1 || rightIndex == -1 {
		return rawPayload
	}
	return rawPayload[leftIndex : rightIndex+1]
}

func convertStrToUnit64(s string) (uint64, error) {
	num, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		log.Debugf("Error while converting %s, failing with : %s", s, err)
	}
	return num, err
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

	upper64hex := getUpper64Hex(headers.Get(TraceTagsHeader))
	if upper64hex != "" {
		executionContext.TraceIDUpper64Hex = upper64hex
	}
}

// searches traceTags for "_dd.p.tid=[upper 64 bits hex]" and returns that value if found
func getUpper64Hex(traceTags string) string {
	if !strings.Contains(traceTags, Upper64BitsTag) {
		return ""
	}
	kvpairs := strings.Split(traceTags, ",")
	for _, pair := range kvpairs {
		if !strings.Contains(pair, Upper64BitsTag) {
			continue
		}
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return ""
		}
		return kv[1]
	}
	return ""
}

// InjectSpanID injects the spanId
func InjectSpanID(executionContext *ExecutionStartInfo, headers http.Header) {
	if value, err := strconv.ParseUint(headers.Get(SpanIDHeader), 10, 64); err == nil {
		log.Debugf("injecting spanID = %v", value)
		executionContext.SpanID = value
	}
}

func capturePayloadAsTags(value interface{}, targetSpan *pb.Span, key string, depth int, maxDepth int) {
	if key == "" {
		return
	}
	if value == nil {
		targetSpan.Meta[key] = ""
		return
	}
	if depth >= maxDepth {
		switch value := value.(type) {
		case map[string]interface{}:
			targetSpan.Meta[key] = convertJSONToString(value)
		default:
			targetSpan.Meta[key] = fmt.Sprintf("%v", value)
		}
		return
	}
	switch value := value.(type) {
	case string:
		var innerPayloadJSON map[string]interface{}
		err := json.Unmarshal([]byte(value), &innerPayloadJSON)
		if err != nil {
			targetSpan.Meta[key] = fmt.Sprintf("%v", value)
		} else {
			capturePayloadAsTags(innerPayloadJSON, targetSpan, key, depth, maxDepth)
		}
	case []byte:
		var innerPayloadJSON map[string]interface{}
		err := json.Unmarshal(value, &innerPayloadJSON)
		if err != nil {
			targetSpan.Meta[key] = fmt.Sprintf("%v", value)
		} else {
			capturePayloadAsTags(innerPayloadJSON, targetSpan, key, depth, maxDepth)
		}
	case map[string]interface{}:
		if len(value) == 0 {
			targetSpan.Meta[key] = "{}"
			return
		}
		for innerKey, value := range value {
			capturePayloadAsTags(value, targetSpan, key+"."+innerKey, depth+1, maxDepth)
		}
	case []interface{}:
		if len(value) == 0 {
			targetSpan.Meta[key] = "[]"
			return
		}
		for i, innerValue := range value {
			capturePayloadAsTags(innerValue, targetSpan, key+"."+strconv.Itoa(i), depth+1, maxDepth)
		}
	default:
		targetSpan.Meta[key] = fmt.Sprintf("%v", value)
	}
}

func convertJSONToString(payloadJSON interface{}) string {
	jsonData, err := json.Marshal(payloadJSON)
	if err != nil {
		return fmt.Sprintf("%v", payloadJSON)
	}
	return string(jsonData)
}
