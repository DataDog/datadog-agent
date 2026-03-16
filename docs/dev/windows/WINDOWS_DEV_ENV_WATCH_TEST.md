# Watch + Test on Windows Dev Env — Design Notes

Goal: after each file-change sync, automatically run a test command on the Windows
VM. When a developer runs `dda inv test --host windows`, instead of blindly
launching a new remote test run, it should be aware of what the watch process is
already doing and avoid redundant work.

---

## Revised orchestration model

The watch process does not stream test output directly to a terminal. Instead it
owns a **shared state file** and a **shared output file**. Any external process
(like `dda inv test --host windows`) reads those files to decide what to do.

### State machine

```
IDLE  ──(file change + sync)──►  RUNNING  ──(test finishes)──►  FINISHED
  ▲                                                                  │
  └──────────────────────────(new change)────────────────────────────┘
```

State is written to `/tmp/windev_<name>_state.json`:

```json
{
  "status": "running | finished",
  "command": "test | linter",
  "packages": ["pkg/util/json", "pkg/util/scrubber"],
  "start_time": "2026-03-11T10:00:00",
  "end_time": "2026-03-11T10:01:00",
  "exit_code": 0,
  "watcher_pid": 42301,
  "output_file": "/tmp/windev_windows-dev-env_output.txt"
}
```

`command` is `"test"` or `"linter"`. `packages` is a sorted list of bare relative
package names (e.g. `pkg/util/json`, without `./` prefix or Go module path prefix).
An empty `packages` list means the full suite for that command type.

Test output (stdout + stderr) is written incrementally to the output file as the
remote test runs.

---

## Behaviour of `dda inv test --host windows`

When invoked, it reads the state file and follows this decision tree:

```
read state file
    │
    ├─ watcher_pid dead?  →  discard state, treat as stale
    │
    ├─ status = RUNNING, command + packages match?
    │       └─► "attach": tail output_file until status → FINISHED, exit with exit_code
    │
    ├─ status = FINISHED, command + packages match?
    │       └─► "replay": cat output_file, exit with stored exit_code  (no remote run)
    │
    └─ otherwise (different command/packages, stale result)
            └─► launch new test run normally, write state + output files
```

"Matches" means `command` strings are equal **and** the requested packages are a
**subset** of the packages in the state file (order-independent). This way,
`--targets=A` is satisfied by a state that ran `{A, B}` — no redundant re-run.
An empty `--targets` (meaning all packages) only matches a state that also ran all
packages (empty `packages` list in the state file).

Package paths are normalized to bare relative names on both sides before comparison,
so `./pkg/util/json`, `github.com/DataDog/datadog-agent/pkg/util/json`, and
`pkg/util/json` are all treated as the same package.

**No TTL on the `FINISHED` state.** Staleness is handled by two mechanisms that are
already in place, making a TTL redundant:

1. **Watcher alive** — it re-runs tests on every file change, so the recorded result
   is always the most recent one. A `FINISHED` result from an hour ago can only exist
   if no files changed in the meantime, in which case it is still valid.
2. **Watcher dead** — the `watcher_pid` check detects this and discards the state,
   falling through to a fresh run.

---

## IPC: state file + output file (no sockets needed)

A shared file is sufficient for this use case. There is no need for a Unix domain
socket or named pipe because:

- The state file is written atomically (write to temp file + rename) by the watch
  process. Readers always see a complete JSON snapshot.
- The output file is append-only. Readers tail it; they never need to seek or lock.
- There is only one writer for each file (the watch process or the external
  `inv test` when it takes over as the active runner).

The only edge case is a crash leaving the state as `running` with no active writer.
A watcher PID field in the state file lets readers detect this: if the PID is dead,
treat the state as stale.

---

## Output streaming for the "attach" case

When `dda inv test --host windows` attaches to a running test, it:

1. Prints everything already in the output file (catch-up).
2. Follows the output file with a simple polling read loop until `status` in the
   state file transitions to `finished`.
3. Exits with the `exit_code` from the state file.

This gives the developer the illusion of having run the test themselves, with full
output and correct exit code, at zero cost if the watch process already did the
work.

---

## Output streaming for the "replay" case

The test finished with the same command and packages. The developer is about to run
the exact same thing. Instead of re-running:

1. Print the stored output file verbatim.
2. Exit with the stored `exit_code`.

---

## Watch process responsibilities

```python
# After successful sync, _build_watch_work determines the targeted command:
#   - calls find_modified_packages() to get the set of changed packages
#   - if packages found: "test" + those packages
#   - if none found: falls back to the command passed to `watch` (e.g. "inv linter.go")

command_type, packages, ssh_cmd = _build_watch_work(ctx, remote_host, fallback_command)

state = {
    "status": "running",
    "command": command_type,       # "test" or "linter"
    "packages": sorted(packages),  # bare relative names, e.g. ["pkg/util/json"]
    "start_time": now(),
    "watcher_pid": os.getpid(),
    "output_file": OUTPUT_FILE,
}
atomic_write(STATE_FILE, state)

with open(OUTPUT_FILE, "w") as out:
    proc = subprocess.Popen(ssh_cmd, stdout=out, stderr=out)
    exit_code = proc.wait()

state.update({"status": "finished", "end_time": now(), "exit_code": exit_code})
atomic_write(STATE_FILE, state)
```

If a new file change arrives while a test is running, the `queue.Queue(maxsize=1)`
ensures the runner picks up the freshest command once the current run finishes
(stale pending items are drained before inserting the new one).

---

## Cancellation of the remote test

Killing the local SSH process on Windows containers is unreliable (SIGHUP may not
propagate). Practical options:

1. **Accept overlap**: let the old test finish, start the new one immediately.
   Use `queue.Queue(maxsize=1)` — the runner always drains to the latest item.
   Simple, no kill logic needed.
2. **`docker stop` + restart**: send a second SSH command to stop the container's
   test process before starting the new run. More complex.

Option 1 is the recommended starting point for a dev loop.

---

## Task signatures

```
# Default: run both test and linter in parallel
dda inv windows-dev-env.watch

# Explicit: run only one command
dda inv windows-dev-env.watch --command "inv test --build-stdlib"

# Explicit: run multiple in parallel
dda inv windows-dev-env.watch --command "inv test --build-stdlib" --command "inv linter.go"

# Attach to / replay the result of whatever the watch process last ran
dda inv test --host windows --only-modified-packages
dda inv linter.go --host windows --only-modified-packages
```

---

## Summary

| Concern | Mechanism |
|---|---|
| IPC between watch and `inv test` | JSON state file (atomic write) |
| Output sharing | Append-only output file |
| Live output for "attach" | Tail output file until state → finished |
| Zero-cost replay | Cat output file if same command type + same packages |
| Match comparison | `command` string equality + requested packages ⊆ state packages |
| Package normalization | Strip `./` and Go module prefix → bare relative name |
| Parallel commands | One runner thread per `--command`; independent queues and state files |
| Cancellation | queue.Queue(maxsize=1), accept overlap |
| Crash detection | `watcher_pid` field — check if PID is alive |
