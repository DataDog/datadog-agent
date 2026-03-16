"""Rich-based TUI for the Windows dev env watcher (dda inv windows-dev-env.tui)."""

from __future__ import annotations

import queue as queue_module
import threading
import time
from dataclasses import dataclass, field
from datetime import datetime

try:
    from rich.console import Console, Group
    from rich.live import Live
    from rich.panel import Panel
    from rich.table import Table
    from rich.text import Text
except ImportError:
    raise ImportError("rich is required to run the TUI. Install it with: pip install rich") from None

from tasks.windows_dev_env import (
    WIN_CONTAINER_NAME,
    RemoteHost,
    _build_rsync_command,
    _build_watch_work,
    _read_state,
    _run_command_on_local_changes,
    _test_runner_loop,
)

_COMMANDS = ["inv test", "inv linter.go"]
_COMMAND_TYPES = ["test", "linter"]


# ---------------------------------------------------------------------------
# Shared state (updated from background threads, read by the render loop)
# ---------------------------------------------------------------------------


@dataclass
class TUIState:
    # VM section
    vm_text: str = "Checking..."
    vm_color: str = "yellow"

    # Watcher section
    watcher_text: str = "Starting..."
    watcher_color: str = "yellow"

    # Watcher readiness
    watcher_ready: bool = False

    # Sync section
    sync_status: str = "idle"  # "idle" | "syncing" | "synced" | "failed"
    sync_files: list[str] = field(default_factory=list)
    sync_time: str = ""


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------


def _status_grid(state: TUIState) -> Table:
    grid = Table.grid(padding=(0, 2))
    grid.add_column(style="bold", width=10)
    grid.add_column()
    grid.add_row("VM", Text(state.vm_text, style=state.vm_color))
    grid.add_row("Watcher", Text(state.watcher_text, style=state.watcher_color))
    return grid


def _sync_grid(state: TUIState) -> Table:
    grid = Table.grid(padding=(0, 2))
    grid.add_column(style="bold", width=10)
    grid.add_column()

    if not state.watcher_ready:
        grid.add_row("Sync", Text("Waiting for watcher...", style="dim"))
    elif state.sync_status == "idle":
        grid.add_row("Sync", Text("Waiting for file changes...", style="dim"))
    elif state.sync_status == "syncing":
        count = len(state.sync_files)
        header = Text(f"Syncing {count} file(s)...", style="yellow")
        grid.add_row("Syncing", header)
        for f in state.sync_files[:5]:
            grid.add_row("", Text(f, style="dim"))
        if count > 5:
            grid.add_row("", Text(f"(+{count - 5} more)", style="dim"))
    elif state.sync_status == "synced":
        count = len(state.sync_files)
        header = Text(f"{state.sync_time} ({count} file(s))", style="green")
        grid.add_row("Synced", header)
        for f in state.sync_files[:5]:
            grid.add_row("", Text(f, style="dim"))
        if count > 5:
            grid.add_row("", Text(f"(+{count - 5} more)", style="dim"))
    elif state.sync_status == "no_packages":
        count = len(state.sync_files)
        header = Text(f"{state.sync_time} ({count} file(s)) · no modified packages", style="dim")
        grid.add_row("Synced", header)
        for f in state.sync_files[:5]:
            grid.add_row("", Text(f, style="dim"))
        if count > 5:
            grid.add_row("", Text(f"(+{count - 5} more)", style="dim"))
    elif state.sync_status == "failed":
        grid.add_row("Sync", Text("Failed", style="red"))

    return grid


def _command_grid(name: str, state: TUIState) -> Table:
    grid = Table.grid(padding=(0, 2))
    grid.add_column(style="bold", width=10)
    grid.add_column()

    for ct in _COMMAND_TYPES:
        label = "Test  " if ct == "test" else "Linter"

        if not state.watcher_ready:
            grid.add_row(label, Text("Waiting for watcher...", style="dim"))
            continue

        st = _read_state(name, ct)

        status_val = st.get("status", "") if st else ""
        if state.sync_status == "no_packages" and status_val != "running":
            status = Text("No modified packages", style="dim")
        elif st is None:
            status = Text("IDLE", style="dim")
        else:
            pkgs = ", ".join(st.get("packages") or ["all"])
            if status_val == "running":
                elapsed = int((datetime.now() - datetime.fromisoformat(st["start_time"])).total_seconds())
                status = Text(f"RUNNING   {pkgs}  {elapsed}s elapsed", style="yellow")
            elif status_val == "finished":
                exit_code = st.get("exit_code", 1)
                start = datetime.fromisoformat(st["start_time"])
                end = datetime.fromisoformat(st["end_time"])
                elapsed = int((end - start).total_seconds())
                end_str = end.strftime("%H:%M:%S")
                if exit_code == 0:
                    status = Text(f"FINISHED  {pkgs}  {elapsed}s · {end_str}", style="green")
                else:
                    status = Text(f"FAILED    {pkgs}  {elapsed}s · {end_str}", style="red")
            elif status_val == "cancelled":
                status = Text(f"CANCELLED  {pkgs}", style="red")
            else:
                status = Text("IDLE", style="dim")

        grid.add_row(label, status)

    return grid


