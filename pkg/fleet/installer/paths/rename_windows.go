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
)

// Rename moves sourcePath to targetPath, retrying on access-denied and
// sharing-violation errors.
//
// TODO: experimental. Support cases have shown intermittent rename failures
// during installation that look transient — the working hypothesis is that
// antimalware scanners briefly hold handles to files in the source
// directory, but we have not confirmed the root cause. This retry is a
// best-effort mitigation; monitor installer telemetry to see whether it
// actually helps and revisit (tune, broaden, or remove) once we have data.
//
// Note: os.Rename also returns "Access is denied" when the target directory
// already exists. Callers that need to handle that case must detect it
// before calling Rename (e.g. via os.Stat); by the time we reach the retry
// loop an access-denied error is treated as a transient lock.
func Rename(sourcePath, targetPath string) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 200 * time.Millisecond
	b.MaxInterval = 5 * time.Second
	_, err := backoff.Retry(context.Background(), func() (struct{}, error) {
		err := os.Rename(sourcePath, targetPath)
		if err == nil {
			return struct{}{}, nil
		}
		if !isRetryableRenameError(err) {
			return struct{}{}, backoff.Permanent(err)
		}
		return struct{}{}, err
	}, backoff.WithBackOff(b), backoff.WithMaxElapsedTime(time.Minute))
	return err
}

func isRetryableRenameError(err error) bool {
	// fs.ErrPermission covers ERROR_ACCESS_DENIED (5) via syscall.Errno.Is.
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
