"""Run `go mod tidy` concurrently across all modules in the workspace.

Processes ~180 go.mod files in ~4 seconds (with a warm Go module cache) using `asyncio`, while reproducing the default
`ThreadPoolExecutor._max_workers` heuristic (still a private API as of Python 3.12).
"""

import asyncio
import os
import sys
from subprocess import PIPE, CalledProcessError

from python.runfiles import runfiles


async def _exec(go, *args, **kwargs):
    proc = await asyncio.create_subprocess_exec(go, *args, **kwargs)
    stdout, _ = await proc.communicate()
    if proc.returncode == 0:
        return stdout
    raise CalledProcessError(proc.returncode, " ".join(("go", *args)))


async def _tidy(max_workers, go, mod_path, args):
    async with max_workers, asyncio.timeout(30):
        await _exec(go, "mod", "tidy", "-C", mod_path, *args)


async def main(go, args):
    mod_paths = await _exec(go, "list", "-f", "{{.Dir}}", "-m", stdout=PIPE)
    max_workers = asyncio.Semaphore((os.cpu_count() or 1) + 4)  # TODO(regis): cpu_count -> Py 3.13's process_cpu_count
    async with asyncio.TaskGroup() as tg:
        for mod_path in mod_paths.decode().splitlines():
            tg.create_task(_tidy(max_workers, go, mod_path, args))


if __name__ == "__main__":
    try:
        asyncio.run(main(runfiles.Create().Rlocation(sys.argv[1]), sys.argv[2:]))
    except* Exception as eg:
        sys.exit("\n".join(str(e) for e in eg.exceptions))