def _make_display(name: str, state: TUIState) -> Group:
    return Group(
        Panel(_status_grid(state), title=f"[bold]Windows Dev Env[/bold] · {name}"),
        Panel(_sync_grid(state), title="Sync"),
        Panel(_command_grid(name, state), title="Commands"),
    )


# ---------------------------------------------------------------------------
# TUI app
# ---------------------------------------------------------------------------


class WatcherApp:
    def __init__(self, ctx, name: str = "windows-dev-env") -> None:
        self._ctx = ctx
        self._name = name
        self._stop_event = threading.Event()
        self._state = TUIState()
        self._state_lock = threading.Lock()
        self._work_queues: dict[str, queue_module.Queue] = {}
        self._current_procs: dict[str, list] = {}
        self._proc_locks: dict[str, threading.Lock] = {}

    def _update(self, **kwargs) -> None:
        """Thread-safe update of TUIState fields."""
        with self._state_lock:
            for k, v in kwargs.items():
                setattr(self._state, k, v)

    def run(self) -> None:
        threading.Thread(target=self._start_watcher, daemon=True).start()
        threading.Thread(target=self._vm_refresh_loop, daemon=True).start()

        console = Console()
        try:
            with self._state_lock:
                snapshot = self._make_snapshot()
            with Live(
                _make_display(self._name, snapshot),
                refresh_per_second=4,
                console=console,
                screen=True,
            ) as live:
                while not self._stop_event.is_set():
                    with self._state_lock:
                        snapshot = self._make_snapshot()
                    live.update(_make_display(self._name, snapshot))
                    time.sleep(0.25)
        except KeyboardInterrupt:
            pass
        finally:
            self._stop_event.set()

    def _make_snapshot(self) -> TUIState:
        """Return a shallow copy of the current state (called under lock)."""
        s = self._state
        return TUIState(
            vm_text=s.vm_text,
            vm_color=s.vm_color,
            watcher_ready=s.watcher_ready,
            watcher_text=s.watcher_text,
            watcher_color=s.watcher_color,
            sync_status=s.sync_status,
            sync_files=list(s.sync_files),
            sync_time=s.sync_time,
        )

    # ------------------------------------------------------------------
    # Background: watcher startup
    # ------------------------------------------------------------------

    def _start_watcher(self) -> None:
        # VM check
        self._update(vm_text="Checking...", vm_color="yellow")
        try:
            with self._ctx.cd('./test/e2e-framework'):
                result = self._ctx.run(
                    f"dda inv -- aws.show-vm --stack-name={self._name}",
                    warn=True,
                    hide=True,
                )
            if result is None or result.exited != 0 or not result.stdout.strip():
                self._update(
                    vm_text="Not reachable",
                    vm_color="red",
                    watcher_text="Stopped — VM unreachable",
                    watcher_color="red",
                )
                return
            remote_host = RemoteHost(result.stdout)
        except Exception as e:
            self._update(vm_text=str(e), vm_color="red")
            return

        host = f"{remote_host.user}@{remote_host.address}"
        self._update(vm_text=f"Connecting... ({remote_host.address})", vm_color="yellow")

        # Container check
        result = self._ctx.run(
            f"ssh {host} 'docker ps -q --filter name={WIN_CONTAINER_NAME}'",
            warn=True,
            hide=True,
        )
        container_ok = result is not None and result.exited == 0 and bool(result.stdout.strip())
        if container_ok:
            self._update(
                vm_text=f"Running · {remote_host.address} · Container: OK",
                vm_color="green",
            )
        else:
            self._update(
                vm_text=f"Running · {remote_host.address} · Container: NOT RUNNING",
                vm_color="red",
                watcher_text="Stopped — container not running",
                watcher_color="red",
            )
            return

        # Initial rsync
        self._update(watcher_text="Syncing...", watcher_color="yellow")
        self._ctx.run(_build_rsync_command(host))

        # Seed work queues
        from tasks.gotest import find_modified_packages

        initial_raw = find_modified_packages(self._ctx)
        for cmd in _COMMANDS:
            ct = "linter" if "linter" in cmd else "test"
            wq: queue_module.Queue = queue_module.Queue(maxsize=1)
            cp: list = [None]
            lock = threading.Lock()
            self._work_queues[ct] = wq
            self._current_procs[ct] = cp
            self._proc_locks[ct] = lock
            initial_work = _build_watch_work(remote_host, cmd, initial_raw)
            _, initial_packages, _ = initial_work
            if initial_packages:
                wq.put_nowait(initial_work)
            threading.Thread(
                target=_test_runner_loop,
                args=(self._name, wq, cp, lock),
                daemon=True,
            ).start()

        if not initial_raw:
            self._update(sync_status="no_packages")
        self._update(watcher_ready=True, watcher_text="Watching for file changes...", watcher_color="green")

        def _enqueue() -> None:
            raw = find_modified_packages(self._ctx)
            any_enqueued = False
            for cmd in _COMMANDS:
                ct = "linter" if "linter" in cmd else "test"
                work = _build_watch_work(remote_host, cmd, raw)
                _, packages, _ = work
                if not packages:
                    continue
                any_enqueued = True
                with self._proc_locks[ct]:
                    proc = self._current_procs[ct][0]
                    if proc is not None and proc.poll() is None:
                        proc.kill()
                try:
                    self._work_queues[ct].get_nowait()
                except queue_module.Empty:
                    pass
                self._work_queues[ct].put_nowait(work)
            if not any_enqueued:
                self._update(sync_status="no_packages")

        # Start watchdog (blocks until stop_event)
        threading.Thread(
            target=_run_command_on_local_changes,
            args=(self._ctx, host),
            kwargs={
                "on_sync": _enqueue,
                "on_sync_start": self._on_sync_start,
                "on_sync_end": self._on_sync_end,
                "stop_event": self._stop_event,
            },
            daemon=True,
        ).start()

    # ------------------------------------------------------------------
    # Background: periodic VM status refresh
    # ------------------------------------------------------------------

    def _vm_refresh_loop(self) -> None:
        while not self._stop_event.is_set():
            for _ in range(20):  # check stop_event every 0.5s, refresh every 10s
                if self._stop_event.is_set():
                    return
                time.sleep(0.5)
            try:
                with self._ctx.cd('./test/e2e-framework'):
                    result = self._ctx.run(
                        f"dda inv -- aws.show-vm --stack-name={self._name}",
                        warn=True,
                        hide=True,
                    )
                if result is None or result.exited != 0 or not result.stdout.strip():
                    self._update(vm_text="Not reachable", vm_color="red")
                    continue
                remote_host = RemoteHost(result.stdout)
                host = f"{remote_host.user}@{remote_host.address}"
                result = self._ctx.run(
                    f"ssh {host} 'docker ps -q --filter name={WIN_CONTAINER_NAME}'",
                    warn=True,
                    hide=True,
                )
                container_ok = result is not None and result.exited == 0 and bool(result.stdout.strip())
                container_str = "Container: OK" if container_ok else "Container: NOT RUNNING"
                color = "green" if container_ok else "red"
                self._update(
                    vm_text=f"Running · {remote_host.address} · {container_str}",
                    vm_color=color,
                )
            except Exception:
                pass

    # ------------------------------------------------------------------
    # Sync callbacks
    # ------------------------------------------------------------------

    def _on_sync_start(self, files: list[str]) -> None:
        self._update(
            sync_status="syncing",
            sync_files=list(files),
            watcher_text=f"Syncing {len(files)} file(s)...",
            watcher_color="yellow",
        )

    def _on_sync_end(self, files: list[str], success: bool) -> None:
        if success:
            has_go = any(f.endswith(".go") for f in files)
            self._update(
                sync_status="synced" if has_go else "no_packages",
                sync_files=list(files),
                sync_time=datetime.now().strftime("%H:%M:%S"),
                watcher_text="Watching for file changes...",
                watcher_color="green",
            )
        else:
            self._update(
                sync_status="failed",
                sync_files=list(files),
                watcher_text="Sync failed, will retry on next change",
                watcher_color="red",
            )
