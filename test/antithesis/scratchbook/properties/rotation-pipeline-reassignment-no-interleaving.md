---
slug: rotation-pipeline-reassignment-no-interleaving
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# rotation-pipeline-reassignment-no-interleaving — No Pre/Post-Rotation Line Interleaving Across Pipeline Reassignment

## What Led to This Property

`sut-analysis.md` §4 (S1) and §10 identify a compound hazard: on file rotation,
the new tailer is assigned to a *different pipeline* than the draining old tailer
(via `launcher.go:663` calling `NextPipelineChan()` round-robin). Both tailers
are simultaneously active during the drain window. Their auditor acks arrive from
different pipelines, with independent `IngestionTimestamp` values. The auditor
uses `IngestionTimestamp` to guard against stale writes (`auditor.go:387`); if
the old tailer's IngestionTimestamp is higher (because it was created first and
has been running longer), the new tailer's acks may be silently discarded.

No existing property covers the *triple* of rotation + pipeline reassignment +
offset interleaving. `per-source-ordering-preserved` covers single-pipeline
ordering; `container-identifier-no-collision` covers tailer ID collision; this
property covers the mid-rotation ordering guarantee across the pipeline boundary.

## Code Paths Involved

**`pkg/logs/launchers/file/launcher.go:663-674`** — `createRotatedTailer()`:

```go
func (s *Launcher) createRotatedTailer(t *tailer.Tailer, file *tailer.File,
    pattern *regexp.Regexp, fingerprint *types.Fingerprint) *tailer.Tailer {
    tailerInfo := t.GetInfo()
    channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()
    // ^ round-robin: new pipeline, not the same as the old tailer's pipeline
    ...
    newTailer := t.NewRotatedTailer(file, channel, monitor, ...)
    return newTailer
}
```

The old tailer continues draining into its original pipeline until `closeTimeout`
(60 seconds). The new tailer starts on a new pipeline immediately. Their outputs
are processed concurrently by two different pipeline goroutine chains.

**`comp/logs/auditor/impl/auditor.go:384-398`** — `updateRegistry()` guard:

```go
func (a *registryAuditor) updateRegistry(identifier, offset, tailingMode string,
    ingestionTimestamp int64, fingerprint types.Fingerprint) {
    ...
    if v, ok := a.registry[identifier]; ok {
        if v.IngestionTimestamp > ingestionTimestamp {
            return  // stale-timestamp guard: new ack discarded
        }
    }
    a.registry[identifier] = &RegistryEntry{
        Offset:             offset,
        IngestionTimestamp: ingestionTimestamp,
        ...
    }
}
```

The stale-timestamp guard is intended to prevent dual-shipping from advancing
the offset backward. But during rotation, both the old and new tailers share the
same `identifier` (the file path) if fingerprinting is not used. If the old
tailer's IngestionTimestamp is larger (it was created earlier and its messages
have higher timestamps), the new tailer's acks are dropped by this guard.

**Old tailer's drain path:**

The old tailer drains into its pipeline for up to 60 seconds (`closeTimeout`
in `tailer.go`). During this window, both pipelines are writing acks for the same
tailer identifier. The auditor processes acks from its single input channel
(100-deep), but the two pipelines' acks can be interleaved in any order.

## What Goes Wrong

Two distinct failure modes:

1. **Post-rotation lines arrive before pre-rotation lines at intake.** The new
   tailer (faster pipeline, no backpressure) races ahead; intake receives
   post-rotation messages out of order relative to pre-rotation messages still
   draining in the old pipeline. This is a correctness violation from the
   perspective of the operator reading the log stream.

2. **Offset regression.** The stale-timestamp guard fires in reverse: the new
   tailer's IngestionTimestamp is lower (it was created after the old tailer,
   but `time.Now().UnixNano()` is called at decode time, not at tailer creation
   time — so the new tailer's early messages may have earlier timestamps than
   the old tailer's late messages). If the old tailer's high-timestamp ack fires
   last and wins, the registry stores the *old* tailer's pre-rotation offset
   indefinitely, even after the new tailer has advanced further. On restart, the
   agent re-reads pre-rotation lines.

## Assertion Design

**`Always`** (workload-side): The workload writes N lines to a file, triggers
rotation (rename + new file), then writes M post-rotation lines. At intake
(fakeintake), assert one of two conditions holds:

- **Strong form:** All N pre-rotation lines appear in intake before any of the M
  post-rotation lines for this logical source (in-order delivery).
- **Weak form (offset safety):** No post-rotation offset committed to the registry
  is less than the last pre-rotation offset (no offset regression after rotation).

