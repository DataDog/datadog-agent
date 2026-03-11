"""
Create a remote Windows development environment and keep it in sync with local changes.
"""

import json
import os
import re
import shutil
import threading
import time
from datetime import datetime, timedelta
from typing import Any

from invoke.context import Context
from invoke.tasks import task

AMI_WINDOWS_DEV_2022 = "ami-09b68440cb06b26d6"
WIN_CONTAINER_NAME = "windows-dev-env"


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
            f'. ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallDeps \\$false -Command {{{command}}}',
        )
    )


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

    _sync_windows_dev_env(ctx, name)
    print("♻️ Windows dev env sync stopped")
    print("Start it again with `dda inv windows-dev-env.sync`")
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


def _run_command_on_local_changes(ctx: Context, host: str):
    # lazy load watchdog to avoid import error on the CI
    from watchdog.events import FileSystemEvent, FileSystemEventHandler
    from watchdog.observers import Observer

    class DDAgentEventHandler(FileSystemEventHandler):
        _DEBOUNCE_SECONDS = 0.5

        def __init__(self, ctx: Context, host: str):
            self.ctx = ctx
            self.host = host
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
            except UnexpectedExit as e:
                print(
                    f"[{datetime.now().strftime('%H:%M:%S')}] Sync failed (exit code {e.result.exited}), will retry on next change"
                )

    event_handler = DDAgentEventHandler(ctx=ctx, host=host)
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
        # run the command on the Windows development environment
        docker_command_parts = [
            'docker',
            'exec',
            '-i',
            '-e',
            'PYTHONUTF8=1',
            WIN_CONTAINER_NAME,
            'powershell',
            f"'{command}'",
        ]
        joined_docker_command_parts = ' '.join(docker_command_parts)
        command_parts = [
            "ssh",
            f'{host.user}@{host.address}',
            "-p",
            f'{host.port}',
            f'"{joined_docker_command_parts}"',
        ]
        result = ctx.run(
            ' '.join(command_parts),
            pty=True,
            warn=True,
        )
        if result is None or not result:
            raise Exception("Failed to run the command on the Windows development environment.")
        return result.exited
