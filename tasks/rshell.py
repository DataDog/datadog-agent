"""CI tasks for the restricted shell module."""

from pathlib import Path

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.fuzz import search_fuzz_tests
from tasks.libs.common.utils import join_command


@task
def verify_source_files(ctx):
    """Check that source-scanning tests receive every Go source file."""
    result = ctx.run(
        join_command(["bazel", "query", 'kind("source file", deps(//pkg/rshell:source_files))', "--output=label"]),
        hide=True,
    )
    declared = {label.removeprefix("//").replace(":", "/") for label in result.stdout.splitlines()}
    expected = {str(path) for path in Path("pkg/rshell").rglob("*.go") if "testdata" not in path.parts}
    missing = sorted(expected - declared)
    if missing:
        raise Exit(f"Add these files to //pkg/rshell:source_files: {missing}")
    print(f"All {len(expected)} Go source files are included in //pkg/rshell:source_files")


@task
def verify_fuzz_matrix(_ctx, config=".gitlab/test/rshell.yml"):
    """Check that the rshell:fuzz matrix lists exactly the packages with fuzz targets."""
    expected = {Path(directory).as_posix() for directory, _ in search_fuzz_tests("pkg/rshell")}
    if not expected:
        raise Exit("No rshell fuzz targets found")
    jobs = yaml.load(Path(config).read_text(), Loader=_GitLabLoader)
    listed = {package for entry in jobs["rshell:fuzz"]["parallel"]["matrix"] for package in entry["FUZZ_PACKAGE"]}
    missing, stale = sorted(expected - listed), sorted(listed - expected)
    if missing or stale:
        raise Exit(f"Update the rshell:fuzz matrix in {config}: add {missing}, remove {stale}")
    print(f"The rshell:fuzz matrix covers all {len(expected)} fuzz packages")


class _GitLabLoader(yaml.SafeLoader):
    """Safe loader that tolerates GitLab's !reference tags."""


_GitLabLoader.add_constructor("!reference", lambda loader, node: loader.construct_sequence(node))
