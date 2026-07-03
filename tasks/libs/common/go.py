from __future__ import annotations

import io
import os
import os.path
import platform
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from time import sleep
from typing import cast

from invoke.context import Context, MockContext
from invoke.exceptions import Exit
from invoke.runners import Local, Result

from tasks.libs.build.bazel import bazel
from tasks.libs.common.retry import run_command_with_retry
from tasks.libs.common.utils import timed


def download_go_dependencies(
    ctx: Context, paths: list[str], verbose: bool = False, max_retry: int = 3, max_workers: int = 8
):
    """
    Download and tidy Go dependencies for all modules in parallel.

    Args:
        ctx: Invoke context
        paths: List of paths to Go modules
        verbose: Enable verbose output
        max_retry: Maximum retries per module (uses exponential backoff: 10s, 100s, 1000s)
        max_workers: Maximum parallel workers (default: 8)
    """
    if not paths:
        return

    if max_retry < 1:
        max_retry = 1

    verbosity = ' -x' if verbose else ''
    cmd = f"go mod download{verbosity} && go mod tidy{verbosity}"

    # Sequential path for MockContext (tests) - ctx.run() isn't thread-safe
    if isinstance(ctx, MockContext) or max_workers == 1:
        for path in paths:
            with ctx.cd(path):
                run_command_with_retry(ctx, cmd, max_retry=max_retry)
        return

    # Parallel path for production - uses subprocess
    print(f"downloading dependencies for {len(paths)} modules (max {max_workers} parallel workers)")

    def process_module(path: str):
        """Process a single module. Raises Exit on failure after retries."""
        result = None
        for attempt in range(max_retry):
            result = subprocess.run(cmd, shell=True, cwd=path, capture_output=True, text=True)
            if result.returncode == 0:
                if verbose:
                    print(f"  [ok] {path}")
                return
            if attempt < max_retry - 1:
                wait = 10 ** (attempt + 1)
                print(f"  [{attempt + 1}/{max_retry}] Failed `{cmd}` in {path}, retrying in {wait}s")
                sleep(wait)
        error_output = result.stderr or result.stdout if result else "unknown error"
        raise Exit(f"go mod failed for {path}: {error_output}", code=1)

    with timed("go mod download && go mod tidy"):
        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            futures = [executor.submit(process_module, path) for path in paths]
            for future in as_completed(futures):
                future.result()  # Raises if module failed


def _with_pdb_extldflag(ldflags: str, bin_path: str) -> str:
    """
    Insert ``-Wl,--pdb=<abs(bin_path)>.pdb`` into the linker's external-
    linker flags so mingw ld emits a PDB next to the binary.

    `tasks/libs/common/utils.py:get_build_flags` emits its accumulated
    extldflags as a single-quoted whole arg ``'-extldflags=...'``. If
    that group is present we splice our flag in just before the closing
    quote, so the Go linker still sees one ``-extldflags=`` token (it
    honors only the last one). Otherwise we append a fresh
    ``'-extldflags=<our flag>'``.
    """
    pdb_flag = f"-Wl,--pdb={os.path.abspath(bin_path)}.pdb"
    open_marker = "'-extldflags="
    open_idx = ldflags.find(open_marker)
    if open_idx != -1:
        close_idx = ldflags.find("'", open_idx + len(open_marker))
        if close_idx != -1:
            return ldflags[:close_idx] + " " + pdb_flag + ldflags[close_idx:]
    suffix = f" '-extldflags={pdb_flag}'"
    return (ldflags + suffix) if ldflags else suffix.lstrip()


def _with_hermetic_mingw_path(ctx: Context, env: dict[str, str] | None) -> dict[str, str] | None:
    """
    Prepend the Bazel hermetic MinGW (GNU ld >= 2.44) to PATH for a Windows cgo
    build, so the linker emits PDBs that Microsoft dbghelp/symstore can read. The
    build image's default mingw is ld 2.43, whose `--pdb` output those tools
    can't parse (WINA-2770). Returns env unchanged if it can't be resolved.

    TODO: remove once migrated fully to the Bazel MinGW toolchain.
    """
    # bazel cquery is idempotent: it fetches/extracts @winlibs_mingw64 only if missing.
    if not (
        gcc := bazel(ctx, "cquery", "@winlibs_mingw64//:gcc", "--output=files", capture_output=True, ignore_errors=True)
    ):
        return env
    if not (output_base := bazel(ctx, "info", "output_base", capture_output=True, ignore_errors=True)):
        return env
    mingw_bin = Path(output_base.strip(), gcc.strip()).parent
    path = (env or {}).get("PATH") or os.environ.get("PATH", "")
    return {**(env or {}), "PATH": f"{mingw_bin}{os.pathsep}{path}"}


