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

type Context struct {
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
