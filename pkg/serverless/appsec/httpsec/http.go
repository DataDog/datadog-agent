// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package httpsec defines is the HTTP instrumentation API and contract for
// AppSec. It defines an abstract representation of HTTP handlers, along with
// helper functions to wrap (aka. instrument) standard net/http handlers.
// HTTP integrations must use this package to enable AppSec features for HTTP,
// which listens to this package's operation events.
package httpsec

import (
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-lambda-go/events"
)

type Context struct {
	RequestClientIP   string
	RequestRawURI     *string
	RequestHeaders    map[string][]string
	RequestCookies    map[string][]string
	RequestQuery      map[string][]string
	RequestPathParams map[string]string
	RequestBody       interface{}

	ResponseStatus  *string
	ResponseHeaders map[string][]string
}

type InvocationSubProcessor struct {
	asm *appsec.AppSec
	lp  *invocationlifecycle.LifecycleProcessor
	ctx Context
}

func (p *InvocationSubProcessor) OnInvokeStart(_ *invocationlifecycle.InvocationStartDetails) {
	p.ctx = Context{} // reset the context
	// In monitoring-only mode - without blocking - we can wait until the request's end to monitor it
}

func (p *InvocationSubProcessor) OnInvokeEnd(endDetails *invocationlifecycle.InvocationEndDetails, invCtx *invocationlifecycle.RequestHandler) {
	var ctx *Context
	switch event := invCtx.Event().(type) {
	case events.APIGatewayProxyRequest:
		ctx = NewContext(
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			event.Body)
		//case trigger.APIGatewayV2Event:
		//	var event events.APIGatewayV2HTTPRequest
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromAPIGatewayV2Event(event, region)
		//	}
		//	if lp.AppSec != nil {
		//		lp.AppSecContext = httpsec.NewContext(
		//			&event.RawPath,
		//			toMultiValueMap(event.Headers),
		//			toMultiValueMap(event.QueryStringParameters),
		//			event.PathParameters,
		//			event.RequestContext.HTTP.SourceIP,
		//			event.Body)
		//	}
		//case trigger.APIGatewayWebsocketEvent:
		//	var event events.APIGatewayWebsocketProxyRequest
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromAPIGatewayWebsocketEvent(event, region)
		//	}
		//	if lp.AppSec != nil {
		//		lp.AppSecContext = httpsec.NewContext(
		//			&event.Path,
		//			event.MultiValueHeaders,
		//			event.MultiValueQueryStringParameters,
		//			event.PathParameters,
		//			event.RequestContext.Identity.SourceIP,
		//			event.Body)
		//	}
		//case trigger.ALBEvent:
		//	var event events.ALBTargetGroupRequest
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromALBEvent(event)
		//	}
		//	if lp.AppSec != nil {
		//		lp.AppSecContext = httpsec.NewContext(
		//			&event.Path,
		//			event.MultiValueHeaders,
		//			event.MultiValueQueryStringParameters,
		//			nil,
		//			"",
		//			event.Body)
		//	}
		//case trigger.CloudWatchEvent:
		//	var event events.CloudWatchEvent
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromCloudWatchEvent(event)
		//	}
		//case trigger.CloudWatchLogsEvent:
		//	var event events.CloudwatchLogsEvent
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil && arnParseErr == nil {
		//		lp.initFromCloudWatchLogsEvent(event, region, account)
		//	}
		//case trigger.DynamoDBStreamEvent:
		//	var event events.DynamoDBEvent
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromDynamoDBStreamEvent(event)
		//	}
		//case trigger.KinesisStreamEvent:
		//	var event events.KinesisEvent
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromKinesisStreamEvent(event)
		//	}
		//case trigger.EventBridgeEvent:
		//	var event inferredspan.EventBridgeEvent
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromEventBridgeEvent(event)
		//	}
		//case trigger.S3Event:
		//	var event events.S3Event
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromS3Event(event)
		//	}
		//case trigger.SNSEvent:
		//	var event events.SNSEvent
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromSNSEvent(event)
		//	}
		//case trigger.SQSEvent:
		//	var event events.SQSEvent
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil {
		//		lp.initFromSQSEvent(event)
		//	}
		//case trigger.LambdaFunctionURLEvent:
		//	var event events.LambdaFunctionURLRequest
		//	if err := json.Unmarshal(payloadBytes, &event); err == nil && arnParseErr == nil {
		//		lp.initFromLambdaFunctionURLEvent(event, region, account, resource)
		//	}
		//	if lp.AppSec != nil {
		//		lp.AppSecContext = httpsec.NewContext(
		//			&event.RawPath,
		//			toMultiValueMap(event.Headers),
		//			toMultiValueMap(event.QueryStringParameters),
		//			nil,
		//			event.RequestContext.HTTP.SourceIP,
		//			event.Body)
		//	}
		//default:
		//	log.Debug("Skipping adding trigger types and inferred spans as a non-supported payload was received.")
	}

	span := invCtx
	SetAppSecEnabledTags(span)

	reqHeaders := ctx.RequestHeaders
	SetClientIPTags(span, ctx.RequestClientIP, reqHeaders)

	ctx.ResponseStatus = &statusCode

	responseRawPayload := []byte(parseLambdaPayload(endDetails.ResponseRawPayload))

	respHeaders, err := trigger.GetHeadersFromHTTPResponse(responseRawPayload)
	if err != nil {
		log.Debugf("appsec: couldn't parse the response payload headers: %v", err)
	}

	if events := p.asm.Monitor(ctx.ToAddresses()); len(events) > 0 {
		SetSecurityEventsTags(span, events, reqHeaders, respHeaders)
	}
}

// NewContext creates a new http monitoring context out of the provided
// arguments.
func NewContext(path *string, headers, queryParams map[string][]string, pathParams map[string]string, sourceIP string, body string) *Context {
	headers, rawCookies := filterHeaders(headers)
	cookies := parseCookies(rawCookies)
	var bodyface interface{}
	if len(body) > 0 {
		bodyface = body
	}
	return &Context{
		RequestClientIP:   sourceIP,
		RequestRawURI:     path,
		RequestHeaders:    headers,
		RequestCookies:    cookies,
		RequestQuery:      queryParams,
		RequestPathParams: pathParams,
		RequestBody:       bodyface,
	}
}

func (c *Context) ToAddresses() map[string]interface{} {
	addr := make(map[string]interface{})
	if c.RequestRawURI != nil {
		addr["server.request.uri.raw"] = *c.RequestRawURI
	}
	if c.RequestHeaders != nil {
		addr["server.request.headers.no_cookies"] = c.RequestHeaders
	}
	if c.RequestCookies != nil {
		addr["server.request.cookies"] = c.RequestCookies
	}
	if c.RequestQuery != nil {
		addr["server.request.query"] = c.RequestQuery
	}
	if c.RequestPathParams != nil {
		addr["server.request.path_params"] = c.RequestPathParams
	}
	if c.RequestBody != nil {
		addr["server.request.body"] = c.RequestBody
	}
	if c.ResponseStatus != nil {
		addr["server.response.status"] = c.ResponseStatus
	}
	return addr
}

// FilterHeaders copies the given map and filters out the cookie entry. The
// resulting map of filtered headers is returned, along with the removed cookie
// entry if any. Note that the keys of the returned map of headers have been
// lower-cased as expected by Datadog's security monitoring rules - accessing
// them using http.(Header).Get(), which expects the MIME header canonical
// format, doesn't work.
func filterHeaders(reqHeaders map[string][]string) (headers map[string][]string, rawCookies []string) {
	if len(reqHeaders) == 0 {
		return nil, nil
	}
	// Walk the map of request headers and filter the cookies out if any
	headers = make(map[string][]string, len(reqHeaders))
	for k, v := range reqHeaders {
		k := strings.ToLower(k)
		if k == "cookie" {
			// Do not include cookies in the request headers
			rawCookies = v
		}
		headers[k] = v
	}
	if len(headers) == 0 {
		headers = nil // avoid returning an empty map
	}
	return headers, rawCookies
}

// ParseCookies returns the parsed cookies as a map of the cookie names to their
// value. Cookies defined more than once have multiple values in their map
// entry.
func parseCookies(rawCookies []string) map[string][]string {
	// net.http doesn't expose its cookie-parsing function, so we are using the
	// http.(*Request).Cookies method instead which reads the request headers.
	r := http.Request{Header: map[string][]string{"Cookie": rawCookies}}
	parsed := r.Cookies()
	if len(parsed) == 0 {
		return nil
	}
	cookies := make(map[string][]string, len(parsed))
	for _, c := range parsed {
		cookies[c.Name] = append(cookies[c.Name], c.Value)
	}
	return cookies
}
