// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProber counts probes and reports the image it was asked about, so the
// tests can assert that the cache spends probes only when it must.
type fakeProber struct {
	mu     sync.Mutex
	calls  int
	images []string
	now    *time.Time
}

func (p *fakeProber) CheckReachability(_ context.Context, image string) *Reachability {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.images = append(p.images, image)
	return &Reachability{
		Registries: []RegistryStatus{{Registry: "registry", Reachable: true}},
		CheckedAt:  *p.now,
	}
}

func (p *fakeProber) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// newTestCache returns a cache with a controllable clock. The prober stamps its
// results with the same clock so TTL expiry is driven entirely by the test.
func newTestCache(t *testing.T, ttl time.Duration, image string) (*ReachabilityCache, *fakeProber, *time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	p := &fakeProber{now: &now}
	c := NewReachabilityCache(p, ttl, image)
	c.now = func() time.Time { return now }
	return c, p, &now
}

func TestReachabilityCacheProbesOnceWithinTTL(t *testing.T) {
	// The daemon refreshes state every 30s. Without the TTL every host in the
	// fleet would hit the registry twice a minute.
	c, p, now := newTestCache(t, time.Hour, "")
	ctx := context.Background()

	first := c.Get(ctx)
	require.NotNil(t, first)
	assert.Equal(t, 1, p.count())

	*now = now.Add(59 * time.Minute)
	assert.Same(t, first, c.Get(ctx))
	assert.Equal(t, 1, p.count())

	*now = now.Add(2 * time.Minute) // now past the TTL
	second := c.Get(ctx)
	assert.Equal(t, 2, p.count())
	assert.NotSame(t, first, second)
}

func TestReachabilityCacheDefaultsImage(t *testing.T) {
	c, p, _ := newTestCache(t, time.Hour, "")
	c.Get(context.Background())
	assert.Equal(t, []string{DefaultProbeImage}, p.images)

	c2, p2, _ := newTestCache(t, time.Hour, "installer-package")
	c2.Get(context.Background())
	assert.Equal(t, []string{"installer-package"}, p2.images)
}

func TestReachabilityCacheZeroTTLDisablesCaching(t *testing.T) {
	c, p, _ := newTestCache(t, 0, "")
	c.Get(context.Background())
	c.Get(context.Background())
	assert.Equal(t, 2, p.count())
}

func TestReachabilityCachePeekNeverProbes(t *testing.T) {
	// refreshState calls Peek because it runs on every task transition and must
	// never block on the network.
	c, p, _ := newTestCache(t, time.Hour, "")

	assert.Nil(t, c.Peek())
	assert.Equal(t, 0, p.count())

	got := c.Get(context.Background())
	assert.Same(t, got, c.Peek())
	assert.Equal(t, 1, p.count())
}

func TestReachabilityCacheSeed(t *testing.T) {
	c, p, now := newTestCache(t, time.Hour, "")

	// Nothing to learn from: must not become the held result, or Peek would
	// report an empty reachability as if it had been measured.
	c.Seed(nil)
	c.Seed(&Reachability{CheckedAt: *now})
	assert.Nil(t, c.Peek())

	seeded := &Reachability{
		Registries:   []RegistryStatus{{Registry: "registry", Reachable: true}},
		CheckedAt:    *now,
		FromDownload: true,
	}
	c.Seed(seeded)
	assert.Same(t, seeded, c.Peek())

	// A seeded result resets the TTL, so no probe is spent.
	assert.Same(t, seeded, c.Get(context.Background()))
	assert.Equal(t, 0, p.count())
}

func TestReachabilityCacheSeedIgnoresOlderResults(t *testing.T) {
	// A slow or out-of-order caller must not move the signal backwards.
	c, _, now := newTestCache(t, time.Hour, "")
	newer := &Reachability{
		Registries: []RegistryStatus{{Registry: "registry", Reachable: true}},
		CheckedAt:  *now,
	}
	c.Seed(newer)

	older := &Reachability{
		Registries: []RegistryStatus{{Registry: "registry", FailureKind: FailureKindDNS}},
		CheckedAt:  now.Add(-time.Minute),
	}
	c.Seed(older)
	assert.Same(t, newer, c.Peek())
}

func TestReachabilityCacheInvalidate(t *testing.T) {
	// A failed task means the held result can no longer be trusted. The daemon
	// invalidates rather than seeding a failure, because it receives the
	// installer subprocess's error as opaque text.
	c, p, _ := newTestCache(t, time.Hour, "")
	ctx := context.Background()

	c.Get(ctx)
	assert.Equal(t, 1, p.count())

	c.Invalidate()
	assert.Nil(t, c.Peek())

	c.Get(ctx)
	assert.Equal(t, 2, p.count())
}

func TestReachabilityCacheConcurrent(t *testing.T) {
	c, _, _ := newTestCache(t, time.Hour, "")
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				c.Get(ctx)
			case 1:
				c.Peek()
			case 2:
				c.Seed(&Reachability{Registries: []RegistryStatus{{Registry: "r"}}, CheckedAt: c.now()})
			default:
				c.Invalidate()
			}
		}(i)
	}
	wg.Wait()
}