def go_build(
    ctx: Context,
    entrypoint: str | Path,
    mod: str | None = None,
    race: bool = False,
    gcflags: str | None = None,
    ldflags: str | None = None,
    build_tags: list[str] | None = None,
    rebuild: bool = False,
    env: dict[str, str] | None = None,
    bin_path: str | Path | None = None,
    verbose: bool = False,
    echo: bool = False,
    check_deadcode: bool = False,
    coverage: bool = False,
    trimpath: bool = True,
) -> Result:
    # When targeting Windows with a known output path, ensure the parent
    # directory exists and ask mingw ld to emit a PDB next to the binary
    # so cdb/WPA/xperf can resolve Go symbols. ld writes the PDB during
    # the link step — before `go build` copies the .exe to bin_path —
    # which is why the pre-create is needed.
    target_is_windows = (
        sys.platform == "win32" or (env or {}).get("GOOS") == "windows" or os.getenv("GOOS") == "windows"
    )
    if bin_path and target_is_windows:
        os.makedirs(os.path.dirname(os.path.abspath(str(bin_path))) or ".", exist_ok=True)
        if os.environ.get("DD_GO_PDB", "1") != "0":
            ldflags = _with_pdb_extldflag(ldflags or "", str(bin_path))
            if sys.platform == "win32":
                env = _with_hermetic_mingw_path(ctx, env)

    cmd = "go build"
    if coverage:
        cmd += " -cover -covermode=atomic"
        build_tags = build_tags or []
        build_tags.append("e2ecoverage")
    if mod:
        cmd += f" -mod={mod}"
    if race:
        cmd += " -race"
    if rebuild:
        cmd += " -a"
    if verbose:
        cmd += " -v"
    if echo:
        cmd += " -x"
    if build_tags:
        cmd += f" -tags \"{' '.join(build_tags)}\""
    if bin_path:
        cmd += f" -o {bin_path}"
    if gcflags:
        cmd += f" -gcflags=\"{gcflags}\""
    if check_deadcode:
        ldflags = (ldflags or "") + " -dumpdep"
    if ldflags:
        cmd += f" -ldflags=\"{ldflags}\""
    if trimpath and 'DELVE' not in os.environ:
        cmd += " -trimpath"

    cmd += f" {entrypoint}"

    if check_deadcode:
        result = _handle_pipe_to_whydeadcode(ctx, os.path.basename(entrypoint), cmd, env)
    else:
        result = cast(Result, ctx.run(cmd, env=env))

    if sys.platform == "win32" or result.exited != 0 or bin_path is None:
        return result

    if os.path.exists(bin_path):
        uid = os.environ.get("HOST_UID", "-1")
        gid = os.environ.get("HOST_GID", "-1")
        if uid != "-1" and gid != "-1":
            os.chown(bin_path, int(uid), int(gid))

    return result


def _handle_pipe_to_whydeadcode(ctx: Context, name: str, cmd: str, env: dict[str, str] | None = None) -> Result:
    """
    - Runs `go build` with the `dumpdep` flag in a custom runner. This runner reads big chunks to improve invoke I/O performance, see https://github.com/pyinvoke/invoke/issues/774
    - Calls `whydeadcode` in the same runner using an associated custom reader.
    """
    runner = Local(ctx)
    runner.read_chunk_size = 1024 * 1024 * 10
    _ = runner.read_chunk_size  # please linters
    runner.input_sleep = 0
    _ = runner.input_sleep  # please linters

    # -dumpdep is very verbose so we hide that
    # any unrecognized log line is shown by whydeadcode anyway
    result = cast(Result, runner.run(cmd, env=env, hide="stderr"))

    # worst case it's already installed and nothing happens
    with ctx.cd("internal/tools"):
        # pass the env to the command so that it can check GOPATH/GOBIN if provided
        ctx.run("go install github.com/aarzilli/whydeadcode", env=env)

    # whydeadcode prints unexpected input on stderr (eg. build warnings), and
    # dead code call stack on stdout
    # it returns non-zero if non-expected input is passed, and 0 otherwise, even if dead code elimination is disabled
    # so we check whether stdout is empty to know if dead code elimination is disabled
    whydeadcoderes = cast(
        Result, runner.run("whydeadcode", in_stream=CustomReader(result.stderr), warn=True, hide="out", env=env)
    )
    if whydeadcoderes.stdout:
        print(
            f"dead code elimination is disabled for {name} on {sys.platform} {platform.machine()} by the following call stack (only the first one is guaranteed to be a true positive):\n{whydeadcoderes.stdout}"
        )

    return result


class CustomReader(io.StringIO):
    """
    Custom reader to read 10MiB at a time.
    This is a workaround to increase invoke performance at reading from stdin
    See https://github.com/pyinvoke/invoke/issues/819
    """

    def __init__(self, data: str):
        super().__init__(data)

    def read(self, n: int | None = None) -> str:
        return super().read(1024 * 1024 * 10)
