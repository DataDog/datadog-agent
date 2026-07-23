---
slug: journald-cursor-recovery-no-gap
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# journald-cursor-recovery-no-gap — No Gap and Bounded Duplicates on journald Cursor Recovery After Crash

> **Topology-extension required**: this property requires a journald source
> container or Unix-socket fault capability. The standard file-tailing topology
> does not exercise this path.

## What Led to This Property

`sut-analysis.md` §6 (bug history) cites regression target `55c63957d9f`:
"journald drops first entry on restart." `sut-analysis.md` §8 (external
dependencies) notes: "journald — `setup()` failure flags the source and skips
the tailer; no reconnect on mid-session failure." §11 (NOT covered) lists
"journald cursor recovery after ungraceful kill (no-gap/no-duplicate)" as an
explicit Antithesis value-add.

journald uses cursor-based tracking rather than byte offsets. The cursor is
stored in the auditor registry under `journald:<id>`. After a crash, the tailer
resumes from the last committed cursor via `SeekCursor` + `NextSkip(1)`. The
regression target `55c63957d9f` was a bug where the first entry after the
cursor was silently dropped on restart; the fix adds `NextSkip(1)` to skip the
already-committed entry. This property guards against:

1. A *gap* (entries after the cursor that are not delivered after restart).
2. *Unbounded duplicates* (entries before the cursor that are re-delivered;
   bounded duplicates from the 1-second flush window are acceptable).
3. *No auto-restart on mid-session journal disconnect*: if the journal fd is
   lost (SIGTERM to the journald socket, container restart, filesystem remount),
   `tail()` exits the error branch (`return` at line 296) without attempting
   reconnection. The launcher does not re-invoke `setupTailer` until the next
   periodic scan. This creates a gap equal to the scan interval (typically 10-30s)
   worth of journal entries.

## Code Paths Involved

**`pkg/logs/tailers/journald/tailer.go:229-273`** — `seek()`:

```go
func (t *Tailer) seek(cursor string) error {
    ...
    if cursor != "" {
        err := t.journal.SeekCursor(cursor)
        if err != nil {
            return err
        }
        // must skip one entry since the cursor points to the last committed one.
        _, err = t.journal.NextSkip(1)
        return err
    }
    ...
}
```

`NextSkip(1)` skips exactly the entry at the cursor (the last committed one).
This means after restart, the first delivered entry is the one *after* the
cursor. If `NextSkip(1)` skips more than one entry (e.g., due to a journal
compaction that merged entries), a gap appears.

**`pkg/logs/tailers/journald/tailer.go:276-337`** — `tail()` error handling:

```go
n, err := t.journal.Next()
if err != nil && err != io.EOF {
    err := fmt.Errorf("cant't tail journal %s: %s", t.journalPath(), err)
    t.source.Status.Error(err)
    log.Error(err)
    return  // exits tail goroutine; no auto-reconnect
}
```

When `journal.Next()` returns a non-EOF error (e.g., the journal fd is
invalidated, or the journal rotates the underlying files), the tailer exits
without reconnecting. The launcher will not restart it until the next periodic
scan, creating a gap equal to the scan interval.

**`pkg/logs/launchers/journald/launcher.go:82-119`** — `run()` loop:

```go
func (l *Launcher) run() {
    for {
        select {
        case source := <-l.sources:
            ...
            tailer, err := l.setupTailer(source)
            ...
            l.tailers[identifier] = tailer
        case <-l.stop:
            return
        }
    }
}
```

This is a source-driven loop only. There is no periodic re-scan of active
tailers to detect that one has exited. If a tailer's goroutine exits due to
a journal error, the launcher is not notified, and the tailer is not restarted.

**`pkg/logs/launchers/journald/launcher.go:151-158`** — `setupTailer()` cursor retrieval:

```go
tailer := tailer.NewTailer(source, l.pipelineProvider.NextPipelineChan(), journal, ...)
cursor := l.registry.GetOffset(tailer.Identifier())
err = tailer.Start(cursor)
```

`GetOffset` returns the last committed cursor string. On restart after crash,
the cursor from the previous run is retrieved and passed to `Start` → `seek`.

## Failure Scenarios

**Scenario 1: Gap after crash.** Agent crashes with 500ms of un-flushed journal
messages in the pipeline. Auditor registry was last flushed 800ms ago. The
committed cursor points to an entry 800ms before the crash. On restart:
- `SeekCursor` positions at the committed cursor.
- `NextSkip(1)` skips the committed entry.
- The next entry delivered is 800ms before crash + 1 entry.
- All entries between the cursor+1 and the crash point are re-delivered
  (duplicates, bounded by the flush window).
- **No gap**: at-least-once preserved. This is the expected at-least-once
  behavior.

**Scenario 2: Gap from mid-session disconnect.** The journal daemon restarts
(or the socket is faulted) during the tailer's `tail()` loop. `journal.Next()`
returns an error. The tailer goroutine exits. New journal entries continue to
accumulate. The launcher does not restart the tailer. Entries from the
disconnect until the next scanner run are **permanently lost** (gap). This is
the regression scenario identified in §8.

