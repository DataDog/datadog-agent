// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-lambda-go/events"
)

// LifecycleProcessor is a InvocationProcessor implementation
type LifecycleProcessor struct {
	ExtraTags            *serverlessLog.Tags
	ProcessTrace         func(p *api.Payload)
	Demux                aggregator.Demultiplexer
	DetectLambdaLibrary  func() bool
	InferredSpansEnabled bool

	requestHandler *RequestHandler
}

// RequestHandler is the struct that stores information about the trace,
// inferred span, and tags about the current invocation
type RequestHandler struct {
	executionInfo *ExecutionStartInfo
	inferredSpan  *inferredspan.InferredSpan
	triggerTags   map[string]string
}

// OnInvokeStart is the hook triggered when an invocation has started
func (lp *LifecycleProcessor) OnInvokeStart(startDetails *InvocationStartDetails) {
	log.Debug("[lifecycle] onInvokeStart ------")
	log.Debugf("[lifecycle] Invocation has started at: %v", startDetails.StartTime)
	log.Debugf("[lifecycle] Invocation invokeEvent payload is: %s", startDetails.InvokeEventRawPayload)
	log.Debug("[lifecycle] ---------------------------------------")

	lambdaPayloadString := parseLambdaPayload(startDetails.InvokeEventRawPayload)

	log.Debugf("Parsed payload string: %v", lambdaPayloadString)

	lowercaseEventPayload, err := trigger.Unmarshal(strings.ToLower(lambdaPayloadString))
	if err != nil {
		log.Debugf("[lifecycle] Failed to parse event payload: %v", err)
	}

	eventType := trigger.GetEventType(lowercaseEventPayload)
	if err != nil {
		log.Debugf("[lifecycle] Failed to extract event type: %v", err)
	}

	// Initialize basic values in the request handler
	lp.newRequest(startDetails.InvokeEventRawPayload, startDetails.StartTime)

	payloadBytes := []byte(lambdaPayloadString)
	region, account, arnParseErr := trigger.ParseArn(startDetails.InvokedFunctionARN)
	if err != nil {
		log.Debugf("[lifecycle] Error parsing ARN: %v", err)
	}

	switch eventType {
	case trigger.APIGatewayEvent:
		var event events.APIGatewayProxyRequest
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromAPIGatewayEvent(event, region)
		}
	case trigger.APIGatewayV2Event:
		var event events.APIGatewayV2HTTPRequest
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromAPIGatewayV2Event(event, region)
		}
	case trigger.APIGatewayWebsocketEvent:
		var event events.APIGatewayWebsocketProxyRequest
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromAPIGatewayWebsocketEvent(event, region)
		}
	case trigger.ALBEvent:
		var event events.ALBTargetGroupRequest
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromALBEvent(event)
		}
	case trigger.CloudWatchEvent:
		var event events.CloudWatchEvent
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromCloudWatchEvent(event)
		}
	case trigger.CloudWatchLogsEvent:
		var event events.CloudwatchLogsEvent
		if err := json.Unmarshal(payloadBytes, &event); err == nil && arnParseErr == nil {
			lp.initFromCloudWatchLogsEvent(event, region, account)
		}
	case trigger.DynamoDBStreamEvent:
		var event events.DynamoDBEvent
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromDynamoDBStreamEvent(event)
		}
	case trigger.KinesisStreamEvent:
		var event events.KinesisEvent
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromKinesisStreamEvent(event)
		}
	case trigger.S3Event:
		var event events.S3Event
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromS3Event(event)
		}
	case trigger.SNSEvent:
		var event events.SNSEvent
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromSNSEvent(event)
		}
	case trigger.SQSEvent:
		var event events.SQSEvent
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromSQSEvent(event)
		}
	case trigger.LambdaFunctionURLEvent:
		var event events.LambdaFunctionURLRequest
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			lp.initFromLambdaFunctionURLEvent(event)
		}
	default:
		log.Debug("Skipping adding trigger types and inferred spans as a non-supported payload was received.")
	}

	if !lp.DetectLambdaLibrary() {
		startExecutionSpan(lp.GetExecutionInfo(), lp.GetInferredSpan(), startDetails.StartTime, lambdaPayloadString, startDetails.InvokeEventHeaders, lp.InferredSpansEnabled)
	}
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (lp *LifecycleProcessor) OnInvokeEnd(endDetails *InvocationEndDetails) {
	log.Debug("[lifecycle] onInvokeEnd ------")
	log.Debugf("[lifecycle] Invocation has finished at: %v", endDetails.EndTime)
	log.Debugf("[lifecycle] Invocation isError is: %v", endDetails.IsError)
	log.Debug("[lifecycle] ---------------------------------------")

	statusCode, err := trigger.GetStatusCodeFromHTTPResponse([]byte(parseLambdaPayload(endDetails.ResponseRawPayload)))
	if err != nil {
		log.Debugf("[lifecycle] Couldn't parse response payload: %v", err)
	}

	// This will only add the status code if it comes from an HTTP-like
	// response struct
	lp.addTag("http.status_code", statusCode)

	if !lp.DetectLambdaLibrary() {
		log.Debug("Creating and sending function execution span for invocation")
		endExecutionSpan(lp.GetExecutionInfo(), lp.requestHandler.triggerTags, lp.ProcessTrace, endDetails.RequestID, endDetails.EndTime, endDetails.IsError, endDetails.ResponseRawPayload)

		if lp.InferredSpansEnabled {
			log.Debug("[lifecycle] Attempting to complete the inferred span")
			log.Debugf("[lifecycle] Inferred span context: %+v", lp.GetInferredSpan().Span)
			if lp.GetInferredSpan().Span.Start != 0 {
				lp.GetInferredSpan().CompleteInferredSpan(lp.ProcessTrace, lp.GetTags(), endDetails.EndTime, endDetails.IsError, lp.GetExecutionInfo().TraceID, lp.GetExecutionInfo().SamplingPriority)
				log.Debugf("[lifecycle] The inferred span attributes are: %v", lp.GetInferredSpan())
			} else {
				log.Debug("[lifecyle] Failed to complete inferred span due to a missing start time. Please check that the event payload was received with the appropriate data")
			}
		}
	}

	if endDetails.IsError {
		serverlessMetrics.SendErrorsEnhancedMetric(
			lp.ExtraTags.Tags, endDetails.EndTime, lp.Demux,
		)
	}
}

