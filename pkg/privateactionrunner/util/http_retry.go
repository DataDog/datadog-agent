// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/cenkalti/backoff/v7"
)

// RetryHTTPOptions controls the retry policy for RetryHTTPRequest.
//
// MaxElapsedTime == 0 disables the elapsed-time cap, meaning retries continue
// until the request succeeds, hits a permanent failure (see IsRetryableHTTPStatus), or the caller's
// context is cancelled.
type RetryHTTPOptions struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
}

// RetryHTTPRequest runs op with exponential backoff. op returns
// (result, statusCode, err); statusCode should be 0 for transport-level errors
// where no HTTP response was received.
//
// 4xx responses are treated as permanent (no retry) since they typically
// indicate a non-transient client problem (bad credentials, malformed payload).
// Transport errors, 5xx, 408, 425 and 429 responses are retried. op can wrap a 429 error
// with WithRetryAfter to wait for the server-requested delay instead of the
// next backoff interval.
func RetryHTTPRequest[T any](ctx context.Context, op func() (T, int, error), opts RetryHTTPOptions) (T, error) {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = opts.InitialInterval
	expBackoff.MaxInterval = opts.MaxInterval

	result, err := backoff.Retry(ctx, func() (T, error) {
		result, statusCode, err := op()
		if err == nil {
			return result, nil
		}
		if statusCode != 0 && !IsRetryableHTTPStatus(statusCode) {
			return result, backoff.Permanent(err)
		}
		log.FromContext(ctx).Warnf("HTTP request failed, will retry: %v", err)
		return result, err
	},
		backoff.WithBackOff(expBackoff),
		backoff.WithMaxElapsedTime(opts.MaxElapsedTime),
	)
	if re := backoff.AsRetryError(err); re != nil && errors.Is(re.Cause, backoff.ErrPermanent) {
		return result, re.LastErr
	}
	return result, err
}

// IsRetryableHTTPStatus reports whether a non-2xx response may succeed if the
// same request is sent again: 5xx, and the 4xx that only mean "not now" (408
// Request Timeout, 425 Too Early, 429 Too Many Requests).
func IsRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	}
	return statusCode < 400 || statusCode >= 500
}

// WithRetryAfter wraps err so that RetryHTTPRequest waits for the delay given
// by a Retry-After header value before the next attempt. err is returned
// unchanged when the header is absent or cannot be parsed.
func WithRetryAfter(err error, retryAfter string) error {
	delay, ok := parseRetryAfter(retryAfter, time.Now())
	if !ok {
		return err
	}
	return backoff.RetryAfter(delay, err)
}

// parseRetryAfter parses a Retry-After header value, either delay-seconds or
// an HTTP-date (RFC 9110 section 10.2.3).
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second, seconds > 0
	}
	if date, err := http.ParseTime(value); err == nil {
		delay := date.Sub(now)
		return delay, delay > 0
	}
	return 0, false
}
