"""
Create a remote Windows development environment and keep it in sync with local changes.
"""

import json
import os
import queue as queue_module
import re
import shutil
import subprocess
import sys
import threading
import time
from datetime import datetime, timedelta
from typing import Any

from invoke.context import Context
from invoke.tasks import task

AMI_WINDOWS_DEV_2022 = "ami-09b68440cb06b26d6"
WIN_CONTAINER_NAME = "windows-dev-env"
_DD_MODULE_PREFIX = "github.com/DataDog/datadog-agent/"


# ---------------------------------------------------------------------------
# State file helpers (shared between the watch process and attach_or_run)
# ---------------------------------------------------------------------------


def _state_file_path(name: str) -> str:
    return f"/tmp/windev_{name}_state.json"


def _output_file_path(name: str) -> str:
    return f"/tmp/windev_{name}_output.txt"


def _write_state(name: str, state: dict) -> None:
    """Atomically write the state JSON (write to temp file then rename)."""
    path = _state_file_path(name)
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(state, f)
    os.rename(tmp, path)


def _read_state(name: str) -> dict | None:
    try:
        with open(_state_file_path(name)) as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return None


def _pid_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
        return True
    except OSError:
        return False


def _normalize_package(pkg: str) -> str:
    """Normalize a package path to a bare relative name like 'pkg/util/json'.

    Handles:
    - './pkg/util/json'      → 'pkg/util/json'
    - 'pkg/util/json/./.'   → 'pkg/util/json'
    - 'github.com/DataDog/datadog-agent/pkg/util/json' → 'pkg/util/json'
    - 'pkg/util/json'       → 'pkg/util/json'  (no-op)
    """
    # Collapse redundant . components (e.g. "pkg/util/json/./." → "pkg/util/json")
    pkg = os.path.normpath(pkg).replace("\\", "/")
    if pkg.startswith("./"):
        pkg = pkg[2:]
    if pkg.startswith(_DD_MODULE_PREFIX):
        pkg = pkg[len(_DD_MODULE_PREFIX) :]
    return pkg


def _normalize_packages(packages) -> frozenset[str]:
    """Return a frozenset of bare relative package names (order-independent)."""
    return frozenset(_normalize_package(p) for p in packages)


# ---------------------------------------------------------------------------
# Invoke tasks
# ---------------------------------------------------------------------------


@task(
    help={
        'name': 'Override the default name of the development environment (windows-dev-env).',
    },
)
def start(
    ctx: Context,
    name: str = "windows-dev-env",
):
    """
    Create a remote Windows development environment and keep it in sync with local changes.
    """
    _start_windows_dev_env(ctx, name)


@task(
    help={
        'name': 'Override the default name of the development environment (windows-dev-env).',
    },
)
def stop(
    ctx: Context,
    name: str = "windows-dev-env",
):
    """
    Removes a remote Windows development environment.
    """
    _stop_windows_dev_env(ctx, name)


@task(
    help={
        'name': 'Override the default name of the development environment (windows-dev-env).',
    },
)
def sync(
    ctx: Context,
    name: str = "windows-dev-env",
):
    """
    Resumes syncing local changes to a running remote Windows development environment.
    Raises an error if the VM or container cannot be reached.
    """
    _sync_windows_dev_env(ctx, name)


@task(
    help={
        'name': 'Override the default name of the development environment (windows-dev-env).',
        'command': 'Command to run on a windows dev container',
    },
)
def run(
    ctx: Context,
    name: str = "windows-dev-env",
    command: str = "",
):
    """
    Runs a command on a remote Windows development environment.
    """

    with ctx.cd('./test/e2e-framework'):
        # find connection info for the VM
        result = ctx.run(f"dda inv -- aws.show-vm --stack-name={name}", hide=True)
        if result is None or not result:
            raise Exception("Failed to find the Windows development environment.")
        host = RemoteHost(result.stdout)
    rsync_command = _build_rsync_command(f"Administrator@{host.address}")
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Syncing changes to the remote Windows development environment...")
    ctx.run(rsync_command)

    exit(
        _run_on_windows_dev_env(
            ctx,
            name,
            f'. ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallDeps \\$false -CheckGoVersion \\$false -Command {{{command}}}',
        )
    )


