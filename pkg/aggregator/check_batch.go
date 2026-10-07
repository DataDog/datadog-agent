// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// checkBatch bounds one sender's outstanding work. Only the producer waits:
// neither a drain nor a timestamp wait may suspend the aggregator's input.
// Holding mu through the acknowledgement also bounds concurrent callers of
// the same sender. There is at most one drain request per sender.
type checkBatch struct {
	mu      sync.Mutex
	limit   int
	samples int
	clock   clock.Clock
	stopped <-chan struct{}
}

func (s *checkSender) submitItem(item senderItem, endOfRun bool) {
	b := s.batch
	if b == nil {
		s.itemsOut <- item
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if !endOfRun {
		select {
		case s.itemsOut <- item:
		case <-b.stopped:
			return
		}
		b.samples++
		if b.samples < b.limit {
			return
		}
	}

	request := &batchCommit{id: s.id, endOfRun: endOfRun, result: make(chan time.Time, 1)}
	for {
		select {
		case s.itemsOut <- request:
		case <-b.stopped:
			return
		}
		select {
		case retryAt := <-request.result:
			if retryAt.IsZero() {
				b.samples = 0
				return
			}
			timer := b.clock.Timer(retryAt.Sub(b.clock.Now()))
			select {
			case <-timer.C:
			case <-b.stopped:
				timer.Stop()
				return
			}
		case <-b.stopped:
			return
		}
	}
}

// A nonzero result asks the producer to retry at the next valid timestamp.
// A zero result acknowledges the drain, not merely receipt of this request.
// The buffered reply lets the aggregator finish even if the producer stops.
type batchCommit struct {
	id       checkid.ID
	endOfRun bool
	result   chan time.Time
}

func (r *batchCommit) handle(agg *BufferedAggregator) {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	cs, ok := agg.checkSamplers[r.id]
	if !ok {
		r.result <- time.Time{}
		return
	}
	now := agg.batchClock.Now()
	if now.Before(cs.nextBatchCommit) {
		r.result <- cs.nextBatchCommit
		return
	}
	if r.endOfRun {
		cs.commit(float64(now.Unix()), &agg.flushFilterList)
	} else {
		cs.commitBatch(float64(now.Unix()), &agg.flushFilterList)
		checkBatchCommits.Add(1)
	}
	cs.nextBatchCommit = time.Unix(now.Unix()+1, 0)
	cs.batchDrained = r.result
	select {
	case agg.batchFlushRequested <- struct{}{}:
	default:
	}
}

// The immutable policy is shared by sender and sampler creation. Internal
// default senders and manual-flush consumers never wait for automatic drains.
func (agg *BufferedAggregator) batchSize(id checkid.ID) int {
	if id == "" || agg.flushInterval <= 0 {
		return 0
	}
	return agg.batchSizes[checkid.IDToCheckName(id)]
}
