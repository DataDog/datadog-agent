// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package procscan

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"maps"
	"sync"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	model "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessID is a unique identifier for a process.
type ProcessID uint32

const (
	// DefaultRetryBackoffCap is the cap that the doubling delay between two
	// looks at the same process grows to.
	DefaultRetryBackoffCap = 5 * time.Minute

	// maxCachedExecutables bounds each generation of the goExecutables cache.
	maxCachedExecutables = 1024
)

type procKey struct {
	pid       uint32
	startTime ticks
}

// candidate is the retry state for a process that has not been instrumented
// yet, whatever the reason.
type candidate struct {
	// startTime distinguishes this process from a later one that reuses its pid.
	startTime ticks
	// seenAt is the time of the last scan that observed the process.
	seenAt time.Time
	// attempts is the number of scans that have looked at this process without
	// instrumenting it.
	attempts uint32
	// nextAttempt is the earliest time of the next look at this process.
	nextAttempt time.Time
}

// Scanner reconciles the set of processes that should be instrumented against
// the set that is, once per Scan.
//
// Thread-safety: Scan is not thread-safe, use from a single goroutine only.
type Scanner struct {
	// backoffBase and backoffCap bound the delay between two metadata reads for
	// the same process.
	backoffBase time.Duration
	backoffCap  time.Duration

	// now returns the current time.
	now func() time.Time

	mu struct {
		sync.Mutex
		// live maps the pid of each process reported as live to the start time
		// that identifies it.
		live map[uint32]ticks
	}

	// listPids returns an iterator over all PIDs in the system.
	listPids func() iter.Seq2[uint32, error]

	// readStartTime reads the start time of a process in ticks since boot.
	readStartTime func(pid int32) (ticks, error)

	// tracerMetadataReader reads tracer metadata from a process.
	tracerMetadataReader func(pid int32) (model.TracerMetadata, error)

	// resolveExecutable resolves the executable metadata for a process.
	resolveExecutable func(pid int32) (process.Executable, error)

	// checkGoExecutable reports whether the executable at the given path is a
	// Go binary.
	checkGoExecutable func(path string) (bool, error)

	// goExecutables records, per executable, whether it is a Go binary, so that
	// each distinct executable is parsed at most once. Entries age out a
	// generation at a time: once the young map fills it becomes the old map and
	// a fresh one starts, and a hit in the old map is promoted back into the
	// young one. Anything still running therefore survives indefinitely while
	// executables that have gone away fall out after two generations.
	goExecutables    map[process.FileKey]bool
	goExecutablesOld map[process.FileKey]bool

	// candidates holds the retry state of processes that are not yet
	// instrumented, keyed by pid. Not just Go processes: every process a scan
	// looked at and did not instrument lands here, kernel threads included,
	// since no verdict is final.
	candidates map[uint32]*candidate
}

// NewScanner creates a new Scanner that discovers processes in the given procfs
// root. The delay before another look at a process that was not instrumented
// starts at backoffBase and doubles up to backoffCap.
func NewScanner(
	procfsRoot string, backoffBase, backoffCap time.Duration,
) *Scanner {
	reader := newStartTimeReader(procfsRoot)
	return newScanner(
		backoffBase, backoffCap,
		time.Now,
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
		func(pid int32) (model.TracerMetadata, error) {
			return tracermetadata.GetTracerMetadata(int(pid), procfsRoot)
		},
		func(pid int32) (process.Executable, error) {
			return process.ResolveExecutable(procfsRoot, pid)
		},
		isGoELFBinary,
	)
}

