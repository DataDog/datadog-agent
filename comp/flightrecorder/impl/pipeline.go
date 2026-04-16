// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"
	"time"
)

// pipeline is a generic signal pipeline that owns one ring buffer for entries
// of type T, an optional ring buffer for context definitions, and a dedicated
// UDS connection. Each pipeline runs its own flush goroutines.
//
// Pipelines are fully independent: one slow pipeline cannot block or starve
// another because each has its own connection and goroutines.
type pipeline[T any] struct {
	transport     Transport
	flushInterval time.Duration
	counters      *counters

	entries ringBuf[T]
	ctxDefs *ringBuf[contextDef] // nil for pipelines without contexts (trace_stats)

	builderPool *builderPool

	encodeEntries encodeFunc[T]
	encodeCtx     encodeFunc[contextDef] // nil when ctxDefs is nil

	// Per-signal telemetry callbacks.
	signalType          string
	incSent             func(uint64)
	incDroppedOverflow  func(uint64)
	incDroppedTransport func(uint64)

	// Flush channels and lifecycle.
	entriesFlushCh chan struct{}
	ctxFlushCh     chan struct{} // nil when ctxDefs is nil
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

func newPipeline[T any](
	transport Transport,
	flushInterval time.Duration,
	entryCap int,
	ctxCap int,
	encodeEntries encodeFunc[T],
	encodeCtx encodeFunc[contextDef],
	signalType string,
	incSent func(uint64),
	incDroppedOverflow func(uint64),
	incDroppedTransport func(uint64),
	pool *builderPool,
	c *counters,
) *pipeline[T] {
	p := &pipeline[T]{
		transport:           transport,
		flushInterval:       flushInterval,
		counters:            c,
		entries:             newRingBuf[T](initialCap(entryCap), entryCap),
		builderPool:         pool,
		encodeEntries:       encodeEntries,
		encodeCtx:           encodeCtx,
		signalType:          signalType,
		incSent:             incSent,
		incDroppedOverflow:  incDroppedOverflow,
		incDroppedTransport: incDroppedTransport,
		entriesFlushCh:      make(chan struct{}, 1),
		stopCh:              make(chan struct{}),
	}

	goroutines := 1
	if ctxCap > 0 && encodeCtx != nil {
		r := newRingBuf[contextDef](initialCap(ctxCap), ctxCap)
		p.ctxDefs = &r
		p.ctxFlushCh = make(chan struct{}, 1)
		goroutines = 2
	}

	p.wg.Add(goroutines)
	go p.flushLoop(p.entriesFlushCh, p.flushEntries)
	if p.ctxDefs != nil {
		go p.flushLoop(p.ctxFlushCh, p.flushContextDefs)
	}
	return p
}

// AddEntry enqueues a signal entry (metric point, log entry, or trace stat).
func (p *pipeline[T]) AddEntry(e T) {
	if p.entries.add(e, func() { p.incDroppedOverflow(1) }) {
		signalCh(p.entriesFlushCh)
	}
}

// AddContextDef enqueues a context definition.
func (p *pipeline[T]) AddContextDef(d contextDef) {
	if p.ctxDefs.add(d, func() { p.incDroppedOverflow(1) }) {
		signalCh(p.ctxFlushCh)
	}
}

// Stop drains all buffers and stops flush goroutines.
func (p *pipeline[T]) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

func (p *pipeline[T]) flushLoop(flushCh <-chan struct{}, flushFn func()) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			flushFn()
			return
		case <-ticker.C:
			flushFn()
		case <-flushCh:
			flushFn()
			select {
			case <-ticker.C:
			default:
			}
		}
	}
}

func (p *pipeline[T]) flushEntries() {
	flushChunked(
		&p.entries,
		p.encodeEntries,
		p.builderPool,
		p.transport,
		p.counters,
		p.signalType,
		p.incSent,
		p.incDroppedTransport,
	)
}

func (p *pipeline[T]) flushContextDefs() {
	flushChunked(
		p.ctxDefs,
		p.encodeCtx,
		p.builderPool,
		p.transport,
		p.counters,
		p.signalType+"_contexts",
		p.incSent,
		p.incDroppedTransport,
	)
}
