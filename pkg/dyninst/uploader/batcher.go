// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type sender interface {
	send(batch []json.RawMessage) error
}

type effects interface {
	sendBatch(id batchID, items []json.RawMessage)
	resetTimer(timestamp time.Time)
}

var _ effects = (*batcher)(nil)

type batchID uint64

type sendResult struct {
	id  batchID
	err error // nil if success, non-nil if failure
}

type batcher struct {
	enqueueCh    chan json.RawMessage
	sendResultCh chan sendResult
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	state        *batcherState
	timer        *time.Timer
	sender       sender

	// stopOnce guarantees that shutdown sequence is only executed once even if
	// Stop is invoked multiple times from different goroutines.
	stopOnce sync.Once
}

func newBatcher(sender sender, opts ...Option) *batcher {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	b := &batcher{
		enqueueCh:    make(chan json.RawMessage),
		sendResultCh: make(chan sendResult),
		ctx:          ctx,
		cancel:       cancel,
		state:        newBatcherState(cfg.batcherConfig),
		timer:        timer,
		sender:       sender,
	}

	b.wg.Add(1)
	go b.run()
	return b
}

func (b *batcher) enqueue(data json.RawMessage) {
	select {
	case b.enqueueCh <- data:
	case <-b.ctx.Done():
		log.Warnf("uploader stopped, dropping message")
	}
}

func (b *batcher) stop() {
	// Ensure the shutdown sequence runs only once.
	b.stopOnce.Do(func() {
		// Cancelling the context will unblock the run loop (via the ctx.Done()
		// case) regardless of what it is currently waiting on, and will also
		// cause any in-flight sender goroutines to take the ctx.Done() branch
		// in their select, preventing them from blocking trying to report the
		// outcome back on the events channel.
		b.cancel()

		// Wait for the run loop and any in-flight sender goroutines to finish.
		b.wg.Wait()
	})
}

func (b *batcher) run() {
	defer b.wg.Done()
	defer b.timer.Stop()

	for {
		select {
		case data := <-b.enqueueCh:
			b.state.handleEnqueueEvent(data, time.Now(), b)
		case ts := <-b.timer.C:
			b.state.handleTimerFiredEvent(ts, b)
		case result := <-b.sendResultCh:
			if err := b.state.handleBatchOutcomeEvent(result, b); err != nil {
				log.Warnf("failed to handle batch outcome event: %v", err)
			}
		case <-b.ctx.Done():
			b.state.handleStopEvent()
			return
		}
	}
}

func (b *batcher) sendBatch(id batchID, items []json.RawMessage) {
	if len(items) == 0 {
		return
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		outcome := sendResult{id: id}
		outcome.err = b.sender.send(items)
		if outcome.err != nil {
			log.Errorf("failed to send batch: %v", outcome.err)
		}
		select {
		case b.sendResultCh <- outcome:
		case <-b.ctx.Done():
		}
	}()
}

func (b *batcher) resetTimer(timestamp time.Time) {
	if !b.timer.Stop() {
		select {
		case <-b.timer.C:
		default:
		}
	}
	if !timestamp.IsZero() {
		b.timer.Reset(time.Until(timestamp))
	}
}
