# Discovery probe retry — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a bounded retry budget for failed discovery probes so a service whose application becomes ready within ~4 minutes of the container being added is discovered and scheduled, while genuinely-mismatched services give up at a fixed cost ceiling.

**Architecture:** Retry budget lives in the discoverer cache. configmgr re-runs the existing `reconcileService` path on a 5 s ticker over services that have at least one pending discovery entry. The cache decides per-call whether to probe (if `now >= nextRetryAt`) or skip; `Discover()` itself stays synchronous and stateless about timing. Service-removal explicitly clears cache entries via a new `Forget(svcID)` method. See `docs/superpowers/specs/2026-05-06-discover-retry-design.md` for the full design.

**Tech Stack:** Go, standard library `time`, existing `comp/core/autodiscovery/discoverer/` and `comp/core/autodiscovery/autodiscoveryimpl/` packages.

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `comp/core/autodiscovery/discoverer/cache.go` | Cache state machine: per-`(svcID, integ)` entry tracks attempts and next-retry-time; `lookup` returns one of `{miss, hit, pending, givenUp}`; `forget(svcID)` evicts all entries for one service. | Replace internals |
| `comp/core/autodiscovery/discoverer/cache_test.go` | Cache unit tests. | Modify (replace TTL test, add schedule/forget tests) |
| `comp/core/autodiscovery/discoverer/types.go` | Discoverer interface; gains `IsPending(svcID, integ) bool` and `Forget(svcID)`. | Modify |
| `comp/core/autodiscovery/discoverer/discoverer.go` | `Discover()` consults cache state + decides probe vs. skip; new `IsPending` and `Forget` implementations. | Modify |
| `comp/core/autodiscovery/discoverer/discoverer_test.go` | Discoverer unit tests. | Modify (add pending/given-up/forget tests) |
| `comp/core/autodiscovery/discoverer/python_bridge_nopython.go` | No-Python build path. | Verify still compiles (no edit expected) |
| `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go` | New `pendingDiscovery map[svcID]struct{}`; `updatePendingDiscovery(svcID)` helper called from `reconcileService`; `processDelService` calls `discoverer.Forget`; new `retryPendingDiscoveries() ConfigChanges`. | Modify |
| `comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go` | Config-manager unit tests. | Modify (pending set + retry tests) |
| `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go` | Start/stop a `discoveryRetryLoop` goroutine ticking every 5 s. | Modify |

---

## Task 1: Cache state machine + retry schedule

Replace the TTL-based failure cache with a schedule-based state machine. Existing `Discover()` callers still see "failure cached, don't probe again" behaviour; the difference is the cache now drives the retry timing instead of a flat TTL.

**Files:**
- Modify: `comp/core/autodiscovery/discoverer/cache.go`
- Modify: `comp/core/autodiscovery/discoverer/cache_test.go`
- Modify: `comp/core/autodiscovery/discoverer/discoverer.go` (caller of `putFailure`/`get`)

- [ ] **Step 1: Write failing tests for the new cache API**