@task(
    help={
        'name': 'Override the default name of the development environment (windows-dev-env).',
        'command': 'Command to run after each sync (e.g. "inv test --build-stdlib --targets=./pkg/util/json").',
    },
)
def watch(
    ctx: Context,
    name: str = "windows-dev-env",
    command: str = "inv test --build-stdlib",
):
    """
    Watch for local changes, sync them to the remote Windows VM and run a command after each sync.
    Writes results to a state file so that `dda inv test --host windows` can attach to a running
    test or replay a recent result instead of launching a redundant remote run.
    """
    with ctx.cd('./test/e2e-framework'):
        result = ctx.run(f"dda inv -- aws.show-vm --stack-name={name}", warn=True, hide=True)
        if result is None or result.exited != 0 or not result.stdout.strip():
            raise Exception(
                f"Windows dev env '{name}' cannot be reached. Make sure it is running with `dda inv windows-dev-env.start`."
            )
        remote_host = RemoteHost(result.stdout)
        host = f"{remote_host.user}@{remote_host.address}"

    result = ctx.run(f"ssh {host} 'docker ps -q --filter name={WIN_CONTAINER_NAME}'", warn=True, hide=True)
    if result is None or result.exited != 0 or not result.stdout.strip():
        raise Exception(
            f"Windows dev container '{WIN_CONTAINER_NAME}' is not running on {host}. Make sure it is running with `dda inv windows-dev-env.start`."
        )

    # Initial rsync before starting the watch loop
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Syncing changes to the remote Windows development environment...")
    ctx.run(_build_rsync_command(host))
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Initial sync done, watching for local changes...")

    # Queue carries (command_type, packages, ssh_cmd) triples.
    # maxsize=1: at most one pending run at a time; _enqueue drains before inserting so
    # the runner always executes the freshest command.
    work_queue: queue_module.Queue[tuple[str, frozenset[str], str]] = queue_module.Queue(maxsize=1)

    # Only seed on startup if there are already modified packages — avoids launching the
    # full test suite just because the watcher was started before any file changes.
    initial_work = _build_watch_work(ctx, remote_host, command)
    _, initial_packages, _ = initial_work
    if initial_packages:
        work_queue.put_nowait(initial_work)
    else:
        print(f"[{datetime.now().strftime('%H:%M:%S')}] No modified packages, waiting for file changes...")

    # Shared reference to the currently running subprocess so _enqueue can kill it.
    current_proc: list[subprocess.Popen | None] = [None]
    proc_lock = threading.Lock()

    threading.Thread(
        target=_test_runner_loop,
        args=(name, work_queue, current_proc, proc_lock),
        daemon=True,
    ).start()

    def _enqueue() -> None:
        work = _build_watch_work(ctx, remote_host, command)
        # Kill the in-flight process so the runner picks up the fresh command immediately.
        with proc_lock:
            proc = current_proc[0]
            if proc is not None and proc.poll() is None:
                proc.kill()
        # Replace any pending (not-yet-started) item with the freshest command.
        try:
            work_queue.get_nowait()
        except queue_module.Empty:
            pass
        work_queue.put_nowait(work)

    _run_command_on_local_changes(ctx, host, on_sync=_enqueue)


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _start_windows_dev_env(ctx, name: str = "windows-dev-env"):
    start_time = time.time()

    if shutil.which("rsync") is None:
        raise Exception(
            "rsync is not installed. Please install rsync by running `brew install rsync` on macOS and try again."
        )

    # Check if the VM already exists before trying to create it.
    host = None
    with ctx.cd('./test/e2e-framework'):
        result = ctx.run(f"dda inv -- aws.show-vm --stack-name={name}", warn=True, hide=True)
        if result is not None and result.exited == 0 and result.stdout.strip():
            remote_host = RemoteHost(result.stdout)
            host = f"{remote_host.user}@{remote_host.address}"
            print(f"♻️ Windows dev env already exists at {host}")

    if host is None:
        # VM does not exist yet: create it and run first-time setup.
        with ctx.cd('./test/e2e-framework'):
            result = ctx.run(
                f"dda inv -- aws.create-vm --ami-id={AMI_WINDOWS_DEV_2022} --os-family=windows --architecture=x86_64 --no-install-agent --stack-name={name} --no-interactive --instance-type=t3.2xlarge"
            )
            if result is None or not result:
                raise Exception("Failed to create the Windows development environment.")
            connection_message_regex = re.compile(r"`ssh ([^@]+@\d+.\d+.\d+.\d+ [^`]+)`")
            match = connection_message_regex.search(result.stdout)
            if match:
                connection_message = match.group(1)
            else:
                raise Exception("Failed to find pulumi output in stdout.")
            host = connection_message.split()[0]

        print("Disabling Windows Defender and rebooting the Windows dev environment...")
        if _disable_WD_and_reboot(ctx, host):
            _wait_for_windows_dev_env(ctx, host)
            print("Host rebooted")

        # Start the Windows dev container
        print("🐳 Starting Windows dev container")
        ctx.run(
            f"ssh {host} 'docker run -m 16384 -v C:\\mnt:c:\\mnt:rw -w C:\\mnt\\datadog-agent -t -d --name {WIN_CONTAINER_NAME} datadog/agent-buildimages-windows_x64:ltsc2022 tail -f /dev/null'"
        )

        # Lift the 260-character path limit required by Bazel.
        # Set-ItemProperty writes to the shared host registry (process isolation).
        print("Lifting the 260-character path limit...")
        _run_on_windows_dev_env(
            ctx,
            name,
            "Set-ItemProperty -Path HKLM:\\SYSTEM\\CurrentControlSet\\Control\\FileSystem -Name LongPathsEnabled -Value 1",
        )

        # Pull the latest version of datadog-agent to make initial sync faster
        print("Pulling the latest version of datadog-agent to make initial sync faster...")
        _run_on_windows_dev_env(ctx, name, "git pull")
        print("Pulling the latest version of datadog-agent done")

        # Sync local changes and install all dependencies
        rsync_command = _build_rsync_command(host)
        print(
            f"[{datetime.now().strftime('%H:%M:%S')}] Syncing changes to the remote Windows development environment..."
        )
        ctx.run(rsync_command)
        print(f"[{datetime.now().strftime('%H:%M:%S')}] Syncing changes to the remote Windows development done")
        print("Installing all dependencies in the Windows dev container... this may take a long time")
        _run_on_windows_dev_env(
            ctx,
            name,
            ". ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallTestingDeps \\$true -InstallDeps \\$true -Command {.\\tasks\\winbuildscripts\\pre-go-build.ps1; dda inv -- -e tidy}",
        )
        elapsed_time = time.time() - start_time
        print(f"[{datetime.now().strftime('%H:%M:%S')}] Windows dev env started in {timedelta(seconds=elapsed_time)}")
    else:
        # VM already exists: check if the container is running.
        result = ctx.run(f"ssh {host} 'docker ps -q --filter name={WIN_CONTAINER_NAME}'", warn=True, hide=True)
        if result is None or result.exited != 0 or not result.stdout.strip():
            print("🐳 Container not running, starting it...")
            ctx.run(
                f"ssh {host} 'docker run -m 16384 -v C:\\mnt:c:\\mnt:rw -w C:\\mnt\\datadog-agent -t -d --name {WIN_CONTAINER_NAME} datadog/agent-buildimages-windows_x64:ltsc2022 tail -f /dev/null'"
            )
        else:
            print("🐳 Windows dev env already running, resuming sync")

    print("Start the file sync with `dda inv windows-dev-env.sync` to only sync changes.")
    print("Start the file watcher with `dda inv windows-dev-env.watch`to sync changes and run tests.")
    print("Destroy the Windows dev env with `dda inv windows-dev-env.stop`")


