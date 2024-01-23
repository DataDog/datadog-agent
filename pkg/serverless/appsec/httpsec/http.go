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
	"encoding/xml"
	"net/textproto"

	waf "github.com/DataDog/go-libddwaf/v2"
)

// Monitorer is the interface type expected by the httpsec invocation
// subprocessor monitoring the given security rules addresses and returning
// the security events that matched.
type Monitorer interface {
	Monitor(addresses map[string]any) *waf.Result
}

// AppSec monitoring context including the full list of monitored HTTP values
// (which must be nullable to know when they were set or not), along with the
// required context to report appsec-related span tags.
type context struct {
	requestSourceIP   string
	requestRoute      *string             // http.route
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
func makeContext(ctx *context, route, path *string, headers, queryParams map[string][]string, pathParams map[string]string, sourceIP string, rawBody *string, isBodyBase64 bool) {
	panic("not called")
}

// parseBody attempts to parse the payload found in rawBody according to the presentation headers. Returns nil if the
// request body could not be parsed (either due to an error, or because no suitable parsing strategy is implemented).
func parseBody(headers map[string][]string, rawBody *string, isBodyBase64 bool) any {
	panic("not called")
}

// / tryParseBody attempts to parse the raw data in raw according to the headers. Returns an error if parsing
// / fails, and a nil body if no parsing strategy was found.
func tryParseBody(headers textproto.MIMEHeader, raw string) (body any, err error) {
	panic("not called")
}

func (c *context) toAddresses() map[string]interface{} {
	panic("not called")
}

// FilterHeaders copies the given map and filters out the cookie entry. The
// resulting map of filtered headers is returned, along with the removed cookie
// entry if any. Note that the keys of the returned map of headers have been
// lower-cased as expected by Datadog's security monitoring rules - accessing
// them using http.(Header).Get(), which expects the MIME header canonical
// format, doesn't work.
func filterHeaders(reqHeaders map[string][]string) (headers map[string][]string, rawCookies []string) {
	panic("not called")
}

// ParseCookies returns the parsed cookies as a map of the cookie names to their
// value. Cookies defined more than once have multiple values in their map
// entry.
func parseCookies(rawCookies []string) map[string][]string {
	panic("not called")
}

// Helper function to convert a single-value map of event values into a
// multi-value one.
func toMultiValueMap(m map[string]string) map[string][]string {
	panic("not called")
}

// xmlMap is used to parse XML documents into a schema-agnostic format (essentially, a `map[string]any`).
type xmlMap map[string]any

// UnmarshalXML implements custom parsing from XML documents into a map-based generic format, because encoding/xml does
// not provide a built-in unmarshal to map (any data that does not fit an `xml` tagged field, or that does not fit the
// shape of that field, is silently ignored).
func (m *xmlMap) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	panic("not called")
}
