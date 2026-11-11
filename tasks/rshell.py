"""CI tasks for the restricted shell module."""

from pathlib import Path

from invoke import task
from invoke.exceptions import Exit

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