def _sync_windows_dev_env(ctx, name: str = "windows-dev-env"):
    with ctx.cd('./test/e2e-framework'):
        result = ctx.run(f"dda inv -- aws.show-vm --stack-name={name}", warn=True, hide=True)
        if result is None or result.exited != 0 or not result.stdout.strip():
            raise Exception(
                f"Windows dev env '{name}' cannot be reached. Make sure it is running with `dda inv windows-dev-env.start`."
            )
        remote_host = RemoteHost(result.stdout)
        host = f"{remote_host.user}@{remote_host.address}"

    result = ctx.run(f"ssh {host} 'docker ps -q --filter name={WIN_CONTAINER_NAME}'", warn=True, hide=True)
    if result is None or result.exited != 0 or not result.stdout.strip():
        raise Exception(
            f"Windows dev container '{WIN_CONTAINER_NAME}' is not running on {host}. Make sure it is running with `dda inv windows-dev-env.start`."
        )

    rsync_command = _build_rsync_command(host)
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Syncing changes to the remote Windows development environment...")
    ctx.run(rsync_command)
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Syncing changes done, watching for local changes...")
    _run_command_on_local_changes(ctx, host)


def _disable_WD_and_reboot(ctx, host) -> bool:
    """Removes Windows Defender and reboots if needed. Returns True if a reboot was triggered."""
    result = ctx.run(f"ssh {host} '(Remove-WindowsFeature Windows-Defender).RestartNeeded'", warn=True, hide=True)
    print(result.stdout)
    if result is None or "Yes" not in result.stdout:
        print("Windows Defender was already removed or no restart needed, skipping reboot.")
        return False
    ctx.run(f"ssh {host} 'Restart-Computer -Force'")
    return True


