// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"bytes"
	"os"
	"strings"
	"time"
	"unsafe"

	json "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/propagation"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LifecycleProcessor is a InvocationProcessor implementation
type LifecycleProcessor struct {
	ExtraTags            *serverlessLog.Tags
	ProcessTrace         func(p *api.Payload)
	Demux                aggregator.Demultiplexer
	DetectLambdaLibrary  func() bool
	InferredSpansEnabled bool
	SubProcessor         InvocationSubProcessor
	Extractor            propagation.Extractor

	requestHandler *RequestHandler
	serviceName    string
}

// RequestHandler is the struct that stores information about the trace,
// inferred span, and tags about the current invocation
// inferred spans may contain a secondary inferred span in certain cases like SNS from SQS
type RequestHandler struct {
	executionInfo  *ExecutionStartInfo
	event          interface{}
	inferredSpans  [2]*inferredspan.InferredSpan
	triggerTags    map[string]string
	triggerMetrics map[string]float64
}

// SetMetaTag sets a meta span tag. A meta tag is a tag whose value type is string.
func (r *RequestHandler) SetMetaTag(tag string, value string) {
	r.triggerTags[tag] = value
}

// GetMetaTag returns the meta span tag value if it exists.
func (r *RequestHandler) GetMetaTag(tag string) (value string, exists bool) {
	value, exists = r.triggerTags[tag]
	return
}

// SetMetricsTag sets a metrics span tag. A metrics tag is a tag whose value type is float64.
func (r *RequestHandler) SetMetricsTag(tag string, value float64) {
	r.triggerMetrics[tag] = value
}

// Event returns the invocation event parsed by the LifecycleProcessor. It is nil if the event type is not supported
// yet. The actual event type can be figured out thanks to a Go type switch on the event types of the package
// github.com/aws/aws-lambda-go/events
func (r *RequestHandler) Event() interface{} {
	return r.event
}

// SetSamplingPriority sets the trace priority
func (r *RequestHandler) SetSamplingPriority(priority sampler.SamplingPriority) {
	r.executionInfo.SamplingPriority = priority
}

