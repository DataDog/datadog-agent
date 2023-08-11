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
)

// Monitorer is the interface type expected by the httpsec invocation
// subprocessor monitoring the given security rules addresses and returning
// the security events that matched.
type Monitorer interface {
	Monitor(addresses map[string]interface{}) (events []byte)
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
