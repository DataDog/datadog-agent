---
slug: container-identifier-no-collision
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# container-identifier-no-collision — Container Rotation Does Not Cause Auditor Offset Regression

## What Led to This Property

SUT unproven assumption #6: `Tailer.Identifier()` can return the same string for
two different tailers during container rotation. This is documented in a FIXME
at `pkg/logs/tailers/file/tailer.go:260`. When two tailers share a registry
key, the auditor uses `IngestionTimestamp` to resolve conflicts — but a stale
timestamp can allow a new (wrong-position) tailer to overwrite the correct
offset, causing re-tailing from an earlier position.

## Code Paths Involved

**The FIXME** — `pkg/logs/tailers/file/tailer.go:258-267`:
```go
// FIXME(remy): during container rotation, this Identifier() method could return
// the same value for different tailers. It is happening during container rotation
// where the dead container still has a tailer running on the log file, and the tailer
// of the freshly spawned container starts tailing this file as well.
//
// This is the identifier used in the registry, so changing it will invalidate existing
// registry entries on upgrade.
func (t *Tailer) Identifier() string {
    return t.file.Identifier()
}
```

**Identifier computation** — `pkg/logs/tailers/file/file.go` (Identifier method).
For file tailers, the identifier is typically `"file:"+path` or similar. During
container rotation, the new container may log to the same file path → same
identifier.

**Conflict resolution** — `auditor.go:385-389`:
```go
if v, ok := a.registry[identifier]; ok {
    if v.IngestionTimestamp > ingestionTimestamp {
        return
    }
}
```
Only rejects updates if the stored timestamp is *strictly greater*. If both
tailers have similar timestamps (within clock resolution), the later-arriving
update wins — which may be the *old* tailer's offset if the messages arrive
out of order due to scheduling.

**Container churn scenario:**
During high container churn (rapid start/stop cycles), the probability of
identifier collision per unit time increases. Each collision creates a window
where the auditor offset can revert to the older tailer's position → re-tail
from a stale offset → duplicate storm.

## Failure Scenario

1. Container A logs to `/var/log/containers/app_default_container-A-<id>.log`.
2. Container A stops. Old tailer (identifier=`file:/var/log/.../container-A-<id>.log`)
   is still draining.
3. Container B starts with the same log path (same name, different container ID)
   → identifier may collide with container A's if the ID is not included in the
   path or if the file is renamed.
4. New tailer starts at offset 0 (no registry entry for the new path/identifier).
5. Both tailers send messages to the auditor. The old tailer's messages (with
   higher offsets) compete with the new tailer's messages (starting from 0).
6. If the new tailer's messages arrive at the auditor *after* the old tailer's
   messages but with a higher `IngestionTimestamp`, the auditor reverts the
   offset to something small.
7. After the old tailer stops, the new tailer re-reads from the stale offset.
8. Duplicate delivery of all lines between offset 0 and the correct position.

Antithesis CPU faults can induce the scheduling interleaving that makes step 6
more likely.

## Why It Matters

Container environments have high churn rates. In Kubernetes, pods are frequently
restarted. Each collision causes a duplicate storm proportional to the file size
at the time of collision. In large log files (e.g., a pod that has been running
for hours), this can mean gigabytes of duplicate data.

## Workload Instrumentation

- Workload rapidly creates and destroys containers in a cycle.
- The agent tails each container's log file.
- After each cycle, the fakeintake checks that log lines from each container
  (identified by embedded container-ID in the log content) are not duplicated.
- The auditor registry file is inspected directly: assert that each identifier
  has a monotonically non-decreasing offset over time.
- SUT-side: a `Sometimes` assertion at `updateRegistry()` confirming that the
  new offset is >= the current stored offset (monotonicity invariant) — currently
  **missing**.

## Open Questions

- Under what conditions do two different containers share the same file path and
  thus the same tailer identifier? The FIXME confirms it's a real scenario —
  which runtime/config does it occur in? `(needs human input)`
- Does the current test topology use container-based log sources, or only file
  sources? If only file sources, this property may not be exercisable in the
  planned topology. `(needs human input)`
- Is there a mechanism to distinguish old-tailer payloads from new-tailer payloads
  in the auditor? Currently `msg.Origin.Identifier` is the same for both — the
  auditor cannot distinguish them.
- Does the `rotatedTailers` cleanup path (`scan()` draining `s.rotatedTailers`)
  guarantee T1 completes before T2's registry writes? `(partial: cleanUpRotatedTailers() checks IsFinished() — no ordering guarantee with registry writes)`

## Merged-in evidence (from rotation-no-identifier-collision)

