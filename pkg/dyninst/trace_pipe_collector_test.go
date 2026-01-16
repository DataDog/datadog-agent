// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// tracePipeCollector reads from the kernel trace_pipe and organizes output
// by PID into separate log files. This is useful for debugging test failures
// where BPF programs emit debug output via bpf_trace_printk.
type tracePipeCollector struct {
	dir  string   // temp directory for log files
	tp   *os.File // trace_pipe file handle
	done chan struct{}
	wg   sync.WaitGroup

	mu struct {
		sync.Mutex
		closed       bool
		err          error            // stored error from reader goroutine
		files        map[int]*os.File // pid -> open file handle for writing
		pendingPID   int              // PID of current incomplete entry (-1 if none)
		pendingData  bytes.Buffer     // accumulated lines for current entry
		flushWaiters []chan struct{}  // channels to notify on flush
	}
}

// traceLineRegex parses trace_pipe lines to extract timestamp, PID, and message.
// Example: "           <...>-117990  [000] ...11 103871.126847: bpf_trace_printk: 117990: message"
// Groups: 1=timestamp, 2=pid, 3=message
var traceLineRegex = regexp.MustCompile(`^\s*\S+\s+\[\d+\]\s+\S+\s+(\d+\.\d+): bpf_trace_printk: (\d+): (.*)$`)

// newTracePipeCollector creates a new collector that reads from trace_pipe
// and writes per-PID log files to t.TempDir().
func newTracePipeCollector(t *testing.T) *tracePipeCollector {
	dir := t.TempDir()
	tp, err := tracefs.OpenFile("trace_pipe", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("failed to open trace_pipe: %v", err)
	}

	c := &tracePipeCollector{
		dir:  dir,
		tp:   tp,
		done: make(chan struct{}),
	}
	c.mu.files = make(map[int]*os.File)
	c.mu.pendingPID = -1

	c.wg.Add(1)
	go c.readLoop()

	t.Logf("trace pipe collector started, logging to %s", dir)
	return c
}

// Close stops the collector and closes all file handles.
// It is safe to call Close multiple times.
func (c *tracePipeCollector) Close() {
	c.mu.Lock()
	if c.mu.closed {
		c.mu.Unlock()
		return
	}
	c.mu.closed = true
	// Notify any pending flush waiters that we're closing
	waiters := c.mu.flushWaiters
	c.mu.flushWaiters = nil
	c.mu.Unlock()

	for _, ch := range waiters {
		close(ch)
	}

	close(c.done)
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Flush any remaining pending entry
	c.flushPendingLocked()

	// Close all file handles
	for _, f := range c.mu.files {
		f.Close()
	}
}

// Flush blocks until the collector has processed all pending trace_pipe data.
// This is signaled when a read times out with no data, indicating the pipe is empty.
// If the collector is already closed, Flush returns immediately.
func (c *tracePipeCollector) Flush() error {
	ch := make(chan struct{})

	c.mu.Lock()
	if c.mu.closed {
		err := c.mu.err
		c.mu.Unlock()
		return err
	}
	c.mu.flushWaiters = append(c.mu.flushWaiters, ch)
	c.mu.Unlock()

	<-ch

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mu.err
}

// GetLogs returns a read-only file handle for the logs of the given PID.
// Returns nil, nil if no logs exist for this PID.
// The caller is responsible for closing the returned file.
func (c *tracePipeCollector) GetLogs(pid int) (*os.File, error) {
	path := filepath.Join(c.dir, fmt.Sprintf("%d.log", pid))
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// readLoop is the main goroutine that reads from trace_pipe.
func (c *tracePipeCollector) readLoop() {
	defer c.wg.Done()

	reader := bufio.NewReader(c.tp)
	for {
		select {
		case <-c.done:
			return
		default:
		}

		if err := c.tp.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			c.mu.Lock()
			c.mu.err = fmt.Errorf("failed to set read deadline: %w", err)
			c.mu.Unlock()
			log.Warnf("trace_pipe set deadline error: %v", err)
			return
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if os.IsTimeout(err) {
				c.mu.Lock()
				c.flushPendingLocked()
				c.mu.Unlock()
				c.notifyFlushWaiters()
				continue
			}
			c.mu.Lock()
			c.mu.err = err
			c.mu.Unlock()
			log.Warnf("trace_pipe read error: %v", err)
			return
		}

		line = strings.TrimSuffix(line, "\n")
		c.processLine(line)
	}
}

// processLine handles a single line from trace_pipe.
func (c *tracePipeCollector) processLine(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if match := traceLineRegex.FindStringSubmatch(line); match != nil {
		// New entry - flush any pending entry first
		c.flushPendingLocked()

		timestamp := match[1]
		pid, _ := strconv.Atoi(match[2])
		message := match[3]

		c.mu.pendingPID = pid
		c.mu.pendingData.WriteString(timestamp)
		c.mu.pendingData.WriteString(": ")
		c.mu.pendingData.WriteString(message)
	} else if c.mu.pendingPID >= 0 {
		// Continuation line - append to pending entry
		c.mu.pendingData.WriteString(" ")
		c.mu.pendingData.WriteString(strings.TrimSpace(line))
	}
}

// flushPendingLocked writes the pending entry to its PID's log file.
// Must be called with c.mu held.
func (c *tracePipeCollector) flushPendingLocked() {
	if c.mu.pendingPID < 0 {
		c.mu.pendingData.Reset()
		return
	}

	f, err := c.getOrCreateFileLocked(c.mu.pendingPID)
	if err != nil {
		log.Warnf("trace_pipe failed to get file for pid %d: %v", c.mu.pendingPID, err)
	} else {
		c.mu.pendingData.WriteString("\n")
		if _, err := f.Write(c.mu.pendingData.Bytes()); err != nil {
			log.Warnf("trace_pipe failed to write to file for pid %d: %v", c.mu.pendingPID, err)
		}
	}

	c.mu.pendingPID = -1
	c.mu.pendingData.Reset()
}

// getOrCreateFileLocked returns the file handle for the given PID, creating it if needed.
// Must be called with c.mu held.
func (c *tracePipeCollector) getOrCreateFileLocked(pid int) (*os.File, error) {
	if f, ok := c.mu.files[pid]; ok {
		return f, nil
	}

	path := filepath.Join(c.dir, fmt.Sprintf("%d.log", pid))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	c.mu.files[pid] = f
	return f, nil
}

// notifyFlushWaiters closes all waiting flush channels and clears the list.
func (c *tracePipeCollector) notifyFlushWaiters() {
	c.mu.Lock()
	waiters := c.mu.flushWaiters
	c.mu.flushWaiters = nil
	c.mu.Unlock()

	for _, ch := range waiters {
		close(ch)
	}
}
