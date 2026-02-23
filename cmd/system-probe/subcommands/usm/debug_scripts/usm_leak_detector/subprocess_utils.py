"""
Subprocess utilities with seccomp-safe timeout handling.

Standard subprocess.run() with timeout uses time.sleep() internally,
which can be blocked by seccomp profiles (nanosleep syscall).
This module provides an alternative using select() which uses
kernel-level timing that's typically allowed.
"""

import os
import select
import subprocess
import time
from typing import List, Optional, Union

from .constants import DEFAULT_SUBPROCESS_TIMEOUT, POLL_INTERVAL, PIPE_READ_BUFFER_SIZE


def _safe_kill(proc: subprocess.Popen) -> None:
    """Attempt to kill a process, ignoring errors if blocked."""
    try:
        proc.kill()
    except (OSError, PermissionError):
        # kill() syscall blocked or process already dead - nothing we can do
        pass
    try:
        proc.wait()
    except Exception:
        pass


def safe_subprocess_run(
    cmd: Union[List[str], str],
    timeout: Optional[float] = DEFAULT_SUBPROCESS_TIMEOUT,
    capture_output: bool = False,
    text: bool = False,
    **kwargs
) -> subprocess.CompletedProcess:
    """
    Run a subprocess with timeout using select() instead of sleep().

    This avoids the seccomp issue where nanosleep is blocked, since
    select() uses kernel-level timing via the select/poll/epoll syscalls.

    Args:
        cmd: Command to run (list or string)
        timeout: Timeout in seconds (default: 5). None for no timeout.
        capture_output: Capture stdout/stderr (default: False)
        text: Return stdout/stderr as text instead of bytes (default: False)
        **kwargs: Additional arguments passed to Popen

    Returns:
        subprocess.CompletedProcess with returncode, stdout, stderr

    Raises:
        subprocess.TimeoutExpired: If the process times out
        FileNotFoundError: If the command is not found
    """
    if timeout is None:
        # No timeout - use standard subprocess.run without timeout
        return subprocess.run(cmd, capture_output=capture_output, text=text, **kwargs)

    # Set up Popen arguments
    popen_kwargs = kwargs.copy()
    if capture_output:
        popen_kwargs["stdout"] = subprocess.PIPE
        popen_kwargs["stderr"] = subprocess.PIPE

    proc = subprocess.Popen(cmd, **popen_kwargs)

    try:
        start_time = time.monotonic()
        stdout_chunks = []
        stderr_chunks = []

        # Make pipes non-blocking to avoid deadlock
        if capture_output:
            if proc.stdout:
                os.set_blocking(proc.stdout.fileno(), False)
            if proc.stderr:
                os.set_blocking(proc.stderr.fileno(), False)

        # Poll for process completion, using select() as our "wait" mechanism
        while True:
            # Calculate actual time remaining based on elapsed time
            elapsed = time.monotonic() - start_time
            time_remaining = timeout - elapsed

            if time_remaining <= 0:
                # Timeout reached - kill the process
                _safe_kill(proc)
                raise subprocess.TimeoutExpired(cmd, timeout)

            # Check if process finished first
            retcode = proc.poll()
            if retcode is not None:
                # Process finished - drain any remaining output
                if capture_output:
                    for fd, chunks in [(proc.stdout, stdout_chunks), (proc.stderr, stderr_chunks)]:
                        if fd:
                            try:
                                while True:
                                    data = fd.read(PIPE_READ_BUFFER_SIZE)
                                    if not data:
                                        break
                                    chunks.append(data)
                            except (IOError, OSError):
                                pass

                stdout = b"".join(stdout_chunks) if capture_output else None
                stderr = b"".join(stderr_chunks) if capture_output else None

                if text:
                    stdout = stdout.decode("utf-8", errors="replace") if stdout else ""
                    stderr = stderr.decode("utf-8", errors="replace") if stderr else ""

                return subprocess.CompletedProcess(cmd, retcode, stdout, stderr)

            # Build list of fds to monitor for reading
            read_fds = []
            if capture_output:
                if proc.stdout:
                    read_fds.append(proc.stdout)
                if proc.stderr:
                    read_fds.append(proc.stderr)

            # Wait for data or timeout using select()
            # This is the key: select() uses kernel timers, not nanosleep
            wait_time = min(POLL_INTERVAL, time_remaining)
            if read_fds:
                ready, _, _ = select.select(read_fds, [], [], wait_time)
                # Read any available data to prevent pipe buffer from filling
                for fd in ready:
                    try:
                        data = fd.read(PIPE_READ_BUFFER_SIZE)
                        if data:
                            if fd == proc.stdout:
                                stdout_chunks.append(data)
                            elif fd == proc.stderr:
                                stderr_chunks.append(data)
                    except (IOError, OSError):
                        pass
            else:
                # No pipes to monitor, just wait
                select.select([], [], [], wait_time)

    except subprocess.TimeoutExpired:
        raise
    except Exception:
        _safe_kill(proc)
        raise
