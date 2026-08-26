# Blocking Analysis for Approach 3

## Current Worker Architecture

The discovery worker is a single `Worker` instance with a pool of goroutines:

```
                    Worker
┌──────────────────────────────────────┐
│  workqueue (delaying)                 │
│     │                                 │
│     ▼                                 │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
│  │ goroutine│ │ goroutine│ │ goroutine│ │ goroutine│  (DefaultWorkerCount = 4)
│  │   #1     │ │   #2     │ │   #3     │ │   #4     │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘
│       │            │            │            │
│       ▼            ▼            ▼            ▼
│     processNext() — calls disco.DiscoverConfig() synchronously
│
│  disco = compositeDiscoverer (routes to Python or JMX bridge)
└──────────────────────────────────────┘
```

Each goroutine runs `processNext()`:
```go
resultJSON, err := w.disco.DiscoverConfig(key.integrationName, serviceJSON)
// blocks until DiscoverConfig returns
```

## How the Python Bridge Blocks Today

The Python bridge calls `python.DiscoverConfig()` which calls into the
Python rtloader's `discover_config()` C function. This **synchronously
runs the Python check** (`check.run()`) in-process. The goroutine blocks
until the check completes.

Typical Python probe duration:
- HTTP/openmetrics check: 100ms–2s (one HTTP request + metric parse)
- SNMP check: 1–5s (SNMP walk)
- Most checks: <1s

So today, each worker goroutine blocks for **~1s on average** during a
Python discovery probe. With 4 workers, the system can handle ~4
concurrent probes. Probes queue up in the workqueue with delay-based
retries.

## How the JMX Bridge Would Block (Approach 3)

The JMX bridge's `DiscoverConfig()`:
1. Registers a discovery request with a result channel
2. Blocks on `<-req.Result` (or timeout at 60s)
3. JMXFetch polls `/configs` (up to 15s later), sees the dummy config
4. JMXFetch runs the probe (connect + inspect + collect: ~5s)
5. JMXFetch posts `/status` with the result
6. Agent's `/status` handler completes the request
7. Bridge unblocks

Worst case: **20s per probe** (15s poll + 5s probe).
Best case: **5s per probe** (immediate poll + 5s probe).
Typical case: **10–15s per probe** (half poll interval + 5s probe).

## The Problem

If 4 JMX containers are discovered at agent startup:

```
Time  0s              15s             20s
      │                │               │
      ▼                ▼               ▼
      4 JMX probes enqueued            All 4 workers blocked
      All 4 workers start              waiting for JMXFetch
      blocking on result channel       to respond via /status
                                       │
                                       ▼
                                       Workers unblock at ~20s
                                       Python probes queued during 0–20s
                                       wait until a worker frees up
```

Any Python discovery probes enqueued during the 0–20s window would sit
in the workqueue, delayed by up to 20s. On a busy cluster with many
services starting simultaneously, this could significantly delay Python
integration discovery.

## Is This Actually a Problem in Practice?

**Arguments that it's acceptable:**

1. Discovery is a **one-time operation** per service. It doesn't repeat
   during normal operation — only at startup or when new containers appear.
2. JMX probes are **rare**. Most agents have 0–2 JMX integrations. Having
   4+ simultaneous JMX probes is uncommon.
3. The Python bridge also blocks. The difference is duration (~1s vs
   ~15s), not the blocking pattern itself.
4. The workqueue has retry with delay. Probes that can't be processed
   immediately just wait in the queue — they don't fail.
5. 20s delay at startup is not critical. Discovery is best-effort and
   eventually consistent.

**Arguments that it's NOT acceptable:**

1. On large clusters, agent startup can discover dozens of services
   simultaneously. If even 2 are JMX, that's 2 workers blocked for 20s,
   halving Python discovery throughput.
2. JMX applications (Kafka, Cassandra) are slow to start. The JMX endpoint
   may not be available for 30–60s after container start. Each probe
   attempt blocks for 20s, then retries after 10s, blocks again for 20s...
   A single JMX service can consume a worker goroutine for minutes.
3. The retry loop (5 attempts × 20s block + 10s delay = 120s per service)
   means a single slow JMX service blocks a worker for 2 minutes.
4. Fleet Automation status depends on timely discovery. Delays in Python
   discovery could confuse users.

## Conclusion: It IS a Problem

The retry loop is the killer. A JMX service that takes 60s to start its
JMX endpoint would cause:
- Attempt 1: blocks 20s (timeout/failure), retry after 10s
- Attempt 2: blocks 20s, retry after 10s
- Attempt 3: blocks 20s, retry after 10s
- Total: 90s of one worker blocked, with only 2 remaining for Python