def _wait_for_windows_dev_env(ctx, host):
    while True:
        r = ctx.run(f"ssh {host} 'Get-MpComputerStatus | select Antivirus'", hide=True, warn=True)
        if "Invalid class" in r.stderr:
            break

        time.sleep(5)


def _build_rsync_command(host: str) -> str:
    # -a: archive mode; equals -rlptgoD (no -H)
    # -z: compress file data during the transfer
    # -r: recurse into directories
    # -c: skip based on checksum, not mod-time & size
    # -I: --ignore-times
    # -P: same as --partial --progress, show partial progress during transfer
    # -R: use relative path names
    # --inplace: write directly to the destination file instead of temp+rename,
    #            avoids NTFS "Permission denied" when a file is locked by a running process
    return (
        f"rsync --chmod=ugo=rwX -azrcIPR --delete --inplace --rsync-path='C:\\cygwin\\bin\\rsync.exe'"
        f" --filter=':- .gitignore'"
        f" --exclude /.git/"
        # Exclude the DatadogInterop folder entirely — the DLL is built separately and
        # the solution does not need to be touched during dev env syncs.
        f" --exclude tools/windows/DatadogInterop/"
        f" . {host}:/cygdrive/c/mnt/datadog-agent/"
    )


def _build_remote_command(host: "RemoteHost", command: str) -> str:
    """Build the full SSH + docker exec command string to run `command` inside the container."""
    docker_parts = [
        'docker',
        'exec',
        '-i',
        '-e',
        'PYTHONUTF8=1',
        WIN_CONTAINER_NAME,
        'powershell',
        f"'{command}'",
    ]
    joined = ' '.join(docker_parts)
    return f'ssh {host.user}@{host.address} -p {host.port} "{joined}"'


def _run_command_on_local_changes(ctx: Context, host: str, on_sync=None):
    # lazy load watchdog to avoid import error on the CI
    from watchdog.events import FileSystemEvent, FileSystemEventHandler
    from watchdog.observers import Observer

    class DDAgentEventHandler(FileSystemEventHandler):
        _DEBOUNCE_SECONDS = 0.5

        def __init__(self, ctx: Context, host: str, on_sync=None):
            self.ctx = ctx
            self.host = host
            self._on_sync = on_sync
            self._timer: threading.Timer | None = None
            self._lock = threading.Lock()
            self._pending_files: set[str] = set()

        def on_any_event(self, event: FileSystemEvent) -> None:  # noqa # called by watchdog callback
            current_dir = os.getcwd()
            relative_path = os.path.relpath(event.src_path, current_dir)
            if relative_path.startswith(".git/"):
                return
            res = self.ctx.run(f"git check-ignore {relative_path} --quiet", warn=True, hide=True)
            if res is not None and res.exited == 0:
                # ignore changes in git ignored files
                # see https://git-scm.com/docs/git-check-ignore#_exit_status
                return
            with self._lock:
                self._pending_files.add(relative_path)
                if self._timer is not None:
                    self._timer.cancel()
                self._timer = threading.Timer(self._DEBOUNCE_SECONDS, self._sync)
                self._timer.start()

        def _sync(self):
            from invoke.exceptions import UnexpectedExit

            with self._lock:
                files = [f for f in self._pending_files if os.path.isfile(f)]
                self._pending_files.clear()

            if not files:
                return

            files_args = " ".join(f"'./{f}'" for f in files)
            rsync_command = (
                f"rsync --chmod=ugo=rwX -azR --inplace --rsync-path='C:\\cygwin\\bin\\rsync.exe'"
                f" {files_args}"
                f" {self.host}:/cygdrive/c/mnt/datadog-agent/"
            )
            print(
                f"[{datetime.now().strftime('%H:%M:%S')}] Syncing {len(files)} file(s) to the remote Windows development environment..."
            )
            try:
                self.ctx.run(rsync_command)
                print(
                    f"[{datetime.now().strftime('%H:%M:%S')}] Syncing changes to the remote Windows development environment done"
                )
                if self._on_sync is not None:
                    self._on_sync()
            except UnexpectedExit as e:
                print(
                    f"[{datetime.now().strftime('%H:%M:%S')}] Sync failed (exit code {e.result.exited}), will retry on next change"
                )

    event_handler = DDAgentEventHandler(ctx=ctx, host=host, on_sync=on_sync)
    observer = Observer()
    observer.schedule(event_handler, ".", recursive=True)
    observer.start()
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        observer.stop()
    finally:
        observer.join()


