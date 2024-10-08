// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"time"
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

// ReqOptions are options when making a request
type ReqOptions struct {
	Conn      ShouldCloseConnection
	Ctx       context.Context
	Authtoken string
}

// GetClient is a convenience function returning an http client
// `GetClient(false)` must be used only for HTTP requests whose destination is
// localhost (ie, for Agent commands).
func GetClient(verify bool) *http.Client {
	return GetClientWithTimeout(0, verify)
}

// GetClientWithTimeout is a convenience function returning an http client
// Arguments correspond to the request timeout duration, and a boolean to
// verify the server TLS client (false should only be used on localhost
// trusted endpoints).
func GetClientWithTimeout(to time.Duration, verify bool) *http.Client {
	if verify {
		return &http.Client{
			Timeout: to,
		}
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	return &http.Client{Transport: tr}
}

// DoGet is a wrapper around performing HTTP GET requests
func DoGet(c *http.Client, url string, conn ShouldCloseConnection) (body []byte, e error) {
	return DoGetWithOptions(c, url, &ReqOptions{Conn: conn})
}

// DoGetWithOptions is a wrapper around performing HTTP GET requests
func DoGetWithOptions(c *http.Client, url string, options *ReqOptions) (body []byte, e error) {
	if options.Authtoken == "" {
		options.Authtoken = GetAuthToken()
	}

	if options.Ctx == nil {
		options.Ctx = context.Background()
	}

	req, e := http.NewRequestWithContext(options.Ctx, "GET", url, nil)
	if e != nil {
		return body, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+options.Authtoken)
	if options.Conn == CloseConnection {
		req.Close = true
	}

	r, e := c.Do(req)
	if e != nil {
		return body, e
	}
	body, e = io.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return body, e
	}
	if r.StatusCode >= 400 {
		return body, errors.New(string(body))
	}
	return body, nil
}

// DoPost is a wrapper around performing HTTP POST requests
func DoPost(c *http.Client, url string, contentType string, body io.Reader) (resp []byte, e error) {
	req, e := http.NewRequest("POST", url, body)
	if e != nil {
		return resp, e
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+GetAuthToken())

	r, e := c.Do(req)
	if e != nil {
		return resp, e
	}
	resp, e = io.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		return resp, e
	}
	if r.StatusCode >= 400 {
		return resp, errors.New(string(resp))
	}
	return resp, nil
}

// DoPostChunked is a wrapper around performing HTTP POST requests that stream chunked data
func DoPostChunked(c *http.Client, url string, contentType string, body io.Reader, onChunk func([]byte)) error {
	req, e := http.NewRequest("POST", url, body)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+GetAuthToken())

	r, e := c.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()

	var m int
	buf := make([]byte, 4096)
	for {
		m, e = r.Body.Read(buf)
		if m < 0 || e != nil {
			break
		}
		onChunk(buf[:m])
	}

	if r.StatusCode == 200 {
		return nil
	}
	return e
}
