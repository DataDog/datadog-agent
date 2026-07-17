"""
Build or use the fake intake client CLI
"""

import os

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_ancestor_base_branch, get_changed_files, get_common_ancestor
from tasks.libs.common.go import go_build

VERSION_FILE = "test/fakeintake/version/VERSION"


@task
def build(ctx):
    """
    Build the fake intake
    """
    with ctx.cd("test/fakeintake"):
        go_build(ctx, "cmd/server/main.go", bin_path="build/fakeintake")
        go_build(ctx, "cmd/client/main.go", bin_path="build/fakeintakectl")


@task
def test(ctx):
    """
    Run the fake intake tests
    """
    with ctx.cd("test/fakeintake"):
        ctx.run("go test ./...")


def _parse_version(raw: str) -> int:
    version = raw.strip()
    if not version.startswith("v") or not version[1:].isdigit():
        raise Exit(
            code=1,
            message=color_message(
                f"Invalid {VERSION_FILE} content {raw!r}: expected a 'v<int>' tag (e.g. 'v1')", "red"
            ),
        )
    return int(version[1:])


@task
def check_version_bump(ctx):
    """
    Ensure test/fakeintake/version/VERSION is bumped whenever fakeintake code changes.

    The pinned tag in VERSION is what e2e-framework's fakeintake defaults resolve to
    (see test/fakeintake/version). Every merge that touches test/fakeintake/ must ship
    a strictly greater VERSION than its base branch so the newly published image gets a
    unique, immutable tag (see test/fakeintake/AGENTS.md).
    """
    base_branch = os.environ.get("COMPARE_TO_BRANCH") or get_ancestor_base_branch()

    # Resolve the merge-base as a concrete commit. get_common_ancestor fetches the
    # base ref when it is missing (CI does shallow clones with S3 caching), which a
    # raw `git diff <base>...HEAD` cannot do — that fails with "unknown revision".
    merge_base = get_common_ancestor(ctx, "HEAD", base_branch)

    changed_files = [f.strip() for f in get_changed_files(ctx, base=merge_base) if f.strip()]
    fakeintake_changes = [f for f in changed_files if f.startswith("test/fakeintake/") and f != VERSION_FILE]

    if not fakeintake_changes:
        print(color_message("No test/fakeintake/ changes detected, VERSION bump not required", "green"))
        return

    with open(VERSION_FILE) as f:
        new_version_raw = f.read()
    new_version = _parse_version(new_version_raw)

    # The VERSION file may not exist at the merge-base yet — this is the case on the
    # PR that first introduces the pinning scheme, and after any baseline reset.
    # `git show` exits non-zero then, so use warn=True and treat a missing base file
    # as version 0 so the initial bump (v1+) passes instead of crashing.
    base_version_result = ctx.run(f"git show {merge_base}:{VERSION_FILE}", hide=True, warn=True)
    base_version = _parse_version(base_version_result.stdout) if base_version_result.ok else 0

    if new_version <= base_version:
        raise Exit(
            code=1,
            message=color_message(
                f"test/fakeintake/ changed ({len(fakeintake_changes)} file(s), e.g. {fakeintake_changes[0]}) but "
                f"{VERSION_FILE} was not bumped: it is 'v{new_version}', which must be strictly greater than "
                f"{base_branch}'s 'v{base_version}'. Bump {VERSION_FILE} to at least 'v{base_version + 1}' in this PR.",
                "red",
            ),
        )

    print(
        color_message(
            f"{VERSION_FILE} bumped from 'v{base_version}' to 'v{new_version}', OK",
            "green",
        )
    )