// newScanner creates a Scanner with injected dependencies. Used by NewScanner
// for production code and by tests for dependency injection.
func newScanner(
	backoffBase, backoffCap time.Duration,
	now func() time.Time,
	listPids func() iter.Seq2[uint32, error],
	readStartTime func(pid int32) (ticks, error),
	tracerMetadataReader func(pid int32) (model.TracerMetadata, error),
	resolveExecutable func(pid int32) (process.Executable, error),
	checkGoExecutable func(path string) (bool, error),
) *Scanner {
	s := &Scanner{
		backoffBase:          backoffBase,
		backoffCap:           backoffCap,
		now:                  now,
		listPids:             listPids,
		readStartTime:        readStartTime,
		tracerMetadataReader: tracerMetadataReader,
		resolveExecutable:    resolveExecutable,
		checkGoExecutable:    checkGoExecutable,
		goExecutables:        make(map[process.FileKey]bool),
		candidates:           make(map[uint32]*candidate),
	}
	s.mu.live = make(map[uint32]ticks)
	return s
}

// DiscoveredProcess represents a newly discovered process that should be
// instrumented.
type DiscoveredProcess struct {
	PID            uint32
	StartTimeTicks uint64
	model.TracerMetadata
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
	now := p.now()

	// Rate-limit logging about errors that are interesting.
	maybeLogErr := func(prefix string, err error) {
		if err == nil ||
			// These errors are expected and not interesting: the process may
			// have exited, or it may simply not have published any tracer
			// metadata, which is true of most processes on a host.
			errors.Is(err, fs.ErrNotExist) ||
			errors.Is(err, fs.ErrPermission) ||
			errors.Is(err, syscall.ESRCH) ||
			errors.Is(err, kernel.ErrMemFdFileNotFound) {
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
	noLongerLive := maps.Clone(p.mu.live)
	p.mu.Unlock()

	var ret []DiscoveredProcess

	for pid, err := range p.listPids() {
		if err != nil {
			return nil, nil, fmt.Errorf("list pids: %w", err)
		}

		// The start time is read on every scan rather than cached by pid: it is
		// the only thing that tells us whether this is still the same process,
		// so a cached copy would be exactly as stale as the answer we need.
		startTime, err := p.readStartTime(int32(pid))
		if err != nil {
			maybeLogErr("read start time", err)
			// The process is here, we just could not confirm which one it is.
			// Hold on to its retry state rather than let this scan look like
			// the process exited and reset its backoff.
			if c, ok := p.candidates[pid]; ok {
				c.seenAt = now
			}
			continue
		}
		key := procKey{pid: pid, startTime: startTime}

		// Skip processes that are already instrumented. A pid whose start time
		// no longer matches is a different process reusing the pid, so the entry
		// stays put and is reported as an exit below.
		if liveStart, ok := noLongerLive[pid]; ok && liveStart == startTime {
			delete(noLongerLive, pid)
			continue
		}

		c := p.candidateFor(key, now)
		if c != nil && now.Before(c.nextAttempt) {
			continue
		}

		// Failing to identify the executable backs the process off like any
		// other verdict. Every kernel thread fails here on every scan, and
		// there are hundreds of them. Backing off costs nothing when the
		// failure was really the process exiting, since the next scan will not
		// see it and will forget it.
		executable, err := p.resolveExecutable(int32(pid))
		if err != nil {
			maybeLogErr("resolve executable", err)
			p.scheduleRetry(key, c, now)
			continue
		}

		isGo, err := p.isGoExecutable(executable)
		if err != nil {
			maybeLogErr("analyze executable", err)
			p.scheduleRetry(key, c, now)
			continue
		}
		if !isGo {
			p.scheduleRetry(key, c, now)
			continue
		}

		tracerMetadata, err := p.tracerMetadataReader(int32(pid))
		if err != nil {
			maybeLogErr("read tracer metadata", err)
			p.scheduleRetry(key, c, now)
			continue
		}
		if tracerMetadata.TracerLanguage != "go" {
			log.Tracef(
				"scanner: pid %d reports tracer language %q",
				pid, tracerMetadata.TracerLanguage,
			)
			p.scheduleRetry(key, c, now)
			continue
		}

		delete(p.candidates, pid)
		ret = append(ret, DiscoveredProcess{
			PID:            pid,
			StartTimeTicks: uint64(startTime),
			TracerMetadata: tracerMetadata,
			Executable:     executable,
		})
	}

	removed = make([]ProcessID, 0, len(noLongerLive))
	p.mu.Lock()
	// Removals are applied before discoveries so that a reused pid ends up
	// mapped to the start time of the process that now holds it.
	for pid := range noLongerLive {
		removed = append(removed, ProcessID(pid))
		delete(p.mu.live, pid)
	}
	for _, newProc := range ret {
		p.mu.live[newProc.PID] = ticks(newProc.StartTimeTicks)
	}
	p.mu.Unlock()

	p.forgetExitedCandidates(now)
	return ret, removed, nil
}

// candidateFor returns the retry state of the given process, or nil if it has
// none. A candidate whose start time no longer matches belongs to a process
// that died and whose pid was reused.
func (p *Scanner) candidateFor(key procKey, now time.Time) *candidate {
	c, ok := p.candidates[key.pid]
	if !ok || c.startTime != key.startTime {
		return nil
	}
	c.seenAt = now
	return c
}

// scheduleRetry records a look that did not instrument the process and pushes
// the next one out, doubling the delay each time up to the cap.
//
// Every verdict short of a discovery lands here, and none of them is terminal.
// A tracer can publish its metadata at any point in the life of a process, and
// exec keeps a process' pid and start time, so a process that is not running a
// Go binary now may be running one later. The backoff bounds what asking again
// costs without ever ruling a process out.
func (p *Scanner) scheduleRetry(key procKey, c *candidate, now time.Time) {
	if c == nil {
		c = &candidate{startTime: key.startTime}
	}
	c.attempts++
	c.seenAt = now
	c.nextAttempt = now.Add(p.retryDelay(c.attempts))
	p.candidates[key.pid] = c
}

func (p *Scanner) retryDelay(attempts uint32) time.Duration {
	delay := p.backoffBase
	for i := uint32(1); i < attempts && delay < p.backoffCap; i++ {
		delay *= 2
	}
	return min(delay, p.backoffCap)
}

// forgetExitedCandidates drops the retry state of processes that this scan did
// not observe. Backoff bounds what retrying a live process costs; only this
// sweep bounds the map.
func (p *Scanner) forgetExitedCandidates(now time.Time) {
	for pid, c := range p.candidates {
		if !c.seenAt.Equal(now) {
			delete(p.candidates, pid)
		}
	}
}

// isGoExecutable reports whether the given executable is a Go binary, parsing
// each distinct executable at most once.
//
// Failures are not cached: the answer must not depend on whether a file happened
// to be readable at the instant we first looked at it.
func (p *Scanner) isGoExecutable(exe process.Executable) (bool, error) {
	if isGo, ok := p.goExecutables[exe.Key]; ok {
		return isGo, nil
	}
	if isGo, ok := p.goExecutablesOld[exe.Key]; ok {
		p.cacheGoExecutable(exe.Key, isGo)
		return isGo, nil
	}
	isGo, err := p.checkGoExecutable(exe.Path)
	if err != nil {
		return false, err
	}
	p.cacheGoExecutable(exe.Key, isGo)
	return isGo, nil
}

func (p *Scanner) cacheGoExecutable(key process.FileKey, isGo bool) {
	if len(p.goExecutables) >= maxCachedExecutables {
		p.goExecutablesOld = p.goExecutables
		p.goExecutables = make(map[process.FileKey]bool, maxCachedExecutables)
	}
	p.goExecutables[key] = isGo
}

// LiveProcesses returns the list of processes that were alive as of the last
// call to Scan. This can be called concurrently with Scan.
func (p *Scanner) LiveProcesses() []ProcessID {
	p.mu.Lock()
	defer p.mu.Unlock()
	ret := make([]ProcessID, 0, len(p.mu.live))
	for pid := range p.mu.live {
		ret = append(ret, ProcessID(pid))
	}
	return ret
}