def _build_watch_work(
    ctx: Context, remote_host: "RemoteHost", fallback_command: str
) -> tuple[str, frozenset[str], str]:
    """
    Compute the (command_type, packages, ssh_cmd) triple for the next test run.

    Calls find_modified_packages() to build a targeted `inv test --targets=...` command
    covering only the packages that differ from the base branch.
    Falls back to `fallback_command` when no modified packages are found (e.g. for
    non-test commands like `inv linter.go`, or when nothing has changed yet).

    Returns:
        command_type – "test" or "linter", stored in the state file.
        packages     – frozenset of bare relative package names (e.g. "pkg/util/json").
        ssh_cmd      – full SSH + docker exec command passed to subprocess.Popen.
    """
    from tasks.gotest import find_modified_packages

    raw_packages = find_modified_packages(ctx)
    if raw_packages:
        packages = _normalize_packages(raw_packages)
        inv_cmd = f"inv test --build-stdlib --targets={','.join(f'./{p}' for p in sorted(packages))}"
        command_type = "test"
    else:
        packages = frozenset()
        inv_cmd = fallback_command
        command_type = "linter" if "linter" in fallback_command else "test"
    wrapped = f'. ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallDeps \\$false -CheckGoVersion \\$false -Command {{{inv_cmd}}}'
    return command_type, packages, _build_remote_command(remote_host, wrapped)


def _test_runner_loop(
    name: str,
    work_queue: "queue_module.Queue[tuple[str, frozenset[str], str]]",
    current_proc: "list[subprocess.Popen | None]",
    proc_lock: threading.Lock,
) -> None:
    """
    Background thread: block on the work queue, run the remote command via subprocess,
    and update the state file before and after each run.
    Output is written to a dedicated file so that attach_or_run can stream or replay it.
    When the process is killed by _enqueue (negative returncode on Unix), the run is
    treated as cancelled: no 'finished' state is written and the loop moves on immediately.
    """
    output_path = _output_file_path(name)
    while True:
        command_type, packages, ssh_cmd = work_queue.get()
        sorted_packages = sorted(packages)
        start_time = datetime.now()
        _write_state(
            name,
            {
                "status": "running",
                "command": command_type,
                "packages": sorted_packages,
                "start_time": start_time.isoformat(),
                "watcher_pid": os.getpid(),
                "output_file": output_path,
            },
        )
        pkg_summary = f" ({', '.join(sorted_packages)})" if sorted_packages else ""
        print(f"[{start_time.strftime('%H:%M:%S')}] Running: {command_type}{pkg_summary}")
        with open(output_path, "w") as out:
            proc = subprocess.Popen(ssh_cmd, shell=True, stdout=out, stderr=subprocess.STDOUT)
            with proc_lock:
                current_proc[0] = proc
            exit_code = proc.wait()
            with proc_lock:
                current_proc[0] = None

        if exit_code < 0:
            # Killed by _enqueue — a newer run is already queued, skip writing finished state.
            print(f"[{datetime.now().strftime('%H:%M:%S')}] Cancelled, picking up next run...")
            continue

        if exit_code == 255:
            # SSH error (connection dropped or watcher interrupted) — the test did not complete.
            # Write a cancelled state so attach_or_run triggers a fresh run instead of replaying
            # an incomplete output with a misleading exit code.
            _write_state(
                name,
                {
                    "status": "cancelled",
                    "command": command_type,
                    "packages": sorted_packages,
                    "watcher_pid": os.getpid(),
                    "output_file": output_path,
                },
            )
            print(f"[{datetime.now().strftime('%H:%M:%S')}] SSH error (exit 255), test did not complete")
            continue

        end_time = datetime.now()
        _write_state(
            name,
            {
                "status": "finished",
                "command": command_type,
                "packages": sorted_packages,
                "start_time": start_time.isoformat(),
                "end_time": end_time.isoformat(),
                "exit_code": exit_code,
                "watcher_pid": os.getpid(),
                "output_file": output_path,
            },
        )
        elapsed = int((end_time - start_time).total_seconds())
        status_str = "passed" if exit_code == 0 else f"failed (exit {exit_code})"
        print(f"[{end_time.strftime('%H:%M:%S')}] {status_str} in {elapsed}s")