The secondary file focused specifically on the **file-rotation variant** of the
identifier collision and provided additional code-level detail and scenario
analysis not present in the canonical:

**Exact race mechanism** — in `restartTailerAfterFileRotation` (`launcher.go:608-632`):
```go
oldTailer.StopAfterFileRotation()  // starts a 60s-timeout goroutine
s.tailers.Remove(oldTailer)
newTailer := s.createRotatedTailer(oldTailer, file, ...)
err := newTailer.StartFromBeginning()  // both tailers now live
s.rotatedTailers = append(s.rotatedTailers, oldTailer)
s.tailers.Add(newTailer)
```
After `newTailer.StartFromBeginning()`, both tailers call
`updateRegistry(identifier, offset)`. The auditor processes these serially (run()
goroutine), but the ordering is arrival-order on `inputChan` — **last writer wins**.

**The two bad outcomes** depending on which tailer's payload arrives last at the
auditor:
- oldTailer's final payload arrives *after* newTailer's first → registry stores
  oldTailer's large (post-drain) offset → on next restart, newTailer seeks to
  that offset in the *new* file → if new file is smaller, seek goes to EOF →
  logs from bytes 0–(old offset) of new file are lost.
- newTailer's offsets arrive after oldTailer finishes → registry is correct.
  The collision is racy and order-dependent.

**Cross-pipeline compound failure** — `createRotatedTailer` calls
`NextPipelineChanWithMonitor()` which round-robins to the *next* pipeline.
During the overlap window, oldTailer and newTailer are on different pipelines.
Under CPU throttle, oldTailer's drain slows and its final payload arrives at the
auditor after newTailer has committed a correct offset — overwriting it with a
stale value.

**Additional assertion (from secondary):**
```go
// In auditor.updateRegistry, if an offset goes backward:
assert.AlwaysOrUnreachable(
    newOffset >= existingOffset || tailerIsRotating,
    "registry offset must not regress unless a rotation is in progress",
    map[string]any{"identifier": identifier, "newOffset": newOffset,
                   "existingOffset": existingOffset})
```
Also: workload-side check — if any identifier appears twice in the active tailer
set simultaneously, flag it.

### Investigation Log

#### Does `use_fingerprint=true` change `file.Identifier()` so rotated tailers get distinct registry keys?

- Examined: `pkg/logs/tailers/file/file.go:49-51` (`Identifier()`), `pkg/logs/tailers/file/tailer.go:258-267`, `pkg/logs/launchers/file/position.go:24-75`, `pkg/logs/tailers/file/fingerprint.go`.
- Found: `file.Identifier()` unconditionally returns `"file:" + t.Path`. Fingerprint data is stored separately in `RegistryEntry.Fingerprint` and is used in `position.go` to detect rotation (fingerprints-don't-align → start from beginning), but it does NOT change the registry key. Two tailers for the same path will always share the same identifier regardless of fingerprint configuration.
- Conclusion: fingerprinting does NOT prevent identifier collision. Removed from Open Questions.

#### Is the `IngestionTimestamp` resolution sufficient to distinguish two concurrent tailers?

- Examined: `comp/logs/auditor/impl/auditor.go:384-398` (`updateRegistry`), `pkg/logs/message/message.go` (IngestionTimestamp type).
- Found: `IngestionTimestamp` is `int64` nanoseconds (set at decode time). For two tailers running concurrently, messages decoded in the same nanosecond could have equal timestamps — the guard `v.IngestionTimestamp > ingestionTimestamp` uses strict greater-than, so equal timestamps let the update through (last-write-wins). Under normal load (microsecond-scale decode latency), nanosecond timestamps rarely collide, but under CPU throttle any ordering becomes possible.
- Conclusion: timestamp resolution is nanoseconds; collision is theoretically possible under throttle but rare in practice. The guard does not prevent same-timestamp races. Tagged remaining question about ordering as `(partial)`.

#### Does `cleanUpRotatedTailers()` guarantee old tailer completes before new tailer's registry writes?

- Examined: `pkg/logs/launchers/file/launcher.go:365-374` (`cleanUpRotatedTailers`), `tailer.go:388-389` (`IsFinished`), stop sequence.
- Found: `cleanUpRotatedTailers()` removes tailers where `IsFinished() == true`. `IsFinished()` is set to true after `forwardMessages()` returns (all decoded messages sent to outputChan). However, the auditor processes messages from `inputChan` asynchronously — a tailer can be `IsFinished()` before its last messages reach the auditor. So there is no guarantee that old-tailer registry writes complete before the new tailer starts writing.
- Conclusion: no ordering guarantee confirmed. Tagged `(partial)` in Open Questions.
