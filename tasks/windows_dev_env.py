"""
Create a remote Windows development environment and keep it in sync with local changes.
"""

import json
import os
import re
import shutil
import time
from datetime import timedelta
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
    print("Syncing changes to the remote Windows development environment...")
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

    with ctx.cd('./test/e2e-framework'):
        ctx.run("dda inv -- setup")
    if shutil.which("rsync") is None:
        raise Exception(
            "rsync is not installed. Please install rsync by running `brew install rsync` on macOS and try again."
        )
    # Create the Windows development environment.
    host = ""
    with ctx.cd('./test/e2e-framework'):
        result = ctx.run(
            f"dda inv -- aws.create-vm --ami-id={AMI_WINDOWS_DEV_2022} --os-family=windows --architecture=x86_64 --no-install-agent --stack-name={name} --no-interactive --instance-type=t3.xlarge"
        )
        if result is None or not result:
            raise Exception("Failed to create the Windows development environment.")
        connection_message_regex = re.compile(r"`ssh ([^@]+@\d+.\d+.\d+.\d+ [^`]+)`")
        match = connection_message_regex.search(result.stdout)
        if match:
            connection_message = match.group(1)
        else:
            raise Exception("Failed to find pulumi output in stdout.")
        # extract username and address from connection message
        host = connection_message.split()[0]
    print("Disabling Windows Defender and rebooting the Windows dev environment...")
    _disable_WD_and_reboot(ctx, host)
    _wait_for_windows_dev_env(ctx, host)
    print("Host rebooted")
    # check if Windows dev container is already running
    should_start_container = True
    result = ctx.run(f"ssh {host} 'docker ps -q --filter name=windows-dev-env'", warn=True, hide=True)
    if result is not None and result.exited == 0 and len(result.stdout) > 0:
        print("🐳 Windows dev env already running")
        should_start_container = False
    # start the Windows dev container, if not already running
    if should_start_container:
        print("🐳 Starting Windows dev container")
        ctx.run(
            f"ssh {host} 'docker run -m 16384 -v C:\\mnt:c:\\mnt:rw -w C:\\mnt\\datadog-agent -t -d --name {WIN_CONTAINER_NAME} datadog/agent-buildimages-windows_x64:ltsc2022 tail -f /dev/null'"
        )

    # Pull the latest version of datadog-agent to make initial sync faster
    print("Pulling the latest version of datadog-agent to make initial sync faster...")
    _run_on_windows_dev_env(ctx, name, "git pull")
    print("Pulling the latest version of datadog-agent done")
    # sync local changes to the remote Windows development environment
    rsync_command = _build_rsync_command(host)
    print("Syncing changes to the remote Windows development environment...")
    ctx.run(rsync_command)
    print("Syncing changes to the remote Windows development done")
    print("Installing all dependencies in the Windows dev container... this may take a long time")
    _run_on_windows_dev_env(
        ctx,
        name,
        ". ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallTestingDeps \\$true -InstallDeps \\$true -Command {.\\tasks\\winbuildscripts\\pre-go-build.ps1; dda inv -- -e tidy}",
    )
    # print the time taken to start the dev env
    elapsed_time = time.time() - start_time
    print("♻️ Windows dev env started in", timedelta(seconds=elapsed_time))
    _run_command_on_local_changes(ctx, rsync_command)
    print("♻️ Windows dev env sync stopped")
    print("Start it again with `dda inv windows_dev_env.start`")
    print("Destroy the Windows dev env with `dda inv windows-dev-env.stop`")


def _disable_WD_and_reboot(ctx, host):
    ctx.run(f"ssh {host} 'Remove-WindowsFeature Windows-Defender'")
    ctx.run(f"ssh {host} 'Restart-Computer -Force'")


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
    return f"rsync --chmod=ugo=rwX -azrcIPR --delete --rsync-path='C:\\cygwin\\bin\\rsync.exe' --filter=':- .gitignore' --exclude /.git/ . {host}:/cygdrive/c/mnt/datadog-agent/"


def _run_command_on_local_changes(ctx: Context, command: str):
    # lazy load watchdog to avoid import error on the CI
    from watchdog.events import FileSystemEvent, FileSystemEventHandler
    from watchdog.observers import Observer

    class DDAgentEventHandler(FileSystemEventHandler):
        def __init__(self, ctx: Context, command: str):
            self.ctx = ctx
            self.command = command

        def on_any_event(self, event: FileSystemEvent) -> None:  # noqa # called by watchdog callback
            _on_changed_path_run_command(self.ctx, event.src_path, self.command)

    event_handler = DDAgentEventHandler(ctx=ctx, command=command)
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


# start file watcher and run rsync on changes
def _on_changed_path_run_command(ctx: Context, path: str, command: str):
    current_dir = os.getcwd()
    relative_path = os.path.relpath(path, current_dir)
    if relative_path.startswith(".git/"):
        # ignore changes in .git directory
        return
    res = ctx.run(f"git check-ignore {relative_path} --quiet", warn=True, hide=True)
    if res is not None and res.exited == 0:
        # ignore changes in git ignored files
        # see https://git-scm.com/docs/git-check-ignore#_exit_status
        return
    print("Syncing changes to the remote Windows development environment...")
    ctx.run(command)
    print("Syncing changes to the remote Windows development environment done")


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
            '-it',
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
            "-t",
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
