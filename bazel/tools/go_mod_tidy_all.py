"""Run `go mod tidy` concurrently across all modules in the workspace.

Processes ~180 go.mod files in ~4 seconds (with a warm Go module cache) using `asyncio`, while reproducing the default
`ThreadPoolExecutor._max_workers` heuristic (still a private API as of Python 3.12).
"""

import asyncio
import logging
import os
import sys
from datetime import timedelta
from subprocess import PIPE, CalledProcessError
from traceback import format_exception_only

from python.runfiles import runfiles


async def _exec(go, *args, **kwargs):
    proc = await asyncio.create_subprocess_exec(go, *args, **kwargs)
    try:
        stdout, _ = await proc.communicate()
    except BaseException:  # reap the process on any CancelledError, KeyboardInterrupt, SystemExit, TimeoutError, etc.
        if proc.returncode is None:
            try:
                proc.terminate()
            except ProcessLookupError:
                pass
            else:
                await asyncio.shield(proc.communicate())
        raise
    if proc.returncode == 0:
        return stdout
    raise CalledProcessError(proc.returncode, " ".join((os.path.basename(go), *args)), output=stdout)


async def _tidy(max_workers, go, mod_path, args):
    async with max_workers:
        await _exec(go, "mod", "tidy", "-C", mod_path, *args)


async def main(go, args):
    mod_paths = await _exec(go, "list", "-f", "{{.Dir}}", "-m", stdout=PIPE)
    max_workers = asyncio.Semaphore((os.cpu_count() or 1) + 4)  # TODO(regis): cpu_count -> Py 3.13's process_cpu_count
    # global timeout: on cold cache, per-task timeouts were unfairly hit because early tasks download most modules
    async with asyncio.timeout(timedelta(minutes=5).total_seconds()), asyncio.TaskGroup() as tg:
        for mod_path in mod_paths.decode().splitlines():
            tg.create_task(_tidy(max_workers, go, mod_path, args))


if __name__ == "__main__":
    logging.getLogger("asyncio").setLevel(logging.ERROR)  # no `Unknown child process pid N, will report returncode 255`
    try:
        asyncio.run(main(runfiles.Create().Rlocation(sys.argv[1]), sys.argv[2:]))
    except* BaseException as eg:
        sys.exit("\n".join(line.rstrip() for e in eg.exceptions for line in format_exception_only(e)))
