---
slug: container-addremovesource-ordering
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# container-addremovesource-ordering — Container Launcher Source Lifecycle Has No Add-After-Remove Holes

## What Led to This Property

The SUT analysis §10 (wildcard) identifies "async `AddSource`/`RemoveSource`
in the container launcher (goroutines to avoid self-deadlock) creates ordering
holes: remove-before-add, or in-flight source goroutine after the launcher
believes it stopped." The concrete code is in
`pkg/logs/launchers/container/tailerfactory/tailers/source.go:34,42` — both
`Start()` and `Stop()` spawn fire-and-forget goroutines to call
`Sources.AddSource` and `Sources.RemoveSource`. This is a cross-lifecycle
coordination problem.

## Code Paths Involved

`pkg/logs/launchers/container/tailerfactory/tailers/source.go`:
```go
func (t *WrappedSource) Start() error {
    // spawned goroutine to avoid self-deadlock
    go t.Sources.AddSource(t.Source)
    return nil
}

func (t *WrappedSource) Stop() {
    // spawned goroutine to avoid self-deadlock
    go t.Sources.RemoveSource(t.Source)
}
```

The container launcher's `startSource` → `tailer.Start()` → `WrappedSource.Start()`
returns immediately. The spawned goroutine races with the `Stop()` path.

**Ordering hole scenario:**
1. Container A starts; `go AddSource(A)` is spawned but not yet scheduled.
2. Container A is immediately removed; `go RemoveSource(A)` is spawned.
3. `RemoveSource(A)` goroutine runs first — `LogSources.RemoveSource` sees A not
   yet in the list (since `AddSource` hasn't run) and is a no-op.
4. `AddSource(A)` goroutine runs — adds A to `LogSources`.
5. A is now permanently in `LogSources` with no corresponding `RemoveSource`
   completing → the file launcher picks up source A and starts tailing, even
   though container A is gone.

**Post-stop goroutine scenario:**
1. `Launcher.Stop()` is called; `addedSourcesDone` is closed.
2. A `WrappedSource.Start()` goroutine that was blocked on `AddSource` (because
   `Services.AddService` was slow) now completes — it adds a source to
   `LogSources` *after* the launcher has already stopped processing additions.
3. The file launcher has already stopped; the new source is never tailed.
4. Alternatively: the goroutine calls `LogSources.AddSource`, which has a
   subscriber (the stopped file launcher's `addedSources` channel). That channel
   is closed (`addedSourcesDone` is closed), so the `select { case stream.ch <-
   source: case <-stream.done: }` arm fires `<-stream.done` — the source is
   silently dropped.

## Why This Matters

A fire-and-forget goroutine that races with component shutdown creates a
permanently orphaned source or a permanently-absent source, both silently
incorrect. The user's container logs either: (a) are still tailed after the
container exits (resource leak, stale data); or (b) are never tailed for the
new container (silent log loss during rapid container churn).

Under Antithesis, container churn at high rates + CPU pause faults at the
goroutine scheduling point make this reachable deterministically.

## Triggering Scenario

1. Workload: create and immediately destroy 10 containers in rapid succession.
2. Antithesis pauses the `AddSource` goroutine for each container at various
   points (before calling `AddSource`, after acquiring the `LogSources` lock,
   etc.).
3. Observe: after all containers are destroyed, does `LogSources` contain zero
   active sources of these container types? If `len(GetSources()) > 0` after
   all removes complete → ordering hole confirmed.
4. Observe: does the file launcher attempt to tail a file for a container that
   is already gone?

## SUT-Side Instrumentation (all missing)

- `Sometimes("wrapedsource-start-goroutine-ran-after-stop")` — inside the
  `go t.Sources.AddSource(t.Source)` goroutine, after `AddSource` returns,
  check whether the `Launcher` is already stopped. This requires exposing
  launcher state or a global flag.
- `Unreachable("source-added-after-launcher-stopped")` — in
  `LogSources.AddSource`, after notifying subscribers, verify that no stopped
  subscriber received a source without being able to act on it (hard to assert
  directly; proxy via a counter).
- Workload-level: after all containers are stopped, assert `GetSources()` for
  container types is empty.

## Open Questions

- The comment in `source.go:27-33` says "the long-term fix is that launchers
  should not be adding sources." Has this fix been prioritized? If the design
  is expected to change, the property may be transient. `(needs human input)`

### Investigation Log

#### Does `LogSources.AddSource` do anything harmful if called after the subscriber's `done` channel is closed?

- Examined: `pkg/logs/sources/sources.go:53-78`.
- Found: `LogSources.AddSource` uses `select { case stream.ch <- source: case <-stream.done: }`.
  If `stream.done` is closed, the `case <-stream.done:` arm fires, the source
  is silently dropped (no panic, no error log). The source is still appended to
  `s.sources` before the lock is released (line 56), so it persists in the
  `LogSources` state but never reaches the stopped subscriber.
- Conclusion: post-stop goroutine results in a dropped source notification (log
  loss / silent miss), not a panic. The property's correctness concern is
  confirmed real; the safety concern (crash) is not present. The source remains
  in `LogSources.sources` permanently, which could cause stale-source effects
  for any future subscriber calling `GetSources()`.

#### Is there a mechanism to track in-flight `AddSource`/`RemoveSource` goroutines?

- Examined: `pkg/logs/launchers/container/tailerfactory/tailers/source.go:26-43`.
- Found: none. Both `Start()` and `Stop()` spawn goroutines with `go` and return
  immediately. No `sync.WaitGroup`, no channel, no counter tracks these goroutines.
- Not found: any join mechanism in `WrappedSource` or its callers.
- Conclusion: confirmed fire-and-forget with no join. The ordering-hole and
  post-stop scenarios described in the property body remain valid bugs.
</content>
