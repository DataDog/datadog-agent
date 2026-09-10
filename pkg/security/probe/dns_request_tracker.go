// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
)

const (
	// dnsRequestTrackerSize bounds how many in-flight DNS questions are remembered at once. Only
	// questions asked by a process an activity dump is tracing are recorded, so this is sized for
	// concurrent lookups from traced workloads rather than for host-wide DNS volume.
	dnsRequestTrackerSize = 1024
	// dnsRequestTrackerTTL is how long a question stays correlatable. It matches the timeout a
	// typical stub resolver gives up after, so an answer arriving later than this belongs to a
	// query the requester has already abandoned.
	dnsRequestTrackerTTL = 5 * time.Second
)

// dnsRequestKey identifies one in-flight DNS question. A DNS response carries no network context
// on the short path, so the transaction ID, the question name, and the query type are all we have
// to correlate on. The name is lowercased because DNS names are case-insensitive and resolvers may
// apply 0x20 randomisation, echoing the question with randomised capitalisation.
type dnsRequestKey struct {
	id    uint16
	qtype uint16
	name  string
}

func newDNSRequestKey(id uint16, name string, qtype uint16) dnsRequestKey {
	return dnsRequestKey{id: id, qtype: qtype, name: strings.ToLower(name)}
}

// dnsPendingRequest is the process that asked a question, held until the answer arrives or the
// question goes stale. A nil entry is a tombstone: two different processes were in flight on the
// same key, so the answer can no longer be attributed to either of them.
type dnsPendingRequest struct {
	entry    *model.ProcessCacheEntry
	deadline time.Time
}

// dnsRequestTracker remembers which process asked each in-flight DNS question so that the matching
// response can be attributed back to it.
//
// A DNS request leaves from within the querying process, so the egress hook resolves a pid. A
// response arrives inbound on the softirq path with no current process, and for container traffic
// the packet's pid resolves to 0. Rather than attributing the response independently, this
// correlates it back to its request.
//
// Process cache entries are garbage collected through runtime.AddCleanup rather than refcounted,
// so holding one here is safe; the bounded LRU caps how long entries stay pinned.
type dnsRequestTracker struct {
	mu      sync.Mutex
	pending *simplelru.LRU[dnsRequestKey, dnsPendingRequest]
	ttl     time.Duration

	hits       *atomic.Uint64
	misses     *atomic.Uint64
	collisions *atomic.Uint64
}

func newDNSRequestTracker(size int, ttl time.Duration) (*dnsRequestTracker, error) {
	pending, err := simplelru.NewLRU[dnsRequestKey, dnsPendingRequest](size, nil)
	if err != nil {
		return nil, err
	}

	return &dnsRequestTracker{
		pending:    pending,
		ttl:        ttl,
		hits:       atomic.NewUint64(0),
		misses:     atomic.NewUint64(0),
		collisions: atomic.NewUint64(0),
	}, nil
}

// recordRequest remembers that entry asked this question.
//
// When a live entry already exists for the key under a different process, the key is poisoned with
// a tombstone for the remainder of the original entry's life. Deleting instead would let a third
// request re-occupy the key and claim the first request's answer, which is exactly the
// mis-attribution this exists to avoid. The tombstone keeps the original deadline so that a run of
// colliding requests cannot keep a key poisoned indefinitely.
func (t *dnsRequestTracker) recordRequest(id uint16, name string, qtype uint16, entry *model.ProcessCacheEntry, now time.Time) {
	if entry == nil {
		return
	}

	key := newDNSRequestKey(id, name, qtype)

	t.mu.Lock()
	defer t.mu.Unlock()

	if existing, ok := t.pending.Peek(key); ok && now.Before(existing.deadline) {
		if existing.entry != entry {
			t.collisions.Inc()
			t.pending.Add(key, dnsPendingRequest{entry: nil, deadline: existing.deadline})
			return
		}
	}

	t.pending.Add(key, dnsPendingRequest{entry: entry, deadline: now.Add(t.ttl)})
}

// matchResponse returns the process that asked the question this response answers, or nil when the
// question is unknown, stale, or ambiguous. Ambiguity resolves as a miss and never as a guess: a
// dump losing one resolved IP is a far better outcome than a dump crediting it to the wrong
// process.
//
// The entry is deliberately not consumed. A duplicated or retransmitted response then merges
// idempotently rather than being dropped.
func (t *dnsRequestTracker) matchResponse(id uint16, name string, qtype uint16, now time.Time) *model.ProcessCacheEntry {
	key := newDNSRequestKey(id, name, qtype)

	t.mu.Lock()
	defer t.mu.Unlock()

	pending, ok := t.pending.Get(key)
	if !ok {
		t.misses.Inc()
		return nil
	}

	if !now.Before(pending.deadline) {
		t.pending.Remove(key)
		t.misses.Inc()
		return nil
	}

	if pending.entry == nil {
		// poisoned by a colliding request
		t.misses.Inc()
		return nil
	}

	t.hits.Inc()
	return pending.entry
}
