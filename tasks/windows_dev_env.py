"""
Create a remote Windows development environment and keep it in sync with local changes.
"""

import os
import re
import shutil
import time
from datetime import timedelta

from invoke.context import Context
from invoke.tasks import task

AMI_WINDOWS_DEV_2022 = "ami-09b68440cb06b26d6"


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


def _start_windows_dev_env(ctx, name: str = "windows-dev-env"):
    start_time = time.time()
    # lazy load watchdog to avoid import error on the CI
    from watchdog.events import FileSystemEvent, FileSystemEventHandler
    from watchdog.observers import Observer

    class DDAgentEventHandler(FileSystemEventHandler):
        def __init__(self, ctx: Context, command: str):
            self.ctx = ctx
            self.command = command

        def on_any_event(self, event: FileSystemEvent) -> None:  # noqa # called by watchdog callback
            _on_changed_path_run_command(self.ctx, event.src_path, self.command)

    # Ensure `test-infra-definitions` is cloned.
    if not os.path.isdir('../test-infra-definitions'):
        with ctx.cd('..'):
            ctx.run("git clone git@github.com:DataDog/test-infra-definitions.git")
            with ctx.cd('test-infra-definitions'):
                # setup test-infra-definitions
                ctx.run("python3 -m pip install -r requirements.txt")
                ctx.run("inv setup")
    if shutil.which("rsync") is None:
        raise Exception(
            "rsync is not installed. Please install rsync by running `brew install rsync` on macOS and try again."
        )
    # Create the Windows development environment.
    host = ""
    with ctx.cd('../test-infra-definitions'):
        result = ctx.run(
            f"inv aws.create-vm --ami-id={AMI_WINDOWS_DEV_2022} --os-family=windows --architecture=x86_64 --no-install-agent --stack-name={name} --no-interactive"
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

    # sync local changes to the remote Windows development environment
    # -aqzrcIR
    # -a: archive mode; equals -rlptgoD (no -H)
    # -z: compress file data during the transfer
    # -r: recurse into directories
    # -c: skip based on checksum, not mod-time & size
    # -I: --ignore-times
    # -P: same as --partial --progress, show partial progress during transfer
    # -R: use relative path names
    rsync_command = f"rsync -azrcIPR --delete --rsync-path='C:\\cygwin\\bin\\rsync.exe' --filter=':- .gitignore' --exclude /.git/ . {host}:/cygdrive/c/mnt/datadog-agent/"
    print("Syncing changes to the remote Windows development environment...")
    ctx.run(rsync_command)
    print("Syncing changes to the remote Windows development done")
    # print the time taken to start the dev env
    elapsed_time = time.time() - start_time
    print("♻️ Windows dev env started in", timedelta(seconds=elapsed_time))

    event_handler = DDAgentEventHandler(ctx=ctx, command=rsync_command)
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
    print("♻️ Windows dev env sync stopped")
    print("Start it again with `inv windows_dev_env.start`")
    print("Destroy the Windows dev env with `inv windows-dev-env.stop`")


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
    with ctx.cd('../test-infra-definitions'):
        ctx.run(f"inv aws.destroy-vm --stack-name={name}")