def _stop_windows_dev_env(ctx, name: str = "windows-dev-env"):
    with ctx.cd('./test/e2e-framework'):
        ctx.run(f"dda inv -- aws.destroy-vm --stack-name={name}")


class RemoteHost:
    def __init__(self, output: str):
        remoteHost: Any = json.loads(output)
        self.address: str = remoteHost["address"]
        self.user: str = remoteHost["user"]
        self.password: str | None = "password" in remoteHost and remoteHost["password"] or None
        self.port: int | None = "port" in remoteHost and remoteHost["port"] or None


def _run_on_windows_dev_env(ctx: Context, name: str = "windows-dev-env", command: str = "") -> int:
    with ctx.cd('./test/e2e-framework'):
        # find connection info for the VM
        result = ctx.run(f"dda inv -- aws.show-vm --stack-name={name}", hide=True)
        if result is None or not result:
            raise Exception("Failed to find the Windows development environment.")
        host = RemoteHost(result.stdout)
        result = ctx.run(
            _build_remote_command(host, command),
            pty=True,
            warn=True,
        )
        if result is None or not result:
            raise Exception("Failed to run the command on the Windows development environment.")
        return result.exited


def attach_or_run(ctx: Context, name: str, command_type: str, packages) -> int:
    """
    Smart entry point for `dda inv test --host windows`.

    Checks the watch process state file before starting a new remote test run:
    - RUNNING + same command_type + requested packages ⊆ state packages → attach.
    - FINISHED + same command_type + requested packages ⊆ state packages → replay.
    - Otherwise → run fresh.

    A non-empty requested set is accepted whenever the state covers a superset of the
    requested packages (e.g. requesting {A} matches a state that ran {A, B}).
    An empty requested set (all packages) only matches a state that also ran all packages.

    `command_type` is "test" or "linter".
    `packages` is a list/set of package paths in any format (./pkg/…, full Go import path, or bare
    relative path); they are normalized to bare relative names before comparison.
    """
    norm_packages = _normalize_packages(packages)

    state = _read_state(name)
    if state:
        pid = state.get("watcher_pid")
        if pid and not _pid_alive(pid):
            state = None  # stale: the watcher process is dead

    if state and state.get("command") == command_type:
        state_packages = frozenset(state.get("packages") or [])
        # Non-empty request: accept if state is a superset of requested packages.
        # Empty request (all packages): only accept if state also covers all packages.
        packages_covered = norm_packages.issubset(state_packages) if norm_packages else not state_packages
        if packages_covered:
            status = state.get("status")
            if status == "running":
                print(f"[{datetime.now().strftime('%H:%M:%S')}] Attaching to running test...")
                return _attach_to_output(name, state)
            if status == "finished":
                print(f"[{datetime.now().strftime('%H:%M:%S')}] Replaying result from {state['end_time']}...")
                return _replay_output(state)

    # Fresh run: reconstruct the inv command from command_type + packages.
    if norm_packages:
        targets = ",".join(f"./{p}" for p in sorted(norm_packages))
        inv_cmd = f"inv test --build-stdlib --targets={targets}"
    elif command_type == "test":
        inv_cmd = "inv test --build-stdlib"
    else:
        inv_cmd = "inv linter.go"
    wrapped = f'. ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallDeps \\$false -CheckGoVersion \\$false -Command {{{inv_cmd}}}'
    return _run_on_windows_dev_env(ctx, name, wrapped)


def _attach_to_output(name: str, state: dict) -> int:
    """Stream the watch process's output file to stdout until the test finishes."""
    output_file = state["output_file"]
    try:
        with open(output_file) as f:
            sys.stdout.write(f.read())  # catch-up: print everything written so far
            sys.stdout.flush()
            while True:
                chunk = f.read()
                if chunk:
                    sys.stdout.write(chunk)
                    sys.stdout.flush()
                else:
                    current = _read_state(name)
                    if current and current["status"] == "finished":
                        remaining = f.read()
                        if remaining:
                            sys.stdout.write(remaining)
                            sys.stdout.flush()
                        return current["exit_code"]
                    time.sleep(0.1)
    except FileNotFoundError:
        return 1


def _replay_output(state: dict) -> int:
    """Print the stored output of a finished run and return its exit code."""
    try:
        with open(state["output_file"]) as f:
            sys.stdout.write(f.read())
            sys.stdout.flush()
    except FileNotFoundError:
        print("Output file not found, cannot replay.")
        return 1
    return state["exit_code"]
