// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"context"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/cenkalti/backoff/v5"
)

// RetryHTTPOptions controls the retry policy for RetryHTTPRequest.
//
// MaxElapsedTime == 0 disables the elapsed-time cap, meaning retries continue
// until the request succeeds, hits a permanent failure (4xx), or the caller's
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
// Transport errors and 5xx responses are retried.
func RetryHTTPRequest[T any](ctx context.Context, op func() (T, int, error), opts RetryHTTPOptions) (T, error) {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = opts.InitialInterval
	expBackoff.MaxInterval = opts.MaxInterval

	return backoff.Retry(ctx, func() (T, error) {
		result, statusCode, err := op()
		if err == nil {
			return result, nil
		}
		if statusCode >= 400 && statusCode < 500 {
			return result, backoff.Permanent(err)
		}
		log.FromContext(ctx).Warnf("HTTP request failed, will retry: %v", err)
		return result, err
	},
		backoff.WithBackOff(expBackoff),
		backoff.WithMaxElapsedTime(opts.MaxElapsedTime),
	)
}
