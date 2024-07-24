from __future__ import annotations

from time import sleep
from typing import TYPE_CHECKING

from tasks.libs.common.utils import timed

if TYPE_CHECKING:
    from invoke import Context


def download_go_dependencies(ctx: Context, paths: list[str], verbose: bool = False, max_retry: int = 3):
    print("downloading dependencies")
    with timed("go mod download"):
        verbosity = ' -x' if verbose else ''
        for path in paths:
            with ctx.cd(path):
                for retry in range(max_retry):
                    result = ctx.run(f"go mod download{verbosity}")
                    if result.exited is None or result.exited > 0:
                        wait = 10 ** (retry + 1)
                        print(f"[{retry + 1} / {max_retry}] Failed downloading {path}, retrying in {wait} seconds")
                        sleep(wait)
                        continue
                    break
