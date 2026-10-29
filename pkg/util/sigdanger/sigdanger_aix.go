// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

// Package sigdanger installs a handler for SIGDANGER, the AIX-specific signal
// the kernel sends to processes when system paging space is nearly exhausted
// ("system crash imminent; free up some page space", signal 33).
//
// Without this handler the signal is invisible to the Agent: the Go runtime
// knows it only as "signal 33" (sigtable: _SigNotify) and silently absorbs it
// when nothing has called signal.Notify for it. If paging space is later fully
// exhausted, the kernel kills processes with SIGKILL — an uncatchable,
// unloggable death that leaves no trace in agent-stderr.log. Catching SIGDANGER
// gives the Agent (and the operator, via the logs) a last warning before that
// happens.
package sigdanger

import (
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// handleInterval is the minimum delay between two handled SIGDANGERs. The
// kernel can send the signal repeatedly while paging space stays low; we do
// not want to spam the logs or run full GCs back to back.
const handleInterval = 30 * time.Second

var (
	initOnce sync.Once

	mu          sync.Mutex
	lastHandled time.Time

	// Overridden in tests.
	now          = time.Now
	freeOSMemory = debug.FreeOSMemory
)

// Init installs the SIGDANGER handler. It is safe to call multiple times; only
// the first call installs the handler.
func Init() {
	initOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, unix.SIGDANGER)
		go func() {
			for range ch {
				handle()
			}
		}()
	})
}

// handle logs a critical warning and returns memory to the OS, honoring
// handleInterval.
func handle() {
	mu.Lock()
	if now().Sub(lastHandled) < handleInterval {
		mu.Unlock()
		return
	}
	lastHandled = now()
	mu.Unlock()

	log.Critical("Received SIGDANGER: AIX paging space is low and the kernel may soon kill this process to free some. " +
		"Forcing a garbage collection to release memory. Check paging space usage with `lsps -s` and consider increasing its size.")

	// The documented intent of SIGDANGER is that receivers free up paging
	// space; return all free heap memory to the OS.
	freeOSMemory()
}
