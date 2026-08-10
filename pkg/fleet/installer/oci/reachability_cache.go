// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"context"
	"sync"
	"time"
)

// Prober probes registry reachability. *Downloader implements it.
type Prober interface {
	CheckReachability(ctx context.Context, image string) *Reachability
}

// ReachabilityCache serves a registry-reachability result, re-probing at most
// once per TTL.
//
// The TTL is the whole point. The daemon refreshes its state every 30s by
// default, and probing on every refresh would mean every host with remote
// updates enabled hitting the package registry twice a minute. Reachability
// changes on the timescale of network and firewall changes, so a result that is
// up to an hour old is a good trade for two orders of magnitude less traffic.
type ReachabilityCache struct {
	mu     sync.Mutex
	prober Prober
	ttl    time.Duration
	image  string
	result *Reachability

	// now is overridden in tests.
	now func() time.Time
}

// NewReachabilityCache returns a cache that probes through prober, re-probing
// when the held result is older than ttl. A ttl of zero or less disables
// caching and probes on every call, which is intended for tests and one-shot
// commands rather than the daemon.
func NewReachabilityCache(prober Prober, ttl time.Duration, image string) *ReachabilityCache {
	if image == "" {
		image = DefaultProbeImage
	}
	return &ReachabilityCache{
		prober: prober,
		ttl:    ttl,
		image:  image,
		now:    time.Now,
	}
}

// Get returns the current reachability, probing if the held result is missing or
// stale. It blocks for the duration of a probe, so callers on a latency-
// sensitive path should hold their own copy.
func (c *ReachabilityCache) Get(ctx context.Context) *Reachability {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.result != nil && c.ttl > 0 && c.now().Sub(c.result.CheckedAt) < c.ttl {
		return c.result
	}
	c.result = c.prober.CheckReachability(ctx, c.image)
	return c.result
}

// Peek returns the held result without ever probing, or nil if there is none.
// Use it where blocking on the network is not acceptable.
func (c *ReachabilityCache) Peek() *Reachability {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.result
}

// Seed records a reachability result observed elsewhere — for instance a
// package download that succeeded, which proves the registry it came from was
// reachable without spending a probe on it. A seeded result resets the TTL.
//
// Seed ignores nil and results that are older than what it already holds, so an
// out-of-order or slow caller cannot move the signal backwards.
func (c *ReachabilityCache) Seed(r *Reachability) {
	if r == nil || len(r.Registries) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.result != nil && r.CheckedAt.Before(c.result.CheckedAt) {
		return
	}
	c.result = r
}

// Invalidate drops the held result so the next Get re-probes.
//
// Call it when something suggests the held result can no longer be trusted —
// a failed download, for example. Invalidating is the right response to a
// failure rather than seeding one, because the daemon receives a subprocess
// failure as opaque text and cannot tell a registry problem from any other
// reason the task failed. Re-probing answers the question directly.
func (c *ReachabilityCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result = nil
}
