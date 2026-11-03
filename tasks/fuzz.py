"""
Helper for running fuzz targets
"""

import os
import subprocess
from concurrent.futures import ThreadPoolExecutor

from invoke import task
from invoke.exceptions import Exit

from tasks.fuzz_infra import get_fuzz_build_tags
from tasks.libs.common.utils import join_command
from tasks.schema.generate import schema_codegen


@task
def fuzz(ctx, fuzztime="10s", targets=".", jobs=1):
    """
    Run fuzz tests (default fuzztime is 10s for each target).
    Go fuzzes one target per run until it supports fuzzing multiple targets
    (https://github.com/golang/go/issues/46312), so targets run sequentially, or
    --jobs at a time with one fuzz worker each.
    """
    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    tags = ",".join(get_fuzz_build_tags())
    tests = list(search_fuzz_tests(targets))
    jobs = int(jobs)
    if jobs <= 1:
        for directory, func in tests:
            with ctx.cd(directory):
                result = ctx.run(join_command(fuzz_command(tags, func, fuzztime)), warn=True)
            if not check_fuzz_result(func, result.ok, result.stdout + result.stderr):
                raise Exit(code=result.exited)
        return

    def run(test):
        directory, func = test
        command = fuzz_command(tags, func, fuzztime, "-parallel=1")
        result = subprocess.run(command, cwd=directory, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        print(f"=== {directory} {func}\n{result.stdout}", flush=True)
        return check_fuzz_result(func, result.returncode == 0, result.stdout)

    with ThreadPoolExecutor(max_workers=jobs) as pool:
        passed = list(pool.map(run, tests))
    failed = [f"{directory} {func}" for (directory, func), ok in zip(tests, passed, strict=True) if not ok]
    if failed:
        raise Exit("Fuzz targets failed:\n" + "\n".join(failed))


def fuzz_command(tags, func, fuzztime, *extra_args):
    return [
        "go",
        "test",
        "-tags",
        tags,
        "-v",
        ".",
        f"-run={func}",
        f"-fuzz={func}$",
        f"-fuzztime={fuzztime}",
        *extra_args,
    ]


def check_fuzz_result(func, ok, output):
    if ok:
        return True
    if is_fuzztime_boundary_timeout(func, output):
        print(f"NOTE: {func} hit Go's fuzz coordinator timeout at the -fuzztime limit; not a failure")
        return True
    return False


def is_fuzztime_boundary_timeout(func, output):
    """
    When -fuzztime expires, Go's fuzz coordinator cancels in-flight inputs and can
    fail the run with a bare "context deadline exceeded" under "--- FAIL: <func>".
    Only that exact failure is tolerated: a real finding writes the failing input
    to testdata/fuzz, and a t.Fatal or a panic prints more than the bare message.
    """
    if "Failing input written to" in output or "panic:" in output:
        return False
    lines = output.splitlines()
    for i, line in enumerate(lines):
        if line.startswith(f"--- FAIL: {func} ("):
            details = []
            for detail in lines[i + 1 :]:
                if not detail.startswith((" ", "\t")):
                    break
                details.append(detail.strip())
            return details == ["context deadline exceeded"]
    return False


def search_fuzz_tests(directory):
    """
    Yields (directory, fuzz function name) tuples.
    """
    for file in os.listdir(directory):
        path = os.path.join(directory, file)
        if os.path.isdir(path):
            yield from search_fuzz_tests(path)
        else:
            if not file.endswith('_test.go'):
                continue
            with open(path) as f:
                for line in f.readlines():
                    if line.startswith('func Fuzz'):
                        fuzzfunc = line[5 : line.find('(')]  # 5 is len('func ')
                        yield directory, fuzzfunc
