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
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Monitorer is the interface type execpted by the httpsec invocation
// subprocessor monitoring the given security rules addresses and returning
// the security events that matched.
type Monitorer interface {
	Monitor(addresses map[string]interface{}) (events []byte)
}

// InvocationSubProcessor type allows to monitor lamdba invocations receiving
// HTTP-based events.
type InvocationSubProcessor struct {
	appsec Monitorer
}

// NewInvocationSubProcessor returns a new httpsec invocation subprocessor
// monitored with the given Monitorer.
func NewInvocationSubProcessor(appsec Monitorer) *InvocationSubProcessor {
	return &InvocationSubProcessor{
		appsec: appsec,
	}
}

func (p *InvocationSubProcessor) OnInvokeStart(_ *invocationlifecycle.InvocationStartDetails, _ *invocationlifecycle.RequestHandler) {
	// In monitoring-only mode - without blocking - we can wait until the request's end to monitor it
}

func (p *InvocationSubProcessor) OnInvokeEnd(endDetails *invocationlifecycle.InvocationEndDetails, invocCtx *invocationlifecycle.RequestHandler) {
	span := invocCtx
	// Set the span tags that are always expected to be there when appsec is enabled
	setAppSecEnabledTags(span)

	var ctx context
	switch event := invocCtx.Event().(type) {
	case events.APIGatewayProxyRequest:
		makeContext(
			&ctx,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			&event.Body,
		)

	case events.APIGatewayV2HTTPRequest:
		makeContext(
			&ctx,
			&event.RawPath,
			toMultiValueMap(event.Headers),
			toMultiValueMap(event.QueryStringParameters),
			event.PathParameters,
			event.RequestContext.HTTP.SourceIP,
			&event.Body,
		)

	case events.APIGatewayWebsocketProxyRequest:
		makeContext(
			&ctx,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			event.PathParameters,
			event.RequestContext.Identity.SourceIP,
			&event.Body,
		)

	case events.ALBTargetGroupRequest:
		makeContext(
			&ctx,
			&event.Path,
			event.MultiValueHeaders,
			event.MultiValueQueryStringParameters,
			nil,
			"",
			&event.Body,
		)

	case events.LambdaFunctionURLRequest:
		makeContext(
			&ctx,
			&event.RawPath,
			toMultiValueMap(event.Headers),
			toMultiValueMap(event.QueryStringParameters),
			nil,
			event.RequestContext.HTTP.SourceIP,
			&event.Body,
		)

	default:
		if event == nil {
			log.Debug("appsec: ignoring unsupported lamdba event")
		} else {
			log.Debugf("appsec: ignoring unsupported lamdba event type %T", event)
		}
		return
	}

	reqHeaders := ctx.requestHeaders
	setClientIPTags(span, ctx.requestSourceIP, reqHeaders)

	respHeaders, err := parseResponseHeaders(endDetails.ResponseRawPayload)
	if err != nil {
		log.Debugf("appsec: couldn't parse the response payload headers: %v", err)
	}

	if status, ok := span.GetMetaTag("http.status_code"); ok {
		ctx.responseStatus = &status
	}
	if ip, ok := span.GetMetaTag("http.client_ip"); ok {
		ctx.requestClientIP = &ip
	}

	if events := p.appsec.Monitor(ctx.toAddresses()); len(events) > 0 {
		setSecurityEventsTags(span, events, reqHeaders, respHeaders)
		invocCtx.SetSamplingPriority(sampler.PriorityUserKeep)
	}
}

// AppSec monitoring context including the full list of monitored HTTP values
// (which must be nullable to know when they were set or not), along with the
// required context to report appsec-related span tags.
type context struct {
	requestSourceIP   string
	requestClientIP   *string             // http.client_ip
	requestRawURI     *string             // server.request.uri.raw
	requestHeaders    map[string][]string // server.request.headers.no_cookies
	requestCookies    map[string][]string // server.request.cookies
	requestQuery      map[string][]string // server.request.query
	requestPathParams map[string]string   // server.request.path_params
	requestBody       interface{}         // server.request.body
	responseStatus    *string             // server.response.status
}

// makeContext creates a http monitoring context out of the provided arguments.
func makeContext(ctx *context, path *string, headers, queryParams map[string][]string, pathParams map[string]string, sourceIP string, rawBody *string) {
	headers, rawCookies := filterHeaders(headers)
	cookies := parseCookies(rawCookies)
	body := parseBody(headers, rawBody)
	*ctx = context{
		requestSourceIP:   sourceIP,
		requestRawURI:     path,
		requestHeaders:    headers,
		requestCookies:    cookies,
		requestQuery:      queryParams,
		requestPathParams: pathParams,
		requestBody:       body,
	}
}

func parseBody(headers map[string][]string, rawBody *string) (body interface{}) {
	if rawBody == nil {
		return nil
	}

	rawStr := *rawBody
	if rawStr == "" {
		return rawStr
	}
	raw := []byte(*rawBody)

	var ct string
	if values, ok := headers["content-type"]; !ok {
		return rawStr
	} else if len(values) > 1 {
		return rawStr
	} else {
		ct = values[0]
	}

	switch ct {
	case "application/json":
		if err := json.Unmarshal(raw, &body); err != nil {
			return rawStr
		}
		return body

	default:
		return rawStr
	}
}

func (c *context) toAddresses() map[string]interface{} {
	addr := make(map[string]interface{})
	if c.requestClientIP != nil {
		addr["http.client_ip"] = *c.requestClientIP
	}
	if c.requestRawURI != nil {
		addr["server.request.uri.raw"] = *c.requestRawURI
	}
	if c.requestHeaders != nil {
		addr["server.request.headers.no_cookies"] = c.requestHeaders
	}
	if c.requestCookies != nil {
		addr["server.request.cookies"] = c.requestCookies
	}
	if c.requestQuery != nil {
		addr["server.request.query"] = c.requestQuery
	}
	if c.requestPathParams != nil {
		addr["server.request.path_params"] = c.requestPathParams
	}
	if c.requestBody != nil {
		addr["server.request.body"] = c.requestBody
	}
	if c.responseStatus != nil {
		addr["server.response.status"] = c.responseStatus
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

// Parses the given raw response payload as an HTTP response payload in order
// to retrieve its status code and response headers if any.
// This function merges the single- and multi-value headers the response may
// contain into a multi-value map of headers. A single-value header is ignored
// if it already exists in the map of multi-value headers.
// TODO: write unit-tests
func parseResponseHeaders(rawPayload []byte) (headers map[string][]string, err error) {
	var res struct {
		Headers           map[string]string   `json:"headers"`
		MultiValueHeaders map[string][]string `json:"multiValueHeaders"`
	}

	if err := json.Unmarshal(rawPayload, &res); err != nil {
		return nil, err
	}

	if len(res.Headers) == 0 && len(res.MultiValueHeaders) == 0 {
		return nil, nil
	}

	headers = res.MultiValueHeaders
	if headers == nil {
		headers = make(map[string][]string, len(res.Headers))
	}
	for k, v := range res.Headers {
		if _, exists := res.MultiValueHeaders[k]; !exists {
			headers[k] = []string{v}
		}
	}
	return headers, nil
}

// Helper function to convert a single-value map of event values into a
// multi-value one.
func toMultiValueMap(m map[string]string) map[string][]string {
	l := len(m)
	if l == 0 {
		return nil
	}
	res := make(map[string][]string, l)
	for k, v := range m {
		res[k] = []string{v}
	}
	return res
}
