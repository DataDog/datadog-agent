// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultBufferSize    = 1024
	defaultBatchSize     = 50
	defaultFlushInterval = 10 * time.Second
)

// Options configures a Pipeline. Zero or negative values are replaced with
// defaults; an empty Processors slice is left empty (records pass through
// unchanged, which is equivalent to a single Noop processor).
type Options struct {
	// BufferSize is the capacity of the Source channel. Defaults to 1024.
	BufferSize int
	// BatchSize is the maximum number of records dispatched in a single
	// Sender.Send call. Defaults to 50.
	BatchSize int
	// FlushInterval is the maximum wall-clock time a partial batch waits
	// before being flushed. Defaults to 10 seconds.
	FlushInterval time.Duration
	// Processors are applied in order to each record before batching. A
	// processor returning nil drops the record. The default is empty
	// (no transformation); wiring code typically sets this to
	// []Processor{processors.Noop()} as scaffolding for future filters.
	Processors []Processor
}

// Pipeline buffers records, runs them through Processors, batches them,
// and dispatches batches via Sender. Submit is non-blocking and drops the
// oldest queued record on overflow so a slow Sender cannot back-pressure
// the calling goroutine.
type Pipeline struct {
	sender        Sender
	processors    []Processor
	batchSize     int
	flushInterval time.Duration

	in chan slog.Record

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
	started  atomic.Bool

	// submitMu protects the closed flag against concurrent Submit during
	// shutdown. It is RLocked on the Submit hot path so concurrent submits
	// are not serialized.
	submitMu sync.RWMutex
	closed   bool

	dropped atomic.Uint64
}

// NewPipeline constructs a Pipeline. The pipeline does not start its
// dispatch goroutine until Run is called.
func NewPipeline(sender Sender, opts Options) *Pipeline {
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = defaultFlushInterval
	}
	return &Pipeline{
		sender:        sender,
		processors:    opts.Processors,
		batchSize:     opts.BatchSize,
		flushInterval: opts.FlushInterval,
		in:            make(chan slog.Record, opts.BufferSize),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Submit non-blockingly enqueues r. If the buffer is full, the oldest
// queued record is dropped to make room; if the drop-and-retry races
// against another submitter and still cannot enqueue, the new record is
// dropped instead. Submit MUST NOT block the caller, so submission from
// the logging hot path remains safe.
func (p *Pipeline) Submit(r slog.Record) {
	p.submitMu.RLock()
	defer p.submitMu.RUnlock()
	if p.closed {
		p.dropped.Add(1)
		return
	}

	// Fast path: buffer has room.
	select {
	case p.in <- r:
		return
	default:
	}

	// Buffer full: drop one queued record (the oldest, since channels are
	// FIFO), count it, then try to enqueue r.
	select {
	case <-p.in:
		p.dropped.Add(1)
	default:
		// Lost the race - someone else drained between our checks.
	}
	select {
	case p.in <- r:
	default:
		// Still full under contention; drop r itself.
		p.dropped.Add(1)
	}
}

// Run is the dispatch loop. It runs until ctx is canceled or Drain is
// called. Run is safe to call exactly once per Pipeline; subsequent calls
// return immediately. Typical wiring spawns Run in its own goroutine.
func (p *Pipeline) Run(ctx context.Context) {
	if !p.started.CompareAndSwap(false, true) {
		return
	}
	defer close(p.doneCh)

	batch := make([]slog.Record, 0, p.batchSize)

	// Idle timer pattern: create stopped, Reset on first record in a batch,
	// Stop+drain on flush.
	timer := time.NewTimer(p.flushInterval)
	if !timer.Stop() {
		<-timer.C
	}
	timerActive := false

	stopTimer := func() {
		if !timerActive {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerActive = false
	}

	flush := func() {
		if len(batch) == 0 {
			return
		}
		p.send(ctx, batch)
		batch = batch[:0]
		stopTimer()
	}

	process := func(r slog.Record) {
		rp := &r
		for _, proc := range p.processors {
			rp = proc.Process(rp)
			if rp == nil {
				return
			}
		}
		if rp == nil {
			return
		}
		batch = append(batch, *rp)
		if len(batch) == 1 {
			timer.Reset(p.flushInterval)
			timerActive = true
		}
		if len(batch) >= p.batchSize {
			flush()
		}
	}

	for {
		select {
		case r := <-p.in:
			process(r)
		case <-timer.C:
			timerActive = false
			flush()
		case <-ctx.Done():
			flush()
			return
		case <-p.stopCh:
			// Drain whatever is still in the channel without blocking,
			// then flush and exit.
			for {
				select {
				case r := <-p.in:
					process(r)
				default:
					flush()
					return
				}
			}
		}
	}
}

// Drain stops accepting new records and waits for the Run goroutine to
// flush any pending batch. ctx limits how long to wait. If Run was never
// started, Drain blocks until ctx expires (the caller has bound the wait
// with their context). Calling Drain repeatedly is safe.
func (p *Pipeline) Drain(ctx context.Context) error {
	p.stopOnce.Do(func() {
		p.submitMu.Lock()
		p.closed = true
		p.submitMu.Unlock()
		close(p.stopCh)
	})

	select {
	case <-p.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Dropped returns the number of records dropped since Pipeline creation,
// either due to buffer overflow on Submit or due to repeated Sender
// failures on flush. Intended for instrumentation; not used internally.
func (p *Pipeline) Dropped() uint64 {
	return p.dropped.Load()
}

// send dispatches a batch via Sender, retrying once on transport error.
// On second failure the batch is dropped (records counted in p.dropped)
// to prevent a misbehaving backend from back-pressuring the source.
func (p *Pipeline) send(ctx context.Context, batch []slog.Record) {
	// Defensive copy: the caller reuses the underlying array.
	cp := make([]slog.Record, len(batch))
	copy(cp, batch)

	if err := p.sender.Send(ctx, cp); err == nil {
		return
	}
	if err := p.sender.Send(ctx, cp); err != nil {
		p.dropped.Add(uint64(len(cp)))
	}
}