Replace the body of `comp/core/autodiscovery/discoverer/cache_test.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestCacheLookupMiss(t *testing.T) {
	c := newCache(time.Now)
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, stateMiss, got.state)
}

func TestCacheLookupHit(t *testing.T) {
	c := newCache(time.Now)
	r := Result{Configs: []integration.Config{{Name: "krakend"}}}
	c.putSuccess("svc-1", "krakend", r)
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, stateHit, got.state)
	assert.Len(t, got.result.Configs, 1)
}

func TestCachePutFailureSchedulesRetries(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	c := newCache(clock)
	schedule := []time.Duration{5 * time.Second, 10 * time.Second}

	c.putFailure("svc-1", "krakend", schedule)
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, statePending, got.state)
	assert.Equal(t, now.Add(5*time.Second), got.nextRetryAt)

	c.putFailure("svc-1", "krakend", schedule)
	got = c.lookup("svc-1", "krakend")
	assert.Equal(t, statePending, got.state)
	assert.Equal(t, now.Add(10*time.Second), got.nextRetryAt)

	c.putFailure("svc-1", "krakend", schedule)
	got = c.lookup("svc-1", "krakend")
	assert.Equal(t, stateGivenUp, got.state)
}

func TestCachePutSuccessClearsFailure(t *testing.T) {
	c := newCache(time.Now)
	c.putFailure("svc-1", "krakend", []time.Duration{5 * time.Second})
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	got := c.lookup("svc-1", "krakend")
	assert.Equal(t, stateHit, got.state)
}

func TestCacheKeyIsolation(t *testing.T) {
	c := newCache(time.Now)
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	got := c.lookup("svc-1", "apache")
	assert.Equal(t, stateMiss, got.state, "different integration is a different key")
	got = c.lookup("svc-2", "krakend")
	assert.Equal(t, stateMiss, got.state, "different service is a different key")
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: compile failures (`stateMiss` undefined, `lookup` undefined, etc.).

- [ ] **Step 3: Replace `cache.go` with schedule-based state machine**

Replace `comp/core/autodiscovery/discoverer/cache.go` entirely:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"sync"
	"time"
)

// cacheState is the state of a (svcID, integration) entry in the cache.
type cacheState int

const (
	stateMiss     cacheState = iota // no entry
	stateHit                        // success entry — return cached configs
	statePending                    // failure entry — may probe again at nextRetryAt
	stateGivenUp                    // failure entry — schedule exhausted, no more probes
)

type cacheEntry struct {
	success bool
	result  Result // valid when success

	// failure-only:
	attemptsMade int       // count of failures so far
	nextRetryAt  time.Time // zero when givenUp
	givenUp      bool
}

type cacheLookupResult struct {
	state       cacheState
	result      Result    // valid when state == stateHit
	nextRetryAt time.Time // valid when state == statePending
}

type cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	now     func() time.Time
}

func newCache(now func() time.Time) *cache {
	if now == nil {
		now = time.Now
	}
	return &cache{entries: make(map[string]cacheEntry), now: now}
}

func cacheKey(svcID, integrationName string) string {
	return svcID + "|" + integrationName
}

func (c *cache) lookup(svcID, integrationName string) cacheLookupResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(svcID, integrationName)]
	if !ok {
		return cacheLookupResult{state: stateMiss}
	}
	if e.success {
		return cacheLookupResult{state: stateHit, result: e.result}
	}
	if e.givenUp {
		return cacheLookupResult{state: stateGivenUp}
	}
	return cacheLookupResult{state: statePending, nextRetryAt: e.nextRetryAt}
}

func (c *cache) putSuccess(svcID, integrationName string, r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, integrationName)] = cacheEntry{success: true, result: r}
}

// putFailure records a probe failure and advances the retry schedule.
// `schedule[attemptsMade-1]` is the wait time before the next probe attempt;
// once attemptsMade > len(schedule), the entry is marked givenUp.
func (c *cache) putFailure(svcID, integrationName string, schedule []time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := cacheKey(svcID, integrationName)
	e := c.entries[k]
	e.success = false
	e.result = Result{}
	e.attemptsMade++
	if e.attemptsMade > len(schedule) {
		e.givenUp = true
		e.nextRetryAt = time.Time{}
	} else {
		e.nextRetryAt = c.now().Add(schedule[e.attemptsMade-1])
	}
	c.entries[k] = e
}

// forget drops all entries for a given svcID. Called from configmgr on
// service removal so a stopped-and-restarted container starts fresh.
func (c *cache) forget(svcID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := svcID + "|"
	for k := range c.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.entries, k)
		}
	}
}
```

- [ ] **Step 4: Update `discoverer.go` to use the new cache API**

Replace the `Discover` method body in `comp/core/autodiscovery/discoverer/discoverer.go`. The existing struct fields and helpers stay; only the cache interaction changes. Also replace the `failureTTL` field with `retrySchedule`:

In the file, change:

```go
const defaultFailureTTL = 30 * time.Second

type defaultDiscoverer struct {
	bridge     Bridge
	cache      *cache
	failureTTL time.Duration
}

func newDiscoverer(bridge Bridge) *defaultDiscoverer {
	return &defaultDiscoverer{
		bridge:     bridge,
		cache:      newCache(time.Now),
		failureTTL: defaultFailureTTL,
	}
}
```

to:

```go
// defaultRetrySchedule is the wait time between probe attempts. Length N
// means an entry is givenUp after the (N+1)th failure. Sum is the total
// retry window per (svcID, integration) pair.
var defaultRetrySchedule = []time.Duration{
	5 * time.Second, 5 * time.Second,
	30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
	30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
}

type defaultDiscoverer struct {
	bridge        Bridge
	cache         *cache
	retrySchedule []time.Duration
	now           func() time.Time
}

func newDiscoverer(bridge Bridge) *defaultDiscoverer {
	now := time.Now
	return &defaultDiscoverer{
		bridge:        bridge,
		cache:         newCache(now),
		retrySchedule: defaultRetrySchedule,
		now:           now,
	}
}
```

In `Discover`, replace the cache-get block at the top:

