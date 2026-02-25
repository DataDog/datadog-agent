// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"bytes"
	"fmt"
)

// maxOutputSize is the maximum combined stdout+stderr size allowed for script execution.
const maxOutputSize = 10 * 1024 * 1024 // 10MB

// errOutputLimitExceeded is returned when a script's output exceeds maxOutputSize.
var errOutputLimitExceeded = fmt.Errorf("script output exceeded %dMB limit", maxOutputSize/(1024*1024))

// limitedWriter wraps a bytes.Buffer and enforces a shared byte limit across
// one or more writers. Once the combined written bytes reach the limit,
// subsequent writes return errOutputLimitExceeded, which causes the OS to
// deliver a broken-pipe signal to the child process.
type limitedWriter struct {
	buf     bytes.Buffer
	shared  *int64 // shared counter across stdout+stderr writers
	limit   int64
	limited bool // sticky flag: once true, all further writes fail
}

// newLimitedWriterPair creates two limitedWriters that share the same byte
// counter, so the combined output of stdout and stderr is bounded by limit.
func newLimitedWriterPair(limit int64) (*limitedWriter, *limitedWriter) {
	shared := new(int64)
	return &limitedWriter{shared: shared, limit: limit},
		&limitedWriter{shared: shared, limit: limit}
}

// Write implements io.Writer. It writes as many bytes as fit within the
// remaining budget and returns an error once the limit is reached.
func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.limited {
		return 0, errOutputLimitExceeded
	}

	remaining := lw.limit - *lw.shared
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
	*lw.shared += int64(n)
	if err != nil {
		return n, err
	}

	if lw.limited {
		return n, errOutputLimitExceeded
	}
	return n, nil
}

// String returns the buffered output.
func (lw *limitedWriter) String() string {
	return lw.buf.String()
}

// Len returns the number of bytes in this writer's buffer.
func (lw *limitedWriter) Len() int {
	return lw.buf.Len()
}

// LimitReached returns true if the combined output limit was exceeded.
func (lw *limitedWriter) LimitReached() bool {
	return lw.limited
}
