// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ebpftest is utilities for tests against eBPF
package ebpftest

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// LogTracePipe logs all messages read from /sys/kernel/[debug/]/tracing/trace_pipe during the test.
// This function will set the environment variable BPF_DEBUG=true for the duration of the test.
func LogTracePipe(t *testing.T) {
	logTracePipe(t, nil)
}

// LogTracePipeSelf logs only messages from the current process read from /sys/kernel/[debug/]/tracing/trace_pipe during the test.
// This function will set the environment variable BPF_DEBUG=true for the duration of the test.
func LogTracePipeSelf(t *testing.T) {
	subtask := make(map[uint32]struct{})
	mypid := getpid()
	pidstr := strconv.Itoa(int(mypid))
	t.Logf("filtering to %d and child tasks", mypid)

	logTracePipe(t, func(ev *TraceEvent) bool {
		if ev.PID == mypid {
			return true
		}
		// check if a thread group of current process
		if _, ok := subtask[ev.PID]; ok {
			return true
		}
		_, err := os.Stat(kernel.HostProc(pidstr, "task", strconv.Itoa(int(ev.PID))))
		if err == nil {
			// cache result for faster lookup
			subtask[ev.PID] = struct{}{}
			return true
		}
		return false
	})
}

// LogTracePipeProcess logs only messages from the provided process read from /sys/kernel/[debug/]/tracing/trace_pipe during the test.
// This function will set the environment variable BPF_DEBUG=true for the duration of the test.
func LogTracePipeProcess(t *testing.T, pid uint32) {
	logTracePipe(t, func(ev *TraceEvent) bool {
		return ev.PID == pid
	})
}

// LogTracePipeFilter logs only messages that pass `filterFn` read from /sys/kernel/[debug/]/tracing/trace_pipe during the test.
// This function will set the environment variable BPF_DEBUG=true for the duration of the test.
func LogTracePipeFilter(t *testing.T, filterFn func(ev *TraceEvent) bool) {
	logTracePipe(t, filterFn)
}

func getpid() uint32 {
	p, err := os.Readlink(kernel.HostProc("/self"))
	if err == nil {
		if pid, err := strconv.ParseInt(p, 10, 32); err == nil {
			return uint32(pid)
		}
	}
	return uint32(os.Getpid())
}

func logTracePipe(t *testing.T, filterFn func(ev *TraceEvent) bool) {
	t.Setenv("BPF_DEBUG", "true")
	tp, err := NewTracePipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tp.Close() })

	ready := make(chan struct{})
	go func() {
		close(ready)
		logs, errs := tp.Channel()
		for {
			select {
			case log, ok := <-logs:
				if !ok {
					return
				}
				if filterFn == nil || filterFn(log) {
					t.Logf("trace_pipe: %s", log)
				}
			case err, ok := <-errs:
				if !ok {
					return
				}
				t.Logf("trace_pipe: error: %s\n", err)
			}
		}
	}()
	<-ready
}
