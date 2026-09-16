// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package servicemain

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/trace"
	"strconv"
	"sync"
	"time"
)

const (
	// EnvStartupTraceDir, when set to a non-empty directory, causes the service to
	// write a Go execution trace of its startup to that directory. Intended for
	// diagnosing startup CPU contention in tests; unset in production so it is a no-op.
	EnvStartupTraceDir = "DD_STARTUP_TRACE_DIR"
	// EnvStartupTraceDurationSeconds optionally overrides how long the startup trace
	// runs before it is stopped and flushed. See defaultStartupTraceDuration.
	EnvStartupTraceDurationSeconds = "DD_STARTUP_TRACE_DURATION_SECONDS"

	defaultStartupTraceDuration = 30 * time.Second
)

// startStartupTrace starts a Go execution trace if EnvStartupTraceDir is set,
// writing to <dir>/startup-trace-<name>-<pid>.out. It returns a function that
// stops and flushes the trace; callers should defer it so the trace is
// finalized on clean shutdown. The trace is also stopped automatically after a
// fixed duration so a finalized snapshot exists before the service is hard-killed
// by the SCM. It is a no-op (returning a no-op function) when the env var is unset.
func startStartupTrace(name string) func() {
	dir := os.Getenv(EnvStartupTraceDir)
	if dir == "" {
		return func() {}
	}

	path := filepath.Join(dir, fmt.Sprintf("startup-trace-%s-%d.out", name, os.Getpid()))
	f, err := os.Create(path)
	if err != nil {
		return func() {}
	}
	if err := trace.Start(f); err != nil {
		_ = f.Close()
		return func() {}
	}

	var once sync.Once
	stop := func() {
		once.Do(func() {
			trace.Stop()
			_ = f.Close()
		})
	}
	time.AfterFunc(startupTraceDuration(), stop)
	return stop
}

func startupTraceDuration() time.Duration {
	if s := os.Getenv(EnvStartupTraceDurationSeconds); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultStartupTraceDuration
}
