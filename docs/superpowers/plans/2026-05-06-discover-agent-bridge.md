# Plan B: Agent Discoverer + rtloader Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Agent-side infrastructure for the Python `discover(service)` advanced auto-config path. Add a new `discoverer` package that calls a Python classmethod via rtloader, retrieves a list of resolved instance configs, and schedules them. Migrate krakend (the only existing user of the previous Go-side prober) onto the new path. Remove the old Go prober and `%%discovered_port%%` template variable.

**Architecture:** Cross-language: Go orchestration in `comp/core/autodiscovery/discoverer/`, C++ rtloader bridge in `rtloader/three/three.cpp` and `rtloader/rtloader/api.cpp`, Python entry-point helper in `datadog_checks_base.utils.discovery`. The bridge serializes `Service` to JSON, calls a generic `_run_discover(check_class, service_json)` Python helper that constructs a `Service`, invokes `cls.discover(service)`, and returns the list-of-dicts as JSON.

**Tech Stack:** Go (datadog-agent), C++ (rtloader/three), Python (datadog_checks_base, krakend), bash (`dda inv` task runner), pytest, the agent's existing `loader_test.go` patterns.

**Spec:** [`integrations-core/docs/superpowers/specs/2026-05-06-advanced-autoconfig-discover-design.md`](https://github.com/DataDog/integrations-core/blob/vitkyrka/disco-autoconfig/docs/superpowers/specs/2026-05-06-advanced-autoconfig-discover-design.md). Plan A (Python helpers in `datadog_checks_base`) has already shipped on the `vitkyrka/disco-autoconfig` branch of `integrations-core`.

**Cross-repo coupling:** Tasks 11–13 touch `integrations-core` (krakend's `check.py` and `auto_conf_discovery.yaml`). The work is cross-cut by repo only at the migration boundary; the agent infrastructure is self-contained until then.

## File Structure

### New (datadog-agent)
- `comp/core/autodiscovery/discoverer/types.go` — `Result`, `Discoverer` interface, `Bridge` interface (decoupled from rtloader for testability).
- `comp/core/autodiscovery/discoverer/cache.go` — moved from `discovery/cache.go`, keyed by `(svcID, integration_name)`.
- `comp/core/autodiscovery/discoverer/discoverer.go` — `defaultDiscoverer` orchestrator. Cache + bridge call + result conversion.
- `comp/core/autodiscovery/discoverer/cache_test.go`, `discoverer_test.go` — unit tests with a fake bridge.
- `comp/core/autodiscovery/discoverer/python_bridge.go` — real bridge that wraps the rtloader entry point (cgo).
- `rtloader/include/datadog_agent_rtloader.h` — new `run_discover` C function declaration.
- `rtloader/include/rtloader.h` — new pure-virtual `runDiscover` on `RtLoader`.
- `rtloader/rtloader/api.cpp` — `run_discover` C export.
- `rtloader/three/three.h`, `three.cpp` — concrete `Three::runDiscover`.

### Modified (datadog-agent)
- `comp/core/autodiscovery/integration/config.go` — simplify `Discovery` to a presence marker (`*Discovery` struct with no required fields).
- `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go` — replace `prober.Probe` call with `discoverer.Discover`.
- `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go` — wire the discoverer into the constructor.

### Deleted (datadog-agent)
- `comp/core/autodiscovery/discovery/openmetrics_prober.go` + tests.
- `comp/core/autodiscovery/discovery/service_wrapper.go` + tests.
- `comp/core/autodiscovery/discovery/types.go` (the old `Prober` interface and `ProbeResult`).
- `comp/core/autodiscovery/discovery/cache.go` (moved to `discoverer/`).
- `comp/core/autodiscovery/discovery/candidates.go` (logic now lives in Python's `candidate_ports`).
- All tests under `comp/core/autodiscovery/discovery/`.
- `pkg/util/tmplvar/resolver.go`: remove `GetDiscoveredPort`, the `"discovered"` entry from `templateVariables`, and related tests.

### New / modified (integrations-core)
- `datadog_checks_base/datadog_checks/base/utils/discovery/_bridge.py` — `_run_discover(check_class, service_json) -> str` helper.
- `datadog_checks_base/tests/base/utils/discovery/test_bridge.py` — unit test.
- `krakend/datadog_checks/krakend/check.py` — new `discover(cls, service)` classmethod.
- `krakend/datadog_checks/krakend/data/auto_conf_discovery.yaml` — drop the `%%discovered_port%%` instance template; keep `ad_identifiers`.

## Test Commands

**Go (datadog-agent)** — never run raw `go test`. Use:

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl
dda inv linter.go
```

For full agent build (e2e smoke): `dda inv agent.build --build-exclude=systemd`.

**C++ rtloader build** — exercised via the agent build because rtloader is built as part of the agent build chain.

**Python (integrations-core)** — known issue on dev hosts: `ddev test datadog_checks_base` fails on a missing `krb5-config`. Fall back to:

```bash
hatch -e datadog-harbor run pytest <path> -v
```

run from the appropriate integration directory (`harbor/` for `datadog-harbor`, etc.).

---

### Task 1: Discoverer package skeleton (types and interfaces)

**Files:**
- Create: `comp/core/autodiscovery/discoverer/types.go`
- Create: `comp/core/autodiscovery/discoverer/types_test.go`

- [ ] **Step 1: Write the failing test**

`comp/core/autodiscovery/discoverer/types_test.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestResultZeroValueIsEmpty(t *testing.T) {
	var r Result
	assert.Empty(t, r.Configs)
}

func TestResultPreservesConfigs(t *testing.T) {
	cfgs := []integration.Config{{Name: "krakend"}, {Name: "krakend"}}
	r := Result{Configs: cfgs}
	assert.Len(t, r.Configs, 2)
	assert.Equal(t, "krakend", r.Configs[0].Name)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: package not found / compile error on missing `Result` type.

- [ ] **Step 3: Implement types and interfaces**

`comp/core/autodiscovery/discoverer/types.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package discoverer implements probe-based "advanced auto-config" by
// dispatching the probe decision to a Python discover() classmethod on the
// integration's check class. The Python side returns the resolved instance
// configs directly; this package handles caching, time budgeting, and
// marshalling.
package discoverer

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
)

// Result is the output of a successful Discover call.
type Result struct {
	// Configs are the integration.Config values to schedule, one per dict
	// returned by the Python discover() classmethod. Each carries Name set to
	// the integration name and Instances populated from the Python result.
	Configs []integration.Config
}

// Discoverer dispatches discovery probes to the Python side via a Bridge.
// Returns ok=false when the probe did not match (no configs to schedule);
// any error is logged internally.
type Discoverer interface {
	Discover(ctx context.Context, integrationName string, svc listeners.Service) (Result, bool)
}

// Bridge is the boundary between the discoverer and the Python runtime.
// Production uses a cgo-backed implementation; tests use an in-memory fake.
type Bridge interface {
	// RunDiscover invokes <check_class>.discover(service) on the integration
	// named integrationName, passing the JSON-encoded service. Returns the
	// JSON-encoded Python result on success (a list of dicts, possibly empty),
	// or "null" if discover() returned None. Returns an error on Python-side
	// exceptions or marshalling failures.
	RunDiscover(integrationName string, serviceJSON string) (string, error)
}
```

- [ ] **Step 4: Run the test**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add comp/core/autodiscovery/discoverer/types.go comp/core/autodiscovery/discoverer/types_test.go
git commit -m "autodiscovery/discoverer: add types and interfaces"
```

---

### Task 2: Move the probe cache to the new package

The existing `comp/core/autodiscovery/discovery/cache.go` is generic enough to reuse. Move it to `discoverer/`, change the cache key from `(svcID, cfgHash)` to `(svcID, integrationName)`, and update its return type to `Result` instead of `ProbeResult`.

**Files:**
- Create: `comp/core/autodiscovery/discoverer/cache.go`
- Create: `comp/core/autodiscovery/discoverer/cache_test.go`
- (Old `discovery/cache.go` and `cache_test.go` are deleted in Task 11.)

- [ ] **Step 1: Write the failing test**

`comp/core/autodiscovery/discoverer/cache_test.go`:

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

func TestCacheMissReturnsFalse(t *testing.T) {
	c := newCache(time.Now)
	_, _, hit := c.get("svc-1", "krakend")
	assert.False(t, hit)
}

func TestCacheStoresSuccess(t *testing.T) {
	c := newCache(time.Now)
	r := Result{Configs: []integration.Config{{Name: "krakend"}}}
	c.putSuccess("svc-1", "krakend", r)
	got, ok, hit := c.get("svc-1", "krakend")
	assert.True(t, hit)
	assert.True(t, ok)
	assert.Len(t, got.Configs, 1)
}

func TestCacheStoresFailureAndExpires(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	c := newCache(clock)
	c.putFailure("svc-1", "krakend", 30*time.Second)

	_, ok, hit := c.get("svc-1", "krakend")
	assert.True(t, hit)
	assert.False(t, ok, "failure cached")

	now = now.Add(31 * time.Second)
	_, _, hit = c.get("svc-1", "krakend")
	assert.False(t, hit, "failure expired")
}

func TestCacheKeyIsolation(t *testing.T) {
	c := newCache(time.Now)
	c.putSuccess("svc-1", "krakend", Result{Configs: []integration.Config{{Name: "krakend"}}})
	_, _, hit := c.get("svc-1", "apache")
	assert.False(t, hit, "different integration is a different key")
	_, _, hit = c.get("svc-2", "krakend")
	assert.False(t, hit, "different service is a different key")
}
```

- [ ] **Step 2: Run the test, expect compile error**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: missing `newCache`/`cache` symbols.

- [ ] **Step 3: Implement the cache**

`comp/core/autodiscovery/discoverer/cache.go`:

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

type cacheEntry struct {
	result    Result
	success   bool
	expiresAt time.Time // zero = never
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

func (c *cache) get(svcID, integrationName string) (Result, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(svcID, integrationName)]
	if !ok {
		return Result{}, false, false
	}
	if !e.expiresAt.IsZero() && c.now().After(e.expiresAt) {
		delete(c.entries, cacheKey(svcID, integrationName))
		return Result{}, false, false
	}
	return e.result, e.success, true
}

func (c *cache) putSuccess(svcID, integrationName string, r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, integrationName)] = cacheEntry{result: r, success: true}
}

func (c *cache) putFailure(svcID, integrationName string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, integrationName)] = cacheEntry{success: false, expiresAt: c.now().Add(ttl)}
}
```

- [ ] **Step 4: Run the test**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: PASS for all 4 cache tests + the 2 from Task 1.

- [ ] **Step 5: Commit**

```bash
git add comp/core/autodiscovery/discoverer/cache.go comp/core/autodiscovery/discoverer/cache_test.go
git commit -m "autodiscovery/discoverer: add cache keyed by (svc, integration)"
```

---

### Task 3: Discoverer orchestrator with a fake bridge

**Files:**
- Create: `comp/core/autodiscovery/discoverer/discoverer.go`
- Create: `comp/core/autodiscovery/discoverer/discoverer_test.go`

- [ ] **Step 1: Write the failing tests**

The test uses a fake `Bridge` that returns canned JSON. The discoverer should:
- Cache lookup → return cached result.
- On miss, call the bridge with a JSON-encoded service.
- Cache the result.
- On bridge error, cache as failure for ~30s.
- On `null` JSON, treat as no-match (cache as failure).
- On `[]` JSON, treat as no-match (cache as failure).
- On a non-empty list, build `integration.Config` per dict and return.

`comp/core/autodiscovery/discoverer/discoverer_test.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBridge struct {
	calls   int
	respond func(integrationName, serviceJSON string) (string, error)
}