// GetTags returns the tagset of the currently executing lambda function
func (lp *LifecycleProcessor) GetTags() map[string]string {
	return lp.requestHandler.triggerTags
}

// GetExecutionInfo returns the trace and payload information of
// the currently executing lambda function
func (lp *LifecycleProcessor) GetExecutionInfo() *ExecutionStartInfo {
	return lp.requestHandler.executionInfo
}

// GetInferredSpan returns the generated inferred span of the
// currently executing lambda function
func (lp *LifecycleProcessor) GetInferredSpan() *inferredspan.InferredSpan {
	return lp.requestHandler.inferredSpan
}

// NewRequest initializes basic information about the current request
// on the LifecycleProcessor
func (lp *LifecycleProcessor) newRequest(lambdaPayloadString string, startTime time.Time) {
	if lp.requestHandler == nil {
		lp.requestHandler = &RequestHandler{}
	}
	lp.requestHandler.executionInfo = &ExecutionStartInfo{
		requestPayload: lambdaPayloadString,
		startTime:      startTime,
	}
	lp.requestHandler.inferredSpan = &inferredspan.InferredSpan{
		CurrentInvocationStartTime: startTime,
		Span: &pb.Span{
			SpanID: random.Random.Uint64(),
		},
	}
	lp.requestHandler.triggerTags = make(map[string]string)
}

func (lp *LifecycleProcessor) addTags(tagSet map[string]string) {
	for k, v := range tagSet {
		lp.requestHandler.triggerTags[k] = v
	}
}

func (lp *LifecycleProcessor) addTag(key string, value string) {
	if value == "" {
		return
	}
	lp.requestHandler.triggerTags[key] = value
}