The weak form is more tractable because it can be checked via the agent's status
endpoint or `registry.json` without requiring intake-side sequencing.

**`Reachable`** (SUT-side): At the point where `createRotatedTailer` assigns a
pipeline different from the old tailer's pipeline, add:

```
antithesis.Reachable(
    "rotation-pipeline-reassignment-no-interleaving: rotation assigned new pipeline",
    map[string]any{
        "old_pipeline": oldPipelineID,
        "new_pipeline": newPipelineID,
        "tailer_id":    tailer.Identifier(),
    },
)
```

This confirms Antithesis explored a rotation event with a cross-pipeline
reassignment, making the interleaving window active.

**`AlwaysOrUnreachable`** (SUT-side, at auditor stale-guard): When
`updateRegistry` discards a new ack because `v.IngestionTimestamp > ingestionTimestamp`,
assert the discarded offset is not *newer* than the stored one:

```
antithesis.AlwaysOrUnreachable(
    "rotation-pipeline-reassignment-no-interleaving: stale-guard discard is safe",
    discardedOffset <= storedOffset,   // no regression: we're keeping a later offset
    map[string]any{...},
)
```

## Why It Matters

File rotation is the dominant production input scenario. Every log rotation
(logrotate, Docker log driver rotation, application-level rotation) triggers this
code path. Under Antithesis CPU throttling, the drain window can be extended
arbitrarily, widening the interleaving window. Under network partition (slow
intake), the old pipeline may back up while the new one flows freely, creating
large out-of-order windows at intake. This is a real production risk with no
existing property covering it.

## Relationship to Other Properties

- `per-source-ordering-preserved` — covers ordering within a single pipeline for
  a single source; this property covers the cross-pipeline ordering guarantee
  that breaks at rotation boundaries.
- `container-identifier-no-collision` — covers the tailer identifier collision
  hazard that shares the same root cause (rotation + same registry key). If
  identifiers collide, the offset-regression risk in this property becomes acute.
- `auditor-offset-safety` — covers the general auditor correctness invariant; this
  property covers the specific interleaving scenario at rotation.

## Open Questions

- Does `NewRotatedTailer` ever assign the *same* pipeline as the old tailer?
  If pipelining count is 1, both old and new tailers would be on the same pipeline,
  and the interleaving scenario does not apply. `(partial: with 1 pipeline,
  NextPipelineChan always returns the same channel; the race only exists with
  pipelines >= 2)`
- Does the `IngestionTimestamp` at decode time (message creation in `tail()`)
  or at send time (when the auditor ack is produced) determine the guard
  behavior? `(partial: timestamp is set at message creation in tailer tail()
  via time.Now().UnixNano(); it reflects when the byte was read, not when the
  ack was processed)`
- What is `closeTimeout`? Is it configurable or hardcoded at 60s?
  `(partial: confirmed 60s from tailer.go; needs verification it's not reduced
  under backpressure)`

### Investigation Log

#### Does createRotatedTailer always assign a different pipeline than the old tailer?

- Examined: `pkg/logs/launchers/file/launcher.go:663-674` (`createRotatedTailer`), `comp/logs-library/pipeline/provider.go` (`NextPipelineChanWithMonitor`), `sut-analysis.md §4` (S1 note on rotation).
- Found: `NextPipelineChanWithMonitor` does a modular round-robin: `index = (index + 1) % numPipelines`. If `numPipelines == 1`, the same channel is always returned. If `numPipelines >= 2`, the new tailer gets a different pipeline. The sut-analysis §4 states "new tailer round-robins to a different pipeline than the draining old tailer" — this is conditional on `numPipelines >= 2`, which is the default (sized for CPU parallelism).
- Not found: a code path that explicitly pins the rotated tailer to the same pipeline.
- Conclusion: the race exists for default configurations with multiple pipelines. Single-pipeline deployments are safe from this specific hazard.

#### Is the IngestionTimestamp guard the root cause of the offset-regression scenario?

- Examined: `comp/logs/auditor/impl/auditor.go:374-398` (`updateRegistry`), `comp/logs/auditor/impl/auditor.go:284-330` (run loop, ack processing).
- Found: The guard `if v.IngestionTimestamp > ingestionTimestamp { return }` is intended for dual-shipping. During rotation, both tailers share the same identifier (pre-fingerprinting). If the old tailer's messages (with higher IngestionTimestamp, created earlier in wall time) are processed by the auditor after the new tailer's acks, the new tailer's offset progress is lost. This is the regression scenario.
- Not found: any comment or test covering this specific rotation interaction with the stale-guard.
- Conclusion: the regression scenario is real. It is an unintended side effect of the dual-shipping stale-guard firing during rotation.
