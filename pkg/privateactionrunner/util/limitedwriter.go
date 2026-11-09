// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"bytes"
	"errors"
	"sync"
)

// ErrOutputLimitExceeded is a sentinel used for errors.Is matching.
var ErrOutputLimitExceeded = errors.New("output limit exceeded")

// LimitedWriter wraps a bytes.Buffer and enforces a shared byte limit across
// one or more writers. Once the combined written bytes reach the limit,
// subsequent writes return ErrOutputLimitExceeded, which causes the OS to
// deliver a broken-pipe signal to the child process.
type LimitedWriter struct {
	buf    bytes.Buffer
	shared *sharedOutputLimit
}

type sharedOutputLimit struct {
	mu           sync.Mutex
	limit        int64
	written      int64
	limited      bool
	limitReached chan struct{}
}

// NewLimitedStdoutStderrWritersPair creates two LimitedWriters that share the same
// synchronized byte counter, so the combined output of stdout and stderr is bounded by limit.
func NewLimitedStdoutStderrWritersPair(limit int64) (*LimitedWriter, *LimitedWriter) {
	shared := &sharedOutputLimit{limit: limit, limitReached: make(chan struct{})}
	return &LimitedWriter{shared: shared}, &LimitedWriter{shared: shared}
}

func (lw *LimitedWriter) Write(p []byte) (int, error) {
	lw.shared.mu.Lock()
	defer lw.shared.mu.Unlock()

	if lw.shared.limited {
		return 0, ErrOutputLimitExceeded
	}

	remaining := lw.shared.limit - lw.shared.written
	if remaining <= 0 {
		lw.markLimitReached()
		return 0, ErrOutputLimitExceeded
	}

	toWrite := p
	if int64(len(p)) > remaining {
		toWrite = p[:remaining]
		lw.markLimitReached()
	}

	n, err := lw.buf.Write(toWrite)
	lw.shared.written += int64(n)
	if err != nil {
		return n, err
	}

	if lw.shared.limited {
		return n, ErrOutputLimitExceeded
	}
	return n, nil
}

func (lw *LimitedWriter) String() string {
	lw.shared.mu.Lock()
	defer lw.shared.mu.Unlock()
	return lw.buf.String()
}

func (lw *LimitedWriter) Len() int {
	lw.shared.mu.Lock()
	defer lw.shared.mu.Unlock()
	return lw.buf.Len()
}

// LimitReached returns true if the combined output limit was exceeded.
func (lw *LimitedWriter) LimitReached() bool {
	lw.shared.mu.Lock()
	defer lw.shared.mu.Unlock()
	return lw.shared.limited
}

// LimitReachedSignal is closed when the combined output limit is exceeded.
func (lw *LimitedWriter) LimitReachedSignal() <-chan struct{} {
	return lw.shared.limitReached
}

// markLimitReached must be called while shared.mu is held.
func (lw *LimitedWriter) markLimitReached() {
	if lw.shared.limited {
		return
	}
	lw.shared.limited = true
	close(lw.shared.limitReached)
}
