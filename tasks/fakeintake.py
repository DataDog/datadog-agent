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

# Paths whose changes rebuild the published fakeintake image (`go build
# cmd/server/main.go`): only server/, aggregator/, api/ and the module/Dockerfile.
# client/, cmd/client/ and docs/ do not enter the image, so they don't need a
# bump. Keep in sync with .fakeintake_server_paths in .gitlab-ci.yml.
SERVER_PATH_PREFIXES = (
    "test/fakeintake/cmd/server/",
    "test/fakeintake/server/",
    "test/fakeintake/aggregator/",
    "test/fakeintake/api/",
)
SERVER_FILES = (
    "test/fakeintake/go.mod",
    "test/fakeintake/go.sum",
    "test/fakeintake/Dockerfile",
)


def _is_server_file(path: str) -> bool:
    """True if changing `path` rebuilds the fakeintake image (needs a VERSION bump)."""
    return path in SERVER_FILES or path.startswith(SERVER_PATH_PREFIXES)


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
    Ensure test/fakeintake/version/VERSION is bumped whenever the fakeintake image changes.

    The pinned tag in VERSION is what e2e-framework's fakeintake defaults resolve to
    (see test/fakeintake/version). Only server-side changes rebuild the published image
    (see _is_server_file); such a merge must ship a strictly greater VERSION than its base
    branch so the newly published image gets a unique, immutable tag (see
    test/fakeintake/AGENTS.md). Client/CLI/docs changes don't touch the image, so they
    don't require a bump.
    """
    base_branch = os.environ.get("COMPARE_TO_BRANCH") or get_ancestor_base_branch()

    # Resolve the merge-base as a concrete commit. get_common_ancestor fetches the
    # base ref when it is missing (CI does shallow clones with S3 caching), which a
    # raw `git diff <base>...HEAD` cannot do — that fails with "unknown revision".
    merge_base = get_common_ancestor(ctx, "HEAD", base_branch)

    changed_files = [f.strip() for f in get_changed_files(ctx, base=merge_base) if f.strip()]
    server_changes = [f for f in changed_files if _is_server_file(f)]

    if not server_changes:
        print(color_message("No fakeintake image (server) changes detected, VERSION bump not required", "green"))
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
                f"fakeintake image changed ({len(server_changes)} server file(s), e.g. {server_changes[0]}) but "
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