```go
	if r, ok, hit := d.cache.get(svcID, integrationName); hit {
		return r, ok
	}
```

with:

```go
	state := d.cache.lookup(svcID, integrationName)
	switch state.state {
	case stateHit:
		return state.result, true
	case stateGivenUp:
		return Result{}, false
	case statePending:
		if d.now().Before(state.nextRetryAt) {
			return Result{}, false
		}
		// fall through and probe
	}
```

Replace every `d.cache.putFailure(svcID, integrationName, d.failureTTL)` call with:

```go
	d.cache.putFailure(svcID, integrationName, d.retrySchedule)
```

(There are 7 such calls — every failure path in `Discover`. `replace_all` on the literal `d.failureTTL` works.)

- [ ] **Step 5: Run tests, verify cache + discoverer pass**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: PASS for all cache and discoverer tests.

- [ ] **Step 6: Run linter**

```bash
dda inv linter.go --targets=./comp/core/autodiscovery/discoverer
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add comp/core/autodiscovery/discoverer/cache.go \
        comp/core/autodiscovery/discoverer/cache_test.go \
        comp/core/autodiscovery/discoverer/discoverer.go
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
autodiscovery/discoverer: replace failure TTL with retry schedule

Cache entries for failed probes now track an attempts counter and a
nextRetryAt computed from a schedule []time.Duration. Once the schedule
is exhausted, the entry transitions to givenUp and is never probed
again. cache.lookup returns one of {miss, hit, pending, givenUp}; the
discoverer's Discover() inspects this and decides probe-or-skip.

Default schedule is [5s, 5s, 30s × 8] — 11 attempts (1 initial + 10
retries) over ~4 min 10 s. The first two slots target common
~10-30 s container-startup races; the remaining slots match the
existing 30 s TTL value to keep the steady-state probe rate the same.

cache.forget(svcID) is added but not yet wired up; that comes in the
next commit.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Cache forget() unit test

Add a unit test that verifies `cache.forget(svcID)` evicts all entries for one service while leaving others intact. (Implementation already landed in Task 1; this is a follow-up TDD verification.)

**Files:**
- Modify: `comp/core/autodiscovery/discoverer/cache_test.go`

- [ ] **Step 1: Add the test**

Append to `cache_test.go`:

```go
func TestCacheForget(t *testing.T) {
	c := newCache(time.Now)
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	c.putFailure("svc-1", "kuma", []time.Duration{5 * time.Second})
	c.putSuccess("svc-2", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})

	c.forget("svc-1")

	assert.Equal(t, stateMiss, c.lookup("svc-1", "krakend").state)
	assert.Equal(t, stateMiss, c.lookup("svc-1", "kuma").state)
	assert.Equal(t, stateHit, c.lookup("svc-2", "krakend").state, "other service untouched")
}
```

- [ ] **Step 2: Run tests, verify pass**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: PASS including the new test.

- [ ] **Step 3: Commit**

```bash
git add comp/core/autodiscovery/discoverer/cache_test.go
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
autodiscovery/discoverer: test cache.forget(svcID)

Verifies that forget evicts all entries for a service and leaves
entries for other services untouched.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Discoverer interface — IsPending and Forget

Expose `IsPending(svcID, integ) bool` and `Forget(svcID)` on the `Discoverer` interface so configmgr can ask "is this entry still in flight" and "drop entries for a removed service".

**Files:**
- Modify: `comp/core/autodiscovery/discoverer/types.go`
- Modify: `comp/core/autodiscovery/discoverer/discoverer.go`
- Modify: `comp/core/autodiscovery/discoverer/discoverer_test.go`

- [ ] **Step 1: Write failing tests**

Append to `comp/core/autodiscovery/discoverer/discoverer_test.go`:

```go
func TestDiscoverIsPendingAfterFailure(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return "null", nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.True(t, d.IsPending("docker://abc", "krakend"))
	assert.False(t, d.IsPending("docker://abc", "other-integration"))
	assert.False(t, d.IsPending("other-svc", "krakend"))
}

func TestDiscoverIsPendingFalseAfterGiveUp(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return "null", nil
	}}
	d := newDiscoverer(bridge)
	d.retrySchedule = []time.Duration{0} // 1 retry, then give up
	d.Discover(context.Background(), "krakend", newFakeService())
	d.Discover(context.Background(), "krakend", newFakeService())
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, d.IsPending("docker://abc", "krakend"))
}

func TestDiscoverIsPendingFalseAfterSuccess(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return `[{"openmetrics_endpoint":"x"}]`, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, d.IsPending("docker://abc", "krakend"))
}

func TestDiscoverForgetClearsEntries(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return "null", nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	require.True(t, d.IsPending("docker://abc", "krakend"))

	d.Forget("docker://abc")
	assert.False(t, d.IsPending("docker://abc", "krakend"))
}
```

