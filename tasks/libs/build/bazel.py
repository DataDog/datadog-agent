"""Provides helper functions for invoking Bazel commands."""

from __future__ import annotations

import asyncio
import codecs
import os
import shutil
import subprocess
import sys
from pathlib import Path
from typing import IO, NamedTuple

from tasks.libs.common.color import color_message
from tasks.libs.common.utils import get_repo_root, join_command


class Label(NamedTuple):
    """Component parts of a Bazel label."""

    repo: str | None
    package: str
    name: str | None


def split_label(label: str) -> Label:
    """Split a Bazel label into its component parts.

    The canonical format is ``@@repo//package/path:name``, where the repo
    prefix is optional and may carry one or two leading ``@`` characters.

    Returns a :class:`Label` namedtuple with:
    - ``repo``: the repository name, or ``None`` when absent (main workspace).
    - ``package``: the package path (empty string ``""`` for root labels such
      as ``//:foo``).
    - ``name``: the target name after ``:``, or ``None`` when omitted.
    """
    repo = None

    if label.startswith('@'):
        # Strip one or two leading '@' characters to reach "repo//…"
        rest = label.lstrip('@')
        slash_idx = rest.find('//')
        if slash_idx >= 0:
            repo_part = rest[:slash_idx]
            # Empty repo part (e.g. "@//" or "@@//") means the main workspace
            repo = repo_part if repo_part else None
            label = rest[slash_idx:]

    # label is now "//package:name" or "//package"
    if label.startswith('//'):
        label = label[2:]

    colon_idx = label.find(':')
    if colon_idx >= 0:
        package = label[:colon_idx]
        name = label[colon_idx + 1 :]
    else:
        package = label
        name = None

    return Label(repo=repo, package=package, name=name)


def package_from_path(path: str) -> str:
    """Return the Bazel package string corresponding to a filesystem path.

    - Relative paths are normalised: backslashes become forward slashes and
      any leading ``./`` is removed.
    - Absolute paths are made relative to the workspace root.

    Forward slashes are always used in the result, regardless of OS.
    """

    # Normalise backslashes to forward slashes before constructing Path so
    # that Windows-style separators are treated as directory separators on all
    # platforms (on POSIX, Path does not interpret '\' as a separator).
    normalised = path.replace('\\', '/')
    p = Path(normalised)
    if p.is_absolute():
        p = p.relative_to(get_repo_root())

    result = p.as_posix()
    # Path('.') represents "current directory" (i.e. the root package)
    if result == '.':
        return ''
    return result


def bazel_not_found_message(color: str) -> str:
    return color_message("Please run `inv install-tools` for `bazel` support!", color)


def _run_command(
    cmd: tuple[str, ...],
    *,
    capture_stdout: bool,
    capture_for_result: bool,
    env: dict[str, str] | None,
    input: str | None,
) -> subprocess.CompletedProcess[str]:
    # Merge the provided `env` with the current outside environment.
    subprocess_env = {**os.environ, **env} if env else None

    if capture_for_result:
        return _run_command_with_tee(
            cmd,
            input=input,
            env=subprocess_env,
            tee_stdout=not capture_stdout,
            tee_stderr=True,
        )

    return subprocess.run(
        cmd,
        env=subprocess_env,
        input=input,
        stdin=subprocess.DEVNULL if input is None else None,
        stdout=subprocess.PIPE if capture_stdout else None,
        stderr=None,
        encoding="utf-8",
        errors="backslashreplace",
    )


async def _collect_output(
    stream: asyncio.StreamReader,
    sink: IO[str] | None,
) -> str:
    decoder = codecs.getincrementaldecoder("utf-8")("backslashreplace")
    chunks: list[str] = []
    while data := await stream.read(8192):
        chunk = decoder.decode(data)
        chunks.append(chunk)
        if sink is not None:
            sink.write(chunk)
            sink.flush()

    tail = decoder.decode(b"", final=True)
    if tail:
        chunks.append(tail)
        if sink is not None:
            sink.write(tail)
            sink.flush()

    return "".join(chunks)


async def _run_command_with_tee_async(
    cmd: tuple[str, ...],
    *,
    input: str | None,
    env: dict[str, str] | None,
    tee_stdout: bool,
    tee_stderr: bool,
) -> subprocess.CompletedProcess[str]:
    proc = await asyncio.create_subprocess_exec(
        *cmd,
        env=env,
        stdin=asyncio.subprocess.PIPE if input is not None else subprocess.DEVNULL,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )

    # Manage stdout and stderr in separate async tasks that tee output while collecting it.
    output_tasks = [
        asyncio.create_task(_collect_output(proc.stdout, sys.stdout if tee_stdout else None)),
        asyncio.create_task(_collect_output(proc.stderr, sys.stderr if tee_stderr else None)),
    ]

    # Feed input to the process stdin
    if input is not None and proc.stdin is not None:
        try:
            proc.stdin.write(input.encode("utf-8"))
            await proc.stdin.drain()
        except (BrokenPipeError, ConnectionResetError):
            pass
        finally:
            proc.stdin.close()
            await proc.stdin.wait_closed()

    returncode = await proc.wait()
    stdout, stderr = await asyncio.gather(*output_tasks)

    return subprocess.CompletedProcess(cmd, returncode, stdout, stderr)


