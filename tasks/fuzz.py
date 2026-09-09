"""
Helper for running fuzz targets
"""

import os

from invoke import task

from tasks.fuzz_infra import get_fuzz_build_tags
from tasks.schema.generate import schema_codegen


@task
def fuzz(ctx, fuzztime="10s"):
    """
    Run fuzz tests sequentially (default fuzztime is 10s for each target).
    This is a temporary approach until Go supports fuzzing multiple targets.
    See https://github.com/golang/go/issues/46312.
    """
    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    for directory, func in search_fuzz_tests(os.getcwd()):
        with ctx.cd(directory):
            tags = ",".join(get_fuzz_build_tags())
            cmd = f'go test -tags {tags} -v . -run={func} -fuzz={func}$ -fuzztime={fuzztime}'
            ctx.run(cmd)


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
