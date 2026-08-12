// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"bytes"
	"errors"
	"sync/atomic"
)

var errOutputLimitExceeded = errors.New("command output limit exceeded")

// LimitedOutput buffers command output within a limit shared with another output stream.
type LimitedOutput struct {
	buf     bytes.Buffer
	shared  *atomic.Int64 // shared counter across stdout+stderr writers
	limit   int64
	limited bool // sticky flag: once true, all further writes fail
}

// NewLimitedOutputPair creates output buffers that share one combined size limit.
func NewLimitedOutputPair(limit int64) (*LimitedOutput, *LimitedOutput) {
	shared := &atomic.Int64{}
	return &LimitedOutput{shared: shared, limit: limit},
		&LimitedOutput{shared: shared, limit: limit}
}

func (lw *LimitedOutput) Write(p []byte) (int, error) {
	if lw.limited {
		return 0, errOutputLimitExceeded
	}

	var reserved int64
	for {
		used := lw.shared.Load()
		remaining := lw.limit - used
		if remaining <= 0 {
			lw.limited = true
			return 0, errOutputLimitExceeded
		}

		reserved = int64(len(p))
		if reserved > remaining {
			reserved = remaining
			lw.limited = true
		}
		if lw.shared.CompareAndSwap(used, used+reserved) {
			break
		}
	}

	n, err := lw.buf.Write(p[:reserved])
	if unwritten := reserved - int64(n); unwritten > 0 {
		lw.shared.Add(-unwritten)
	}
	if err != nil {
		return n, err
	}

	if lw.limited {
		return n, errOutputLimitExceeded
	}
	return n, nil
}

func (lw *LimitedOutput) String() string {
	return lw.buf.String()
}

func (lw *LimitedOutput) Len() int {
	return lw.buf.Len()
}

// LimitReached returns true if the combined output limit was exceeded.
func (lw *LimitedOutput) LimitReached() bool {
	return lw.limited
}