func (f *fakeBridge) RunDiscover(integrationName, serviceJSON string) (string, error) {
	f.calls++
	return f.respond(integrationName, serviceJSON)
}

type fakeService struct {
	id    string
	hosts map[string]string
	ports []workloadmeta.ContainerPort
}

func (s *fakeService) GetServiceID() string                                  { return s.id }
func (s *fakeService) GetADIdentifiers() []string                            { return []string{"krakend"} }
func (s *fakeService) GetHosts() (map[string]string, error)                  { return s.hosts, nil }
func (s *fakeService) GetTags() ([]string, error)                            { return nil, nil }
func (s *fakeService) GetTagsWithCardinality(string) ([]string, error)       { return nil, nil }
func (s *fakeService) GetPid() (int, error)                                  { return 0, nil }
func (s *fakeService) GetHostname() (string, error)                          { return "", nil }
func (s *fakeService) IsReady() bool                                         { return true }
func (s *fakeService) GetExtraConfig(string) (string, error)                 { return "", nil }
func (s *fakeService) GetImageName() string                                  { return "" }
func (s *fakeService) GetPorts() ([]workloadmeta.ContainerPort, error)       { return s.ports, nil }
func (s *fakeService) HasFilter(workloadfilter.Scope) bool                   { return false }
func (s *fakeService) FilterTemplates(map[string]integration.Config)         {}
func (s *fakeService) Equal(listeners.Service) bool                          { return false }

