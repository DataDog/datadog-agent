// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"bytes"
	"errors"
	"fmt"
	"sync/atomic"
)

const defaultMaxOutputSize = 10 * 1024 * 1024 // 10MB

// errOutputLimitExceeded is a sentinel used for errors.Is matching.
var errOutputLimitExceeded = errors.New("script output limit exceeded")

func newOutputLimitError(limit int64) error {
	return fmt.Errorf("script output exceeded %dMB limit: %w", limit/(1024*1024), errOutputLimitExceeded)
}

// limitedWriter wraps a bytes.Buffer and enforces a shared byte limit across
// one or more writers. Once the combined written bytes reach the limit,
// subsequent writes return errOutputLimitExceeded, which causes the OS to
// deliver a broken-pipe signal to the child process.
type limitedWriter struct {
	buf     bytes.Buffer
	shared  *atomic.Int64 // shared counter across stdout+stderr writers
	limit   int64
	limited bool // sticky flag: once true, all further writes fail
}

// newLimitedStdoutStderrWritersPair creates two limitedWriters that share the same atomic
// byte counter, so the combined output of stdout and stderr is bounded by limit.
func newLimitedStdoutStderrWritersPair(limit int64) (*limitedWriter, *limitedWriter) {
	shared := &atomic.Int64{}
	return &limitedWriter{shared: shared, limit: limit},
		&limitedWriter{shared: shared, limit: limit}
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.limited {
		return 0, errOutputLimitExceeded
	}

	remaining := lw.limit - lw.shared.Load()
	if remaining <= 0 {
		lw.limited = true
		return 0, errOutputLimitExceeded
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
		return n, errOutputLimitExceeded
	}
	return n, nil
}

func (lw *limitedWriter) String() string {
	return lw.buf.String()
}

func (lw *limitedWriter) Len() int {
	return lw.buf.Len()
}

// LimitReached returns true if the combined output limit was exceeded.
func (lw *limitedWriter) LimitReached() bool {
	return lw.limited
}
