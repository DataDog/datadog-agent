// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package paths

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/cenkalti/backoff/v7"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const removeAllMaxElapsedTime = time.Minute

// RemoveAll removes path, retrying failures with exponential backoff.
func RemoveAll(ctx context.Context, path string) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 200 * time.Millisecond
	b.MaxInterval = 5 * time.Second
	start := time.Now()
	attempts := 0

	err := removeAll(ctx, func() error {
		attempts++
		return os.RemoveAll(path)
	},
		backoff.WithBackOff(b),
		backoff.WithMaxElapsedTime(removeAllMaxElapsedTime),
	)
	if attempts > 1 {
		if span, ok := telemetry.SpanFromContext(ctx); ok {
			span.SetTag("paths.remove_all.attempts", attempts)
			span.SetTag("paths.remove_all.retry_duration_ms", time.Since(start).Milliseconds())
		}
	}
	return err
}

func removeAll(ctx context.Context, remove func() error, opts ...backoff.RetryOption) error {
	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		err := remove()
		if err != nil && !isRetryableRemoveAllError(err) {
			return struct{}{}, backoff.Permanent(err)
		}
		return struct{}{}, err
	}, opts...)
	return err
}

func isRetryableRemoveAllError(err error) bool {
	// fs.ErrPermission covers ERROR_ACCESS_DENIED (5) via syscall.Errno.Is.
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_DIR_NOT_EMPTY)
}