def _run_command_with_tee(
    cmd: tuple[str, ...],
    *,
    input: str | None,
    env: dict[str, str] | None,
    tee_stdout: bool,
    tee_stderr: bool,
) -> subprocess.CompletedProcess[str]:
    return asyncio.run(
        _run_command_with_tee_async(
            cmd,
            input=input,
            env=env,
            tee_stdout=tee_stdout,
            tee_stderr=tee_stderr,
        )
    )


def bazel(
    *args: str,
    capture_output: bool = False,
    env: dict[str, str] | None = None,
    ignore_errors: bool = False,
    input: str | None = None,
    sudo: bool = False,
) -> str | subprocess.CompletedProcess[str]:
    """Execute a bazel command.

    env: environment variables when passing them through the corresponding Bazel `--*_env=` flags is not suitable.
    input: text to pass to the Bazel subprocess stdin.
    ignore_errors: do not fail fast, but instead return the raw `CompletedProcess`, whether the Bazel command
        succeeded or not.
    """

    if not (bazelisk := shutil.which("bazelisk")):  # `/usr/bin/bazel` may otherwise take precedence in DD Workspaces
        raise SystemExit(bazel_not_found_message("red"))
    cmd = (("sudo",) if sudo else ()) + (bazelisk, *_insert_omnibazel_flags(args))
    cmdline = join_command(cmd)
    print(color_message(cmdline.replace(bazelisk, "bazel", 1), "bold"), file=sys.stderr)  # brevity: abspath -> bazel

    completed = _run_command(
        cmd,
        capture_stdout=capture_output,
        capture_for_result=ignore_errors,
        env=env,
        input=input,
    )
    if ignore_errors:
        return completed
    if completed.returncode != 0:
        raise SystemExit(completed.returncode)
    return completed.stdout if capture_output else ""


def build_binary_with_bazel(
    target: str, args: list[str] | None = None, bin_path: str = None, embedded_path: str | None = None
) -> None:
    """Build a Bazel target and copy its output to bin_path.

    Args:
        target: Bazel target
        args: extra arguments passed to both the build and cquery invocations
        bin_path: directory to copy the binary to. None for no copy.
        embedded_path: directory holding shared libraries the binary links
        against.
        When set, rewrites the copied binary's RPATH to point there instead of
        Bazel's sandbox paths.
    """
    args = args or []
    bazel("build", target, *args)
    # We need cquery to find the output path that has the configuration hash in it.
    output = bazel("cquery", "--output=files", target, *args, capture_output=True).strip()
    outputs = [line for line in output.splitlines() if line]
    if len(outputs) != 1:
        raise SystemExit(f"Expected exactly one output file for Bazel target {target!r}, got: {outputs!r}")
    src = os.path.join(get_repo_root(), outputs[0])

    if bin_path:
        os.makedirs(os.path.dirname(bin_path), exist_ok=True)
        shutil.copy2(src, bin_path)
        os.chmod(bin_path, 0o755)
        uid = os.environ.get("HOST_UID", "-1")
        gid = os.environ.get("HOST_GID", "-1")
        if uid != "-1" and gid != "-1":
            os.chown(bin_path, int(uid), int(gid))

        if embedded_path:
            # `bazel run` executes with the execution root as cwd, not the caller's cwd,
            # so bin_path must be absolute for the tool to find it.
            bazel("run", "//bazel/rules:replace_prefix", "--", "--prefix", embedded_path, os.path.abspath(bin_path))


def _insert_omnibazel_flags(args: tuple[str, ...]) -> tuple[str, ...]:
    """Insert --//packages/agent:flavor, --//:install_dir and --//:output_config_dir, pinned from the corresponding
    omnibus build environment variables.
    💡 Mirrors `omnibazel_flags` in omnibus/lib/ostools.rb.
    """
    flags = []
    if agent_flavor := os.environ.get("AGENT_FLAVOR"):
        flags.append(f"--//packages/agent:flavor={agent_flavor}")
    if install_dir := os.environ.get("INSTALL_DIR"):
        # In macos, omnibus install_dir is the build location, which is different from the expected install location
        if sys.platform == "darwin":
            flags.append("--//:install_dir=/opt/datadog-agent")
        else:
            flags.append(f"--//:install_dir={install_dir}")
        flags.append(f"--//:output_config_dir={os.environ.get("OUTPUT_CONFIG_DIR", "")}")
    if not flags:
        return args
    # insert flags right after the bazel command, preserving startup options before it and subcommand arguments after it
    index = next((i for i, a in enumerate(args, 1) if not a.startswith("-")), len(args))
    return (*args[:index], *flags, *args[index:])
