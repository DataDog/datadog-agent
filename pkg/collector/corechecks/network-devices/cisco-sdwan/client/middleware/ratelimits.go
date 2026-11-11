// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package middleware implements http.RoundTripper middlewares for the Cisco SD-WAN API client
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// ErrRateLimitTimeout is returned when a request cannot get a rate limiter token within the maximum wait
var ErrRateLimitTimeout = errors.New("timed out waiting for rate limiter")

// RateLimitedTransport paces every request sent through it, including authentication
// requests, retries and redirects
type RateLimitedTransport struct {
	next    http.RoundTripper
	limiter *rate.Limiter
	maxWait time.Duration
}

// NewRateLimitedTransport creates a RateLimitedTransport waiting at most maxWait for a token
// before sending a request to next. A nil next uses http.DefaultTransport.
func NewRateLimitedTransport(next http.RoundTripper, limiter *rate.Limiter, maxWait time.Duration) *RateLimitedTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &RateLimitedTransport{
		next:    next,
		limiter: limiter,
		maxWait: maxWait,
	}
}

// RoundTrip waits for a rate limiter token then sends the request. It fails with
// ErrRateLimitTimeout when no token is available within maxWait, and with the request
// context error when the request is cancelled while waiting.
func (t *RateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), t.maxWait)
	defer cancel()
	if err := t.limiter.Wait(ctx); err != nil {
		if req.Context().Err() != nil {
			return nil, fmt.Errorf("error waiting for rate limiter: %w", req.Context().Err())
		}
		return nil, fmt.Errorf("%w: %w", ErrRateLimitTimeout, err)
	}
	return t.next.RoundTrip(req)
}
