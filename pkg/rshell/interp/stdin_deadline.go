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
// [os.ErrDeadlineExceeded]. The returned function stops the watchdog and, only
// if the watchdog set that deadline, clears it again.
//
// A run that finishes before ctx is done never touches f's deadline, so a read
// deadline the caller configured is preserved. A run that is cancelled or
// times out replaces it: Go cannot read back an existing deadline, so it is
// reset to none on return. If ctx ends at the same moment the run finishes,
// the watchdog may still win that race and reset the caller's deadline.
//
// f is never closed and no extra Read is issued, so no bytes are consumed on
// the caller's behalf. Only files Go registered with its poller support
// deadlines: those from [os.Pipe], or opened with [os.Open] on a FIFO, socket
// or terminal. Regular files, /dev/null, a blocking descriptor wrapped with
// [os.NewFile] (such as a process's inherited os.Stdin), and anonymous pipes
// on Windows reject the deadline and are left alone.
func bindStdinToContext(ctx context.Context, f *os.File) (release func()) {
	if f == nil || ctx.Done() == nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	armed := false
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			armed = f.SetReadDeadline(time.Now()) == nil
		case <-stop:
		}
	}()
	return func() {
		close(stop)
		<-done
		if armed {
			_ = f.SetReadDeadline(time.Time{})
		}
	}
}
