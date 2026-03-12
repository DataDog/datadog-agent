"""
Helper for running fuzz targets
"""

import itertools
import os
from pathlib import Path
import re
import typing as tp

from invoke import task


@task
def fuzz(ctx, fuzztime="10s", filter=None, rounds=1, list=False):
    """
    Run fuzz tests sequentially (default fuzztime is 10s for each target).
    This is a temporary approach until Go supports fuzzing multiple targets.
    See https://github.com/golang/go/issues/46312.

    --list     Print all discovered targets (pkg:FuzzFunc), one per line, and exit.

    --filter   Python regex matched against fuzz function names

    --rounds   Number of full cycles when --filter is given
    """
    cwd =  Path(os.getcwd())
    all_targets = [*search_fuzz_tests(cwd, cwd)]

    if list:
        for directory, func in all_targets:
            print(f"{directory}:{func}")
        return

    if filter:
        pattern = re.compile(filter)
        matched = [(directory, func) for directory, func in all_targets if pattern.search(f"{directory}:{func}")]
        if not matched:
            print(f"No fuzz targets matched pattern {filter!r}")
            return
    else:
        matched = all_targets

    assert rounds>=1
    sequence = itertools.islice(itertools.cycle(matched), rounds * len(matched))

    for directory, func in sequence:
        with ctx.cd(directory):
            cmd = f'go test -tags test -v . -run={func} -fuzz={func}$ -fuzztime={fuzztime}'
            ctx.run(cmd)

def search_fuzz_tests(directory: Path, base: Path) -> tp.Iterator[tuple[Path, str]]:
    """
    Yields (directory, fuzz function name) tuples.
    directory paths are relative to base
    """
    for file in os.listdir(directory):
        path= directory / file
        if path.is_dir():
            yield from search_fuzz_tests(path, base)
        else:
            if not file.endswith('_test.go'):
                continue
            with open(path) as f:
                for line in f.readlines():
                    if line.startswith('func Fuzz'):
                        fuzzfunc = line[5 : line.find('(')]  # 5 is len('func ')
                        yield directory.relative_to(base), fuzzfunc
