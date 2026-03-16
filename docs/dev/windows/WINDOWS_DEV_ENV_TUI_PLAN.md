# Plan: Windows Dev Env TUI (`dda inv windows-dev-env.tui`)

## Context

The watcher currently runs headlessly, printing timestamped lines to the terminal. There is
no at-a-glance view of VM status, what files just synced, or whether the test/linter run
passed. The goal is a Textual-based TUI that starts and manages the watcher internally,
replacing the plain `watch` task for interactive use. State is read from the existing
`/tmp/windev_*_state.json` files and the output files — no new IPC needed.

---

## Layout

```
┌─────────────────────────────────────────────────────────┐
│ Windows Dev Env                       windows-dev-env   │  Header
├─────────────────────────────────────────────────────────┤
│ VM      [green]Running[/] · 18.234.x.x · Container: OK  │
│ Watcher [green]Watching[/] · last sync 17:42:01 (3 files)│
├─────────────────────────────────────────────────────────┤
│ Synced  pkg/util/json/json.go                           │
│         pkg/util/json/json_test.go                      │
├─────────────────────────────────────────────────────────┤
│ Test    [yellow]RUNNING[/]  pkg/util/json  42s elapsed  │
│         go test -v ./pkg/util/json/...                  │  ← last output line
│ Linter  [green]PASSED[/]   pkg/util/json  12s · 17:41:55│
│         [no issues]                                     │  ← last output line
├─────────────────────────────────────────────────────────┤
│  Test  │  Linter                                        │  Tab bar
│  ...scrollable full output...                           │
└─────────────────────────────────────────────────────────┘
```

### Color mapping

| State | Color |
|---|---|
| VM Running, Container OK | green |
| VM Not Running, Container not found | red |
| Watcher Watching | green |
| Watcher Starting, Syncing | yellow |
| Watcher Error / stopped | red |
| Test/Linter PASSED | green |
| Test/Linter RUNNING | yellow |
| Test/Linter FAILED, CANCELLED | red |
| Test/Linter IDLE (no state yet) | dim (default) |

---

## Files

| File | Change |
|---|---|
| `tasks/windows_dev_env.py` | Add `tui` task; add sync event callbacks to `DDAgentEventHandler`; add `stop_event` to `_run_command_on_local_changes` |
| `tasks/windows_dev_env_tui.py` | **New** — Textual `WatcherApp` + all widgets |

---

## Changes

### 1. `tasks/windows_dev_env.py`

#### 1a. Sync event callbacks on `DDAgentEventHandler`

Add two optional callbacks to `__init__`:
```python
def __init__(self, ctx, host, on_sync=None, on_sync_start=None, on_sync_end=None):
    ...
    self._on_sync_start = on_sync_start  # called with files: list[str] before rsync
    self._on_sync_end   = on_sync_end    # called with (files, success: bool) after rsync
```

Call them in `_sync()`:
```python
if self._on_sync_start:
    self._on_sync_start(files)
try:
    self.ctx.run(rsync_command)
    if self._on_sync_end:
        self._on_sync_end(files, True)
    if self._on_sync and any(f.endswith(".go") for f in files):
        self._on_sync()
except UnexpectedExit:
    if self._on_sync_end:
        self._on_sync_end(files, False)
```

#### 1b. Stop event for `_run_command_on_local_changes`

Add `stop_event: threading.Event | None = None` parameter. Replace the blocking loop:
```python
# before
while True:
    time.sleep(1)

# after
while not (stop_event and stop_event.is_set()):
    time.sleep(0.5)
```

Pass `on_sync_start`, `on_sync_end`, `stop_event` through to `DDAgentEventHandler` and
the observer loop.

#### 1c. `tui` task

```python
@task(help={'name': '...'})
def tui(ctx: Context, name: str = "windows-dev-env"):
    """Start the Windows dev env TUI — watches for changes, syncs, runs tests and linter."""
    from tasks.windows_dev_env_tui import WatcherApp
    app = WatcherApp(ctx=ctx, name=name)
    app.run()
```

---

### 2. `tasks/windows_dev_env_tui.py` (new file)

#### Textual import (lazy, same pattern as watchdog)

All Textual classes imported at module level inside `windows_dev_env_tui.py`, which is
only imported when the `tui` task runs. Clear error message if `textual` is not installed.

#### Widgets

**`StatusRow(Static)`** — single key/value row. The value is rendered with Textual markup
color based on the state it represents (see color mapping above).

**`SyncPanel(Static)`** — shows last-synced file list. Updated via reactive `synced_files`.
Files shown in dim color while syncing, normal color once done.

