// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

// Package sigdanger handles SIGDANGER, the AIX signal the kernel sends when
// system paging space is nearly exhausted. The Go runtime silently absorbs it
// when nothing registers interest, and the kernel can later kill the process
// with SIGKILL without leaving any trace in the logs — this handler logs a
// last warning first.
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

// handleInterval is the minimum delay between two handled SIGDANGERs; the
// kernel can send it repeatedly while paging space stays low.
const handleInterval = 30 * time.Second

var (
	mu          sync.Mutex
	lastHandled time.Time

	// Overridden in tests.
	now          = time.Now
	freeOSMemory = debug.FreeOSMemory
)

// Init installs the SIGDANGER handler. Call it once per process.
func Init() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, unix.SIGDANGER)
	go func() {
		for range ch {
			handle()
		}
	}()
}

// handle logs a critical warning and releases memory, rate-limited by
// handleInterval.
func handle() {
	mu.Lock()
	if now().Sub(lastHandled) < handleInterval {
		mu.Unlock()
		return
	}
	lastHandled = now()
	mu.Unlock()

	log.Critical("Received SIGDANGER: AIX paging space is low, the kernel may soon kill this process. " +
		"Forcing a garbage collection to release memory. Check paging space usage with `lsps -s`.")

	freeOSMemory()
}
