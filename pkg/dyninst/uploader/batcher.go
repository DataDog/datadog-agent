// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"encoding/json"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type sender interface {
	send(batch []json.RawMessage) error
}

type effects interface {
	sendBatch(id batchID, items []json.RawMessage)
	resetTimer(timestamp time.Time)
	clearTimer()
}

var _ effects = (*batcher)(nil)

type batchID uint64

type sendResult struct {
	id  batchID
	err error // nil if success, non-nil if failure
}

type batcher struct {
	name          string
	enqueueCh     chan json.RawMessage
	sendResultCh  chan sendResult
	stopCh        chan struct{} // closed by stop() to signal run() to begin shutdown
	stoppedCh     chan struct{} // closed by run() when fully drained
	wg            sync.WaitGroup
	state         *batcherState
	timer         *time.Timer
	sender        sender
	stopOnce      sync.Once
	errLogLimiter *rate.Limiter
}

func newBatcher(name string, sender sender, cfg batcherConfig, metrics *Metrics) *batcher {
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	b := &batcher{
		name:         name,
		enqueueCh:    make(chan json.RawMessage),
		sendResultCh: make(chan sendResult),
		stopCh:       make(chan struct{}),
		stoppedCh:    make(chan struct{}),
		state:        newBatcherState(name, cfg, metrics),
		timer:        timer,
		sender:       sender,
		// Used to rate-limit log messages about failed batches.
		//
		// TODO: Keep metrics for the number of failed batches and rate limit
		// the actual attempts to send batches.
		errLogLimiter: rate.NewLimiter(rate.Every(10*time.Second), 1),
	}

	b.wg.Add(1)
	go b.run()
	return b
}

func (b *batcher) enqueue(data json.RawMessage) {
	select {
	case b.enqueueCh <- data:
	case <-b.stoppedCh: // batcher.run is stopped, drop message
	}
}

func (b *batcher) stop() {
	b.stopOnce.Do(func() {
		log.Debugf("stopping batcher %s", b.name)
		defer log.Debugf("batcher %s stopped", b.name)
		// Signal the run loop to begin shutdown.
		close(b.stopCh)

		// Wait for the run loop and any in-flight sender goroutines to finish.
		b.wg.Wait()
	})
}

func (b *batcher) run() {
	defer b.wg.Done()
	defer b.timer.Stop()
	defer close(b.stoppedCh)

	name := any(b.name) // avoid allocating a new string for each log message

	// Phase 1: Normal event loop.
	for {
		select {
		case data := <-b.enqueueCh:
			log.Tracef(
				"uploader %s: received enqueue event of %d bytes",
				name, len(data),
			)
			b.state.handleEnqueueEvent(data, time.Now(), b)
		case <-b.timer.C:
			log.Tracef(
				"uploader %s: timer fired event", name,
			)
			if err := b.state.handleTimerFiredEvent(b); err != nil {
				log.Warnf(
					"uploader %s: failed to handle timer fired event: %v",
					name, err,
				)
			}
		case result := <-b.sendResultCh:
			b.handleSendResult(name, result)
		case <-b.stopCh:
			log.Debugf("uploader %s: received stop event", name)
			b.state.handleStopEvent(b)
			// Fall through to drain loop.
			goto drain
		}
	}

drain:
	// Phase 2: Drain in-flight batches.
	for !b.state.drainComplete() {
		result := <-b.sendResultCh
		b.handleSendResult(name, result)
	}
}

func (b *batcher) handleSendResult(name any, result sendResult) {
	stats, err := b.state.handleBatchOutcomeEvent(result, b)
	if err != nil {
		log.Warnf(
			"uploader %s: failed to handle batch outcome event: %v",
			name, err,
		)
		return
	}
	if result.err != nil {
		if b.errLogLimiter.Allow() {
			log.Warnf(
				"uploader %s: batch outcome id=%d (items=%d, bytes=%d): err=%v",
				name, result.id, stats.items, stats.bytes, result.err,
			)
		} else if log.ShouldLog(log.DebugLvl) {
			log.Debugf(
				"uploader %s: batch outcome id=%d (items=%d, bytes=%d): err=%v",
				name, result.id, stats.items, stats.bytes, result.err,
			)
		}
	} else if log.ShouldLog(log.TraceLvl) {
		log.Tracef(
			"uploader %s: batch outcome id=%d (items=%d, bytes=%d): success",
			name, result.id, stats.items, stats.bytes,
		)
	}
}

func (b *batcher) sendBatch(id batchID, items []json.RawMessage) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		res := sendResult{id: id}
		res.err = b.sender.send(items)
		b.sendResultCh <- res
	}()
}

func (b *batcher) resetTimer(timestamp time.Time) {
	b.clearTimer()
	b.timer.Reset(time.Until(timestamp))
}

func (b *batcher) clearTimer() {
	if !b.timer.Stop() {
		select {
		case <-b.timer.C:
		default:
		}
	}
}
