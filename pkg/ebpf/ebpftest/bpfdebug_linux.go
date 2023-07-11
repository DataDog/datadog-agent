// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// LogTracePipe logs all messages read from /sys/kernel/[debug/]/tracing/trace_pipe during the test.
// This function will set the environment variable BPF_DEBUG=true for the duration of the test.
func LogTracePipe(t *testing.T) {
	logTracePipe(t, nil)
}

// LogTracePipeSelf logs only messages from the current process read from /sys/kernel/[debug/]/tracing/trace_pipe during the test.
// This function will set the environment variable BPF_DEBUG=true for the duration of the test.
func LogTracePipeSelf(t *testing.T) {
	LogTracePipeProcess(t, getpid())
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
	p, err := os.Readlink(filepath.Join(util.HostProc(), "/self"))
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