You also need a `time` import at the top of the file:

```go
import (
	"context"
	"errors"
	"testing"
	"time"
	// ... existing
)
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: compile failures (`IsPending`, `Forget` not defined on `*defaultDiscoverer`).

- [ ] **Step 3: Add interface methods + implementations**

In `comp/core/autodiscovery/discoverer/types.go`, replace the `Discoverer` interface block:

```go
type Discoverer interface {
	Discover(ctx context.Context, integrationName string, svc listeners.Service) (Result, bool)
}
```

with:

```go
type Discoverer interface {
	Discover(ctx context.Context, integrationName string, svc listeners.Service) (Result, bool)

	// IsPending reports whether the cache holds a "still retrying" entry for
	// this (svcID, integration) pair (i.e. a failure entry whose retry
	// schedule isn't exhausted).
	IsPending(svcID, integrationName string) bool

	// Forget drops all cache entries for one service. Called by configmgr on
	// service removal so a restarted container starts fresh.
	Forget(svcID string)
}
```

In `comp/core/autodiscovery/discoverer/discoverer.go`, append the new methods to `*defaultDiscoverer`:

```go
// IsPending reports whether the cache has a pending failure entry for this pair.
func (d *defaultDiscoverer) IsPending(svcID, integrationName string) bool {
	return d.cache.lookup(svcID, integrationName).state == statePending
}

// Forget drops all cache entries for a service.
func (d *defaultDiscoverer) Forget(svcID string) {
	d.cache.forget(svcID)
}
```

- [ ] **Step 4: Run tests, verify pass**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: PASS.

- [ ] **Step 5: Verify the no-Python build path still compiles**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer --build-tags=
```

