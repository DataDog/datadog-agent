// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"net/http"
	"strconv"
	"strings"
)

// WrapRoundTripper wraps the round tripper with the telemetry round tripper.
func WrapRoundTripper(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	if wrapped, ok := rt.(*roundTripper); ok {
		rt = wrapped.base
	}
	return &roundTripper{
		base: rt,
	}
}

type roundTripper struct {
	base http.RoundTripper
}

func (rt *roundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	span, _ := StartSpanFromContext(req.Context(), "http.request")
	defer func() { span.Finish(err) }()

	url := *req.URL
	url.User = nil

	span.span.Type = "http"
	span.SetResourceName(req.Method + " " + urlFromRequest(req))
	span.span.Meta["http.method"] = req.Method
	span.span.Meta["http.url"] = req.URL.String()
	span.span.Meta["span.kind"] = "client"
	span.span.Meta["network.destination.name"] = url.Hostname()
	res, err = rt.base.RoundTrip(req)
	if err != nil {
		span.SetTag("http.errors", err.Error())
		return res, err
	}
	span.SetTag("aws_pop", res.Header.Get("X-Amz-Cf-Pop"))
	span.SetTag("http.status_code", strconv.Itoa(res.StatusCode))
	if res.StatusCode >= 400 {
		span.SetTag("http.errors", res.Status)
	}
	return res, err
}

// urlFromRequest returns the URL from the HTTP request. The URL query string is included in the return object iff queryString is true
// See https://docs.datadoghq.com/tracing/configure_data_security#redacting-the-query-in-the-url for more information.
func urlFromRequest(r *http.Request) string {
	// Quoting net/http comments about net.Request.URL on server requests:
	// "For most requests, fields other than Path and RawQuery will be
	// empty. (See RFC 7230, Section 5.3)"
	// This is why we don't rely on url.URL.String(), url.URL.Host, url.URL.Scheme, etc...
	var url string
	path := r.URL.EscapedPath()
	scheme := r.URL.Scheme
	if r.TLS != nil {
		scheme = "https"
	}
	if r.Host != "" {
		url = strings.Join([]string{scheme, "://", r.Host, path}, "")
	} else {
		url = path
	}
	// Collect the query string if we are allowed to report it and obfuscate it if possible/allowed
	if r.URL.RawQuery != "" {
		query := r.URL.RawQuery
		url = strings.Join([]string{url, query}, "?")
	}
	if frag := r.URL.EscapedFragment(); frag != "" {
		url = strings.Join([]string{url, frag}, "#")
	}
	return url
}
