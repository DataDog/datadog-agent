// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

package httpsec

import (
	"bytes"
	"encoding/json"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-lambda-go/events"
)

// ProxyLifecycleProcessor is an implementation of the invocationlifecycle.InvocationProcessor
// interface called by the Runtime API proxy on every function invocation calls and responses.
// This allows AppSec to run by monitoring the function invocations, and run the security
// rules upon reception of the HTTP request span in the SpanModifier function created by
// the WrapSpanModifier() method.
// A value of this type can be used by a single function invocation at a time.
type ProxyLifecycleProcessor struct {
	// AppSec instance
	appsec Monitorer

	// Parsed invocation event value
	invocationEvent interface{}
}

// NewProxyLifecycleProcessor returns a new httpsec proxy processor monitored with the
// given Monitorer.
func NewProxyLifecycleProcessor(appsec Monitorer) *ProxyLifecycleProcessor {
	return &ProxyLifecycleProcessor{
		appsec: appsec,
	}
}

// GetExecutionInfo exported method should have comment or be unexported
func (lp *ProxyLifecycleProcessor) GetExecutionInfo() *invocationlifecycle.ExecutionStartInfo {
	return nil // not used in the runtime api proxy case
}

// OnInvokeStart is the hook triggered when an invocation has started
func (lp *ProxyLifecycleProcessor) OnInvokeStart(startDetails *invocationlifecycle.InvocationStartDetails) {
	log.Debugf("appsec: proxy-lifecycle: invocation started with raw payload `%s`", startDetails.InvokeEventRawPayload)

	payloadBytes := invocationlifecycle.ParseLambdaPayload(startDetails.InvokeEventRawPayload)
	log.Debugf("Parsed payload string: %s", bytesStringer(payloadBytes))

	lowercaseEventPayload, err := trigger.Unmarshal(bytes.ToLower(payloadBytes))
	if err != nil {
		log.Debugf("appsec: proxy-lifecycle: Failed to parse event payload: %v", err)
	}

	eventType := trigger.GetEventType(lowercaseEventPayload)
	if eventType == trigger.Unknown {
		log.Debugf("appsec: proxy-lifecycle: Failed to extract event type")
	}

	var event interface{}
	switch eventType {
	default:
		log.Debug("appsec: proxy-lifecycle: ignoring unsupported lambda event type %s", eventType)
		return
	case trigger.APIGatewayEvent:
		event = &events.APIGatewayProxyRequest{}
	case trigger.APIGatewayV2Event:
		event = &events.APIGatewayV2HTTPRequest{}
	case trigger.APIGatewayWebsocketEvent:
		event = &events.APIGatewayWebsocketProxyRequest{}
	case trigger.ALBEvent:
		event = &events.ALBTargetGroupRequest{}
	case trigger.LambdaFunctionURLEvent:
		event = &events.LambdaFunctionURLRequest{}
	}

	if err := json.Unmarshal(payloadBytes, event); err != nil {
		log.Errorf("appsec: proxy-lifecycle: unexpected lambda event parsing error: %v", err)
		return
	}

	// In monitoring-only mode - without blocking - we can wait until the request's end to monitor it
	lp.invocationEvent = event
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (lp *ProxyLifecycleProcessor) OnInvokeEnd(_ *invocationlifecycle.InvocationEndDetails) {
	// noop: not suitable for both nodejs and python because the python tracer is sending the span before this gets
	// called, so we miss our chance to run the appsec monitoring and attach our tags.
	// So the final appsec monitoring logic moved to the SpanModifier instead and we use it as "invocation end" event.
}

func (lp *ProxyLifecycleProcessor) spanModifier(lastReqId string, chunk *pb.TraceChunk, s *pb.Span) {
	// Add appsec tags to the aws lambda function service entry span
	if s.Name != "aws.lambda" || s.Type != "serverless" {
		return
	}
	currentReqId := s.Meta["request_id"]
	if spanReqId := lastReqId; currentReqId != spanReqId {
		log.Debugf("appsec: ignoring service entry span with an unexpected request id: expected `%s` but got `%s`", currentReqId, spanReqId)
		return
	}
	log.Debugf("appsec: found service entry span of the currently monitored request id `%s`", currentReqId)

	if lp.invocationEvent == nil {
		log.Debug("appsec: ignoring unsupported lamdba event")
		return // skip: unsupported event
	}

	span := (*spanWrapper)(s)

	var ctx context
	switch event := lp.invocationEvent.(type) {
	default:
		log.Debugf("appsec: ignoring unsupported lamdba event type %T", event)
		return

	case *events.APIGatewayProxyRequest:
		makeContext(
			&ctx,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			&event.Body,
		)

	case *events.APIGatewayV2HTTPRequest:
		makeContext(
			&ctx,
			&event.RawPath,
			toMultiValueMap(event.Headers),
			toMultiValueMap(event.QueryStringParameters),
			event.PathParameters,
			event.RequestContext.HTTP.SourceIP,
			&event.Body,
		)

	case *events.APIGatewayWebsocketProxyRequest:
		makeContext(
			&ctx,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			&event.Body,
		)

	case *events.ALBTargetGroupRequest:
		makeContext(
			&ctx,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			nil,
			"",
			&event.Body,
		)

	case *events.LambdaFunctionURLRequest:
		makeContext(
			&ctx,
			&event.RawPath,
			toMultiValueMap(event.Headers),
			toMultiValueMap(event.QueryStringParameters),
			nil,
			event.RequestContext.HTTP.SourceIP,
			&event.Body,
		)
	}

	// Set the span tags that are always expected to be there when appsec is enabled
	setAppSecEnabledTags(span)

	reqHeaders := ctx.requestHeaders
	setClientIPTags(span, ctx.requestSourceIP, reqHeaders)

	if ip, ok := span.GetMetaTag("http.client_ip"); ok {
		ctx.requestClientIP = &ip
	}

	if status, ok := span.GetMetaTag("http.status_code"); ok {
		ctx.responseStatus = &status
	} else {
		log.Debug("appsec: missing span tag http.status_code")
	}

	if events := lp.appsec.Monitor(ctx.toAddresses()); len(events) > 0 {
		setSecurityEventsTags(span, events, reqHeaders, nil)
		chunk.Priority = int32(sampler.PriorityUserKeep)
	}
}

// ExecutionContext exported type should have comment or be unexported
type ExecutionContext interface {
	LastRequestID() string
}

// WrapSpanModifier wraps the given SpanModifier function with AppSec monitoring
// and returns it. When non nil, the given modifySpan function is called first,
// before the AppSec monitoring.
// The resulting function will run AppSec when the span's request_id span tag
// matches the one observed at function invocation with OnInvokeStat() through
// the Runtime API proxy.
func (lp *ProxyLifecycleProcessor) WrapSpanModifier(ctx ExecutionContext, modifySpan func(*pb.TraceChunk, *pb.Span)) func(*pb.TraceChunk, *pb.Span) {
	return func(chunk *pb.TraceChunk, span *pb.Span) {
		if modifySpan != nil {
			modifySpan(chunk, span)
		}
		lp.spanModifier(ctx.LastRequestID(), chunk, span)
	}
}

type spanWrapper pb.Span

func (s *spanWrapper) SetMetaTag(tag string, value string) {
	s.Meta[tag] = value
}

func (s *spanWrapper) SetMetricsTag(tag string, value float64) {
	s.Metrics[tag] = value
}

func (s *spanWrapper) GetMetaTag(tag string) (value string, exists bool) {
	value, exists = s.Meta[tag]
	return
}

type bytesStringer []byte

func (b bytesStringer) String() string {
	return string(b)
}
