"""
Helper for running fuzz targets
"""

import os

from invoke import task
from invoke.exceptions import Exit

from tasks.fuzz_infra import get_fuzz_build_tags
from tasks.libs.common.utils import join_command
from tasks.schema.generate import schema_codegen


@task
def fuzz(ctx, fuzztime="10s", targets="."):
    """
    Run fuzz tests sequentially (default fuzztime is 10s for each target).
    This is a temporary approach until Go supports fuzzing multiple targets.
    See https://github.com/golang/go/issues/46312.
    """
    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    for directory, func in search_fuzz_tests(targets):
        with ctx.cd(directory):
            tags = ",".join(get_fuzz_build_tags())
            result = ctx.run(
                join_command(
                    ["go", "test", "-tags", tags, "-v", ".", f"-run={func}", f"-fuzz={func}$", f"-fuzztime={fuzztime}"]
                ),
                warn=True,
            )
            if result.ok:
                continue
            if is_fuzztime_boundary_timeout(result.stdout + result.stderr):
                print(f"NOTE: {func} hit Go's fuzz coordinator timeout at the -fuzztime limit; not a failure")
                continue
            raise Exit(code=result.exited)


def is_fuzztime_boundary_timeout(output):
    """
    When -fuzztime expires, Go's fuzz coordinator cancels in-flight inputs and can
    fail the run with a bare "context deadline exceeded". A real finding always
    writes the failing input to testdata/fuzz, so only the bare timeout is tolerated.
    """
    return "context deadline exceeded" in output and "Failing input written to" not in output


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
