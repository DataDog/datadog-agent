// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Package httpsec defines is the HTTP instrumentation API and contract for
// AppSec. It defines an abstract representation of HTTP handlers, along with
// helper functions to wrap (aka. instrument) standard net/http handlers.
// HTTP integrations must use this package to enable AppSec features for HTTP,
// which listens to this package's operation events.
package httpsec

import (
	"net/http"
	"strings"
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
func FilterHeaders(reqHeaders map[string][]string) (headers map[string][]string, rawCookies []string) {
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
func ParseCookies(rawCookies []string) map[string][]string {
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
