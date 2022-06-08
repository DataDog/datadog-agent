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

type RequestHandler struct {
	executionContext    *ExecutionStartInfo
	inferredSpanContext *inferredspan.InferredSpan
	triggerTags         map[string]string
}

func (r *RequestHandler) GetExecutionContext() *ExecutionStartInfo {
	return r.executionContext
}

func (r *RequestHandler) GetInferredSpanContext() *inferredspan.InferredSpan {
	return r.inferredSpanContext
}

func (r *RequestHandler) CreateNewExecutionContext(lambdaPayloadString string, startTime time.Time) {
	r.executionContext = &ExecutionStartInfo{
		requestPayload: lambdaPayloadString,
		startTime:      startTime,
	}
}

func (r *RequestHandler) CreateNewInferredSpan(currentInvocationStartTime time.Time) {
	r.inferredSpanContext = &inferredspan.InferredSpan{
		CurrentInvocationStartTime: currentInvocationStartTime,
		Span: &pb.Span{
			SpanID: random.Random.Uint64(),
		},
	}
}

func (r *RequestHandler) AddTags(tagSet map[string]string) {
	for k, v := range tagSet {
		r.triggerTags[k] = v
	}
}

func (r *RequestHandler) AddTag(key string, value string) {
	if value == "" {
		return
	}
	r.triggerTags[key] = value
}

// LifecycleProcessor is a InvocationProcessor implementation
type LifecycleProcessor struct {
	ExtraTags            *serverlessLog.Tags
	ProcessTrace         func(p *api.Payload)
	Demux                aggregator.Demultiplexer
	DetectLambdaLibrary  func() bool
	InferredSpansEnabled bool

	requestHandler *RequestHandler
}

// GetExecutionContext implements InvocationProcessor
func (lp *LifecycleProcessor) GetExecutionContext() *ExecutionStartInfo {
	return lp.requestHandler.executionContext
}

// DO WE NEED THESE IVAN!>?!??!?!?>!?!?
func (lp *LifecycleProcessor) GetInferredSpanContext() *inferredspan.InferredSpan {
	return lp.requestHandler.inferredSpanContext
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

	// Singleton instance of request handler
	if lp.requestHandler == nil {
		lp.requestHandler = &RequestHandler{}
	}

	// Each new request will get a new execution context and inferred span.
	// We're guaranteed by the lambda API that each invocation runs sequentially,
	// so we don't need to worry about race conditions here.
	lp.requestHandler.CreateNewExecutionContext(startDetails.InvokeEventRawPayload, startDetails.StartTime)
	lp.requestHandler.CreateNewInferredSpan(startDetails.StartTime)
	lp.requestHandler.triggerTags = make(map[string]string)

	errorFunc := func(err error) {
		if err != nil {
			log.Errorf("Error parsing lambda payload: %v", err)
		}
	}

	payloadBytes := []byte(lambdaPayloadString)

	// TODO: Fix to use context value from tracer
	// ctx := context.Background()
	// splitFunctionArn := ...ctx.Value(X)...
	splitFunctionArn := strings.Split("arn:partition:service:region:namespace:relative-id", ":")
	region := splitFunctionArn[3]
	accountID := splitFunctionArn[4]

	switch eventType {
	case trigger.APIGatewayEvent:
		var event events.APIGatewayProxyRequest
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromAPIGatewayEvent(event, region)
	case trigger.APIGatewayV2Event:
		var event events.APIGatewayV2HTTPRequest
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromAPIGatewayV2Event(event, region)
	case trigger.APIGatewayWebsocketEvent:
		var event events.APIGatewayWebsocketProxyRequest
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromAPIGatewayWebsocketEvent(event, region)
	case trigger.ALBEvent:
		var event events.ALBTargetGroupRequest
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromALBEvent(event)
	case trigger.CloudWatchEvent:
		var event events.CloudWatchEvent
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromCloudWatchEvent(event)
	case trigger.CloudWatchLogsEvent:
		var event events.CloudwatchLogsEvent
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromCloudWatchLogsEvent(event, region, accountID)
	case trigger.DynamoDBStreamEvent:
		var event events.DynamoDBEvent
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromDynamoDBStreamEvent(event)
	case trigger.KinesisStreamEvent:
		var event events.KinesisEvent
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromKinesisStreamEvent(event)
	case trigger.S3Event:
		var event events.S3Event
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromS3Event(event)
	case trigger.SNSEvent:
		var event events.SNSEvent
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromSNSEvent(event)
	case trigger.SQSEvent:
		var event events.SQSEvent
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromSQSEvent(event)
	case trigger.LambdaFunctionURLEvent:
		var event events.LambdaFunctionURLRequest
		err := json.Unmarshal(payloadBytes, &event)
		errorFunc(err)
		lp.initFromLambdaFunctionURLEvent(event)
	default:
		log.Debug("Skipping trigger types and inferred spans as a non-supported payload was received.")
	}

	if !lp.DetectLambdaLibrary() {
		// TODO: pull this functionality out and add it into the init functions
		startExecutionSpan(lp.requestHandler.executionContext, lp.requestHandler.inferredSpanContext, startDetails.StartTime, lambdaPayloadString, startDetails.InvokeEventHeaders, lp.InferredSpansEnabled)
	}
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (lp *LifecycleProcessor) OnInvokeEnd(endDetails *InvocationEndDetails) {
	log.Debug("[lifecycle] onInvokeEnd ------")
	log.Debugf("[lifecycle] Invocation has finished at: %v", endDetails.EndTime)
	log.Debugf("[lifecycle] Invocation isError is: %v", endDetails.IsError)
	log.Debug("[lifecycle] ---------------------------------------")

	if !lp.DetectLambdaLibrary() {
		log.Debug("Creating and sending function execution span for invocation")
		endExecutionSpan(lp.requestHandler.executionContext, lp.ProcessTrace, endDetails.RequestID, endDetails.EndTime, endDetails.IsError, endDetails.ResponseRawPayload)

		if lp.InferredSpansEnabled {
			log.Debug("[lifecycle] Attempting to complete the inferred span")
			log.Debugf("[lifecycle] Inferred span context: %+v", lp.requestHandler.inferredSpanContext.Span)
			if lp.requestHandler.inferredSpanContext.Span.Start != 0 {
				lp.requestHandler.inferredSpanContext.CompleteInferredSpan(lp.ProcessTrace, endDetails.EndTime, endDetails.IsError, lp.requestHandler.executionContext.TraceID, lp.requestHandler.executionContext.SamplingPriority)
				log.Debugf("[lifecycle] The inferred span attributes are: %v", lp.requestHandler.inferredSpanContext)
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
