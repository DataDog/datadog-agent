// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"bytes"
	"sync"

	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// Bounds on how much of ADP's output is retained. ADP is not chatty, but a process failing to
// start can loop on the same error, and an unbounded buffer in the Agent is not an acceptable
// outcome of a pre-flight.
const (
	maxCaptureLines = 2000
	maxCaptureBytes = 1 << 20 // 1 MiB
)

// capture is an io.Writer that splits ADP's output into whole lines and keeps a bounded
// number of them for the post-run scan.
//
// It is used as cmd.Stdout and cmd.Stderr rather than reading from StdoutPipe, because
// os/exec closes a pipe when Wait sees the command exit — so reading from a pipe races with
// Wait, while assigning a writer makes os/exec own the copy and guarantees Wait returns only
// once all output has been written here.
//
// Lines are kept whole or dropped whole, never truncated. A truncated JSON record would be
// indistinguishable from output that bypassed ADP's logger, which the scan reports as an
// error — so truncating would manufacture failures out of ordinary long records.
type capture struct {
	log logcomp.Component

	mu      sync.Mutex
	pending []byte
	lines   []string
	bytes   int
	dropped int
	// discarding is set once the current line has outgrown the buffer, so the rest of it is
	// skipped up to the next newline instead of being emitted as if it were a new line.
	discarding bool
}

func newCapture(log logcomp.Component) *capture {
	return &capture{log: log}
}

// Write implements io.Writer. It always reports the full length: a short write would make
// os/exec's io.Copy treat the stream as failed.
func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := len(p)
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')

		if c.discarding {
			if idx < 0 {
				break // still skipping the tail of an over-long line
			}
			c.discarding = false
			p = p[idx+1:]
			continue
		}

		if idx < 0 {
			c.pending = append(c.pending, p...)
			p = nil
		} else {
			c.pending = append(c.pending, p[:idx]...)
			c.commitLocked()
			p = p[idx+1:]
		}

		if len(c.pending) > maxCaptureBytes {
			c.pending = c.pending[:0]
			c.dropped++
			c.discarding = true
		}
	}
	return total, nil
}

// commitLocked emits the pending bytes as one line and resets the buffer.
func (c *capture) commitLocked() {
	line := string(bytes.TrimRight(c.pending, "\r"))
	c.pending = c.pending[:0]
	if line == "" {
		return
	}

	// Mirrored at debug only: the raw stream can carry operator-controlled text, so it stays
	// local and reachable in a flare via the Agent log, never shipped.
	c.log.Debugf("ADP-PREFLIGHT-MODE: %s", line)

	if len(c.lines) >= maxCaptureLines || c.bytes+len(line) > maxCaptureBytes {
		c.dropped++
		return
	}
	c.lines = append(c.lines, line)
	c.bytes += len(line)
}

// snapshot returns the lines captured so far and how many were dropped. It is read-only: a
// partial line is deliberately not committed, since splitting a record would leave the second
// half looking like output that bypassed ADP's logger.
func (c *capture) snapshot() ([]string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slicesClone(c.lines), c.dropped
}

// finish flushes a trailing line that was never newline-terminated and returns the complete
// capture. Call it once, after cmd.Wait has returned.
func (c *capture) finish() ([]string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pending) > 0 && !c.discarding {
		c.commitLocked()
	}
	c.pending = c.pending[:0]
	return slicesClone(c.lines), c.dropped
}

func slicesClone(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}