Expected: PASS. (The no-Python path uses `discoverer.New(nil)` → returns nil. Configmgr already nil-checks before calling `Discover`; we'll do the same for `IsPending` and `Forget` in Task 4.)

- [ ] **Step 6: Commit**

```bash
git add comp/core/autodiscovery/discoverer/types.go \
        comp/core/autodiscovery/discoverer/discoverer.go \
        comp/core/autodiscovery/discoverer/discoverer_test.go
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
autodiscovery/discoverer: expose IsPending and Forget

Adds two methods to the Discoverer interface so configmgr can:
- ask whether a (svcID, integration) pair still has retries pending
  (used to populate the retry-loop's pending set), and
- drop cache entries when a service is removed (so a restarted
  container with a new svcID isn't affected, and an in-place restart
  with the same svcID doesn't see stale state).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Configmgr — pendingDiscovery set and processDelService cleanup

Add a `pendingDiscovery` set to `reconcilingConfigManager`, populate / prune it via a helper called from `reconcileService`, and clear entries when a service is removed.

**Files:**
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go`
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go`

- [ ] **Step 1: Add a fake discoverer and tests**

Append to `comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go`:

```go
// fakeDiscoverer implements discoverer.Discoverer for tests.
type fakeDiscoverer struct {
	results map[string]bool // key svcID|integ -> match (true = scheduled)
	pending map[string]bool // key svcID|integ -> pending
	forgot  map[string]bool // svcIDs that received Forget
}

func newFakeDiscoverer() *fakeDiscoverer {
	return &fakeDiscoverer{
		results: map[string]bool{},
		pending: map[string]bool{},
		forgot:  map[string]bool{},
	}
}

func (f *fakeDiscoverer) Discover(_ context.Context, integ string, svc listeners.Service) (discoverer.Result, bool) {
	k := svc.GetServiceID() + "|" + integ
	if f.results[k] {
		return discoverer.Result{Configs: []integration.Config{{Name: integ, Instances: []integration.Data{integration.Data("{}")}}}}, true
	}
	return discoverer.Result{}, false
}

func (f *fakeDiscoverer) IsPending(svcID, integ string) bool {
	return f.pending[svcID+"|"+integ]
}

func (f *fakeDiscoverer) Forget(svcID string) {
	f.forgot[svcID] = true
}

func TestPendingDiscoveryPopulatedOnUnmatched(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.Mock(t)
	disco := newFakeDiscoverer()
	disco.pending["docker://abc|krakend"] = true

	cm := newReconcilingConfigManager(&mockResolver, hp, disco).(*reconcilingConfigManager)

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.Discovery{},
		Provider:      "file",
	}
	svc := &dummyService{
		ID:            "docker://abc",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	cm.processNewConfig(tpl)
	cm.processNewService(svc)

	_, isPending := cm.pendingDiscovery["docker://abc"]
	assert.True(t, isPending, "svcID should be tracked as pending discovery")
}

func TestPendingDiscoveryPrunedOnGiveUp(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.Mock(t)
	disco := newFakeDiscoverer()
	// Discoverer reports the entry as NOT pending (i.e. given up).
	cm := newReconcilingConfigManager(&mockResolver, hp, disco).(*reconcilingConfigManager)

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.Discovery{},
		Provider:      "file",
	}
	svc := &dummyService{
		ID:            "docker://abc",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	cm.processNewConfig(tpl)
	cm.processNewService(svc)

	_, isPending := cm.pendingDiscovery["docker://abc"]
	assert.False(t, isPending, "given-up svcID must not be tracked")
}

func TestProcessDelServiceCallsForget(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.Mock(t)
	disco := newFakeDiscoverer()
	disco.pending["docker://abc|krakend"] = true

	cm := newReconcilingConfigManager(&mockResolver, hp, disco).(*reconcilingConfigManager)

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.Discovery{},
		Provider:      "file",
	}
	svc := &dummyService{
		ID:            "docker://abc",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	cm.processNewConfig(tpl)
	cm.processNewService(svc)

	cm.processDelService(svc)

	assert.True(t, disco.forgot["docker://abc"], "Forget should be called on service deletion")
	_, isPending := cm.pendingDiscovery["docker://abc"]
	assert.False(t, isPending, "pendingDiscovery entry should be cleared")
}
```

You may need a `context` import and a `discoverer` import; check existing imports in the file before adding.

- [ ] **Step 2: Run tests, verify they fail**

```bash
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl -- -run TestPendingDiscovery
```

Expected: compile failure (`pendingDiscovery` not defined on `reconcilingConfigManager`).

- [ ] **Step 3: Add the field and helper**

In `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go`, in the `reconcilingConfigManager` struct, add:

```go
	// pendingDiscovery contains svcIDs with at least one discovery template
	// that is not-yet-given-up. The retry loop walks this set on each tick.
	pendingDiscovery map[string]struct{}
```

Initialize in `newReconcilingConfigManager`:

```go
	return &reconcilingConfigManager{
		// ... existing fields ...
		pendingDiscovery: map[string]struct{}{},
		// ... existing fields ...
	}
```

Add the helper near `reconcileService`:

```go
// updatePendingDiscovery rebuilds the pendingDiscovery membership for a service:
// the service is in the set iff at least one matching discovery template has a
// pending (not-yet-given-up) cache entry.
func (cm *reconcilingConfigManager) updatePendingDiscovery(svcID string) {
	if cm.discoverer == nil {
		return
	}

	svcAndADIDs, found := cm.activeServices[svcID]
	if !found {
		delete(cm.pendingDiscovery, svcID)
		return
	}

	seen := map[string]bool{}
	for _, adID := range svcAndADIDs.adIDs {
		for _, tplDigest := range cm.templatesByADID.get(adID) {
			tpl, ok := cm.activeConfigs[tplDigest]
			if !ok || tpl.Discovery == nil {
				continue
			}
			if seen[tpl.Name] {
				continue
			}
			seen[tpl.Name] = true
			if cm.discoverer.IsPending(svcID, tpl.Name) {
				cm.pendingDiscovery[svcID] = struct{}{}
				return
			}
		}
	}
	delete(cm.pendingDiscovery, svcID)
}
```

Call `updatePendingDiscovery` at the end of `reconcileService`, just before `return changes`:

```go
	if len(existingResolutions) == 0 {
		delete(cm.serviceResolutions, svcID)
	} else {
		cm.serviceResolutions[svcID] = existingResolutions
	}

	cm.updatePendingDiscovery(svcID) // <-- add this line

	return changes
}
```

In `processDelService`, after the `delete(cm.activeServices, svcID)` line, add the cache eviction:

```go
	delete(cm.activeServices, svcID)

	if cm.discoverer != nil {
		cm.discoverer.Forget(svcID)
	}
	delete(cm.pendingDiscovery, svcID)
```

- [ ] **Step 4: Run tests, verify pass**

```bash
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl -- -run TestPendingDiscovery
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl -- -run TestProcessDelService
```

Expected: PASS for new tests; existing configmgr tests should still pass.

```bash
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl
```

Expected: PASS overall.

- [ ] **Step 5: Commit**

```bash
git add comp/core/autodiscovery/autodiscoveryimpl/configmgr.go \
        comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
