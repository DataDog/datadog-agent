// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	// peerKeepShort covers a request/response peer, which closes right after reading
	peerKeepShort = 5 * time.Second
	// peerKeepLong covers a peer on this host that notices the close later, such as an idle client
	peerKeepLong = time.Minute
)

// peerTranslations keeps the other direction of conntrack entries deleted at a close, keyed like the conntracker's
// map, so the other end of a same-host NAT'd pair still finds its translation after the entry is gone from the map.
// Entries expire after their keep time, or when the oldest is evicted at the cap.
type peerTranslations struct {
	mu      sync.Mutex
	max     int
	seq     uint64
	entries map[netebpf.ConntrackTuple]peerTranslation
	// short and long hold keys in expiry order, since every entry in one has the same keep time
	short, long []peerKey
}

type peerTranslation struct {
	value   netebpf.ConntrackTuple
	expires time.Time
	seq     uint64
}

type peerKey struct {
	key     netebpf.ConntrackTuple
	expires time.Time
	seq     uint64
}

func newPeerTranslations(maxEntries int) *peerTranslations {
	return &peerTranslations{max: maxEntries, entries: make(map[netebpf.ConntrackTuple]peerTranslation)}
}

// add keeps key -> value for keep, and returns how many entries the cap evicted.
func (p *peerTranslations) add(key, value netebpf.ConntrackTuple, keep time.Duration, now time.Time) (evicted int) {
	if p == nil || p.max <= 0 {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expireLocked(now)
	for len(p.entries) >= p.max {
		// short-lived entries are the least likely to still be needed
		if !p.popLocked(&p.short) && !p.popLocked(&p.long) {
			break
		}
		evicted++
	}
	p.seq++
	expires := now.Add(keep)
	p.entries[key] = peerTranslation{value: value, expires: expires, seq: p.seq}
	k := peerKey{key: key, expires: expires, seq: p.seq}
	if keep > peerKeepShort {
		p.long = append(p.long, k)
	} else {
		p.short = append(p.short, k)
	}
	return evicted
}

// get returns the value kept for key, if it hasn't expired.
func (p *peerTranslations) get(key netebpf.ConntrackTuple, now time.Time) (netebpf.ConntrackTuple, bool) {
	if p == nil {
		return netebpf.ConntrackTuple{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[key]
	if !ok || !now.Before(e.expires) {
		return netebpf.ConntrackTuple{}, false
	}
	return e.value, true
}

func (p *peerTranslations) remove(key netebpf.ConntrackTuple) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, key)
}

func (p *peerTranslations) len() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

func (p *peerTranslations) expireLocked(now time.Time) {
	for _, q := range []*[]peerKey{&p.short, &p.long} {
		for len(*q) > 0 && !now.Before((*q)[0].expires) {
			p.popLocked(q)
		}
	}
}

// popLocked drops the oldest key in q, and its entry unless the key was re-added or removed since.
func (p *peerTranslations) popLocked(q *[]peerKey) bool {
	for len(*q) > 0 {
		k := (*q)[0]
		(*q)[0] = peerKey{}
		*q = (*q)[1:]
		if e, ok := p.entries[k.key]; ok && e.seq == k.seq {
			delete(p.entries, k.key)
			return true
		}
	}
	return false
}

// localAddrSet holds the local endpoints' addresses from the last two fetches. Closes are stored concurrently with
// fetches, so each fetch swaps in a new immutable generation.
type localAddrSet struct {
	gen atomic.Pointer[localAddrGen]
}

type localAddrGen struct {
	cur, prev map[util.Address]struct{}
}

func (l *localAddrSet) update(conns []network.ConnectionStats) {
	cur := make(map[util.Address]struct{})
	for i := range conns {
		cur[conns[i].Source] = struct{}{}
	}
	g := &localAddrGen{cur: cur}
	if old := l.gen.Load(); old != nil {
		g.prev = old.cur
	}
	l.gen.Store(g)
}

func (l *localAddrSet) contains(a util.Address) bool {
	g := l.gen.Load()
	if g == nil {
		return false
	}
	if _, ok := g.cur[a]; ok {
		return true
	}
	_, ok := g.prev[a]
	return ok
}
