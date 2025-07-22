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
)

// TODO: Implement limits on the number of batches allowed to be in flight.
// also we'll want to have some rate limits that eventually lead to dropping
// data if we queue too much in order to bound memory usage.

// TODO: Implement some sort of retry logic for failed batches.

type batcherState struct {
	cfg         batcherConfig
	buffer      []json.RawMessage
	bufferBytes int
	timerSet    bool
	metrics     *Metrics
	nextBatchID batchID
	inFlight    map[batchID]batchStat
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
		inFlight: make(map[batchID]batchStat),
	}
}

func (s *batcherState) handleEnqueueEvent(data json.RawMessage, now time.Time, eff effects) {
	s.buffer = append(s.buffer, data)
	s.bufferBytes += len(data)

	shouldFlushDueToSize := len(s.buffer) >= s.cfg.maxBatchItems ||
		s.bufferBytes >= s.cfg.maxBatchSizeBytes

	if shouldFlushDueToSize {
		s.flush(eff)
	} else if !s.timerSet && s.cfg.maxBufferDuration > 0 {
		eff.resetTimer(now.Add(s.cfg.maxBufferDuration))
		s.timerSet = true
	}
}

// handleTimerFiredEvent handles the event when the timer fires.
//
// If the timer was not expected, it returns an error. The state is not
// modified in this case -- but it does imply an invariant violation.
func (s *batcherState) handleTimerFiredEvent(eff effects) error {
	if !s.timerSet {
		return fmt.Errorf("timer fired event received but timer is not set")
	}
	s.flush(eff)
	return nil
}

// handleBatchOutcomeEvent handles the outcome of a batch that has been sent.
// It updates the metrics and removes the batch from the inflight map.
//
// If the batch was not expected, it return an error. The state is not
// modified in this case -- but it does imply an invariant violation.
func (s *batcherState) handleBatchOutcomeEvent(res sendResult, _ effects) error {
	stats, ok := s.inFlight[res.id]
	if !ok {
		return fmt.Errorf("outcome for unknown batch id %d", res.id)
	}
	delete(s.inFlight, res.id)
	if res.err == nil {
		s.metrics.BatchesSent.Add(1)
		s.metrics.BytesSent.Add(int64(stats.bytes))
		s.metrics.ItemsSent.Add(int64(stats.items))
	} else {
		s.metrics.Errors.Add(1)
	}
	return nil
}

func (s *batcherState) handleStopEvent(eff effects) {
	s.clearDeadlines(eff)
}

func (s *batcherState) flush(eff effects) {
	id := s.nextBatchID
	s.nextBatchID++
	stat := batchStat{items: len(s.buffer), bytes: s.bufferBytes}
	var batch []json.RawMessage
	batch, s.buffer = s.buffer, nil
	s.bufferBytes = 0
	s.inFlight[id] = stat
	eff.sendBatch(id, batch)
	s.clearDeadlines(eff)
}

func (s *batcherState) clearDeadlines(eff effects) {
	if s.timerSet {
		eff.clearTimer()
		s.timerSet = false
	}
}
