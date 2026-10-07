// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket/layers"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
)

const (
	// dnsRequestTrackerSize is the maximum number of in-flight DNS requests tracked at once
	dnsRequestTrackerSize = 1024
	// dnsRequestTrackerTTL is how long a request can be matched, the usual resolver timeout
	dnsRequestTrackerTTL = 5 * time.Second
)

// dnsRequestKey identifies an in-flight DNS request. The name is lowercased as resolvers may randomise its case.
type dnsRequestKey struct {
	id    uint16
	qtype uint16
	name  string
}

func newDNSRequestKey(id uint16, name string, qtype uint16) dnsRequestKey {
	return dnsRequestKey{id: id, qtype: qtype, name: strings.ToLower(name)}
}

// dnsPendingRequest is the process that sent a DNS request. A nil entry means two processes sent the same request.
type dnsPendingRequest struct {
	entry    *model.ProcessCacheEntry
	deadline time.Time
}

// dnsRequestTracker tracks in-flight DNS requests so that responses, which have no process context,
// can be attributed to the process that sent the request.
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

// recordRequest records that entry sent this request. A request already sent by another process
// poisons the key until its original deadline, so that the response is attributed to neither.
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

// matchResponse returns the process that sent the request this response answers, or nil when the
// request is unknown, stale or ambiguous. The entry is kept so that retransmitted responses match too.
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

// len returns the number of tracked requests
func (t *dnsRequestTracker) len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pending.Len()
}

// newCorrelatedDNSEvent fills ev with a DNS event carrying the response and the process context of the request
func newCorrelatedDNSEvent(ev *model.Event, entry *model.ProcessCacheEntry, id uint16, question model.DNSQuestion, response *model.DNSResponse, ts time.Time, tsRaw uint64) {
	ev.Type = uint32(model.DNSEventType)
	ev.Source = model.EventSourceRelated
	ev.Timestamp = ts
	ev.TimestampRaw = tsRaw
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext
	ev.DNS = model.DNSEvent{
		ID:       id,
		Question: question,
		Response: response,
	}
}

// correlateDNSResponseForActivityDump attributes a DNS response to the process that sent the request
// and hands it to the profile manager. It skips DispatchEvent so that rules don't match the same
// DNS request twice.
func (p *EBPFProbe) correlateDNSResponseForActivityDump(dnsLayer *layers.DNS, ips []net.IPNet, cnames []string) {
	if p.profileManager == nil || p.dnsRequests == nil {
		return
	}

	// no answer, nothing to add to the profile
	if len(ips) == 0 && len(cnames) == 0 {
		return
	}

	if len(dnsLayer.Questions) == 0 {
		return
	}
	question := dnsLayer.Questions[0]

	// don't count misses when nothing is tracked, it would only measure the host DNS volume
	if p.dnsRequests.len() == 0 {
		return
	}

	questionName := string(question.Name)

	now := time.Now()
	entry := p.dnsRequests.matchResponse(dnsLayer.ID, questionName, uint16(question.Type), now)
	if entry == nil {
		return
	}

	ev := p.getPoolEvent()
	defer p.putBackPoolEvent(ev)

	newCorrelatedDNSEvent(ev, entry, dnsLayer.ID, model.DNSQuestion{
		Name:  questionName,
		Type:  uint16(question.Type),
		Class: uint16(question.Class),
	}, &model.DNSResponse{
		ResponseCode: uint8(dnsLayer.ResponseCode),
		IPs:          ips,
		CNames:       cnames,
	}, now, uint64(p.Resolvers.TimeResolver.ComputeMonotonicTimestamp(now)))

	// the profile manager copies what it keeps, so the event can go back to the pool
	p.profileManager.ProcessEvent(ev)
}