func newFakeService() *fakeService {
	return &fakeService{
		id:    "docker://abc",
		hosts: map[string]string{"bridge": "10.0.0.1"},
		ports: []workloadmeta.ContainerPort{{Port: 9090, Name: ""}},
	}
}

func TestDiscoverHappyPath(t *testing.T) {
	bridge := &fakeBridge{respond: func(name, _ string) (string, error) {
		require.Equal(t, "krakend", name)
		return `[{"openmetrics_endpoint": "http://10.0.0.1:9090/metrics"}]`, nil
	}}
	d := newDiscoverer(bridge)
	r, ok := d.Discover(context.Background(), "krakend", newFakeService())
	require.True(t, ok)
	require.Len(t, r.Configs, 1)
	assert.Equal(t, "krakend", r.Configs[0].Name)
	assert.Contains(t, string(r.Configs[0].Instances[0]), "10.0.0.1")
	assert.Contains(t, string(r.Configs[0].Instances[0]), "9090")
}

func TestDiscoverNullResultNoMatch(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) { return "null", nil }}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
}

func TestDiscoverEmptyListNoMatch(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) { return "[]", nil }}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
}

func TestDiscoverErrorIsFailureCached(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return "", errors.New("python blew up")
	}}
	d := newDiscoverer(bridge)
	_, ok := d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
	// Second call hits cache, doesn't re-invoke bridge.
	_, ok = d.Discover(context.Background(), "krakend", newFakeService())
	assert.False(t, ok)
	assert.Equal(t, 1, bridge.calls, "negative cache should prevent re-invocation")
}

func TestDiscoverSuccessCached(t *testing.T) {
	bridge := &fakeBridge{respond: func(string, string) (string, error) {
		return `[{"openmetrics_endpoint":"x"}]`, nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.Equal(t, 1, bridge.calls, "successful result should be cached")
}

func TestDiscoverServiceJSONFormat(t *testing.T) {
	var captured string
	bridge := &fakeBridge{respond: func(_, j string) (string, error) {
		captured = j
		return "null", nil
	}}
	d := newDiscoverer(bridge)
	d.Discover(context.Background(), "krakend", newFakeService())
	assert.Contains(t, captured, `"id":"docker://abc"`)
	assert.Contains(t, captured, `"host":"10.0.0.1"`)
	assert.Contains(t, captured, `"number":9090`)
}
```

(The `fakeService` boilerplate is a chore but matches `listeners.Service`. Imports needed: `"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"`, `workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"`. Add them to the test file's imports as needed when compile errors surface.)

- [ ] **Step 2: Run the test, expect compile errors / missing symbols**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: missing `newDiscoverer`.

- [ ] **Step 3: Implement the discoverer**

`comp/core/autodiscovery/discoverer/discoverer.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

// New constructs a Discoverer wrapping the given Bridge. Pass nil bridge in
// configurations where Python is unavailable (cluster agent today); resolution
// of templates with Discovery set will then fail-closed.
func New(bridge Bridge) Discoverer {
	if bridge == nil {
		return nil
	}
	return newDiscoverer(bridge)
}

// servicePayload is the JSON shape passed across the rtloader bridge.
type servicePayload struct {
	ID    string        `json:"id"`
	Host  string        `json:"host"`
	Ports []portPayload `json:"ports"`
}

type portPayload struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

func (d *defaultDiscoverer) Discover(_ context.Context, integrationName string, svc listeners.Service) (Result, bool) {
	svcID := svc.GetServiceID()
	if r, ok, hit := d.cache.get(svcID, integrationName); hit {
		return r, ok
	}

	host, ok := pickHost(svc)
	if !ok {
		log.Debugf("autodiscovery/discoverer: %s has no host, skipping", svcID)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}
	exposed, err := svc.GetPorts()
	if err != nil {
		log.Debugf("autodiscovery/discoverer: %s GetPorts error: %v", svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	payload := servicePayload{ID: svcID, Host: host}
	for _, p := range exposed {
		payload.Ports = append(payload.Ports, portPayload{Number: p.Port, Name: p.Name})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("autodiscovery/discoverer: marshal failed for %s: %v", svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	resJSON, err := d.bridge.RunDiscover(integrationName, string(body))
	if err != nil {
		log.Warnf("autodiscovery/discoverer: %s.discover() failed for %s: %v", integrationName, svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}
	if resJSON == "" || resJSON == "null" {
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	var instances []json.RawMessage
	if err := json.Unmarshal([]byte(resJSON), &instances); err != nil {
		log.Errorf("autodiscovery/discoverer: %s returned non-list JSON for %s: %v", integrationName, svcID, err)
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}
	if len(instances) == 0 {
		d.cache.putFailure(svcID, integrationName, d.failureTTL)
		return Result{}, false
	}

	configs := make([]integration.Config, 0, len(instances))
	for _, raw := range instances {
		configs = append(configs, integration.Config{
			Name:      integrationName,
			Instances: []integration.Data{integration.Data(raw)},
		})
	}
	r := Result{Configs: configs}
	d.cache.putSuccess(svcID, integrationName, r)
	return r, true
}

func pickHost(svc listeners.Service) (string, bool) {
	hosts, err := svc.GetHosts()
	if err != nil || len(hosts) == 0 {
		return "", false
	}
	if h, ok := hosts["bridge"]; ok && h != "" {
		return h, true
	}
	for _, h := range hosts {
		if h != "" {
			return h, true
		}
	}
	return "", false
}
```

- [ ] **Step 4: Run the test**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: PASS for all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add comp/core/autodiscovery/discoverer/discoverer.go comp/core/autodiscovery/discoverer/discoverer_test.go
git commit -m "autodiscovery/discoverer: add orchestrator with bridge interface"
```

---

### Task 4: Python-side bridge entry point in datadog_checks_base

This is the integrations-core piece that runs inside Python. It accepts the JSON service payload, builds a `Service` instance, calls the integration's `discover()` classmethod, and returns the JSON-serialized result. The function is in `datadog_checks_base.utils.discovery` so any check class can use it without per-integration glue.

**Working directory:** `/home/vagrant/go/src/github.com/DataDog/integrations-core`. Branch: `vitkyrka/disco-autoconfig`.

**Files:**
- Create: `datadog_checks_base/datadog_checks/base/utils/discovery/_bridge.py`
- Create: `datadog_checks_base/tests/base/utils/discovery/test_bridge.py`
- Modify: `datadog_checks_base/datadog_checks/base/utils/discovery/__init__.pyi` to export `_run_discover`.

- [ ] **Step 1: Write the failing test**

`datadog_checks_base/tests/base/utils/discovery/test_bridge.py`:

```python
# (C) Datadog, Inc. 2026-present
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)
import json

import pytest

from datadog_checks.base.utils.discovery._bridge import _run_discover
from datadog_checks.base.utils.discovery.service import Port, Service


class _Found:
    @classmethod
    def discover(cls, service: Service):
        return [{"openmetrics_endpoint": f"http://{service.host}:{service.ports[0].number}/metrics"}]


class _NotFound:
    @classmethod
    def discover(cls, service: Service):
        return None


class _EmptyList:
    @classmethod
    def discover(cls, service: Service):
        return []


class _Raises:
    @classmethod
    def discover(cls, service: Service):
        raise RuntimeError("boom")


SVC_JSON = json.dumps({
    "id": "docker://abc",
    "host": "10.0.0.1",
    "ports": [{"number": 9090, "name": "metrics"}],
})


def test_bridge_returns_json_list_on_match():
    out = _run_discover(_Found, SVC_JSON)
    parsed = json.loads(out)
    assert parsed == [{"openmetrics_endpoint": "http://10.0.0.1:9090/metrics"}]


def test_bridge_returns_null_on_no_match():
    assert _run_discover(_NotFound, SVC_JSON) == "null"


def test_bridge_returns_empty_list_on_explicit_empty():
    assert _run_discover(_EmptyList, SVC_JSON) == "[]"


def test_bridge_returns_null_on_exception():
    assert _run_discover(_Raises, SVC_JSON) == "null"


def test_bridge_constructs_service_correctly():
    captured = {}

    class C:
        @classmethod
        def discover(cls, service: Service):
            captured["id"] = service.id
            captured["host"] = service.host
            captured["ports"] = [(p.number, p.name) for p in service.ports]
            return None

    _run_discover(C, SVC_JSON)
    assert captured == {
        "id": "docker://abc",
        "host": "10.0.0.1",
        "ports": [(9090, "metrics")],
    }


def test_bridge_handles_missing_discover_method():
    class NoDiscover:
        pass
    assert _run_discover(NoDiscover, SVC_JSON) == "null"
```

```bash
hatch -e datadog-harbor run pytest datadog_checks_base/tests/base/utils/discovery/test_bridge.py -v
```

Expected: ImportError on `_bridge`.

- [ ] **Step 2: Implement**

`datadog_checks_base/datadog_checks/base/utils/discovery/_bridge.py`:

```python
# (C) Datadog, Inc. 2026-present
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)
"""Bridge entry point invoked from the Agent's rtloader to run a check class's
``discover(service)`` method.

The Agent serializes the listeners.Service projection to JSON, calls this
function with the check class, and receives a JSON string in return:

- ``"null"`` — discover returned None, raised, or the class has no discover().
- ``"[]"`` — discover explicitly returned an empty list.
- ``"[{...}, {...}]"`` — one entry per resolved instance config.
"""
import json
import logging
from typing import Any