autodiscovery/configmgr: track pendingDiscovery + clear on service del

reconcilingConfigManager now maintains a pendingDiscovery set —
svcIDs with at least one discovery template whose cache entry is
still pending (not given up). Membership is recomputed at the end
of every reconcileService via updatePendingDiscovery.

processDelService calls discoverer.Forget(svcID) and removes the
entry from pendingDiscovery, so a stopped service doesn't leak
state and a same-svcID restart starts fresh.

The retry loop that consumes this set comes in the next commit.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Configmgr — retryPendingDiscoveries

Add the method that the retry goroutine will call. Returns merged `ConfigChanges` for the caller to apply via the scheduler.

**Files:**
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go`
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go`

- [ ] **Step 1: Write failing test**

Append to `comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go`:

```go
func TestRetryPendingDiscoveriesScheduledOnLateMatch(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.Mock(t)
	disco := newFakeDiscoverer()
	disco.pending["docker://abc|krakend"] = true // initial: pending, no match

	cm := newReconcilingConfigManager(&mockResolver, hp, disco).(*reconcilingConfigManager)

	tpl := integration.Config{
		Name:          "krakend",
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.Discovery{},
		Provider:      "file",
	}
	svc := &dummyService{
		ID:            "docker://abc",
		ADIdentifiers: []string{"krakend"},
		Hosts:         map[string]string{"main": "10.0.0.1"},
	}
	cm.processNewConfig(tpl)
	cm.processNewService(svc)
	require.Contains(t, cm.pendingDiscovery, "docker://abc")

	// App becomes ready: discoverer now reports a match.
	disco.results["docker://abc|krakend"] = true
	disco.pending["docker://abc|krakend"] = false

	changes := cm.retryPendingDiscoveries()

	assert.Len(t, changes.Schedule, 1, "the late-arriving discovery should produce a scheduled config")
	assert.NotContains(t, cm.pendingDiscovery, "docker://abc",
		"successful late discovery removes svcID from pending set")
}

func TestRetryPendingDiscoveriesNoOpWhenEmpty(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.Mock(t)
	disco := newFakeDiscoverer()

	cm := newReconcilingConfigManager(&mockResolver, hp, disco).(*reconcilingConfigManager)

	changes := cm.retryPendingDiscoveries()
	assert.Empty(t, changes.Schedule)
	assert.Empty(t, changes.Unschedule)
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl -- -run TestRetryPendingDiscoveries
```

Expected: compile failure (`retryPendingDiscoveries` not defined).

- [ ] **Step 3: Add the method to the interface and implementation**

In `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go`, add to the `configManager` interface:

```go
	// retryPendingDiscoveries re-runs reconcileService for each service in
	// pendingDiscovery. Returns the aggregated ConfigChanges so the caller
	// can apply them via the scheduler outside of cm's lock.
	retryPendingDiscoveries() integration.ConfigChanges
```

Add the implementation on `*reconcilingConfigManager`:

```go
// retryPendingDiscoveries implements configManager#retryPendingDiscoveries.
func (cm *reconcilingConfigManager) retryPendingDiscoveries() integration.ConfigChanges {
	cm.m.Lock()
	defer cm.m.Unlock()

	// Snapshot to avoid mutating the map mid-iteration: reconcileService
	// updates pendingDiscovery via updatePendingDiscovery.
	pending := make([]string, 0, len(cm.pendingDiscovery))
	for svcID := range cm.pendingDiscovery {
		pending = append(pending, svcID)
	}

	var changes integration.ConfigChanges
	for _, svcID := range pending {
		changes.Merge(cm.reconcileService(svcID))
	}
	return changes
}
```

- [ ] **Step 4: Run tests, verify pass**

```bash
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl -- -run TestRetryPendingDiscoveries
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl
```

Expected: PASS for new and existing tests.

- [ ] **Step 5: Run linter on touched packages**

```bash
dda inv linter.go --targets=./comp/core/autodiscovery/autodiscoveryimpl
```

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add comp/core/autodiscovery/autodiscoveryimpl/configmgr.go \
        comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
autodiscovery/configmgr: retryPendingDiscoveries

Walks pendingDiscovery under cm.m, reruns reconcileService for each
svcID, returns merged ConfigChanges for the caller to apply via the
scheduler outside the lock. Snapshot-then-iterate to handle
reconcileService mutating pendingDiscovery via updatePendingDiscovery.

The goroutine that calls this on a 5 s tick comes in the next commit.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: AutoConfig — discovery retry goroutine

