// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"bytes"
	"errors"
	"sync/atomic"
)

// ErrOutputLimitExceeded is a sentinel used for errors.Is matching.
var ErrOutputLimitExceeded = errors.New("output limit exceeded")

// LimitedWriter wraps a bytes.Buffer and enforces a shared byte limit across
// one or more writers. Once the combined written bytes reach the limit,
// subsequent writes return ErrOutputLimitExceeded, which causes the OS to
// deliver a broken-pipe signal to the child process.
type LimitedWriter struct {
	buf     bytes.Buffer
	shared  *atomic.Int64 // shared counter across stdout+stderr writers
	limit   int64
	limited bool // sticky flag: once true, all further writes fail
}

// NewLimitedStdoutStderrWritersPair creates two LimitedWriters that share the same atomic
// byte counter, so the combined output of stdout and stderr is bounded by limit.
func NewLimitedStdoutStderrWritersPair(limit int64) (*LimitedWriter, *LimitedWriter) {
	shared := &atomic.Int64{}
	return &LimitedWriter{shared: shared, limit: limit},
		&LimitedWriter{shared: shared, limit: limit}
}

func (lw *LimitedWriter) Write(p []byte) (int, error) {
	if lw.limited {
		return 0, ErrOutputLimitExceeded
	}

	remaining := lw.limit - lw.shared.Load()
	if remaining <= 0 {
		lw.limited = true
		return 0, ErrOutputLimitExceeded
	}

	toWrite := p
	if int64(len(p)) > remaining {
		toWrite = p[:remaining]
		lw.limited = true
	}

	n, err := lw.buf.Write(toWrite)
	lw.shared.Add(int64(n))
	if err != nil {
		return n, err
	}

	if lw.limited {
		return n, ErrOutputLimitExceeded
	}
	return n, nil
}

func (lw *LimitedWriter) String() string {
	return lw.buf.String()
}

func (lw *LimitedWriter) Len() int {
	return lw.buf.Len()
}

// LimitReached returns true if the combined output limit was exceeded.
func (lw *LimitedWriter) LimitReached() bool {
	return lw.limited
}