**`CommandPanel(Static)`** — shows one command's status (test or linter). Reads:
- `command_type`, `status`, `packages`, `start_time`, `end_time`, `exit_code` from state
- Last non-empty line of the output file (for the sub-line below status)
- Status badge colored per the color mapping: `[green]PASSED[/]`, `[yellow]RUNNING[/]`,
  `[red]FAILED[/]`, `[red]CANCELLED[/]`, dim `IDLE`

**`OutputTabs(TabbedContent)`** — two tabs (Test / Linter), each contains a `RichLog`.
Active tab streams the output file while status is RUNNING; shows full file otherwise.

#### `WatcherApp(App)`

```python
class WatcherApp(App):
    CSS = """..."""

    vm_info: reactive[dict | None] = reactive(None)
    container_ok: reactive[bool] = reactive(False)
    watcher_status: reactive[str] = reactive("Starting...")
    synced_files: reactive[list] = reactive([])
    last_sync_time: reactive[str] = reactive("")

    def __init__(self, ctx, name):
        super().__init__()
        self._ctx = ctx
        self._name = name
        self._stop_event = threading.Event()
        self._work_queues = {}     # command_type → Queue
        self._current_procs = {}   # command_type → [Popen|None]
        self._proc_locks = {}      # command_type → Lock
        self._output_offsets = {}  # command_type → int (byte offset for tailing)

    def compose(self) -> ComposeResult:
        yield Header()
        yield StatusRow(id="vm-status")
        yield StatusRow(id="watcher-status")
        yield SyncPanel(id="sync-panel")
        yield CommandPanel("test", id="test-panel")
        yield CommandPanel("linter", id="linter-panel")
        yield OutputTabs(id="output-tabs")
        yield Footer()

    def on_mount(self):
        self.run_worker(self._start_watcher, thread=True, exclusive=True)
        self.set_interval(0.5, self._poll_state)
        self.set_interval(10.0, self._check_vm)

    def _start_watcher(self):
        """Worker thread: validate VM, rsync, seed queues, start runners + watchdog."""
        # 1. VM check (aws.show-vm)        → updates vm_info
        # 2. Container check (docker ps)   → updates container_ok
        # 3. Initial rsync                 → updates watcher_status
        # 4. find_modified_packages + seed work queues (reuse _build_watch_work)
        # 5. Start one _test_runner_loop daemon thread per command (reuse as-is)
        # 6. _run_command_on_local_changes(...,
        #        stop_event=self._stop_event,
        #        on_sync_start=self._on_sync_start,
        #        on_sync_end=self._on_sync_end)
        #    blocks until stop_event is set

    def _poll_state(self):
        """Read state files + tail output every 0.5s."""
        for command_type in ("test", "linter"):
            state = _read_state(self._name, command_type)
            self.query_one(f"#{command_type}-panel").update_state(state)
        self._tail_active_output()

    def _tail_active_output(self):
        """Append new bytes from the active output file to the matching RichLog."""
        # For the active tab: open output file, seek to _output_offsets[command_type],
        # read new content, append to RichLog, update offset.
        # On new run (state start_time changed): reset offset to 0 and clear the log.

    def _check_vm(self):
        """Refresh VM + container reachability every 10s."""

    def _on_sync_start(self, files):
        self.call_from_thread(self._set_syncing, files)

    def _on_sync_end(self, files, success):
        self.call_from_thread(self._set_sync_done, files, success)

    def on_unmount(self):
        self._stop_event.set()
```

---

## What does NOT change

- `_test_runner_loop` — reused as-is
- `_build_watch_work` — reused as-is
- `_write_state` / `_read_state` — reused as-is (TUI only reads state files)
- `watch` task — unchanged, still works headlessly
- State file format — no changes

---

## Dependency

`textual` is not in `deps/py_dev_requirements.txt`. Import it at the top of
`windows_dev_env_tui.py` (which is only imported when `tui` task runs — same lazy
pattern as `watchdog`). If missing, a clear `ImportError` message guides the user:
`pip install textual`.

---

## Verification

1. `pip install textual`
2. `dda inv windows-dev-env.tui` — TUI launches, VM/container status visible
3. Modify a `.go` file locally — Synced files section updates, test + linter rows go RUNNING
4. Wait for completion — rows show PASSED/FAILED with elapsed time
5. Output tabs show full output streamable for each command
6. Press `q` or `Ctrl+C` — TUI exits cleanly, watcher threads stop
7. Confirm `dda inv windows-dev-env.watch` still works (no regression)