Start a 5 s ticker in `AutoConfig.start()` that calls `retryPendingDiscoveries` and applies the changes. Stop it cleanly in `AutoConfig.stop()`.

**Files:**
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go`

- [ ] **Step 1: Add the discoveryRetryStop channel field**

In `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go`, find the `AutoConfig` struct (around line 80) and add a field:

```go
	listenerStop             chan struct{}
	discoveryRetryStop       chan struct{}   // <-- add
```

In `createNewAutoConfig` (around line 200), initialize it alongside `listenerStop`:

```go
		listenerStop:             make(chan struct{}),
		discoveryRetryStop:       make(chan struct{}),
```

- [ ] **Step 2: Add the retry loop method**

Append to `autoconfig.go` (or place near `serviceListening`):

```go
// discoveryRetryInterval matches the fastest retry slot in the discoverer's
// default schedule (5 s). Coarser ticks would miss the 5 s slots by up to
// one tick interval.
const discoveryRetryInterval = 5 * time.Second

// discoveryRetryLoop periodically re-runs reconcileService for services
// whose discovery probes haven't matched yet but haven't given up. The
// discoverer's cache decides per-call whether to actually probe (cache
// returns "not yet due" for entries inside their nextRetryAt window).
func (ac *AutoConfig) discoveryRetryLoop() {
	ticker := time.NewTicker(discoveryRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ac.discoveryRetryStop:
			return
		case <-ticker.C:
			changes := ac.cfgMgr.retryPendingDiscoveries()
			ac.applyChanges(changes)
		}
	}
}
```

- [ ] **Step 3: Start the goroutine in `start()`**

In `AutoConfig.start()` (around line 360), add the goroutine launch:

```go
func (ac *AutoConfig) start() {
	listeners.RegisterListeners(ac.serviceListenerFactories)
	providers.RegisterProviders(ac.providerCatalog)
	setupAcErrors()
	// Start the service listener
	go ac.serviceListening()
	// Start the discovery retry loop
	go ac.discoveryRetryLoop()
}
```

- [ ] **Step 4: Stop the goroutine in `stop()`**

In `AutoConfig.stop()` (around line 371), add the stop signal:

```go
func (ac *AutoConfig) stop() {
	// stop polled config providers without holding ac.m
	for _, pd := range ac.getConfigPollers() {
		pd.stop()
	}

	// stop the service listener
	ac.listenerStop <- struct{}{}

	// stop the discovery retry loop
	ac.discoveryRetryStop <- struct{}{}
	// ... rest of existing function
```

Place the new send right after the existing `ac.listenerStop <- struct{}{}`.

- [ ] **Step 5: Build the agent to verify it compiles**

```bash
dda inv agent.build --build-exclude=systemd
```

Expected: build succeeds.

- [ ] **Step 6: Run unit tests for the package**

```bash
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl
```

Expected: PASS. (No new tests in this task — the goroutine is hard to unit-test cleanly without a clock fixture in AutoConfig; the e2e in Task 7 covers it.)

- [ ] **Step 7: Run linter**

```bash
dda inv linter.go --targets=./comp/core/autodiscovery/autodiscoveryimpl
```

Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
autodiscovery: tick the discoverer retry loop from AutoConfig

5 s ticker in AutoConfig.start() calls cfgMgr.retryPendingDiscoveries
and applies the resulting ConfigChanges via the existing scheduler
path. The discoverer's cache decides per-call whether to actually
probe; ticks while no entry is due short-circuit cheaply.

5 s matches the fastest slot in the default retry schedule.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: E2E validation — delayed-krakend repro

Re-run the delayed-krakend reproducer from earlier in the session and confirm the late discovery now schedules the check.

**Files:**
- (no source changes; doc-only at the end)

- [ ] **Step 1: Rebuild agent + restore bazel rtloader + rebuild discovery-dev image**

```bash
dda inv agent.build --build-exclude=systemd
cp -P dev/embedded/lib/libdatadog-agent-rtloader* dev/lib/
cp -P dev/embedded/lib/libdatadog-agent-three.so dev/lib/
dda inv discovery-dev.build-image
```

Verify the image was produced:

```bash
docker image inspect datadog/agent-dev:discovery-local --format '{{.Created}}'
```

Expected: a timestamp from the last few minutes.

- [ ] **Step 2: Re-run the delayed-krakend reproducer**

```bash
chmod +x /tmp/krakend-delayed/run_repro.sh
/tmp/krakend-delayed/run_repro.sh 2>&1 | tee /tmp/krakend-delayed/run-after-fix.log
```

The script starts the agent, then a krakend container with `entrypoint: sh -c "sleep 60 && krakend run …"`, watches for ~2 min.

- [ ] **Step 3: Verify discover() ran multiple times and the check eventually scheduled**

```bash
docker logs dd-agent-repro 2>&1 | grep -cE "python discover: krakend returned"
```

Expected: a count > 1 (one initial probe + multiple retries).

```bash
docker logs dd-agent-repro 2>&1 | grep -E "Initializing rtloader|python discover: krakend returned|discover did not match for template krakend|krakend.*\[OK\]" | tail -20
```

Expected: a sequence of `discover did not match` lines, ending in a successful match (`Discover` returns a config, `discover did not match` stops, the krakend check `[OK]` appears in `agent status`).

```bash
docker exec dd-agent-repro agent configcheck 2>&1 | grep -A 3 "krakend "
```

Expected: a `=== krakend check ===` block with `Configuration source: file:/etc/datadog-agent/conf.d/krakend.d/auto_conf_discovery.yaml` and an `openmetrics_endpoint` field.

```bash
docker exec dd-agent-repro agent status 2>&1 | sed -n '/krakend (/,/^$/p' | head -10
```

Expected: `krakend (...)` block with `Instance ID: krakend:<digest> [OK]` and a non-zero `Metric Samples`.

- [ ] **Step 4: Cleanup**

```bash
docker rm -f dd-agent-repro
docker compose -f /tmp/krakend-delayed/docker-compose.yml -p krakend-delayed down --volumes
```

- [ ] **Step 5: Update the smoke doc with the retry validation note**

In `docs/superpowers/2026-05-06-discover-e2e-smoke.md`, append a section:

```markdown
## Late-arriving service: delayed-startup retry

The discovery probe retry validation uses a krakend container whose
entrypoint sleeps before exec'ing the actual binary, so the AD event
fires while the HTTP endpoint is still unreachable:

```yaml
# /tmp/krakend-delayed/docker-compose.yml
services:
  krakend:
    image: krakend:2.10
    entrypoint: ["sh", "-c"]
    command: ["sleep 60 && exec /usr/bin/krakend run -d -c /etc/krakend/krakend.json"]
    # ... ports + volumes per the regular setup
```

Expected sequence with the retry loop in place:

- t ≈ 2 s: first probe, `discover did not match` (HTTP connection refused).
- t ≈ 5 s, 10 s: fast retry slots fire (logged but still no match).
- t ≈ 10-60 s: 30 s retry slots fire periodically (still no match).
- t ≈ 60 s: krakend starts listening on :9090.
- Next retry tick after that (≤ 5 s later): probe succeeds, krakend
  check goes [OK].

To regenerate the harness see `/tmp/krakend-delayed/` (not committed —
quick reproducer for local validation only).
```

- [ ] **Step 6: Commit**

```bash
git add docs/superpowers/2026-05-06-discover-e2e-smoke.md
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
docs: add delayed-krakend retry validation to smoke

Captures the manual smoke that validates the discoverer retry loop:
a krakend container with a 60 s sleep before exec'ing the binary,
expected log sequence (initial probe miss → retries through 5 s and
30 s slots → late-arriving match → check [OK]).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Update the PR description**

Add to the "What's in this PR" section, after the discoverer-package bullets:

```markdown
**Retry budget for late-starting services.** The discoverer cache
tracks attempts on a `[5s, 5s, 30s × 8]` schedule (11 attempts total
over ~4 min 10 s); a 5 s ticker in `AutoConfig` re-runs
`reconcileService` for services with at least one not-yet-given-up
discovery entry, and applies the resulting ConfigChanges via the
existing scheduler path. Fixes the bug where a container whose
application starts after the AD event (slow boot, healthchecks,
etc.) would never get its check scheduled because `discover()`
fired once and never again.
```

```bash
gh pr view 50372 --json body -q .body > /tmp/pr_body.md
# edit /tmp/pr_body.md to add the bullet
gh pr edit 50372 --body-file /tmp/pr_body.md
```

---

## Validation summary

After all 7 tasks, the following should be true:

- `dda inv test --targets=./comp/core/autodiscovery/discoverer` — all tests pass, including 4 new cache tests and 4 new discoverer tests.
- `dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl` — all tests pass, including 4 new configmgr tests.
- `dda inv linter.go --targets=./comp/core/autodiscovery/...` — clean.
- The agent builds cleanly with `dda inv agent.build --build-exclude=systemd`.
- The delayed-krakend reproducer ends with the krakend check `[OK]` and metrics flowing, instead of "no krakend section" as it does on the current branch.