from .service import Port, Service

_log = logging.getLogger(__name__)


def _run_discover(check_class: Any, service_json: str) -> str:
    """Run the discover() classmethod and return the JSON-encoded result.

    Never raises — any error is caught, logged, and returned as ``"null"``.
    """
    try:
        payload = json.loads(service_json)
        ports = tuple(
            Port(number=int(p["number"]), name=p.get("name", ""))
            for p in payload.get("ports", [])
        )
        service = Service(id=payload["id"], host=payload["host"], ports=ports)
    except Exception:
        _log.exception("discover bridge: failed to parse service payload")
        return "null"

    discover = getattr(check_class, "discover", None)
    if discover is None:
        return "null"

    try:
        result = discover(service)
    except Exception:
        _log.exception("discover bridge: %s.discover raised", getattr(check_class, "__name__", "?"))
        return "null"

    if result is None:
        return "null"

    try:
        return json.dumps(list(result))
    except (TypeError, ValueError):
        _log.exception("discover bridge: %s.discover returned non-JSON-serializable", check_class)
        return "null"
```

- [ ] **Step 3: Update __init__.pyi**

Add `from ._bridge import _run_discover` and append `'_run_discover'` to `__all__`. Underscore prefix is intentional — the bridge isn't user-facing.

`datadog_checks_base/datadog_checks/base/utils/discovery/__init__.pyi`:

Add:
```python
from ._bridge import _run_discover
```
to the imports, and `'_run_discover'` to `__all__`.

- [ ] **Step 4: Run the tests**

```bash
hatch -e datadog-harbor run pytest datadog_checks_base/tests/base/utils/discovery/ -v
```

Expected: all previous discovery tests still pass + 6 new test_bridge tests pass.

- [ ] **Step 5: Commit (in integrations-core)**

```bash
cd /home/vagrant/go/src/github.com/DataDog/integrations-core
git add datadog_checks_base/datadog_checks/base/utils/discovery/_bridge.py \
        datadog_checks_base/tests/base/utils/discovery/test_bridge.py \
        datadog_checks_base/datadog_checks/base/utils/discovery/__init__.pyi
git commit -m "datadog_checks_base: add discover() rtloader bridge helper"
```

Then `cd` back to `/home/vagrant/go/src/github.com/DataDog/datadog-agent` for the rest of the plan.

---

### Task 5: rtloader — `runDiscover` virtual method on RtLoader (C++)

**Working directory:** `/home/vagrant/go/src/github.com/DataDog/datadog-agent`.

**Files:**
- Modify: `rtloader/include/rtloader.h` — add the pure-virtual declaration.
- Modify: `rtloader/three/three.h` — declare the override.
- Modify: `rtloader/three/three.cpp` — implement.
- Modify: `rtloader/test/rtloader/rtloader.go` (and any siblings) — add a stub for the test mock.

This is modeled directly on the existing `runCheck` pair (`rtloader.h:117-122` declaration; `three.cpp:468-498` implementation).

- [ ] **Step 1: Read the existing `runCheck` pair as the template**

```bash
sed -n '100,130p' rtloader/include/rtloader.h   # virtual declaration
sed -n '468,498p' rtloader/three/three.cpp      # implementation
```

Note the pattern:
- Pure-virtual returning `char *`.
- `Three` implementation acquires `py_check`, calls `PyObject_CallMethod`, validates the result with `PyUnicode_Check`, copies to a freshly-malloc'd `char *` via `as_string`, decrefs the Python result, returns the C string.

- [ ] **Step 2: Add the virtual declaration**

In `rtloader/include/rtloader.h`, after `runCheck`'s declaration (around line 122), add:

```cpp
    //! Pure virtual runDiscover member.
    /*!
      \param py_class The python class object on which to call the
                      ``_run_discover`` classmethod (datadog_checks_base
                      bridge helper).
      \param service_json A C-string JSON payload representing the
                          autodiscovery Service.
      \return A C-string with the JSON-serialized discover result. Caller
              must free the returned pointer.
    */
    virtual char *runDiscover(RtLoaderPyObject *py_class, const char *service_json) = 0;
