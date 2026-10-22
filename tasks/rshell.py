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
def generate_fuzz_pipeline(_ctx, output="rshell-fuzz-gitlab-ci.yml"):
    """Generate one GitLab fuzz job per rshell package."""
    root = Path("pkg/rshell")
    packages = sorted({Path(directory) for directory, _ in search_fuzz_tests(str(root))})
    if not packages:
        raise Exit("No rshell fuzz targets found")

    config = {"include": [{"local": ".gitlab/rshell/fuzz.yml"}]}
    for package in packages:
        config[f"rshell:fuzz:{package.relative_to(root).as_posix()}"] = {
            "extends": ".rshell:fuzz",
            "variables": {"FUZZ_PACKAGE": package.as_posix()},
        }
    Path(output).write_text(yaml.safe_dump(config, sort_keys=False))
    print(f"Generated {len(packages)} fuzz jobs in {output}")
