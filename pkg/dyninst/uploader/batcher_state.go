// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: Implement limits on the number of batches allowed to be in flight.
// also we'll want to have some rate limits that eventually lead to dropping
// data if we queue too much in order to bound memory usage.

// TODO: Implement some sort of retry logic for failed batches.

type batcherState struct {
	name        string
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

func newBatcherState(name string, cfg batcherConfig, metrics *Metrics) *batcherState {
	return &batcherState{
		name:     name,
		cfg:      cfg,
		metrics:  metrics,
		inFlight: make(map[batchID]batchStat),
	}
}

var singleMessageExceedsLimitLogLimiter = rate.NewLimiter(rate.Every(time.Minute), 10)

func (s *batcherState) handleEnqueueEvent(data json.RawMessage, now time.Time, eff effects) {
	// Check if we should flush before adding the data to the buffer.
	if len(s.buffer) > 0 &&
		s.bufferBytes+len(data) >= s.cfg.maxBatchSizeBytes {
		s.flush(eff)
	}

	// Add the data to the buffer.
	s.buffer = append(s.buffer, data)
	s.bufferBytes += len(data)

	// If the one item exceeds the limit, log about it.
	if len(s.buffer) == 1 && s.bufferBytes > s.cfg.maxBatchSizeBytes &&
		singleMessageExceedsLimitLogLimiter.Allow() {
		log.Warnf(
			"%s uploader: flushing single message that exceeds the byte limit: %d",
			s.name, s.bufferBytes,
		)
	}

	// Check if we should flush immediately now that we've added the data to the
	// buffer.
	if s.bufferBytes >= s.cfg.maxBatchSizeBytes ||
		len(s.buffer) >= s.cfg.maxBatchItems {
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
		return errors.New("timer fired event received but timer is not set")
	}
	s.flush(eff)
	return nil
}

// handleBatchOutcomeEvent handles the outcome of a batch that has been sent.
// It updates the metrics and removes the batch from the inflight map.
//
// If the batch was not expected, it return an error. The state is not
// modified in this case -- but it does imply an invariant violation.
func (s *batcherState) handleBatchOutcomeEvent(res sendResult, _ effects) (batchStat, error) {
	stats, ok := s.inFlight[res.id]
	if !ok {
		return batchStat{}, fmt.Errorf("outcome for unknown batch id %d", res.id)
	}
	delete(s.inFlight, res.id)
	if res.err == nil {
		s.metrics.BatchesSent.Add(1)
		s.metrics.BytesSent.Add(int64(stats.bytes))
		s.metrics.ItemsSent.Add(int64(stats.items))
	} else {
		s.metrics.Errors.Add(1)
	}
	return stats, nil
}

func (s *batcherState) handleStopEvent(eff effects) {
	if len(s.buffer) > 0 {
		s.flush(eff)
		return
	}
	s.clearDeadlines(eff)
}

func (s *batcherState) drainComplete() bool {
	return len(s.inFlight) == 0
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
