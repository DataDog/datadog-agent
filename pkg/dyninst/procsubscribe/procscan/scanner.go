// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"sync"
	"syscall"
	"time"

	"github.com/google/btree"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessID is a unique identifier for a process.
type ProcessID uint32

// timeWindow represents a time range for process discovery based on how long
// a process has been alive. The window advances with each scan, capturing
// processes that have "aged into" the eligibility threshold.
type timeWindow struct {
	// startDelay is how long a process must be alive before being eligible.
	startDelay ticks
}

// contains checks if a process start time falls within this window for the
// given current time.
func (w *timeWindow) contains(startTime, now, lastScan ticks) bool {
	lastWatermark := computeWatermark(lastScan, w.startDelay)
	nextWatermark := computeWatermark(now, w.startDelay)
	return startTime > lastWatermark && startTime <= nextWatermark
}

// computeNextWatermark calculates the upper bound of the current time window.
func computeWatermark(now ticks, delay ticks) ticks {
	if now < delay {
		return 0
	}
	return now - delay
}

// Scanner discovers Go processes for instrumentation using a watermark-based
// algorithm. Processes are analyzed exactly once when they've been alive for
// at least the duration specified by one of the time windows. Processes that
// exit before becoming eligible are never analyzed.
//
// Thread-safety: Scan is not thread-safe, use from a single goroutine only.
type Scanner struct {
	// lastScan is the last scan time in ticks since boot.
	lastScan ticks
	// windows defines the time windows for process eligibility. A process is
	// analyzed if its start time falls within any of these windows.
	windows []timeWindow

	// nowTicks returns the current time in ticks since boot.
	nowTicks func() (ticks, error)

	mu struct {
		sync.Mutex
		// live tracks discovered processes that have been reported as live.
		live *btree.BTreeG[uint32]
	}

	// listPids returns an iterator over all PIDs in the system.
	listPids func() iter.Seq2[uint32, error]

	// readStartTime reads the start time of a process in ticks since boot.
	readStartTime func(pid int32) (ticks, error)

	// tracerMetadataReader reads tracer metadata from a process.
	tracerMetadataReader func(pid int32) (tracermetadata.TracerMetadata, error)

	// resolveExecutable resolves the executable metadata for a process.
	resolveExecutable func(pid int32) (process.Executable, error)

	// startTimeCache caches process start times keyed by PID.
	startTimeCache startTimeCache
}

// NewScanner creates a new Scanner that discovers processes in the given
// procfs root.
//
// Each processDelay defines a time window for discovering processes. A process
// becomes eligible for discovery when it has been alive for at least the
// specified delay. Multiple delays provide redundancy: if metadata isn't
// available when the first window covers a process, a longer delay window may
// still catch it later. Windows with smaller delays catch processes sooner but
// have less time for metadata to become available.
func NewScanner(
	procfsRoot string,
	processDelays ...time.Duration,
) *Scanner {
	windows := make([]timeWindow, 0, len(processDelays))
	for _, delay := range processDelays {
		windows = append(windows, timeWindow{
			startDelay: ticks(
				(delay.Nanoseconds() * int64(clkTck)) / time.Second.Nanoseconds(),
			),
		})
	}
	reader := newStartTimeReader(procfsRoot)
	return newScanner(
		windows,
		nowTicks,
		func() iter.Seq2[uint32, error] {
			return listPids(procfsRoot, 512)
		},
		func(pid int32) (ticks, error) {
			startTime, err := reader.read(pid)
			if err != nil {
				return 0, err
			}
			return ticks(startTime), nil
		},
		func(pid int32) (tracermetadata.TracerMetadata, error) {
			return tracermetadata.GetTracerMetadata(int(pid), procfsRoot)
		},
		func(pid int32) (process.Executable, error) {
			return process.ResolveExecutable(procfsRoot, pid)
		},
	)
}

// newScanner creates a Scanner with injected dependencies. Used by NewScanner
// for production code and by tests for dependency injection.
func newScanner(
	windows []timeWindow,
	nowTicks func() (ticks, error),
	listPids func() iter.Seq2[uint32, error],
	readStartTime func(pid int32) (ticks, error),
	tracerMetadataReader func(pid int32) (tracermetadata.TracerMetadata, error),
	resolveExecutable func(pid int32) (process.Executable, error),
) *Scanner {
	s := &Scanner{
		windows:              windows,
		nowTicks:             nowTicks,
		listPids:             listPids,
		readStartTime:        readStartTime,
		tracerMetadataReader: tracerMetadataReader,
		resolveExecutable:    resolveExecutable,
		startTimeCache:       makeStartTimeCache(defaultStartTimeCacheSize),
	}
	s.mu.live = btree.NewG(16, cmp.Less[uint32])
	return s
}

