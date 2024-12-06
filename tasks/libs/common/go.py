from __future__ import annotations

from typing import TYPE_CHECKING

from tasks.libs.common.retry import run_command_with_retry
from tasks.libs.common.utils import timed

if TYPE_CHECKING:
    from invoke import Context


def download_go_dependencies(ctx: Context, paths: list[str], verbose: bool = False, max_retry: int = 3):
    print("downloading dependencies")
    with timed("go mod download && go mod tidy"):
        verbosity = ' -x' if verbose else ''
        for path in paths:
            with ctx.cd(path):
                run_command_with_retry(
                    ctx, f"go mod download{verbosity} && go mod tidy{verbosity}", max_retry=max_retry
                )
