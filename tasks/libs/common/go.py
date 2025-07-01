from __future__ import annotations

from pathlib import Path

from invoke import Context
from invoke.exceptions import Exit
from invoke.runners import Result

from tasks.libs.common.retry import run_command_with_retry
from tasks.libs.common.utils import timed


def download_go_dependencies(ctx: Context, paths: list[str], verbose: bool = False, max_retry: int = 3):
    print("downloading dependencies")
    with timed("go mod download && go mod tidy"):
        verbosity = ' -x' if verbose else ''
        for path in paths:
            with ctx.cd(path):
                run_command_with_retry(
                    ctx, f"go mod download{verbosity} && go mod tidy{verbosity}", max_retry=max_retry
                )


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
) -> Result:
    cmd = "go build"
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

    cmd += f" {entrypoint}"
    # -dumpdep is very verbose so we hide that
    # any unrecognized log line is shown by whydeadcode anyway
    res = ctx.run(cmd, env=env, hide="stderr" if check_deadcode else None)

    if check_deadcode:
        # whydeadcode prints unexpected input on stderr (eg. build warnings), and
        # dead code call stack on stdout
        # it returns non-zero if non-expected input is passed, and 0 otherwise, even if dead code elimination is disabled
        # so we check whether stdout is empty to know if dead code elimination is disabled
        res = ctx.run("whydeadcode", in_stream=res.stderr, warn=True, hide="stdout")
        if res.stdout:
            raise Exit(f"dead code elimination is disabled by the following call stack:\n{res.stdout}")

    return res
