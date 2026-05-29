# Wildcard / Cross-Cutting Discovery — Logs Pipeline SUT

**Scope examined:** `pkg/logs/` and `comp/logs/` at commit 8ff8f30e10b  
**Method:** Systematic code reading, comment/FIXME mining, cross-component tracing

---

## 1. Pipeline Reassignment on Rotation Breaks Ordering

**File:** `pkg/logs/launchers/file/launcher.go:663-675` (`createRotatedTailer`),
`pkg/logs/launchers/file/launcher.go:608-631` (`restartTailerAfterFileRotation`)

On file rotation (non-fingerprint path), `restartTailerAfterFileRotation` calls
`pipelineProvider.NextPipelineChanWithMonitor()` for the **new** tailer, which does a
round-robin across all pipelines. The **old** (draining) tailer keeps its original channel.

Both tailers are concurrently active, possibly writing to **different pipelines**. The
old tailer drains the pre-rotation content; the new tailer starts from the beginning of
the new file. There is no synchronization or sequencing guarantee across these two
pipelines. Logs from the same logical file can arrive at the intake interleaved or
out of order.

This is structurally acknowledged as a limitation (`rotatedTailers` list), but the
ordering property ("file A's logs arrive in file order") is silently broken across
rotation boundaries whenever the round-robin lands on a different pipeline.

For the fingerprint-based rotation path (`rotateTailerWithoutRestart`, line 582), the
pattern is identical — a new channel is fetched — so the same ordering issue applies.

**Why significant:** The documented guarantee is in-order delivery *within* a pipeline.
Rotation breaks the per-file identity assumption by introducing a second pipeline.

---

## 2. Known-But-Unfixed Identifier Collision on Container Rotation

**File:** `pkg/logs/tailers/file/tailer.go:259-268`

```go
// FIXME(remy): during container rotation, this Identifier() method could return
// the same value for different tailers. It is happening during container rotation
// where the dead container still has a tailer running on the log file, and the tailer
// of the freshly spawned container starts tailing this file as well.
```

`Identifier()` returns `"file:" + path` (`file.go:49-51`). For containers sharing a
log path (same pod, container restart), two active tailers emit the same registry
identifier. The auditor's `updateRegistry` (auditor.go:374) does timestamp-based
de-dup to handle dual shipping, but **does not account for two concurrent tailers**
sharing an identifier — whichever sends acks later wins, potentially reverting the
registered offset backwards and causing re-reads or missed reads on next restart.

**Why significant:** This is a documented known-unfixed bug. In Antithesis, container
churn is exactly what fault injection would cause, making this code path extremely hot.

---

## 3. FilesToTail Runs Concurrently with addSource — Mitigation Is Incomplete

**File:** `pkg/logs/launchers/file/launcher.go:173`, `218-229`, `448`

The file provider's `FilesToTail` is always launched in a goroutine (line 176-178),
running concurrently with the main `run()` loop. Sources added during the scan go to
`filesTailedBetweenScans`. However:

1. The slice is **cleared** at scan start (line 173) and **merged** at scan completion
   (line 228). If an `addSource` arrives after the goroutine launches but before the
   scan result arrives, the source appears in neither the snapshot passed to
   `FilesToTail` nor `filesTailedBetweenScans` — the comment calls this out
   explicitly: "it is possible that addSource() can be called while FilesToTail() is
   still running."
2. The mitigation (`filesTailedBetweenScans`) only helps if the tailer was *started*
   during the concurrent scan. If `launchTailers` finds it is at the limit and returns
   early (line 396), the file is not added to `filesTailedBetweenScans` and will be
   dropped from the next `resolveActiveTailers` pass, stopping the tailer mid-scan.

**Why significant:** Under high churn (container starts with fault injection), this
creates a window where a new source causes an existing tailer to be incorrectly
stopped and restarted.

---

## 4. ShouldIgnore Walks /var/log/containers on Every Selected File, Every Scan

**File:** `pkg/logs/launchers/file/provider/file_provider.go:406-473`

When `validate_pod_container_id` is enabled, `ShouldIgnore` calls
`filepath.WalkDir(ContainersLogsDir, ...)` for **every file evaluated**. With N files
and M scans per scan period, this is N*M `WalkDir` calls against the container symlink
directory. No caching is implemented.

Under load (many container files + slow filesystem) this makes the scan goroutine slow.
A slow scan goroutine means the launcher stays in the `scanTicker.C` → `filesChan`
cycle for longer, during which any `addSource` events are buffered. Combined with #3
above, this amplifies the window for race conditions.

---

## 5. Auditor Flush Races with Stop — Ordering Not Guaranteed

**File:** `comp/logs/auditor/impl/auditor.go:313-331` (`run()` flush path), `146-161` (`Flush()`)