With 2 slow JMX services, 2 workers are blocked for 90s each. Python
discovery throughput drops to 50% for 1.5 minutes.

## Solution: Separate Worker for JMX

### Design

Create **two** `Worker` instances:

```
                    discoveryState
┌─────────────────────────────────────────────────────┐
│                                                       │
│  pythonDiscoveryWorker          jmxDiscoveryWorker    │
│  ┌────────────────────┐         ┌────────────────────┐│
│  │ workqueue           │         │ workqueue           ││
│  │ 4 goroutines        │         │ 2 goroutines        ││
│  │ disco = PythonBridge│         │ disco = JmxBridge   ││
│  └─────────┬──────────┘         └─────────┬──────────┘│
│            │                               │           │
│            └───────────┬───────────────────┘           │
│                        ▼                               │
│                 cm.onDiscoveryResult()                 │
│                        │                               │
│                        ▼                               │
│                 cm.discoveredCh                        │
└─────────────────────────────────────────────────────────┘
```

- **Python worker**: 4 goroutines (default), Python bridge, default
  retry config (5 attempts, 10s delay)
- **JMX worker**: 2 goroutines, JMX bridge, JMX-tuned retry config
  (more attempts, longer delay to accommodate slow JVM startup)

### Routing

`scheduleDiscovery` checks if the template is a JMX config and routes
to the appropriate worker:

```go
func (cm *reconcilingConfigManager) scheduleDiscovery(svcID, tplDigest, integrationName string, tpl integration.Config) {
    if check.IsJMXConfig(tpl) {
        cm.jmxDiscoveryWorker.Enqueue(svcID, tplDigest, integrationName)
    } else {
        cm.pythonDiscoveryWorker.Enqueue(svcID, tplDigest, integrationName)
    }
}
```

The caller (`resolveTemplateForService`) already has the full template,
so passing it to `scheduleDiscovery` is a trivial signature change.

### JMX Worker Config

```go
jmxWorkerCfg := discoverer.Config{
    MaxAttempts: 10,        // JMX apps are slow to start, be patient
    RetryDelay:  15 * time.Second, // match JMXFetch poll interval
    Workers:     2,          // JMX probes are rare, 2 is enough
}
```

With 2 JMX workers, even if both are blocked for 90s, the 4 Python
workers are unaffected. Python discovery throughput stays at 100%.

### Changes Required

**`configmgr_discovery.go`** (python build):

```go
type discoveryState struct {
    pythonDiscoveryWorker *discoverer.Worker
    jmxDiscoveryWorker    *discoverer.Worker
    discoveredCh          chan integration.ConfigChanges
}

func initDiscoveryWorker(cm *reconcilingConfigManager, pythonDisco, jmxBridge discoverer.ConfigDiscoverer) {
    cm.discoveredCh = make(chan integration.ConfigChanges, discoveredChangesBuffer)
    cm.pythonDiscoveryWorker = discoverer.NewWorker(pythonDisco, cmServiceLookup{cm}, cm.onDiscoveryResult, discoverer.Config{}, cm.telemetryStore)
    cm.jmxDiscoveryWorker = discoverer.NewWorker(jmxBridge, cmServiceLookup{cm}, cm.onDiscoveryResult, discoverer.Config{
        MaxAttempts: 10,
        RetryDelay:  15 * time.Second,
        Workers:     2,
    }, cm.telemetryStore)
}

func (cm *reconcilingConfigManager) scheduleDiscovery(svcID, tplDigest, integrationName string, tpl integration.Config) {
    if check.IsJMXConfig(tpl) {
        cm.jmxDiscoveryWorker.Enqueue(svcID, tplDigest, integrationName)
    } else {
        cm.pythonDiscoveryWorker.Enqueue(svcID, tplDigest, integrationName)
    }
}

func (cm *reconcilingConfigManager) start() {
    cm.pythonDiscoveryWorker.Start()
    cm.jmxDiscoveryWorker.Start()
}

func (cm *reconcilingConfigManager) stop() {
    cm.pythonDiscoveryWorker.Stop()
    cm.jmxDiscoveryWorker.Stop()
}
```

**`configmgr_nodiscovery.go`** (non-python build):

```go
func initDiscoveryWorker(_ *reconcilingConfigManager, _ discoverer.ConfigDiscoverer, _ discoverer.ConfigDiscoverer) {}

func (cm *reconcilingConfigManager) scheduleDiscovery(_, _, _ string, _ integration.Config) {}
```

**`configmgr.go`**:

```go
func newReconcilingConfigManager(
    secretResolver secrets.Component,
    healthPlatform healthplatformdef.Component,
    staticConfigIndex *listeners.StaticConfigInfo,
    pythonDisco discoverer.ConfigDiscoverer,
    jmxBridge discoverer.ConfigDiscoverer,
    telStore *actelemetry.Store,
) configManager {
    // ...
    initDiscoveryWorker(cm, pythonDisco, jmxBridge)
    return cm
}
```

