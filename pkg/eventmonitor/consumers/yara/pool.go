// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// PoolConfig configures a Pool
type PoolConfig struct {
	// Workers is the number of concurrent scans. It must be at least 1.
	Workers int
	// QueueSize is the number of jobs that can wait for a worker before Submit drops. It may be 0,
	// in which case a job is only accepted when a worker is idle.
	QueueSize int
	// ScanTimeout bounds each scan. 0 disables the timeout.
	ScanTimeout time.Duration
	// MaxFileSize is the largest file the reader hands to the pool. It is only used to compute
	// the default MaxBufferedBytes.
	MaxFileSize int64
	// MaxBufferedBytes caps the total size of the Data of every job held by the pool, queued and
	// in flight. When it is 0 it defaults to Workers × MaxFileSize. When both are 0 the pool
	// holds at most Workers + QueueSize jobs of any size.
	MaxBufferedBytes int64
}

// byteBudget returns the effective byte budget, or 0 for no budget
func (c PoolConfig) byteBudget() int64 {
	if c.MaxBufferedBytes > 0 {
		return c.MaxBufferedBytes
	}
	if c.MaxFileSize > 0 {
		return int64(c.Workers) * c.MaxFileSize
	}
	return 0
}

// errScanPanic is reported when the scanner panicked
var errScanPanic = errors.New("scanner panicked")

// Pool is a ScanPool that runs scans on a fixed number of worker goroutines, fed by a bounded
// queue.
//
// Memory: a queued job holds its Data just like a job being scanned, so bounding only the queue
// length is not enough (64 queued jobs of 64 MB each would hold 4 GB). The pool therefore
// enforces a byte budget over the Data of every job it holds, queued and in flight, and Submit
// drops a job that would go over it. The default budget is Workers × MaxFileSize, the memory
// bound of the design. Since a single job can't be larger than MaxFileSize, the workers can
// always be kept busy with large files, while small files can fill the whole queue.
//
// Stop cancels the scans in flight and drops the queued jobs: every job still gets its Done
// call and its hash released, but no report.
type Pool struct {
	cfg      PoolConfig
	budget   int64
	scanner  Scanner
	deduper  Deduper
	reporter Reporter
	stats    *Stats

	queue chan ScanJob

	// ctx is cancelled by Stop to abort the scans in flight
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// mu guards the fields below. Submit holds it while it does a non-blocking send on queue, so
	// that no job can enter the queue after Stop has drained it.
	mu       sync.Mutex
	buffered int64
	started  bool
	stopped  bool
}

var _ ScanPool = (*Pool)(nil)

