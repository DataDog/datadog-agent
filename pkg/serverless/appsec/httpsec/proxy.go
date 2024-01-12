// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpsec

import (
	"bytes"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-lambda-go/events"
	json "github.com/json-iterator/go"
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

//nolint:revive // TODO(ASM) Fix revive linter
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
	} else {
		log.Debugf("appsec: proxy-lifecycle: Extracted event type: %v", eventType)
	}

	var event interface{}
	switch eventType {
	default:
		log.Debugf("appsec: proxy-lifecycle: ignoring unsupported lambda event type %v", eventType)
		return
	case trigger.APIGatewayEvent:
		event = &events.APIGatewayProxyRequest{}
	case trigger.APIGatewayV2Event:
		event = &events.APIGatewayV2HTTPRequest{}
	case trigger.APIGatewayWebsocketEvent:
		event = &events.APIGatewayWebsocketProxyRequest{}
	case trigger.APIGatewayLambdaAuthorizerTokenEvent:
		event = &events.APIGatewayCustomAuthorizerRequest{}
	case trigger.APIGatewayLambdaAuthorizerRequestParametersEvent:
		event = &events.APIGatewayCustomAuthorizerRequestTypeRequest{}
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

//nolint:revive // TODO(ASM) Fix revive linter
func (lp *ProxyLifecycleProcessor) spanModifier(lastReqId string, chunk *pb.TraceChunk, s *pb.Span) {
	// Add relevant standalone tags to the chunk (TODO: remove per span tagging once backend handles chunk tags)
	if config.IsStandalone() {
		if chunk.Tags == nil {
			chunk.Tags = make(map[string]string)
		}
		chunk.Tags["_dd.apm.enabled"] = "0"
		// By the spec, only the service entry span needs to be tagged.
		// We play it safe by tagging everything in case the service entry span gets changed by the agent
		for _, s := range chunk.Spans {
			(*spanWrapper)(s).SetMetricsTag("_dd.apm.enabled", 0)
		}
	}
	if s.Name != "aws.lambda" || s.Type != "serverless" {
		return
	}
	//nolint:revive // TODO(ASM) Fix revive linter
	currentReqId := s.Meta["request_id"]
	//nolint:revive // TODO(ASM) Fix revive linter
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
			&event.Resource,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			&event.Body,
			event.IsBase64Encoded,
		)

	case *events.APIGatewayV2HTTPRequest:
		makeContext(
			&ctx,
			&event.RouteKey,
			&event.RawPath,
			toMultiValueMap(event.Headers),
			toMultiValueMap(event.QueryStringParameters),
			event.PathParameters,
			event.RequestContext.HTTP.SourceIP,
			&event.Body,
			event.IsBase64Encoded,
		)

	case *events.APIGatewayWebsocketProxyRequest:
		makeContext(
			&ctx,
			&event.Resource,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			&event.Body,
			event.IsBase64Encoded,
		)

	case *events.APIGatewayCustomAuthorizerRequest:
		makeContext(
			&ctx,
			nil,
			nil,
			// NOTE: The header name could have been different (depends on API GW configuration)
			map[string][]string{"Authorization": {event.AuthorizationToken}},
			nil,
			nil,
			"", // Not provided by API Gateway
			nil,
			false,
		)

	case *events.APIGatewayCustomAuthorizerRequestTypeRequest:
		makeContext(
			&ctx,
			&event.Resource,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			nil,
			false,
		)

	case *events.ALBTargetGroupRequest:
		makeContext(
			&ctx,
			nil,
			&event.Path,
			// Depending on how the ALB is configured, headers will be either in MultiValueHeaders or Headers (not both).
			multiOrSingle(event.MultiValueHeaders, event.Headers),
			// Depending on how the ALB is configured, query parameters will be either in MultiValueQueryStringParameters or QueryStringParameters (not both).
			multiOrSingle(event.MultiValueQueryStringParameters, event.QueryStringParameters),
			nil,
			"",
			&event.Body,
			event.IsBase64Encoded,
		)

	case *events.LambdaFunctionURLRequest:
		makeContext(
			&ctx,
			nil,
			&event.RawPath,
			toMultiValueMap(event.Headers),
			toMultiValueMap(event.QueryStringParameters),
			nil,
			event.RequestContext.HTTP.SourceIP,
			&event.Body,
			event.IsBase64Encoded,
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

	if res := lp.appsec.Monitor(ctx.toAddresses()); res.HasEvents() {
		setSecurityEventsTags(span, res.Events, reqHeaders, nil)
		chunk.Priority = int32(sampler.PriorityUserKeep)
		if ctx.requestRoute != nil {
			span.SetMetaTag("http.route", *ctx.requestRoute)
		}
		setAPISecurityTags(span, res.Derivatives)
	}
}

// multiOrSingle picks the first non-nil map, and returns the content formatted
// as the multi-map.
func multiOrSingle(multi map[string][]string, single map[string]string) map[string][]string {
	if multi == nil && single != nil {
		// There is no multi-map, but there is a single-map, so we'll make a multi-map out of that.
		multi = make(map[string][]string, len(single))
		for key, value := range single {
			multi[key] = []string{value}
		}
	}
	return multi
}

//nolint:revive // TODO(ASM) Fix revive linter
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
	if s.Meta == nil {
		s.Meta = make(map[string]string)
	}
	s.Meta[tag] = value
}

func (s *spanWrapper) SetMetricsTag(tag string, value float64) {
	if s.Metrics == nil {
		s.Metrics = make(map[string]float64)
	}
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