The `Flush()` method snapshots `len(a.inputChan)` and drains that many entries, then
flushes to disk. But between `len()` and the drain loop, new payloads can arrive from
concurrent tailers. These newly arrived payloads are not included in the flush.

More importantly: on `Stop()` (line 132-138), the code calls `closeChannels()`, which
closes `inputChan`, waits for the run goroutine to exit via `<-done`, then calls
`cleanupRegistry()` and `flushRegistry()`. The run loop exits on channel close but only
after processing the channel close — it does **not** drain buffered payloads first.
`close(a.inputChan)` causes the run loop to return on the next `payload, isOpen :=
<-a.inputChan` with `isOpen = false`. Any messages already in the buffered channel at
close time are abandoned.

**Result:** On agent shutdown or restart under pressure, the last batch of acked
messages may not be persisted. At-most-once: logs are re-read on restart.

---

## 6. Wildcard File Ordering Assumes Glob Returns Lexicographic Order — Documented But Unfixed

**File:** `pkg/logs/launchers/file/provider/file_provider.go:362-381`

```go
// FIXME - this codepath assumes that the 'paths' will arrive in lexicographical order
// This is true in the current go implementation, but it is unsafe to assume
```

The `applyReverseLexicographicalOrdering` function reverses the slice (assuming it is
sorted) then re-sorts by filename descending. If `filepath.Glob` ever returns results
in non-lexicographic order (e.g., on a filesystem that doesn't sort directory entries),
the "reverse then sort" heuristic produces wrong priority ordering.

Also: when `recursiveGlobEnabled` and `**` is in the pattern, `doublestar.FilepathGlob`
is used. This third-party library makes no documented sorting guarantee, so the FIXME
applies doubly. The `doublestar` implementation does actually sort, but this is an
undocumented implementation detail.

**Severity:** Wrong wildcard priority → wrong files chosen when at the `filesLimit` cap.

---

## 7. KeepAlive Called for Files Beyond the Open-File Limit

**File:** `pkg/logs/launchers/file/provider/file_provider.go:136-154`

`registry.KeepAlive(file.Identifier())` is called **before** the `filesLimit` check.
This means that even files that are not tailed (because the limit is reached) have their
registry entries kept alive on every scan.

Semantically this is intentional (comment says so). But the effect is that the registry
can accumulate entries for every file matching a wildcard, including those never tailed.
This is a subtle behaviour: the registry grows unboundedly with wildcard patterns that
match many files, even if most are never tailed, and cleanup only happens after the TTL
with no active KeepAlive. Under a churn scenario (files appear/disappear frequently),
the registry may transiently hold many stale entries.

---

## 8. IngestionTimestamp Is Set at Read Time (First 4096 Bytes) — Multi-Line Aggregation Shifts Effective Timestamp

**Files:** `pkg/logs/internal/decoder/decoder.go:31`, `pkg/logs/tailers/file/tailer_nix.go:52-65`

`NewInput` sets `IngestionTimestamp = time.Now().UnixNano()` at the moment the 4096-byte
read buffer is passed to the decoder (tailer_nix.go:64). For multi-line aggregated
messages, the framer carries the first buffer's timestamp through
(framer.go:284), so a large multi-line message (e.g., a Java stack trace spanning
many reads) gets the timestamp of the *first* buffer, not the last line read.

Meanwhile, the auditor's `updateRegistry` de-dups on `IngestionTimestamp` to handle
dual shipping (auditor.go:387: `if v.IngestionTimestamp > ingestionTimestamp: return`).
If two concurrent tailers emit messages with the same identifier (the container rotation
case, #2 above), an older tailer sending acks could be rejected by the newer tailer's
earlier timestamp, silently ignoring the ack. The de-dup logic is timestamp-based, but
the timestamps come from wall-clock reads at different concurrency levels across
goroutines.

---

## 9. Pipeline Failover Destroys Per-Source Ordering

**File:** `comp/logs-library/pipeline/provider.go:353-388` (`forwardWithFailover`)

When `logs_config.pipeline_failover.enabled = true`, `forwardWithFailover` tries the
primary pipeline non-blocking, then falls back to any non-blocking pipeline. Multiple
consecutive messages from the same source/tailer (same router channel) are thus
potentially routed to **different pipelines** under load — pipeline[0] gets message M1,
pipeline[2] gets M2 because pipeline[0] was full for M2.

The per-pipeline ordering guarantee (messages within one pipeline are in order) no
longer implies per-source ordering. For a chatty file tailer hitting backpressure
intermittently, some messages go to pipeline A and others to pipeline B, which may
process, batch, and transmit at different rates. The intake may receive M2 before M1.

This is the biggest cross-cutting finding: failover and per-source ordering are
mutually exclusive, but this is not documented or guarded.

---

## 10. Container Launcher Creates Child Sources Asynchronously — Source/Service Reconciliation Is Racy

**File:** `pkg/logs/launchers/container/tailerfactory/tailers/source.go:26-43`

`WrappedSource.Start()` spawns a goroutine to call `sources.AddSource()` (line 34) to
avoid a deadlock described in the comment ("if we send this synchronously, it causes a
deadlock because the added source is delivered to the container launcher"). The
`Stop()` method similarly does `go t.Sources.RemoveSource(t.Source)`.

The acknowledged workaround is: launchers should not be adding sources. But the fix
path (structural refactor) is deferred. In the meantime:

- The add goroutine races with any concurrent container lifecycle event. If the
  container dies before the goroutine fires, a `RemoveSource` from the death event may
  arrive before `AddSource`. The file launcher then gets an `addSource` for a dead
  container, tails the file, and is never told to stop it.
- The `Stop()` `RemoveSource` goroutine similarly has no sequencing guarantee with
  respect to the main `Stop()` call's completion. The container launcher may declare
  itself stopped while a child source goroutine is still dispatching.

The `README.md:85` for `pkg/logs/` documents: "This functionality has some inherent
race conditions."

---

## 11. Auto-Multiline Flush Timeout Is Shared Config — Cannot Be Per-Source

**File:** `pkg/logs/internal/decoder/decoder.go:273` (`buildLineHandler`),
`comp/logs/agent/config/config.go:512`

The aggregation flush timeout (`logs_config.aggregation_timeout`, default 1000ms) is
read once from global config and applied to all line handlers. High-throughput sources
and slow/verbose sources share this timeout, meaning a slow source that is flushed
every second creates a 1-second latency guarantee even when the downstream is ready.

More subtly: the flush timeout is implemented by a `time.Ticker` inside the decoder's
run goroutine. If the main select loop is busy processing a high volume of data, the
flush ticker can starve — multi-line messages may be held longer than the timeout in
practice.

---

## 12. Rotation Detection on Windows Is Purely Size-Based — Race Window for Mis-Classification

**File:** `pkg/logs/tailers/file/rotate_windows.go:34-36`

The comment acknowledges: "It is important to gather these values in this order, as
both the file size and read offset may be changing concurrently. However, the offset
increases monotonically, and increments occur *after* the file size has increased."

This ordering assumption may not hold under a hypervisor or OS that reorders memory
operations differently from the Go memory model's sequential consistency for ordinary
reads. More practically: on Windows, log rotation via truncation and file size < offset
is the only signal. A file being truncated while being read may transiently show size
< lastReadOffset, causing a spurious rotation detection that resets the tailer to the
beginning and re-reads already-processed content.

---

## 13. Adaptive Sampler's `isImportant()` Protection Is Keyword-Based, Not Structurally Correct

**File:** `pkg/logs/internal/decoder/preprocessor/sampler.go:139-149`

The `isImportant` function matches tokens including `Warn`, `Exception`, `Timeout`. A
log message that contains the *word* "timeout" as a field name or value in a JSON
payload may match a token that the tokenizer assigns `Timeout`, causing it to bypass
adaptive sampling even if it is a high-frequency debug log.

Conversely, a FATAL log whose severity keyword is deeply nested in a multi-line message
or uses a non-standard capitalization not recognized by the tokenizer would be subject
to rate limiting and potentially dropped.

---

## 14. `newInput` Buffer Allocates 4096 bytes on Every Read — Throughput Bottleneck Hidden in Hot Path

**File:** `pkg/logs/tailers/file/tailer_nix.go:52`

```go
inBuf := make([]byte, 4096)
```

A new 4096-byte buffer is allocated on every call to `read()`. This is called in a
tight loop (`readForever`), polling at `sleepDuration` intervals when no data, and
potentially many times per second when data flows. For high-throughput log sources,
this creates GC pressure. No pooling is used (note: user has documented preference
against `sync.Pool`; recorded here as observation, not a suggestion).

---

## 15. KeepAlive Does Not Create Registry Entries — Silent Miss for Files Never Processed

**File:** `comp/logs/auditor/impl/auditor.go:227-233`

`KeepAlive` only updates `LastUpdated` if the key already exists in the registry
(line 230: `if _, ok := a.registry[identifier]; ok {`). For files at the open-file
limit that have never previously been processed (not in registry), `KeepAlive` is a
no-op. When these files become the first in the priority order (e.g., all higher-priority
files disappear), they have no registry entry. `Position()` returns end-of-file (default
for unknown files), so historical log content is skipped.

This is the expected behavior for "tail from end" on new files, but creates a subtle
interaction: a file that was briefly over the limit but had content can be tailed from
the beginning if `start_position: beginning` is set... but only if fingerprints match
or no fingerprint. If the file is a rolling log that was partially beyond the limit, the
agent picks it up from the end on promotion. No warning is emitted.

---

## 16. The Greedy Selection Strategy + Source Ordering Creates Source Starvation

**File:** `pkg/logs/launchers/file/provider/file_provider.go:73-79`, `227-248`

In `greedySelection` mode (the default when `by_name` strategy is selected), sources
are processed in the order they were added to `activeSources`. The first source to
match many wildcard files can consume all `filesLimit` slots before later sources are
evaluated. Sources that are added first (typically via config files loaded first) always
win. Sources discovered via AD after the limit is reached never get a tailer slot until
existing ones are removed.

The `globalSelection` mode (triggered by `by_modification_time`) pools all wildcard
files and selects the top N by mtime — this is fairer but introduces the dual-pass
structure (lines 180-226) where non-wildcard sources consume slots first, potentially
leaving wildcards starved if non-wildcards reach the limit.

---

## 17. Container_Collect_All Race: Config Must Arrive After Annotations

**File:** `comp/core/autodiscovery/providers/container.go:241-248`

The comment explicitly notes:

```
// container_collect_all configs must be added after
// configs generated from annotations, since services
// are reconciled against configs one-by-one instead of
// as a set, so if a container_collect_all config
// appears before an annotation one, it'll cause a logs
// config to be scheduled as container_collect_all,
// unscheduled, and then re-scheduled correctly.
```

This unschedule+reschedule cycle means that for every container starting during agent
startup, there may be a brief window where logs are collected under the
`container_collect_all` source (wrong tags, wrong metadata), then collection stops and
restarts under the correct annotated source (potentially missing logs between the two).
Fault injection that delays AD processing could extend this window arbitrarily.

---

## Summary Table

| # | Finding | Probability in Antithesis | Impact |
|---|---------|--------------------------|--------|
| 1 | Rotation reassigns pipeline, breaks per-file ordering | High (any rotation) | Medium |
| 2 | Identifier collision on container rotation (FIXME) | High (container churn) | High |
| 3 | addSource race during FilesToTail goroutine | Medium | Medium |
| 4 | ShouldIgnore walks /var/log/containers per-file, per-scan | High (K8s env) | Low (perf) |
| 5 | Stop() drops in-flight auditor payloads | Medium (shutdown) | Medium |
| 6 | Wildcard ordering FIXME — doublestar not guaranteed sorted | Low | Low |
| 7 | KeepAlive for beyond-limit files inflates registry | High (many wildcards) | Low |
| 8 | IngestionTimestamp at read time; multi-line shifts timestamp | Medium | Low |
| 9 | Pipeline failover destroys per-source ordering | High (backpressure) | High |
| 10 | Async AddSource/RemoveSource in container launcher | High (container churn) | High |
| 11 | Aggregation timeout is global, starves under load | Medium | Low |
| 12 | Windows rotation detection race on truncation | Low | Medium |
| 13 | Adaptive sampler isImportant is keyword-based | Medium | Low |
| 14 | 4096-byte allocation per read | Low | Low (perf) |
| 15 | KeepAlive no-op for unprocessed files; promotes to tail-end | Low | Medium |
| 16 | Greedy source starvation | Medium | Medium |
| 17 | container_collect_all race: duplicate collection window | High (startup) | Medium |

---

## Files Examined

- `pkg/logs/launchers/file/launcher.go`
- `pkg/logs/launchers/file/provider/file_provider.go`
- `pkg/logs/launchers/file/position.go`
- `pkg/logs/launchers/container/launcher.go`
- `pkg/logs/launchers/container/tailerfactory/file.go`
- `pkg/logs/launchers/container/tailerfactory/tailers/source.go`
- `pkg/logs/tailers/file/tailer.go`
- `pkg/logs/tailers/file/tailer_nix.go`
- `pkg/logs/tailers/file/rotate.go`
- `pkg/logs/tailers/file/rotate_nix.go`
- `pkg/logs/tailers/file/rotate_windows.go`
- `pkg/logs/tailers/file/file.go`
- `pkg/logs/tailers/file/fingerprint.go`
- `pkg/logs/internal/decoder/decoder.go`
- `pkg/logs/internal/decoder/preprocessor/sampler.go`
- `pkg/logs/internal/decoder/preprocessor/aggregator.go`
- `pkg/logs/internal/decoder/preprocessor/pattern_table.go`
- `pkg/logs/internal/util/containersorpods/chooser.go`
- `pkg/logs/sources/sources.go`
- `pkg/logs/sources/source.go`
- `comp/logs/auditor/impl/auditor.go`
- `comp/logs/auditor/impl/registry_writer.go`
- `comp/logs-library/pipeline/provider.go`
- `comp/core/autodiscovery/providers/container.go`
- `pkg/logs/README.md`