// OnInvokeStart is the hook triggered when an invocation has started
func (lp *LifecycleProcessor) OnInvokeStart(startDetails *InvocationStartDetails) {
	log.Debug("[lifecycle] onInvokeStart ------")
	log.Debugf("[lifecycle] Invocation has started at: %v", startDetails.StartTime)
	log.Debugf("[lifecycle] Invocation invokeEvent payload is: %s", startDetails.InvokeEventRawPayload)
	log.Debug("[lifecycle] ---------------------------------------")

	payloadBytes := ParseLambdaPayload(startDetails.InvokeEventRawPayload)
	log.Debugf("Parsed payload string: %s", unsafe.String(unsafe.SliceData(payloadBytes), len(payloadBytes)))

	lowercaseEventPayload, err := trigger.Unmarshal(bytes.ToLower(payloadBytes))
	if err != nil {
		log.Debugf("[lifecycle] Failed to parse event payload: %v", err)
	}

	eventType := trigger.GetEventType(lowercaseEventPayload)
	if eventType == trigger.Unknown {
		log.Debugf("[lifecycle] Failed to extract event type")
	}

	// Initialize basic values in the request handler
	lp.newRequest(startDetails.InvokeEventRawPayload, startDetails.StartTime)

	region, account, resource, arnParseErr := trigger.ParseArn(startDetails.InvokedFunctionARN)
	if arnParseErr != nil {
		log.Debugf("[lifecycle] Error parsing ARN: %v", err)
	}

	var ev interface{}
	switch eventType {
	case trigger.APIGatewayEvent:
		var event events.APIGatewayProxyRequest
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", apiGateway, err)
			break
		}
		ev = event
		lp.initFromAPIGatewayEvent(event, region)
	case trigger.APIGatewayV2Event:
		var event events.APIGatewayV2HTTPRequest
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", apiGateway, err)
			break
		}
		ev = event
		lp.initFromAPIGatewayV2Event(event, region)
	case trigger.APIGatewayWebsocketEvent:
		var event events.APIGatewayWebsocketProxyRequest
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", apiGateway, err)
			break
		}
		ev = event
		lp.initFromAPIGatewayWebsocketEvent(event, region)
	case trigger.APIGatewayLambdaAuthorizerTokenEvent:
		var event events.APIGatewayCustomAuthorizerRequest
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", apiGateway, err)
			break
		}
		ev = event
		lp.initFromAPIGatewayLambdaAuthorizerTokenEvent(event)
	case trigger.APIGatewayLambdaAuthorizerRequestParametersEvent:
		var event events.APIGatewayCustomAuthorizerRequestTypeRequest
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", apiGateway, err)
			break
		}
		ev = event
		lp.initFromAPIGatewayLambdaAuthorizerRequestParametersEvent(event)
	case trigger.ALBEvent:
		var event events.ALBTargetGroupRequest
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", applicationLoadBalancer, err)
			break
		}
		ev = event
		lp.initFromALBEvent(event)
	case trigger.CloudWatchEvent:
		var event events.CloudWatchEvent
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", cloudwatchEvents, err)
			break
		}
		ev = event
		lp.initFromCloudWatchEvent(event)
	case trigger.CloudWatchLogsEvent:
		var event events.CloudwatchLogsEvent
		if err := json.Unmarshal(payloadBytes, &event); err != nil && arnParseErr != nil {
			log.Debugf("Failed to unmarshal %s event: %s", cloudwatchLogs, err)
			break
		}
		ev = event
		lp.initFromCloudWatchLogsEvent(event, region, account)
	case trigger.DynamoDBStreamEvent:
		var event events.DynamoDBEvent
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", dynamoDB, err)
			break
		}
		ev = event
		lp.initFromDynamoDBStreamEvent(event)
	case trigger.KinesisStreamEvent:
		var event events.KinesisEvent
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", kinesis, err)
			break
		}
		ev = event
		lp.initFromKinesisStreamEvent(event)
	case trigger.EventBridgeEvent:
		var event events.EventBridgeEvent
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", eventBridge, err)
			break
		}
		ev = event
		lp.initFromEventBridgeEvent(event)
	case trigger.S3Event:
		var event events.S3Event
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", s3, err)
			break
		}
		ev = event
		lp.initFromS3Event(event)
	case trigger.SNSEvent:
		var event events.SNSEvent
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", sns, err)
			break
		}
		ev = event
		lp.initFromSNSEvent(event)
	case trigger.SQSEvent:
		var event events.SQSEvent
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			log.Debugf("Failed to unmarshal %s event: %s", sqs, err)
			break
		}
		ev = event
		lp.initFromSQSEvent(event)
	case trigger.LambdaFunctionURLEvent:
		var event events.LambdaFunctionURLRequest
		if err := json.Unmarshal(payloadBytes, &event); err != nil && arnParseErr != nil {
			log.Debugf("Failed to unmarshal %s event: %s", functionURL, err)
			break
		}
		ev = event
		lp.initFromLambdaFunctionURLEvent(event, region, account, resource)
	default:
		log.Debug("Skipping adding trigger types and inferred spans as a non-supported payload was received.")
	}

	if lp.SubProcessor != nil {
		lp.SubProcessor.OnInvokeStart(startDetails, lp.requestHandler)
	}

	if !lp.DetectLambdaLibrary() {
		lp.startExecutionSpan(ev, payloadBytes, startDetails)
	}
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (lp *LifecycleProcessor) OnInvokeEnd(endDetails *InvocationEndDetails) {
	log.Debug("[lifecycle] onInvokeEnd ------")
	log.Debugf("[lifecycle] Invocation has finished at: %v", endDetails.EndTime)
	log.Debugf("[lifecycle] Invocation isError is: %v", endDetails.IsError)
	log.Debug("[lifecycle] ---------------------------------------")

	endDetails.ResponseRawPayload = ParseLambdaPayload(endDetails.ResponseRawPayload)

	// Add the status code if it comes from an HTTP-like response struct
	statusCode, err := trigger.GetStatusCodeFromHTTPResponse(endDetails.ResponseRawPayload)
	if err != nil {
		log.Debugf("[lifecycle] Couldn't parse the response payload status code: %v", err)
	} else if statusCode == "" {
		log.Debug("[lifecycle] No http status code found in the response payload")
	} else {
		lp.addTag("http.status_code", statusCode)
	}

	if lp.SubProcessor != nil {
		lp.SubProcessor.OnInvokeEnd(endDetails, lp.requestHandler)
	}

	if !lp.DetectLambdaLibrary() {
		log.Debug("Creating and sending function execution span for invocation")

		if len(statusCode) == 3 && strings.HasPrefix(statusCode, "5") {
			serverlessMetrics.SendErrorsEnhancedMetric(
				lp.ExtraTags.Tags, endDetails.EndTime, lp.Demux,
			)
			endDetails.IsError = true
		}

		spans := make([]*pb.Span, 0, 3)
		span := lp.endExecutionSpan(endDetails)
		spans = append(spans, span)

		if lp.InferredSpansEnabled {
			inferredSpans := lp.endInferredSpan(statusCode, endDetails.EndTime, endDetails.IsError)
			spans = append(spans, inferredSpans...)
		}
		lp.processTrace(spans)
	}

	// We don't submit an error metric on timeouts since it should have already been submitted when the Extension receives a SHUTDOWN event
	if endDetails.IsError && !endDetails.IsTimeout {
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
	return lp.requestHandler.inferredSpans[0]
}

func (lp *LifecycleProcessor) getInferredSpanStart() time.Time {
	return time.Unix(lp.GetInferredSpan().Span.Start, 0)
}

// GetServiceName returns the value stored in the environment variable
// DD_SERVICE. Also assigned into `lp.serviceName` if not previously set
func (lp *LifecycleProcessor) GetServiceName() string {
	if lp.serviceName != "" {
		return lp.serviceName
	}
	lp.serviceName = os.Getenv("DD_SERVICE")
	return lp.serviceName
}

// NewRequest initializes basic information about the current request
// on the LifecycleProcessor
func (lp *LifecycleProcessor) newRequest(lambdaPayloadString []byte, startTime time.Time) {
	if lp.requestHandler == nil {
		lp.requestHandler = &RequestHandler{}
	}
	lp.requestHandler.event = nil
	lp.requestHandler.executionInfo = &ExecutionStartInfo{
		requestPayload: lambdaPayloadString,
		startTime:      startTime,
	}
	lp.requestHandler.inferredSpans[0] = &inferredspan.InferredSpan{
		CurrentInvocationStartTime: startTime,
		Span: &pb.Span{
			SpanID: inferredspan.GenerateSpanId(),
		},
	}
	lp.requestHandler.triggerTags = make(map[string]string)
	lp.requestHandler.triggerMetrics = make(map[string]float64)
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

// Sets the parent and span IDs when multiple inferred spans are necessary.
// Inferred spans of index 1 are generally sent inside of inferred span index 0.
// Like an SNS event inside an SQS message, and the parenting order is essential.
func (lp *LifecycleProcessor) setParentIDForMultipleInferredSpans() {
	lp.requestHandler.inferredSpans[1].Span.ParentID = lp.requestHandler.inferredSpans[0].Span.ParentID
	lp.requestHandler.inferredSpans[0].Span.ParentID = lp.requestHandler.inferredSpans[1].Span.SpanID
}

// endInferredSpan attempts to complete any inferred spans and send them to intake
func (lp *LifecycleProcessor) endInferredSpan(statusCode string, endTime time.Time, isError bool) []*pb.Span {
	spans := make([]*pb.Span, 0, 2)
	log.Debug("[lifecycle] Attempting to complete the inferred span")
	log.Debugf("[lifecycle] Inferred span context: %+v", lp.GetInferredSpan().Span)
	if lp.GetInferredSpan().Span.Start != 0 {
		span0, span1 := lp.requestHandler.inferredSpans[0], lp.requestHandler.inferredSpans[1]
		if span1 != nil {
			log.Debug("[lifecycle] Completing a secondary inferred span")
			lp.setParentIDForMultipleInferredSpans()
			span1.AddTagToInferredSpan("http.status_code", statusCode)
			span1.AddTagToInferredSpan("peer.service", lp.GetServiceName())
			span := lp.completeInferredSpan(span1, lp.getInferredSpanStart(), isError)
			spans = append(spans, span)
			log.Debug("[lifecycle] The secondary inferred span attributes are %v", lp.requestHandler.inferredSpans[1])
		}
		span0.AddTagToInferredSpan("http.status_code", statusCode)
		span0.AddTagToInferredSpan("peer.service", lp.GetServiceName())
		span := lp.completeInferredSpan(span0, endTime, isError)
		spans = append(spans, span)
		log.Debugf("[lifecycle] The inferred span attributes are: %v", lp.GetInferredSpan())
	} else {
		log.Debug("[lifecyle] Failed to complete inferred span due to a missing start time. Please check that the event payload was received with the appropriate data")
	}
	return spans
}
