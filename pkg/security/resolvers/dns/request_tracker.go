// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package dns

import (
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
)

const (
	// requestTrackerSize is the maximum number of in-flight DNS requests tracked at once
	requestTrackerSize = 1024
	// requestTrackerTTL is how long a request can be matched, the usual resolver timeout
	requestTrackerTTL = 5 * time.Second
)

// requestKey identifies an in-flight DNS request. The name is lowercased as resolvers may randomise its case.
type requestKey struct {
	id    uint16
	qtype uint16
	name  string
}

func newRequestKey(id uint16, name string, qtype uint16) requestKey {
	return requestKey{id: id, qtype: qtype, name: strings.ToLower(name)}
}

// pendingRequest is the process that sent a DNS request, and the name as it spelled it. A nil entry
// means two processes sent the same request.
type pendingRequest struct {
	entry    *model.ProcessCacheEntry
	name     string
	deadline time.Time
}

// RequestTracker tracks in-flight DNS requests so that responses, which have no process context,
// can be attributed to the process that sent the request.
type RequestTracker struct {
	mu      sync.Mutex
	pending *simplelru.LRU[requestKey, pendingRequest]
	ttl     time.Duration

	hits       atomic.Uint64
	misses     atomic.Uint64
	collisions atomic.Uint64
}

// NewRequestTracker returns a new DNS request tracker
func NewRequestTracker() (*RequestTracker, error) {
	return newRequestTracker(requestTrackerSize, requestTrackerTTL)
}

func newRequestTracker(size int, ttl time.Duration) (*RequestTracker, error) {
	pending, err := simplelru.NewLRU[requestKey, pendingRequest](size, nil)
	if err != nil {
		return nil, err
	}

	return &RequestTracker{
		pending: pending,
		ttl:     ttl,
	}, nil
}

// RecordRequest records that entry sent this request. A request already sent by another process
// poisons the key until the matching window of the latest of them closes, so that the response is
// attributed to none.
func (t *RequestTracker) RecordRequest(id uint16, name string, qtype uint16, entry *model.ProcessCacheEntry, now time.Time) {
	if entry == nil {
		return
	}

	key := newRequestKey(id, name, qtype)

	t.mu.Lock()
	defer t.mu.Unlock()

	if existing, ok := t.pending.Peek(key); ok && now.Before(existing.deadline) {
		if existing.entry != entry {
			t.collisions.Inc()
			t.pending.Add(key, pendingRequest{entry: nil, deadline: now.Add(t.ttl)})
			return
		}
	}

	t.pending.Add(key, pendingRequest{entry: entry, name: name, deadline: now.Add(t.ttl)})
}

// MatchResponse returns the process that sent the request this response answers, and the question
// name as that request spelled it. The entry is nil when the request is unknown, stale or ambiguous.
// It is kept so that retransmitted responses match too.
func (t *RequestTracker) MatchResponse(id uint16, name string, qtype uint16, now time.Time) (*model.ProcessCacheEntry, string) {
	key := newRequestKey(id, name, qtype)

	t.mu.Lock()
	defer t.mu.Unlock()

	// don't count misses when nothing is tracked, it would only measure the host DNS volume
	if t.pending.Len() == 0 {
		return nil, ""
	}

	pending, ok := t.pending.Get(key)
	if !ok {
		t.misses.Inc()
		return nil, ""
	}

	if !now.Before(pending.deadline) {
		t.pending.Remove(key)
		t.misses.Inc()
		return nil, ""
	}

	if pending.entry == nil {
		// poisoned by a colliding request
		t.misses.Inc()
		return nil, ""
	}

	t.hits.Inc()
	return pending.entry, pending.name
}

// SendStats sends the correlation counters and resets them
func (t *RequestTracker) SendStats(client statsd.ClientInterface) error {
	for _, m := range []struct {
		name    string
		counter *atomic.Uint64
	}{
		{metrics.MetricDNSADCorrelationHits, &t.hits},
		{metrics.MetricDNSADCorrelationMisses, &t.misses},
		{metrics.MetricDNSADCorrelationCollisions, &t.collisions},
	} {
		if err := client.Count(m.name, int64(m.counter.Swap(0)), []string{}, 1.0); err != nil {
			return err
		}
	}
	return nil
}