// DiscoveredProcess represents a newly discovered process that should be
// instrumented.
type DiscoveredProcess struct {
	PID            uint32
	StartTimeTicks uint64
	tracermetadata.TracerMetadata
	Executable process.Executable
}

// scannerLogLimiter rate-limits non-interesting errors during scanning to
// avoid log spam from common transient errors like ENOENT and ESRCH.
var scannerLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

// Scan discovers new Go processes and detects removed processes since the last
// Scan call.
//
// Returns:
//   - new: Processes discovered in this scan
//   - removed: Processes that have exited since the last scan
//   - err: Fatal error that prevented the scan from completing
func (p *Scanner) Scan() (
	new []DiscoveredProcess,
	removed []ProcessID,
	err error,
) {
	now, err := p.nowTicks()
	if err != nil {
		return nil, nil, fmt.Errorf("get timestamp: %w", err)
	}

	// Rate-limit logging about errors that are interesting.
	maybeLogErr := func(prefix string, err error) {
		if err == nil ||
			// These errors are expected and not interesting (process may have
			// exited, etc).
			errors.Is(err, fs.ErrNotExist) ||
			errors.Is(err, fs.ErrPermission) ||
			errors.Is(err, syscall.ESRCH) {
			return
		}
		if scannerLogLimiter.Allow() {
			log.Warnf("scanner: %s: %v", prefix, err)
		} else {
			log.Tracef("scanner: %s: %v", prefix, err)
		}
	}

	// Clone the live set. Processes still alive will be removed from this
	// clone. Whatever remains has exited.
	p.mu.Lock()
	noLongerLive := p.mu.live.Clone()
	p.mu.Unlock()

	var ret []DiscoveredProcess

	for pid, err := range p.listPids() {
		if err != nil {
			return nil, nil, fmt.Errorf("list pids: %w", err)
		}

		// Skip processes we've already discovered.
		if _, ok := noLongerLive.Delete(pid); ok {
			continue
		}

		// Only analyze processes whose start time falls within a time window.
		startTime, ok := p.startTimeCache.getStartTime(pid)
		if !ok {
			if startTime, err = p.readStartTime(int32(pid)); err != nil {
				maybeLogErr("read start time", err)
				continue
			}
			p.startTimeCache.insert(pid, startTime)
		}
		if !p.matchesAnyWindow(startTime, now, p.lastScan) {
			continue
		}

		// Only instrument Go processes.
		tracerMetadata, err := p.tracerMetadataReader(int32(pid))
		if err != nil {
			continue
		}
		if tracerMetadata.TracerLanguage != "go" {
			continue
		}

		executable, err := p.resolveExecutable(int32(pid))
		if err != nil {
			maybeLogErr("resolve executable", err)
			continue
		}

		ret = append(ret, DiscoveredProcess{
			PID:            pid,
			StartTimeTicks: uint64(startTime),
			TracerMetadata: tracerMetadata,
			Executable:     executable,
		})
	}

	removed = make([]ProcessID, 0, noLongerLive.Len())
	noLongerLive.Ascend(func(pid uint32) bool {
		removed = append(removed, ProcessID(pid))
		p.mu.live.Delete(pid)
		return true
	})
	noLongerLive.Clear(true)

	p.mu.Lock()
	for _, newProc := range ret {
		p.mu.live.ReplaceOrInsert(newProc.PID)
	}
	p.mu.Unlock()

	p.lastScan = now
	p.startTimeCache.sweep()
	return ret, removed, nil
}

// matchesAnyWindow returns true if the given start time falls within any of
// the scanner's time windows.
func (p *Scanner) matchesAnyWindow(startTime, now, lastScan ticks) bool {
	for i := range p.windows {
		if p.windows[i].contains(startTime, now, lastScan) {
			return true
		}
	}
	return false
}

// LiveProcesses returns the list of processes that were alive as of the last
// call to Scan. This can be called concurrently with Scan.
func (p *Scanner) LiveProcesses() []ProcessID {
	p.mu.Lock()
	defer p.mu.Unlock()
	ret := make([]ProcessID, 0, p.mu.live.Len())
	p.mu.live.Ascend(func(pid uint32) bool {
		ret = append(ret, ProcessID(pid))
		return true
	})
	return ret
}