```

- [ ] **Step 3: Add the override declaration in three.h**

Find the `runCheck` declaration in `rtloader/three/three.h` and add a sibling line:

```cpp
    char *runDiscover(RtLoaderPyObject *py_class, const char *service_json) override;
```

- [ ] **Step 4: Implement in three.cpp**

Append after `Three::runCheck` (around line 498) in `rtloader/three/three.cpp`:

```cpp
char *Three::runDiscover(RtLoaderPyObject *py_class, const char *service_json)
{
    if (py_class == NULL || service_json == NULL) {
        return NULL;
    }

    PyObject *klass = reinterpret_cast<PyObject *>(py_class);

    char *ret = NULL;
    char run_discover[] = "_run_discover";
    char format[] = "(s)";
    PyObject *result = NULL;
    PyObject *bridge_module = NULL;
    PyObject *bridge_func = NULL;

    // Resolve datadog_checks.base.utils.discovery._run_discover and call it.
    bridge_module = PyImport_ImportModule("datadog_checks.base.utils.discovery");
    if (bridge_module == NULL) {
        setError("error importing discovery bridge module: " + _fetchPythonError());
        goto done;
    }
    bridge_func = PyObject_GetAttrString(bridge_module, run_discover);
    if (bridge_func == NULL) {
        setError("error resolving _run_discover: " + _fetchPythonError());
        goto done;
    }
    result = PyObject_CallFunction(bridge_func, format, klass, service_json);
    if (result == NULL || !PyUnicode_Check(result)) {
        setError("error invoking discovery bridge: " + _fetchPythonError());
        goto done;
    }

    ret = as_string(result);
    if (ret == NULL) {
        setError("error converting discovery result to string");
        goto done;
    }

done:
    Py_XDECREF(result);
    Py_XDECREF(bridge_func);
    Py_XDECREF(bridge_module);
    return ret;
}
```

Note: the bridge is invoked as a free function `_run_discover(check_class, service_json)` rather than as a method on `klass`. This matches Task 4's design — the helper lives in `datadog_checks_base` and accepts the class as its first argument.

- [ ] **Step 5: Add a stub in the test mock**

`rtloader/test/rtloader/rtloader.go` mocks the `RtLoader` for tests. It will fail to compile until a stub for `runDiscover` is added. Locate the `runCheck` stub (search for "runCheck") and add a sibling:

```go
// Approximate shape — the file is a Go-cgo wrapper. Look for the existing
// runCheck stub function and mirror it. The implementation can return ""
// for the test mock.
```

(If multiple test mocks exist — `rtloader/test/datadog_agent/`, etc. — add stubs everywhere needed. Compile error messages will tell you exactly which files to touch.)

- [ ] **Step 6: Build the agent to verify the C++ compiles**

```bash
dda inv agent.build --build-exclude=systemd
```

Expected: clean build. If link errors mention any test mock missing `runDiscover`, add stubs there.

- [ ] **Step 7: Commit**

```bash
git add rtloader/include/rtloader.h rtloader/three/three.h rtloader/three/three.cpp rtloader/test/
git commit -m "rtloader: add runDiscover virtual for advanced auto-config"
```

---

### Task 6: rtloader C API + Go cgo wrapper

The `runDiscover` method is exposed to Go via the C API (`rtloader/include/datadog_agent_rtloader.h`, `rtloader/rtloader/api.cpp`). Then a Go wrapper in `pkg/collector/python/` calls into it.

**Files:**
- Modify: `rtloader/include/datadog_agent_rtloader.h` — add `run_discover` declaration.
- Modify: `rtloader/rtloader/api.cpp` — add `run_discover` definition.
- Modify: `pkg/collector/python/loader.go` (or a new file alongside) — add Go-side cgo call.

- [ ] **Step 1: Locate the existing `run_check` C export**

```bash
grep -n "run_check" rtloader/include/datadog_agent_rtloader.h
grep -n "run_check" rtloader/rtloader/api.cpp
```

The export pattern is two lines in `api.cpp`:

```cpp
char *run_check(rtloader_t *rtloader, rtloader_pyobject_t *check)
{
    return AS_TYPE(RtLoader, rtloader)->runCheck(AS_TYPE(RtLoaderPyObject, check));
}
```

And one declaration in the header.

- [ ] **Step 2: Add the C export header declaration**

In `rtloader/include/datadog_agent_rtloader.h`, after the `run_check` declaration:

```cpp
/*! \fn char *run_discover(rtloader_t *, rtloader_pyobject_t *py_class, const char *service_json)
    \brief Invoke datadog_checks.base.utils.discovery._run_discover on the
           given class with the given service JSON payload.
    \param rtloader The rtloader handle.
    \param py_class The check class previously loaded via get_class.
    \param service_json A null-terminated JSON payload for the service.
    \return Heap-allocated C string with the JSON-encoded result, or NULL on
            error. Caller must free with free().
*/
DATADOG_AGENT_RTLOADER_API char *run_discover(rtloader_t *, rtloader_pyobject_t *py_class, const char *service_json);
```

- [ ] **Step 3: Add the C export definition**

In `rtloader/rtloader/api.cpp`, after `run_check`:

```cpp
char *run_discover(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *service_json)
{
    return AS_TYPE(RtLoader, rtloader)->runDiscover(AS_TYPE(RtLoaderPyObject, py_class), service_json);
}
```

- [ ] **Step 4: Write a failing Go-side test for the bridge**

`comp/core/autodiscovery/discoverer/python_bridge_test.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package discoverer

import "testing"

func TestNewPythonBridgeIsNonNil(t *testing.T) {
	b := NewPythonBridge(nil) // pass nil pythonLoader; bridge should handle gracefully
	if b == nil {
		t.Fatal("NewPythonBridge returned nil")
	}
}
```

(End-to-end behavior is exercised in Task 13's smoke test; this is a smoke test that the build chain compiles and the constructor returns.)

- [ ] **Step 5: Implement the Go bridge**

`comp/core/autodiscovery/discoverer/python_bridge.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package discoverer

