// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"os"
	"time"
)

// bindStdinToContext makes a Read blocked on f return once ctx is done, so a
// builtin waiting on an idle stdin pipe cannot outlive MaxExecutionTime or a
// caller cancellation. When ctx finishes, a watchdog sets an already-expired
// read deadline on f, which wakes every blocked and future Read with
// [os.ErrDeadlineExceeded]. The returned function stops the watchdog and
// clears the deadline so the caller's file is left as it was found.
//
// f is never closed and no extra Read is issued, so no bytes are consumed on
// the caller's behalf. Files that do not support deadlines (regular files,
// /dev/null, and anonymous pipes on Windows) are left alone: regular files do
// not block, and on Windows a blocked pipe Read cannot be interrupted.
func bindStdinToContext(ctx context.Context, f *os.File) (release func()) {
	if f == nil || ctx.Done() == nil || f.SetReadDeadline(time.Time{}) != nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = f.SetReadDeadline(time.Now())
		case <-stop:
		}
	}()
	return func() {
		close(stop)
		<-done
		_ = f.SetReadDeadline(time.Time{})
	}
}