**Scenario 3: Duplicate from regression `55c63957d9f`.** Before the fix,
`seek` called `SeekCursor` without `NextSkip(1)`. After restart, `Next()` would
return the already-committed entry (the cursor entry itself), delivering it again.
The property guards against re-introduction of this regression.

## Assertion Design

**`Always`** (workload-side): The workload writes journal entries with unique
monotonically-increasing sequence IDs embedded in the message field. After a
crash-restart cycle:
- Assert that no sequence ID appears twice in the intake (or: at most one
  delivery per ID, with bounded duplicate count ≤ 1-second-worth-of-IDs).
- Assert that the sequence numbers are contiguous at intake (no gaps), within
  the at-least-once tolerance.

**`Reachable`** (SUT-side): At the point where `seek()` finds a non-empty
cursor and calls `NextSkip(1)`, add:

```
antithesis.Reachable(
    "journald-cursor-recovery-no-gap: cursor-based seek on restart",
    map[string]any{"cursor": cursor, "tailer_id": t.Identifier()},
)
```

This confirms the recovery path (not the cold-start path) was exercised.

**`AlwaysOrUnreachable`** (SUT-side): At the `return` in the `tail()` error
branch (journal disconnect), add:

```
antithesis.AlwaysOrUnreachable(
    "journald-cursor-recovery-no-gap: tailer-exit-without-reconnect",
    // Document that this exit creates a gap; assertion is always-true (structural)
    // but the Reachable signal alerts Antithesis to explore this path.
    true,
    map[string]any{"tailer_id": t.Identifier(), "error": err.Error()},
)
```

Combined with the `Reachable` at restart, if the workload detects entries
delivered out of sequence (gap), the fault trace includes the disconnect event.

## Why It Matters

journald is a primary log source for systemd-based Linux hosts. In containerized
environments, journal entries are the canonical source for kernel, system, and
container runtime logs (Docker, containerd). A crash-and-recover scenario that
drops the first post-cursor entry (the `55c63957d9f` regression) silently loses
exactly the entry most relevant to debugging the crash. A mid-session disconnect
that is not auto-recovered creates a gap of up to 10-30 seconds of system logs,
precisely when the system is under stress and logging is most valuable.

This property is topology-dependent: it requires a journald source (either a
real systemd journal or a mock that speaks the sd-journal API) and the ability
to fault the journal fd or the journald socket mid-session.

## Relationship to Other Properties

- `at-least-once-no-loss` — covers the general at-least-once delivery guarantee;
  this property is the journald-specific instantiation, which uses cursor tracking
  rather than byte offsets.
- `registry-recovers-after-crash` — covers the auditor registry recovery path;
  this property depends on the registry correctly storing and retrieving the
  journal cursor.

## Open Questions

- Does `NextSkip(1)` skip exactly one entry in all journal implementations
  (sdjournal, mock journal in tests)? If a journal compaction collapses two
  entries into one cursor position, `NextSkip(1)` could skip further than intended.
  `(partial: sdjournal.NextSkip(n) skips n entries by calling Next() n times
  internally; compaction should not affect this, but the mock journal behavior
  needs verification)`
- Is there a mechanism in the launcher to detect that a tailer goroutine has
  exited unexpectedly (e.g., via a done channel)? If not, the mid-session
  disconnect gap is undetectable at the launcher level. `(partial: confirmed
  no such detection in launcher.go run loop; the launcher only reacts to
  new sources from the AD channel)`
- What is the journald launcher scan interval (how often does it check for new
  sources)? This bounds the maximum gap from a mid-session disconnect. `(needs
  human input)`
- Does the planned Antithesis topology include a journald-capable container, or
  will this property require a separate harness variant? `(needs human input)`

### Investigation Log

#### Does the tail() error-exit path create a gap and is the launcher notified?

- Examined: `pkg/logs/tailers/journald/tailer.go:276-337` (`tail()`), `pkg/logs/launchers/journald/launcher.go:82-119` (`run()`), `pkg/logs/launchers/journald/launcher.go:120-133` (`Stop()`).
- Found: `tail()` exits via `return` (no panic, no signal to launcher) on `journal.Next()` error. The launcher `run()` loop is select-only on `l.sources` and `l.stop` — there is no tailer health monitoring. The tailer map `l.tailers[identifier]` still contains the dead tailer. No restart occurs until a new source event arrives (AD reschedules the journald source).
- Not found: any ticker-based health check or tailer-exit notification in the journald launcher.
- Conclusion: mid-session disconnect creates a silent gap equal to the time until the next AD rescan. This is a real production bug, not a theoretical one.

#### Does regression 55c63957d9f apply to the current code?

- Examined: `pkg/logs/tailers/journald/tailer.go:257-266` (`seek()` cursor path), `pkg/logs/tailers/journald/tailer_test.go` (cursor seek tests).
- Found: `NextSkip(1)` is present in the current code. The regression fix is applied. The property guards against re-introduction.
- Not found: a test that specifically exercises the crash-and-restart sequence with a real cursor (all tailer tests use mock journals with pre-positioned state).
- Conclusion: the fix is in place but untested end-to-end under crash conditions. The property provides this coverage.