/*
#include <datadog_agent_rtloader.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
)

// pythonClassResolver fetches the loaded RtLoaderPyObject for an integration
// name. The agent's existing python.Loader knows this; the bridge just needs
// the resolved class pointer so it can pass it to run_discover.
type pythonClassResolver interface {
	ResolveCheckClass(integrationName string) (unsafe.Pointer, error)
}

type pythonBridge struct {
	resolver pythonClassResolver
}

// NewPythonBridge returns a Bridge backed by rtloader. The resolver is
// expected to return the *RtLoaderPyObject for the given integration name.
func NewPythonBridge(resolver pythonClassResolver) Bridge {
	return &pythonBridge{resolver: resolver}
}

func (b *pythonBridge) RunDiscover(integrationName, serviceJSON string) (string, error) {
	if b.resolver == nil {
		return "", errors.New("python bridge: no class resolver")
	}
	klass, err := b.resolver.ResolveCheckClass(integrationName)
	if err != nil {
		return "", fmt.Errorf("python bridge: resolve %s: %w", integrationName, err)
	}
	if klass == nil {
		return "", fmt.Errorf("python bridge: integration %s has no loaded class", integrationName)
	}

	cJSON := C.CString(serviceJSON)
	defer C.free(unsafe.Pointer(cJSON))

	rt := python.GetRtLoader()
	if rt == nil {
		return "", errors.New("python bridge: rtloader not initialized")
	}

	cResult := C.run_discover(
		(*C.rtloader_t)(rt),
		(*C.rtloader_pyobject_t)(klass),
		cJSON,
	)
	if cResult == nil {
		return "", fmt.Errorf("python bridge: %s discover failed (rtloader returned NULL)", integrationName)
	}
	defer C.free(unsafe.Pointer(cResult))

	return C.GoString(cResult), nil
}
```

This depends on two things that the implementer needs to wire up:
1. `python.GetRtLoader()` — does the agent's `pkg/collector/python` already expose a getter for the global `rtloader_t *`? Look for `rtloader_pyobject_t`, `RtLoader`, `s_rtloader`. If a getter exists with a different name, use it; if none exists, add a small one. Don't introduce a new dependency direction — `discoverer` can depend on `pkg/collector/python` but not vice versa.
2. The `pythonClassResolver` interface — the agent's existing Python loader caches classes per integration. Find the loader's class lookup function and have the resolver wrap it.

If either dependency is hard to wire cleanly, fall back to a simpler design: have the loader inject the bridge during initialization rather than the bridge calling back into the loader. Either shape works; pick the one that makes the test in Task 7 simplest.

- [ ] **Step 6: Run the test (just compile-check)**

```bash
dda inv test --targets=./comp/core/autodiscovery/discoverer
```

Expected: compiles and the smoke test passes.

- [ ] **Step 7: Commit**

```bash
git add rtloader/include/datadog_agent_rtloader.h \
        rtloader/rtloader/api.cpp \
        comp/core/autodiscovery/discoverer/python_bridge.go \
        comp/core/autodiscovery/discoverer/python_bridge_test.go
git commit -m "autodiscovery/discoverer: add cgo-backed Python bridge"
```

---

### Task 7: Simplify `integration.Config.Discovery` to a presence marker

The current `DiscoveryConfig` has `Type`, `Ports`, `Path` — useful only for the deprecated Go-side prober. With the Python `discover()` model, the only signal we need is "this template wants discovery." The Python side carries all the per-integration knowledge.

**Files:**
- Modify: `comp/core/autodiscovery/integration/config.go`

- [ ] **Step 1: Inspect current shape**

```bash
grep -n "DiscoveryConfig\|Discovery " comp/core/autodiscovery/integration/config.go
```

- [ ] **Step 2: Update the Discovery field**

In `comp/core/autodiscovery/integration/config.go`, replace the existing `DiscoveryConfig` with an empty struct used as a marker:

```go
// Discovery, when non-nil, signals that this config is a discovery template:
// AutoDiscovery must call the integration's Python discover() method against
// the matched service to obtain concrete instances.
Discovery *Discovery `json:"discovery"` // (include in digest: true)
```

And the type:

```go
// Discovery is the marker payload for advanced auto-config templates. It is
// intentionally empty — the per-integration logic lives on the Python side
// in the integration's discover(service) classmethod.
type Discovery struct{}
```

Remove the old `DiscoveryConfig` type if no other callers use it; if other callers exist, mark it deprecated and keep until Task 11.

- [ ] **Step 3: Run the integration package's tests**

```bash
dda inv test --targets=./comp/core/autodiscovery/integration
```

Expected: PASS. The existing `config_test.go` may need a small update if it asserts on the old `DiscoveryConfig` shape.

- [ ] **Step 4: Commit**

```bash
git add comp/core/autodiscovery/integration/config.go comp/core/autodiscovery/integration/config_test.go
git commit -m "autodiscovery/integration: make Discovery a presence marker"
```

---

### Task 8: Update the providers config_reader to parse the simplified Discovery shape

The existing `comp/core/autodiscovery/providers/config_reader.go` parses `auto_conf_discovery.yaml` into the old `DiscoveryConfig`. Update to parse into the new marker.

**Files:**
- Modify: `comp/core/autodiscovery/providers/config_reader.go`
- Modify: `comp/core/autodiscovery/providers/config_reader_test.go`
- Modify: `comp/core/autodiscovery/providers/tests/auto_conf_discovery.yaml` — drop the `type`/`ports`/`path` fields if present.

- [ ] **Step 1: Locate the parser**

```bash
grep -n "Discovery\|DiscoveryConfig" comp/core/autodiscovery/providers/config_reader.go
```

The `configFormat` struct in this file has a `Discovery *integration.DiscoveryConfig` field. Change to `Discovery *integration.Discovery`.

- [ ] **Step 2: Update the test fixture**

`comp/core/autodiscovery/providers/tests/auto_conf_discovery.yaml`:

```yaml
ad_identifiers:
  - krakend
discovery: {}
init_config:
instances: []
```

(`{}` is a valid YAML empty mapping; it round-trips into `*Discovery` as a non-nil pointer to an empty struct.)

- [ ] **Step 3: Update the parser test**

In `config_reader_test.go`, the test that loads `auto_conf_discovery.yaml` should assert `cfg.Discovery != nil` rather than asserting on `Type`/`Ports`/`Path`.

- [ ] **Step 4: Run the providers tests**

```bash
dda inv test --targets=./comp/core/autodiscovery/providers
```

- [ ] **Step 5: Commit**

```bash
git add comp/core/autodiscovery/providers/
git commit -m "autodiscovery/providers: parse simplified discovery marker"
```

---

### Task 9: Wire the discoverer into `configmgr`

Replace the `prober.Probe` call with `discoverer.Discover`. Result is a slice of configs, scheduled via the existing `applyChanges` flow.

**Files:**
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go`
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go` — constructor wiring.
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/configmgr_test.go`

- [ ] **Step 1: Replace the prober field with a discoverer field**

In `configmgr.go`, swap:

```go
prober         discovery.Prober
```

for:

```go
discoverer discoverer.Discoverer
```

Update `newReconcilingConfigManager` accordingly.

- [ ] **Step 2: Replace the resolution branch**

In `resolveTemplateForService` (currently around line 414), replace the prober-based block:

```go
if tpl.Discovery != nil {
    if cm.prober == nil { ... }
    result, ok := cm.prober.Probe(...)
    if !ok { ... }
    resolvedSvc = discovery.WrapWithProbeResult(svc, result)
}

config, err := configresolver.Resolve(tpl, resolvedSvc)
```

with:

```go
if tpl.Discovery != nil {
    if cm.discoverer == nil {
        msg := fmt.Sprintf("template %s has Discovery set but no discoverer is configured", tpl.Name)
        log.Errorf("autodiscovery: %s", msg)
        errorStats.setResolveWarning(tpl.Name, msg)
        return tpl, false
    }
    result, ok := cm.discoverer.Discover(context.Background(), tpl.Name, svc)
    if !ok {
        msg := fmt.Sprintf("discover did not match for template %s and service %s", tpl.Name, svc.GetServiceID())
        log.Debugf("autodiscovery: %s", msg)
        errorStats.setResolveWarning(tpl.Name, msg)
        return tpl, false
    }
    // Discovery returns full configs directly; carry the template's
    // identifiers/source forward and merge the resolved instances.
    if len(result.Configs) == 0 {
        return tpl, false
    }
    // Take the first config's instances. Multi-instance support (multiple
    // dicts from one discover call) requires a different return shape from
    // resolveTemplateForService — out of scope for this task; track as
    // follow-up.
    resolved := tpl
    resolved.Instances = result.Configs[0].Instances
    resolvedConfig, err := decryptConfig(resolved, cm.secretResolver, tpl.Digest())
    if err != nil { ... }
    return resolvedConfig, true
}
```

(Note: the multi-config-per-service case is real for `druid`, `tekton`, `torchserve`. The cleanest fix is to change `resolveTemplateForService`'s signature to return `[]integration.Config, bool` and update its callers in `reconcileService`. That refactor is non-trivial — track as a follow-up if Task 13's krakend smoke test passes with the single-config form.)

- [ ] **Step 3: Update the constructor**

`newReconcilingConfigManager` and the call site in `autoconfig.go` (search for `newReconcilingConfigManager`) take a `discoverer.Discoverer` instead of `discovery.Prober`. Pass `discoverer.New(discoverer.NewPythonBridge(...))` from the AutoConfig constructor; if the Python loader isn't initialized in this build (e.g. cluster agent), pass nil.

- [ ] **Step 4: Update tests**

`configmgr_test.go` previously used a fake prober. Replace with a fake `Discoverer` matching the interface from Task 1.

- [ ] **Step 5: Build and test**

```bash
dda inv linter.go
dda inv test --targets=./comp/core/autodiscovery/autodiscoveryimpl
dda inv agent.build --build-exclude=systemd
```

- [ ] **Step 6: Commit**

```bash
git add comp/core/autodiscovery/autodiscoveryimpl/
git commit -m "autodiscovery: replace prober with discoverer in configmgr"
```

---

### Task 10: Krakend `discover()` classmethod (integrations-core)

**Working directory:** `/home/vagrant/go/src/github.com/DataDog/integrations-core`. Branch: `vitkyrka/disco-autoconfig`.

**Files:**
- Modify: `krakend/datadog_checks/krakend/check.py` — add `discover` classmethod.
- Modify: `krakend/datadog_checks/krakend/data/auto_conf_discovery.yaml` — drop the instance template; keep ad_identifiers + discovery marker.

- [ ] **Step 1: Inspect the current krakend check**

```bash
cat krakend/datadog_checks/krakend/check.py
```

- [ ] **Step 2: Add the discover classmethod**

Insert (typically just before the `class KrakendCheck` body's first method):

```python
    @classmethod
    def discover(cls, service):
        from datadog_checks.base.utils.discovery import (
            candidate_ports,
            http_probe,
            is_prometheus_exposition,
        )

        for port in candidate_ports(service, [9090]):
            url_host = service.host
            if http_probe(url_host, port.number, "/metrics",
                          verifier=is_prometheus_exposition()):
                return [{"openmetrics_endpoint": f"http://{url_host}:{port.number}/metrics"}]
        return None
```

- [ ] **Step 3: Update auto_conf_discovery.yaml**

Replace the file's contents with:

```yaml
ad_identifiers:
  - krakend
discovery: {}
init_config:
instances: []
```

(No `%%discovered_port%%` anywhere; no `type`/`ports`/`path` in `discovery`. Python's `discover()` decides everything.)

- [ ] **Step 4: Run krakend's tests to confirm no regression**

```bash
hatch -e py3.13-1.0 -d run pytest krakend/tests/ -v
```

(Or whichever krakend hatch env exists — `hatch env show krakend` will list them.)

- [ ] **Step 5: Commit (in integrations-core)**

```bash
cd /home/vagrant/go/src/github.com/DataDog/integrations-core
git add krakend/
git commit -m "krakend: migrate to Python discover() classmethod"
```

---

### Task 11: Remove the old Go prober and `%%discovered_port%%`

**Working directory:** `/home/vagrant/go/src/github.com/DataDog/datadog-agent`.

**Files to delete:**
- `comp/core/autodiscovery/discovery/openmetrics_prober.go` and `..._test.go`
- `comp/core/autodiscovery/discovery/service_wrapper.go` and `..._test.go`
- `comp/core/autodiscovery/discovery/types.go` (old `Prober`, `ProbeResult`)
- `comp/core/autodiscovery/discovery/cache.go` and `..._test.go` (now in `discoverer/`)
- `comp/core/autodiscovery/discovery/candidates.go` and `..._test.go` (logic moved to Python)

**Files to modify:**
- `pkg/util/tmplvar/resolver.go` — remove `GetDiscoveredPort`, the `"discovered"` key in the `templateVariables` map, and any test references.
- `pkg/util/tmplvar/resolver_test.go` — drop tests for `%%discovered_port%%`.

- [ ] **Step 1: Delete the old discovery package**

```bash
rm -rf comp/core/autodiscovery/discovery
```

- [ ] **Step 2: Remove `%%discovered_port%%` from the resolver**

In `pkg/util/tmplvar/resolver.go`:
- Drop the `"discovered": GetDiscoveredPort,` entry from the `templateVariables` map (around line 103).
- Delete `GetDiscoveredPort` (around line 484).

In `pkg/util/tmplvar/resolver_test.go`:
- Delete the discovered-port tests (search for `discovered_port`).

- [ ] **Step 3: Build and run lints**

```bash
dda inv linter.go
dda inv agent.build --build-exclude=systemd
```

Expect no compile errors. If any caller still references the deleted symbols (unlikely after Tasks 7–9), remove or update them.

- [ ] **Step 4: Run the affected test packages**

```bash
dda inv test --targets=./pkg/util/tmplvar
dda inv test --targets=./comp/core/autodiscovery/...
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "autodiscovery: drop old Go prober and %%discovered_port%%"
```

---

### Task 12: Update the AutoConfig constructor wiring

The component that previously wired up the prober now wires up the discoverer. Specifically, the entrypoint in `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go` must construct a Python-bridge-backed Discoverer and pass it to `newReconcilingConfigManager`.

**Files:**
- Modify: `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go`

- [ ] **Step 1: Locate the construction site**

```bash
grep -n "newReconcilingConfigManager\|prober" comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go
```

- [ ] **Step 2: Replace prober wiring with discoverer wiring**

Where the constructor currently passes `discovery.NewOpenMetricsProber(...)`, instead pass `discoverer.New(discoverer.NewPythonBridge(resolver))` where `resolver` is wired to the agent's existing Python loader. The exact resolver shape depends on what's already exposed in `pkg/collector/python`; if no usable type exists, expose a minimal `ResolveCheckClass(integrationName) (unsafe.Pointer, error)` adapter on the Python loader.

- [ ] **Step 3: Build**

```bash
dda inv agent.build --build-exclude=systemd
```

- [ ] **Step 4: Commit**

```bash
git add comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go
git commit -m "autodiscovery: wire Python bridge into AutoConfig constructor"
```

---

### Task 13: End-to-end smoke test — krakend container

The existing krakend dev-env compose file in `integrations-core/krakend/tests/docker/` is the smoke target. Use it the same way the original krakend experiment design described.

- [ ] **Step 1: Build the agent**

```bash
cd /home/vagrant/go/src/github.com/DataDog/datadog-agent
dda inv agent.build --build-exclude=systemd
```

- [ ] **Step 2: Start the krakend container**

Follow the krakend integration's existing `tests/docker/` instructions (most likely `cd integrations-core/krakend/tests/docker && docker compose up -d`).

- [ ] **Step 3: Run the agent against the krakend container**

Per the original experiment plan: bind-mount the locally built agent binary plus the local krakend integration source into the nightly Docker image. The instructions in `integrations-core/reference_docker_integration_testing.md` cover the exact invocation.

- [ ] **Step 4: Verify the krakend check is scheduled**

```bash
docker exec <agent-container> agent status | grep -A 8 krakend
```

Expected:
- `krakend (...)` block.
- `openmetrics_endpoint: http://<container-ip>:9090/metrics` in the resolved config.
- Metrics flowing (e.g. `krakend.api.responded`, `krakend.router.connected`).

- [ ] **Step 5: Three-scenario verification (per the experiment design)**

- **Default port (9090):** scenario above. Probe matches, check runs.
- **Non-default port (e.g. 9000):** restart krakend bound to 9000 instead of 9090. Confirm the agent's Python `discover()` falls back to the rest of `service.ports` and finds the working port. (`candidate_ports([9090])` will yield 9090 first if exposed, otherwise iterate the others.)
- **Negative case:** start a non-krakend container with the `krakend` AD identifier label. Probe should fail; no check scheduled. Verify only DEBUG-level log messages, no errors.

- [ ] **Step 6: No commit needed for the smoke test itself.** If anything breaks during these scenarios, treat it as a regression in Tasks 1–12 and fix it there.

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| `discoverer` Go package + cache + orchestration | 1, 2, 3 |
| Python bridge entry point | 4 |
| rtloader virtual method | 5 |
| rtloader C API + Go wrapper | 6 |
| `integration.Config.Discovery` simplification | 7 |
| `auto_conf_discovery.yaml` parsing | 8 |
| `configmgr` integration | 9 |
| Krakend `discover()` migration | 10 |
| Old prober + `%%discovered_port%%` removal | 11 |
| AutoConfig constructor wiring | 12 |
| End-to-end smoke test | 13 |

**Cross-repo dependencies:** Tasks 4 and 10 are integrations-core changes. The order is: Plan A landed already → Task 4 (bridge helper) → Tasks 1–9 (agent infrastructure) → Task 10 (krakend migration) → Tasks 11–13 (cleanup + smoke test). Task 4 must precede Task 13 because the smoke test exercises the bridge end-to-end. The plan order respects this.

**Placeholder scan:** No `TBD`, `TODO`, `implement later`. Each task has concrete code or commands. Two locations have escape clauses:
- Task 5 step 5 ("add stubs everywhere needed") — the rtloader test mocks vary slightly by build configuration; the implementer follows compile errors. This is necessary because we can't enumerate every mock without inspecting each file.
- Task 6 step 5 ("If either dependency is hard to wire cleanly...") — the integration with the existing python loader has multiple valid shapes; the plan picks one and lets the implementer adjust based on what's already exposed.

These are not placeholders — they're explicit instructions where the task acknowledges optional adaptation against the existing code shape.

**Type consistency:** `Discoverer.Discover` returns `(Result, bool)` everywhere; `Result.Configs` is `[]integration.Config`; `Bridge.RunDiscover` returns `(string, error)`; `servicePayload` JSON shape (`id`, `host`, `ports[]{number,name}`) is used identically in Task 3 (Go-side marshal) and Task 4 (Python-side parse). Task 6's `pythonClassResolver.ResolveCheckClass` is named consistently across both files.

**Scope:** This plan is one logical change — replace the Go prober with the Python bridge. It is large because the change is cross-language, but each task is independent enough to commit and review separately. If the rtloader pieces (Tasks 5–6, 12) prove harder than expected, the natural split point is between Task 9 (configmgr accepts a `Discoverer` interface) and Task 12 (real bridge wiring). With a fake bridge, Tasks 1–9 form a coherent "infrastructure" PR; Tasks 10–13 form an "activate it" PR.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-06-discover-agent-bridge.md`. Two execution options:

1. **Subagent-Driven (recommended)** — Dispatch a fresh subagent per task, review between tasks. Cross-language tasks (5, 6, 12) benefit most from focused-context subagents.
2. **Inline Execution** — Execute tasks in this session via executing-plans with checkpoints.

Which approach?
