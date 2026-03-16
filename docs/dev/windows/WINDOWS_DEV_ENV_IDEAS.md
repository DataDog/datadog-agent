# Windows Dev Env — Deferred Ideas

## Watcher as sole owner of state/output files

**Context**: `dda inv test --host windows` currently writes to the state/output files
when it does a fresh run (`_run_and_record`). This creates a race with the watcher when
both run concurrently — they share the same output file and can corrupt each other's output.

**Idea**: make the watcher the **only** writer of the state/output files. `dda inv test
--host windows` becomes purely a reader/consumer:

- Watcher alive + usable result → attach or replay (unchanged)
- Watcher alive + no usable result (cancelled, different packages, etc.) → run directly
  via `_run_on_windows_dev_env`, **no state file writing**
- Watcher dead → same: run directly, no state file writing

This eliminates `_run_and_record` and the race entirely.

**Tradeoff**: a manual fresh run is not cached — a second immediate `dda inv test` would
re-run instead of replaying. Acceptable since the watcher is the intended cache mechanism.

**Why not "call" the watcher**: the watcher's work queue is in-memory inside its process.
Cross-process injection requires IPC (socket, signal+file, etc.) which the design
explicitly avoids. So there is no way to push work to the watcher's queue from another
process without adding new IPC infrastructure.
