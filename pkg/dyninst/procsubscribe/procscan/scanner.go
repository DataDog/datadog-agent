// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"sync"
	"syscall"
	"time"

	"github.com/google/btree"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"

	model "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessID is a unique identifier for a process.
type ProcessID uint32

const (
	// DefaultMinProcessAge is how long a process must have been alive before it
	// is looked at.
	DefaultMinProcessAge = time.Second
	// DefaultRetryBackoffBase is the delay before the second attempt at reading
	// a process' tracer metadata. It matches the scan interval so that the
	// first retry happens on the next scan.
	DefaultRetryBackoffBase = 3 * time.Second
	// DefaultRetryBackoffCap is the longest delay between two attempts at
	// reading a process' tracer metadata.
	DefaultRetryBackoffCap = 5 * time.Minute
	// DefaultNonGoBackoffCap is the longest delay between two looks at a
	// process that is not running a Go binary, and therefore how late an exec
	// into one can be noticed. It is shorter than DefaultRetryBackoffCap
	// because identifying an executable costs a fraction of searching a process
	// for tracer metadata: at 2000 processes, re-identifying every one of them
	// this often costs well under a tenth of a percent of a core.
	DefaultNonGoBackoffCap = time.Minute
	// DefaultMaxCandidates bounds the candidate set, which holds every process
	// still awaiting a verdict rather than only the Go ones. It matches the
	// common pid_max default so that the set can hold an entire host; above the
	// bound, evicted processes lose their backoff and are re-examined sooner.
	// Entries are allocated on demand, so a host with few processes pays little.
	DefaultMaxCandidates = 32768

	// defaultExecutableCacheSize bounds the number of distinct executables for
	// which we remember whether they are Go binaries.
	defaultExecutableCacheSize = 1024
)

// procKey identifies a process. The start time is part of the identity so that
// a process which happens to reuse a pid does not inherit the status of the one
// that died.
type procKey struct {
	pid       uint32
	startTime ticks
}

func procKeyLess(a, b procKey) bool {
	if a.pid != b.pid {
		return a.pid < b.pid
	}
	return a.startTime < b.startTime
}

// candidate is the retry state for a process that has not been instrumented
// yet, whatever the reason.
type candidate struct {
	// startTime distinguishes this process from a later one that reuses its pid.
	startTime ticks
	// exeKey identifies the executable the last verdict was reached about, and
	// is zero if the executable could not be identified. An exec is the one way
	// a verdict about a process' executable stops being true without its pid or
	// start time changing, so a scan that finds a different key here knows to
	// start the schedule over.
	exeKey process.FileKey
	// seenAt is the time of the last scan that observed the process. Candidates
	// not observed by a scan have exited and are dropped.
	seenAt ticks
	// attempts is the number of metadata reads that have failed.
	attempts uint32
	// nextAttempt is the earliest time of the next metadata read.
	nextAttempt ticks
}

