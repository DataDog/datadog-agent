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

	"github.com/cenkalti/backoff/v5"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// Rename moves sourcePath to targetPath, retrying on access-denied and
// sharing-violation errors.
//
// On Windows, os.Rename returns "Access is denied" both when the target
// directory already exists and when a file is transiently locked. To give
// callers a clearer error and to avoid burning the retry budget on a
// non-transient failure, we os.Stat the target first and return
// os.ErrExist if a directory already exists at that path. Files at the
// target path are left to the underlying os.Rename to handle.
//
// TODO: experimental. Support cases have shown intermittent rename failures
// during installation that look transient — the working hypothesis is that
// antimalware scanners briefly hold handles to files in the source
// directory, but we have not confirmed the root cause. This retry is a
// best-effort mitigation; monitor installer telemetry to see whether it
// actually helps and revisit (tune, broaden, or remove) once we have data.
func Rename(ctx context.Context, sourcePath, targetPath string) error {
	// MoveFileEx returns "Access is denied" when the target directory already exists
	// which is confusing. Return fs.ErrExist instead.
	if info, err := os.Stat(targetPath); err == nil {
		if info.IsDir() {
			return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: fs.ErrExist}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 200 * time.Millisecond
	b.MaxInterval = 5 * time.Second
	maxElapsedTime := time.Minute
	start := time.Now()
	attempts := 0
	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		attempts++
		err := os.Rename(sourcePath, targetPath)
		if err == nil {
			return struct{}{}, nil
		}
		if !isRetryableRenameError(err) {
			return struct{}{}, backoff.Permanent(err)
		}
		return struct{}{}, err
	}, backoff.WithBackOff(b), backoff.WithMaxElapsedTime(maxElapsedTime))
	if attempts > 1 {
		if span, ok := telemetry.SpanFromContext(ctx); ok {
			span.SetTag("paths.rename.attempts", attempts)
			span.SetTag("paths.rename.retry_duration_ms", time.Since(start).Milliseconds())
		}
	}
	return err
}

func isRetryableRenameError(err error) bool {
	// fs.ErrPermission covers ERROR_ACCESS_DENIED (5) via syscall.Errno.Is.
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
