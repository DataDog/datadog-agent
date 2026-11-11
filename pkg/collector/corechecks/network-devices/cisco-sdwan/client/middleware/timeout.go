// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package middleware

import (
	"context"
	"io"
	"net/http"
	"time"
)

// TimeoutTransport bounds the time spent on a request, from sending it until its response
// body is closed. Unlike http.Client.Timeout, it excludes the time spent in the middlewares
// wrapping it, like waiting for a rate limiter token.
type TimeoutTransport struct {
	next    http.RoundTripper
	timeout time.Duration
}

// NewTimeoutTransport creates a TimeoutTransport sending requests to next. A nil next uses
// http.DefaultTransport.
func NewTimeoutTransport(next http.RoundTripper, timeout time.Duration) *TimeoutTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &TimeoutTransport{
		next:    next,
		timeout: timeout,
	}
}

// RoundTrip sends the request with a deadline that is released when the response body is closed
func (t *TimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
	resp, err := t.next.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelOnCloseBody releases the request context once the response body is closed, so the
// deadline still applies while the body is read
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
