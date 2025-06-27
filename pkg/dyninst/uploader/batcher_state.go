// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: Implement limits on the number of batches allowed to be in flight.
// also we'll want to have some rate limits that eventually lead to dropping
// data if we queue too much in order to bound memory usage.

type batcherState struct {
	cfg                batcherConfig
	batch              []json.RawMessage
	currentSize        int
	nextIdleFlush      time.Time
	nextMaxBufferFlush time.Time
	metrics            *Metrics
	nextBatchID        batchID
	inflight           map[batchID]batchStat
}

// batchStat holds the size information for a batch that has been sent but is
// still awaiting an outcome.
type batchStat struct {
	items int
	bytes int
}

func newBatcherState(cfg batcherConfig) *batcherState {
	return &batcherState{
		cfg:      cfg,
		metrics:  &Metrics{},
		inflight: make(map[batchID]batchStat),
	}
}

func (s *batcherState) handleEnqueueEvent(data json.RawMessage, now time.Time, eff effects) {
	log.Debugf("uploader received enqueue event of %d", len(data))
	isFirstItem := len(s.batch) == 0
	s.batch = append(s.batch, data)
	s.currentSize += len(data)

	if isFirstItem && s.cfg.maxBufferDuration > 0 {
		s.nextMaxBufferFlush = now.Add(s.cfg.maxBufferDuration)
	}

	if s.cfg.idleFlushDuration > 0 {
		s.nextIdleFlush = now.Add(s.cfg.idleFlushDuration)
	}

	// If we will flush immediately, skip arming the timer first to avoid a double
	// reset for the same event.
	shouldFlushDueToSize := len(s.batch) >= s.cfg.maxItems ||
		s.currentSize >= s.cfg.maxSizeBytes

	if shouldFlushDueToSize {
		s.flush(eff)
		s.clearDeadlines()
		eff.resetTimer(time.Time{})
	} else {
		// (Re-)arm the timer to the earliest upcoming deadline.
		eff.resetTimer(earliest(s.nextIdleFlush, s.nextMaxBufferFlush))
	}
}

func (s *batcherState) handleTimerFiredEvent(now time.Time, eff effects) {
	log.Debugf("uploader received timer fired event")
	if len(s.batch) == 0 {
		return
	}
	shouldFlushDueToTime := !s.nextIdleFlush.After(now) ||
		!s.nextMaxBufferFlush.After(now)
	if shouldFlushDueToTime {
		if s.flush(eff) {
			s.clearDeadlines()
			eff.resetTimer(time.Time{})
		}
	}
}

// handleBatchOutcomeEvent handles the outcome of a batch that has been sent.
// It updates the metrics and removes the batch from the inflight map.
//
// If the batch was not expected, it return an error. The state is not
// modified in this case -- but it does imply an invariant violation.
func (s *batcherState) handleBatchOutcomeEvent(res sendResult, _ effects) error {
	stats, ok := s.inflight[res.id]
	if !ok {
		// unknown batch, ignore for now (error handling next step)
		return fmt.Errorf("outcome for unknown batch id %d", res.id)
	}
	delete(s.inflight, res.id)
	if res.err == nil {
		log.Debugf(
			"uploader received batch outcome event id=%d of %d items and %d bytes",
			res.id, stats.items, stats.bytes,
		)
		s.metrics.BatchesSent.Add(1)
		s.metrics.BytesSent.Add(int64(stats.bytes))
		s.metrics.ItemsSent.Add(int64(stats.items))
	} else {
		log.Debugf(
			"uploader received failed batch outcome id=%d: %v",
			res.id, res.err,
		)
		s.metrics.Errors.Add(1)
	}
	return nil
}

func (s *batcherState) handleStopEvent() {
	log.Debugf("uploader received stop event")
	// Clear deadlines so the state is consistent; no need to reset the timer
	s.clearDeadlines()
}

// flush clears the current batch and returns its contents.
func (s *batcherState) flush(eff effects) (flushed bool) {
	if len(s.batch) == 0 {
		return false
	}
	id := s.nextBatchID
	s.nextBatchID++
	stat := batchStat{items: len(s.batch), bytes: s.currentSize}
	var batch []json.RawMessage
	batch, s.batch = s.batch, nil
	s.currentSize = 0
	s.inflight[id] = stat
	eff.sendBatch(id, batch)
	return true
}

func (s *batcherState) clearDeadlines() {
	s.nextIdleFlush = time.Time{}
	s.nextMaxBufferFlush = time.Time{}
}

// earliest returns the earlier of a and b, ignoring zero (unset) values.
// If both are zero it returns zero.
func earliest(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b // only b is set (or both are zero)
	case b.IsZero():
		return a // only a is set
	case a.Before(b):
		return a // both set – pick the smaller
	default:
		return b
	}
}