// Scanner reconciles the set of processes that should be instrumented against
// the set that is, once per Scan.
//
// Nothing about it is edge-triggered: a process is examined on every scan until
// it is either instrumented or gone, with an exponential backoff on the reads
// that turn out to be expensive. A read that fails because the tracer has not
// published its metadata yet is therefore retried for as long as the process
// lives, which is what makes a missed read, a slow scan or an agent restart
// self-healing.
//
// Thread-safety: Scan is not thread-safe, use from a single goroutine only.
type Scanner struct {
	// minAge is how long a process must have been alive before it is looked at.
	minAge ticks
	// backoffBase and backoffCap bound the delay between two metadata reads for
	// the same process.
	backoffBase ticks
	backoffCap  ticks
	// nonGoBackoffCap bounds the delay between two looks at a process that is
	// not running a Go binary. Revisiting one is much cheaper than searching a
	// process for tracer metadata, so it is worth doing more often.
	nonGoBackoffCap ticks

	// nowTicks returns the current time in ticks since boot.
	nowTicks func() (ticks, error)

	mu struct {
		sync.Mutex
		// live tracks discovered processes that have been reported as live.
		live *btree.BTreeG[procKey]
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

	// goExecutables caches whether an executable is a Go binary. This is the
	// filter that makes looking at every process on every scan affordable.
	goExecutables *lru.Cache[process.FileKey, bool]

	// candidates holds the retry state of processes whose tracer metadata is
	// not readable yet, keyed by pid.
	candidates *lru.Cache[uint32, *candidate]

	metrics Metrics
}

type scannerConfig struct {
	minProcessAge       time.Duration
	backoffBase         time.Duration
	backoffCap          time.Duration
	nonGoBackoffCap     time.Duration
	maxCandidates       int
	executableCacheSize int
}

var defaultScannerConfig = scannerConfig{
	minProcessAge:       DefaultMinProcessAge,
	backoffBase:         DefaultRetryBackoffBase,
	backoffCap:          DefaultRetryBackoffCap,
	nonGoBackoffCap:     DefaultNonGoBackoffCap,
	maxCandidates:       DefaultMaxCandidates,
	executableCacheSize: defaultExecutableCacheSize,
}

// Option configures a Scanner.
type Option interface {
	apply(*scannerConfig)
}

type optionFunc func(*scannerConfig)

func (f optionFunc) apply(c *scannerConfig) { f(c) }

// WithMinProcessAge sets how long a process must have been alive before the
// scanner looks at it.
func WithMinProcessAge(d time.Duration) Option {
	return optionFunc(func(c *scannerConfig) { c.minProcessAge = d })
}

// WithRetryBackoff sets the delay before the first retry of a tracer metadata
// read and the cap that the doubling delay grows to.
func WithRetryBackoff(base, maxDelay time.Duration) Option {
	return optionFunc(func(c *scannerConfig) {
		c.backoffBase, c.backoffCap = base, maxDelay
	})
}

// WithNonGoBackoffCap sets the longest delay between two looks at a process
// that is not running a Go binary, which bounds how late an exec into one is
// noticed.
func WithNonGoBackoffCap(d time.Duration) Option {
	return optionFunc(func(c *scannerConfig) { c.nonGoBackoffCap = d })
}

// WithMaxCandidates sets the maximum number of processes for which retry state
// is kept.
func WithMaxCandidates(n int) Option {
	return optionFunc(func(c *scannerConfig) { c.maxCandidates = n })
}

// NewScanner creates a new Scanner that discovers processes in the given
// procfs root.
func NewScanner(procfsRoot string, opts ...Option) *Scanner {
	reader := newStartTimeReader(procfsRoot)
	return newScanner(
		opts,
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
		func(pid int32) (model.TracerMetadata, error) {
			return readTracerMetadata(procfsRoot, pid)
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
	opts []Option,
	nowTicks func() (ticks, error),
	listPids func() iter.Seq2[uint32, error],
	readStartTime func(pid int32) (ticks, error),
	tracerMetadataReader func(pid int32) (model.TracerMetadata, error),
	resolveExecutable func(pid int32) (process.Executable, error),
	checkGoExecutable func(path string) (bool, error),
) *Scanner {
	cfg := defaultScannerConfig
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	s := &Scanner{
		minAge:               durationToTicks(cfg.minProcessAge),
		backoffBase:          durationToTicks(cfg.backoffBase),
		backoffCap:           durationToTicks(cfg.backoffCap),
		nonGoBackoffCap:      durationToTicks(cfg.nonGoBackoffCap),
		nowTicks:             nowTicks,
		listPids:             listPids,
		readStartTime:        readStartTime,
		tracerMetadataReader: tracerMetadataReader,
		resolveExecutable:    resolveExecutable,
		checkGoExecutable:    checkGoExecutable,
		goExecutables: mustNewLRU[process.FileKey, bool](
			cfg.executableCacheSize,
		),
		candidates: mustNewLRU[uint32, *candidate](cfg.maxCandidates),
	}
	s.mu.live = btree.NewG(16, procKeyLess)
	return s
}

// mustNewLRU panics if the cache creation fails, which only happens if the size
// is non-positive.
func mustNewLRU[K comparable, V any](size int) *lru.Cache[K, V] {
	c, err := lru.New[K, V](size)
	if err != nil {
		panic(err)
	}
	return c
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

// Stats returns a snapshot of the scanner's counters.
func (p *Scanner) Stats() map[string]any { return p.metrics.asStats() }

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

		// The start time is read on every scan rather than cached by pid: it is
		// the only thing that tells us whether this is still the same process,
		// so a cached copy would be exactly as stale as the answer we need.
		startTime, err := p.readStartTime(int32(pid))
		if err != nil {
			maybeLogErr("read start time", err)
			// The process is here, we just could not confirm which one it is.
			// Hold on to its retry state rather than let this scan look like
			// the process exited and reset its backoff.
			if c, ok := p.candidates.Peek(pid); ok {
				c.seenAt = now
			}
			continue
		}
		key := procKey{pid: pid, startTime: startTime}

		// Skip processes that are already instrumented.
		if _, ok := noLongerLive.Delete(key); ok {
			continue
		}

		// Give the process a moment to prove that it is not about to exit
		// before we attach probes to it and extract symbols from its binary.
		if startTime+p.minAge > now {
			continue
		}

		c := p.candidateFor(key, now)
		if c != nil && now < c.nextAttempt {
			continue
		}
		p.metrics.candidatesEvaluated.Add(1)

		// Failing to identify the executable backs the process off like any
		// other verdict. Every kernel thread fails here on every scan, and
		// there are hundreds of them. Backing off costs nothing when the
		// failure was really the process exiting, since the next scan will not
		// see it and will forget it.
		executable, err := p.resolveExecutable(int32(pid))
		if err != nil {
			p.metrics.executablesUnresolved.Add(1)
			maybeLogErr("resolve executable", err)
			p.scheduleRetry(key, c, process.FileKey{}, now, p.backoffCap)
			continue
		}
		if c != nil && c.exeKey != executable.Key {
			// The process exec'd, so whatever its previous executable earned
			// does not apply to this one. Start the schedule over instead of
			// making the new binary wait out the old one's backoff.
			p.metrics.executablesChanged.Add(1)
			c.attempts = 0
		}

		// Reading tracer metadata means walking the process' open file
		// descriptors, so only do it for processes that could plausibly have
		// published any.
		isGo, err := p.isGoExecutable(executable)
		if err != nil {
			maybeLogErr("analyze executable", err)
			p.scheduleRetry(key, c, executable.Key, now, p.backoffCap)
			continue
		}
		if !isGo {
			p.metrics.nonGoExecutables.Add(1)
			// The verdict is remembered per executable, but reaching it costs a
			// readlink, an open and a stat to identify the executable at all.
			// Back off so that cost is not paid every scan, under the shorter
			// cap, since revisiting the process is how an exec into a Go binary
			// gets noticed and that should not take five minutes.
			p.scheduleRetry(key, c, executable.Key, now, p.nonGoBackoffCap)
			continue
		}

		tracerMetadata, err := p.tracerMetadataReader(int32(pid))
		if err != nil {
			p.recordMetadataReadFailure(pid, err)
			p.scheduleRetry(key, c, executable.Key, now, p.backoffCap)
			continue
		}
		if tracerMetadata.TracerLanguage != "go" {
			// A Go binary reporting some other tracer language is not something
			// we can instrument, but back off rather than giving up so that the
			// only terminal state remains "the process exited".
			p.metrics.nonGoTracers.Add(1)
			log.Tracef(
				"scanner: pid %d reports tracer language %q",
				pid, tracerMetadata.TracerLanguage,
			)
			p.scheduleRetry(key, c, executable.Key, now, p.backoffCap)
			continue
		}

		p.candidates.Remove(pid)
		p.metrics.discovered.Add(1)
		ret = append(ret, DiscoveredProcess{
			PID:            pid,
			StartTimeTicks: uint64(startTime),
			TracerMetadata: tracerMetadata,
			Executable:     executable,
		})
	}

	removed = make([]ProcessID, 0, noLongerLive.Len())
	p.mu.Lock()
	noLongerLive.Ascend(func(key procKey) bool {
		removed = append(removed, ProcessID(key.pid))
		p.mu.live.Delete(key)
		return true
	})
	for _, newProc := range ret {
		p.mu.live.ReplaceOrInsert(procKey{
			pid:       newProc.PID,
			startTime: ticks(newProc.StartTimeTicks),
		})
	}
	p.mu.Unlock()
	noLongerLive.Clear(true)

	p.forgetExitedCandidates(now)
	return ret, removed, nil
}

// candidateFor returns the retry state of the given process, or nil if it has
// none. A candidate whose start time no longer matches belongs to a process
// that died and whose pid was reused.
func (p *Scanner) candidateFor(key procKey, now ticks) *candidate {
	c, ok := p.candidates.Peek(key.pid)
	if !ok || c.startTime != key.startTime {
		return nil
	}
	c.seenAt = now
	return c
}

// scheduleRetry records a failed attempt against the process running exeKey and
// pushes the next attempt out, doubling the delay each time up to maxDelay.
//
// A zero exeKey means the executable could not be identified this time. The next
// scan that does identify it will read that as an exec and restart the schedule,
// which costs a few cheap attempts and is the safe way to be wrong.
//
// There is no attempt limit. A tracer can publish its metadata at any point in
// the life of the process, and a process can exec into a Go binary at any point,
// so the backoff bounds the cost of waiting without ever ruling either out.
func (p *Scanner) scheduleRetry(
	key procKey, c *candidate, exeKey process.FileKey, now, maxDelay ticks,
) {
	if c == nil {
		c = &candidate{startTime: key.startTime}
	}
	c.exeKey = exeKey
	c.attempts++
	c.seenAt = now
	c.nextAttempt = now + p.retryDelay(c.attempts, maxDelay)
	// Adding refreshes the entry's position, so the candidate evicted when the
	// set is full is the one that has gone longest without a retry.
	if evicted := p.candidates.Add(key.pid, c); evicted {
		p.metrics.candidatesEvicted.Add(1)
	}
}

func (p *Scanner) retryDelay(attempts uint32, maxDelay ticks) ticks {
	delay := p.backoffBase
	for i := uint32(1); i < attempts && delay < maxDelay; i++ {
		delay *= 2
	}
	return min(delay, maxDelay)
}

// forgetExitedCandidates drops the retry state of processes that this scan did
// not observe. Backoff bounds the cost of retrying a live process forever; it
// does nothing for entries that will never be retried again.
func (p *Scanner) forgetExitedCandidates(now ticks) {
	for _, pid := range p.candidates.Keys() {
		if c, ok := p.candidates.Peek(pid); ok && c.seenAt != now {
			p.candidates.Remove(pid)
		}
	}
}

// isGoExecutable reports whether the given executable is a Go binary, parsing
// each distinct executable at most once.
//
// Failures are not cached: the answer must not depend on whether a file happened
// to be readable at the instant we first looked at it.
func (p *Scanner) isGoExecutable(exe process.Executable) (bool, error) {
	if isGo, ok := p.goExecutables.Get(exe.Key); ok {
		return isGo, nil
	}
	p.metrics.executablesAnalyzed.Add(1)
	isGo, err := p.checkGoExecutable(exe.Path)
	if err != nil {
		return false, err
	}
	p.goExecutables.Add(exe.Key, isGo)
	return isGo, nil
}

// recordMetadataReadFailure counts a failed tracer metadata read, separating
// the case where the tracer has not published anything yet, which resolves
// itself, from the case where we cannot read what it published, which does not.
func (p *Scanner) recordMetadataReadFailure(pid uint32, err error) {
	if errors.Is(err, errTracerMemfdNotFound) ||
		errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, syscall.ESRCH) {
		p.metrics.metadataNotPublished.Add(1)
		log.Tracef(
			"scanner: pid %d has not published tracer metadata: %v", pid, err,
		)
		return
	}
	p.metrics.metadataUnreadable.Add(1)
	if scannerLogLimiter.Allow() {
		log.Warnf("scanner: cannot read tracer metadata for pid %d: %v", pid, err)
	} else {
		log.Tracef("scanner: cannot read tracer metadata for pid %d: %v", pid, err)
	}
}

// LiveProcesses returns the list of processes that were alive as of the last
// call to Scan. This can be called concurrently with Scan.
func (p *Scanner) LiveProcesses() []ProcessID {
	p.mu.Lock()
	defer p.mu.Unlock()
	ret := make([]ProcessID, 0, p.mu.live.Len())
	p.mu.live.Ascend(func(key procKey) bool {
		ret = append(ret, ProcessID(key.pid))
		return true
	})
	return ret
}
