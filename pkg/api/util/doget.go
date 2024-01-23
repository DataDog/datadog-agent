// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"io"
	"net/http"
)

// ShouldCloseConnection is an option to DoGet to indicate whether to close the underlying
// connection after reading the response
type ShouldCloseConnection int

const (
	// LeaveConnectionOpen keeps the underlying connection open after reading the request response
	LeaveConnectionOpen ShouldCloseConnection = iota
	// CloseConnection closes the underlying connection after reading the request response
	CloseConnection
)

// GetClient is a convenience function returning an http client
// `GetClient(false)` must be used only for HTTP requests whose destination is
// localhost (ie, for Agent commands).
func GetClient(verify bool) *http.Client {
	panic("not called")
}

// DoGet is a wrapper around performing HTTP GET requests
func DoGet(c *http.Client, url string, conn ShouldCloseConnection) (body []byte, e error) {
	panic("not called")
}

// DoGetWithContext is a wrapper around performing HTTP GET requests
func DoGetWithContext(ctx context.Context, c *http.Client, url string, conn ShouldCloseConnection) (body []byte, e error) {
	panic("not called")
}

// DoPost is a wrapper around performing HTTP POST requests
func DoPost(c *http.Client, url string, contentType string, body io.Reader) (resp []byte, e error) {
	panic("not called")
}

// DoPostChunked is a wrapper around performing HTTP POST requests that stream chunked data
func DoPostChunked(c *http.Client, url string, contentType string, body io.Reader, onChunk func([]byte)) error {
	panic("not called")
}