// NewPool returns a new Pool. Jobs can be submitted before Start; they wait in the queue.
// stats may be nil.
func NewPool(cfg PoolConfig, scanner Scanner, deduper Deduper, reporter Reporter, stats *Stats) (*Pool, error) {
	if cfg.Workers < 1 {
		return nil, fmt.Errorf("yara: invalid workers count %d, must be at least 1", cfg.Workers)
	}
	if cfg.QueueSize < 0 {
		return nil, fmt.Errorf("yara: invalid queue size %d, must not be negative", cfg.QueueSize)
	}
	if cfg.ScanTimeout < 0 {
		return nil, fmt.Errorf("yara: invalid scan timeout %s, must not be negative", cfg.ScanTimeout)
	}
	if cfg.MaxFileSize < 0 || cfg.MaxBufferedBytes < 0 {
		return nil, errors.New("yara: file size and buffered bytes limits must not be negative")
	}
	if scanner == nil || deduper == nil || reporter == nil {
		return nil, errors.New("yara: scanner, deduper and reporter are required")
	}
	if stats == nil {
		stats = &Stats{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		cfg:      cfg,
		budget:   cfg.byteBudget(),
		scanner:  scanner,
		deduper:  deduper,
		reporter: reporter,
		stats:    stats,
		queue:    make(chan ScanJob, cfg.QueueSize),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Start starts the workers. It is a no-op if the pool was already started or stopped.
func (p *Pool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.stopped {
		return
	}
	p.started = true
	p.wg.Add(p.cfg.Workers)
	for i := 0; i < p.cfg.Workers; i++ {
		go p.worker()
	}
}

// Stop cancels the scans in flight, drops the queued jobs and waits for the workers to exit.
// Every job the pool accepted has had its Done called when Stop returns. Stop returns once
// every in-flight Scan has returned, so it relies on the Scanner honoring ctx cancellation.
// Submit drops every job after Stop. Stop is safe to call more than once.
func (p *Pool) Stop() {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()

	p.cancel()
	p.wg.Wait()

	// No job can enter the queue anymore, and the workers are gone: drop what is left.
	for {
		select {
		case job := <-p.queue:
			p.release(job)
		default:
			return
		}
	}
}

// Submit implements ScanPool. It never blocks.
func (p *Pool) Submit(job ScanJob) bool {
	size := int64(len(job.Data))

	p.mu.Lock()
	accepted := false
	if !p.stopped && (p.budget == 0 || p.buffered+size <= p.budget) {
		select {
		case p.queue <- job:
			p.buffered += size
			accepted = true
		default:
		}
	}
	p.mu.Unlock()

	if !accepted {
		p.stats.QueueDrops.Add(1)
		p.deduper.ReleaseHash(job.Sum)
		callDone(job)
	}
	return accepted
}

// QueueDepth implements ScanPool
func (p *Pool) QueueDepth() int {
	return len(p.queue)
}

// bufferedBytes returns the size of the Data held by the pool
func (p *Pool) bufferedBytes() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buffered
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		// check for Stop first, so that a worker never picks a new job once it was requested
		if p.ctx.Err() != nil {
			return
		}
		select {
		case <-p.ctx.Done():
			return
		case job := <-p.queue:
			p.process(job)
		}
	}
}

// process scans a job, and always releases it
func (p *Pool) process(job ScanJob) {
	if p.ctx.Err() != nil {
		// Stop raced with the dequeue
		p.release(job)
		return
	}

	defer p.finish(job)

	ctx := p.ctx
	if p.cfg.ScanTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.ScanTimeout)
		defer cancel()
	}

	start := time.Now()
	matches, err := p.scan(ctx, job.Data)
	duration := time.Since(start)

	switch {
	case err == nil:
		p.stats.Scans.Add(1)
		// Matches counts individual rule matches, not scans with at least one match: a file
		// matching 3 rules adds 3. Scans with a match are the "yara: match" log lines.
		p.stats.Matches.Add(int64(len(matches)))
	case p.ctx.Err() != nil:
		// cancelled by Stop: not a scan failure, and not a completed scan attempt either
		p.deduper.ReleaseHash(job.Sum)
		return
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded:
		p.stats.ScanTimeouts.Add(1)
		p.deduper.ReleaseHash(job.Sum)
	default:
		p.stats.ScanErrors.Add(1)
		p.deduper.ReleaseHash(job.Sum)
	}

	p.stats.ScanDone(duration)
	p.reporter.Report(job.File, job.Sum, matches, err)
}

// scan runs the scanner, turning a panic into an error so that a scanner bug can't crash
// system-probe
func (p *Pool) scan(ctx context.Context, data []byte) (matches []Match, err error) {
	defer func() {
		if r := recover(); r != nil {
			matches = nil
			err = fmt.Errorf("%w: %v", errScanPanic, r)
		}
	}()
	return p.scanner.Scan(ctx, data)
}

// release drops a job that was accepted but will not be scanned
func (p *Pool) release(job ScanJob) {
	p.deduper.ReleaseHash(job.Sum)
	p.finish(job)
}

// finish returns the job's bytes to the budget and calls its Done
func (p *Pool) finish(job ScanJob) {
	p.mu.Lock()
	p.buffered -= int64(len(job.Data))
	p.mu.Unlock()
	callDone(job)
}

func callDone(job ScanJob) {
	if job.Done != nil {
		job.Done()
	}
}