**`autoconfig.go`**:

```go
cfgMgr := newReconcilingConfigManager(
    secretResolver, hp, staticConfigIndex,
    discovererPkg.NewPythonBridge(),
    discovererPkg.NewJmxBridge(),
    telStore,
)
```

**`resolveTemplateForService`** in `configmgr.go`:

```go
if tpl.Discovery != nil {
    cm.scheduleDiscovery(svc.GetServiceID(), tpl.Digest(), tpl.Name, tpl)
    return tpl, false
}
```

### Composite Discoverer: No Longer Needed

With two workers each getting their own discoverer, the composite
discoverer is unnecessary. Routing happens at the worker level, not
inside the discoverer. The `composite.go` file can be removed.

### Impact on Existing Python Discovery

**Zero impact.** The Python worker is completely unchanged:
- Same `ConfigDiscoverer` (Python bridge)
- Same worker count (4)
- Same retry config (5 attempts, 10s delay)
- Same `onResult` callback

The only change to the Python path is that `scheduleDiscovery` now
takes an extra parameter (the template) and routes to the Python worker
explicitly instead of to a single shared worker.

### Impact on Non-Python Builds

The `configmgr_nodiscovery.go` stubs need updated signatures but
remain no-ops. The `jmx_bridge_nojmx.go` returns nil, which is fine —
`initDiscoveryWorker` receives nil for the JMX bridge and creates a
JMX worker with a nil discoverer. The worker's `processNext` would
call `nil.DiscoverConfig()` which panics — but `scheduleDiscovery` is
also a no-op in non-python builds, so nothing is ever enqueued.

Wait — that's a problem. In a `jmx` but non-`python` build, the JMX
worker would be created with a nil discoverer. If somehow a JMX
discovery probe were enqueued, it would panic.

**Fix**: In `configmgr_nodiscovery.go`, don't create the JMX worker
at all. Or: guard the JMX worker creation with a nil check.

Actually, looking more carefully: `configmgr_nodiscovery.go` has
`//go:build !python`. In non-python builds, `scheduleDiscovery` is a
no-op, so nothing is enqueued. The `initDiscoveryWorker` is also a
no-op. So no workers are created at all. This is fine.

But what about `jmx` + `python` builds? Both workers are created.
The JMX bridge is non-nil (from `jmx_bridge.go` with `//go:build jmx`).
This works.

What about `!jmx` + `python` builds? The JMX bridge is nil (from
`jmx_bridge_nojmx.go` with `//go:build !jmx`). The JMX worker would
be created with a nil discoverer. If a JMX integration template is
discovered, `scheduleDiscovery` would route it to the JMX worker,
which would call `nil.DiscoverConfig()` and panic.

**Fix**: In `scheduleDiscovery`, check if the JMX worker is nil before
enqueuing:

```go
func (cm *reconcilingConfigManager) scheduleDiscovery(svcID, tplDigest, integrationName string, tpl integration.Config) {
    if check.IsJMXConfig(tpl) {
        if cm.jmxDiscoveryWorker != nil {
            cm.jmxDiscoveryWorker.Enqueue(svcID, tplDigest, integrationName)
        } else {
            log.Debugf("JMX discovery bridge not available, skipping probe for %s", integrationName)
        }
    } else {
        cm.pythonDiscoveryWorker.Enqueue(svcID, tplDigest, integrationName)
    }
}
```

Or: in `initDiscoveryWorker`, only create the JMX worker if the bridge
is non-nil:

```go
func initDiscoveryWorker(cm *reconcilingConfigManager, pythonDisco, jmxBridge discoverer.ConfigDiscoverer) {
    cm.discoveredCh = make(chan integration.ConfigChanges, discoveredChangesBuffer)
    cm.pythonDiscoveryWorker = discoverer.NewWorker(pythonDisco, ...)
    if jmxBridge != nil {
        cm.jmxDiscoveryWorker = discoverer.NewWorker(jmxBridge, ...)
    }
}
```

Both approaches work. The nil check in `initDiscoveryWorker` is cleaner.

### Summary

| Aspect | Single Worker (original) | Separate Workers (this design) |
|---|---|---|
| JMX probes block Python | Yes (up to 90s per slow service) | **No** |
| Worker goroutines | 4 shared | 4 Python + 2 JMX = 6 total |
| Retry config | One size fits all | Tuned per bridge type |
| Composite discoverer | Needed | **Not needed** (removed) |
| New code | Minimal | Small refactor of discovery state |
| Risk | Low | Low (Python path unchanged) |
