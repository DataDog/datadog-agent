---
slug: registry-recovers-after-crash
focus: "3 — Failure Recovery"
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# Property: registry-recovers-after-crash

## What led to this property

The auditor registry (`registry.json`) is the only durable state the logs agent
has about how far each tailer has read. On restart, `recoverRegistry()` reads
this file. If it is missing or corrupt, the auditor silently falls back to an
empty map, which causes every tailer to start from its configured `TailingMode`
(default: end-of-file for running containers, beginning-of-file for files). That
silent fallback is the root cause of the data-loss (end-of-file) or mass-replay
(beginning-of-file) bug class described in sut-analysis.md §2.

## Key code locations

- `comp/logs/auditor/impl/auditor.go:337-353` — `recoverRegistry()`: reads the
  file; on any error (not-exist or unmarshal failure) returns `make(map[string]*RegistryEntry)`.
- `comp/logs/auditor/impl/auditor.go:123-129` — `Start()`: assigns
  `a.registry = a.recoverRegistry()`. There is no error path that aborts startup
  or alerts the operator beyond a log line.
- `comp/logs/auditor/impl/auditor.go:440-462` — `unmarshalRegistry()`: on a
  version number it does not recognize, returns `errors.New("invalid registry
  version number")`, also producing an empty registry.
- `comp/logs/auditor/impl/registry_writer.go:23-45` — atomic writer: `CreateTemp
  + Write + Chmod + Close + os.Rename`. Crash-safe because rename is atomic on
  POSIX.
- `comp/logs/auditor/impl/registry_writer.go:56-73` — non-atomic writer (Fargate
  path, controlled by `logs_config.atomic_registry_write`): `os.Create` (truncates)
  then `Write`. A crash between truncate and write produces a zero-length file that
  `recoverRegistry` treats as corrupt → silent fall-through to empty map.

## What fault triggers it

**Node termination (kill -9 / ungraceful stop)** is the primary trigger. The
1-second flush ticker means up to 1 second of acknowledged-but-unflushed offsets
are lost on any ungraceful termination. Even for a clean stop, `Stop()` calls
`closeChannels()` first (which closes `inputChan` and drains `done`), then
`cleanupRegistry()`, then `flushRegistry()`. Payloads that were in `inputChan`
at shutdown — acknowledged by the HTTP destination but not yet processed by the
auditor `run()` loop — are not flushed (the run loop exits on `inputChan` close
without draining the remaining items). This is H5 in sut-analysis.md §3.

## Why it matters

Registry loss is the root cause of at-least-once-with-mass-replay (beginning-of-file
mode) or silent-data-loss (end-of-file mode). Neither outcome is observable
without an external workload that counts lines delivered and lines injected.

## Assertions needed (all net-new SUT instrumentation)

1. **`Sometimes(registry recovered with non-empty offsets)`** (SUT-side, in
   `recoverRegistry()`) — proves the restart-from-registry path is reachable
   during the test run. Assertion type: `Sometimes(cond)`.
2. **`Reachable(recovery from missing/corrupt registry taken)`** (SUT-side, in
   the `if os.IsNotExist(err)` / `unmarshalRegistry` error branch of
   `recoverRegistry()`) — confirms Antithesis actually exercised the fallback
   path at least once.
3. **Workload `Always(lines_delivered_after_restart >= lines_injected_before_restart - replay_window)`** —
   reconciliation check: fakeintake should receive at least as many lines as were
   injected, minus a bounded replay window (≈1 second of throughput). Any larger
   gap indicates registry-loss data loss.

## Recovery window requirement

Needs `ANTITHESIS_STOP_FAULTS` / fault-quiet window after each restart so
workload can complete reconciliation before the next kill.

## Open questions

None.

### Investigation Log

#### Is `recoverRegistry()` the only place the registry is read at startup, or can `GetOffset()`/`GetTailingMode()` be called before `Start()` completes?

- Examined: `comp/logs/auditor/impl/auditor.go:122-129`. `Start()` calls `a.createChannels()`, then `a.registry = a.recoverRegistry()`, then `go a.run()` — all sequentially before returning. `GetOffset()` and `GetTailingMode()` both call `readOnlyRegistryEntryCopy()` which reads `a.registry` under `registryMutex`. The `registryMutex` is not held during `Start()`, so a caller who calls `GetOffset()` concurrently before `Start()` returns would see the zero value (nil map → empty entry). In practice, the audit component is started before launchers/tailers (per `startPipeline()` in `agent.go:284-293`), so callers only call `GetOffset()` after `Start()` has returned and `a.registry` is populated.
- Found: `recoverRegistry()` is the only read path at startup. Callers are ordered correctly. A race is only possible if `GetOffset()` is called before `Start()` from a concurrent goroutine; no such path found.
- Not found: any test or code path where `GetOffset()` is called before `Start()` completes.
- Conclusion: resolved. `recoverRegistry()` is the only read path and ordering guarantees it completes before tailers call `GetOffset()`. Removed from Open Questions.

#### Does `Stop()` correctly drain `inputChan` before `flushRegistry()`?

- Examined: `comp/logs/auditor/impl/auditor.go:131-184`, `run()` loop lines 283-291.
- Found: `Stop()` calls `closeChannels()` → `close(a.inputChan)` → blocks on `<-a.done`. The run loop, on seeing `inputChan` closed, enters the `if !isOpen { return }` branch immediately. Because `select` is used (not `range`), there is no guarantee that remaining buffered items in `inputChan` are drained before the `!isOpen` arm fires. After `closeChannels()` returns, `flushRegistry()` writes whatever `a.registry` was last updated to — which omits the unprocessed buffered payloads. The drain gap is real and live in the current code.
- Conclusion: resolved. `Stop()` does NOT drain `inputChan` correctly. The duplicate-on-restart risk from graceful stop is confirmed. Removed from Open Questions.
