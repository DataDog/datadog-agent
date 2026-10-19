# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Prepare bounded dependency updates for GitHub's signed-commit API."""

import base64
import re
import subprocess
from dataclasses import dataclass

from tasks.libs.common.git import get_commit_sha
from tasks.libs.common.utils import join_command


@dataclass(frozen=True)
class TidyUpdate:
    file_changes: dict
    expected_tree: str


def _git(ctx, *args):
    return ctx.run(join_command(["git", *args]), hide=True).stdout.strip()


def staged_changes(raw_diff):
    """Parse NUL-delimited paths and reject modes the publication API cannot preserve."""
    entries = raw_diff.rstrip("\0").split("\0")
    if len(entries) % 2:
        raise RuntimeError("Malformed staged Git diff")
    changes = []
    for index in range(0, len(entries), 2):
        header, path = entries[index : index + 2]
        old_mode, new_mode, *_ = header.removeprefix(":").split()
        if not (
            new_mode == "000000"
            or (old_mode == "000000" and new_mode == "100644")
            or (old_mode == new_mode and new_mode in {"100644", "100755"})
        ):
            raise RuntimeError(
                f"Cannot publish the mode change {old_mode} -> {new_mode} for {path!r}. "
                "The signed-commit publisher cannot preserve it; fix the tracked mode explicitly."
            )
        changes.append((path, new_mode))
    return changes


def file_changes(raw_diff):
    additions, deletions = [], []
    for path, new_mode in staged_changes(raw_diff):
        if new_mode == "000000":
            deletions.append({"path": path})
        else:
            # Invoke decodes stdout as text. Read index blobs as bytes so even
            # non-UTF-8 generated files are published without changing contents.
            contents = subprocess.check_output(["git", "show", f":{path}"])
            additions.append({"path": path, "contents": base64.b64encode(contents).decode("ascii")})
    return {"additions": additions, "deletions": deletions}


def prepare_tidy_update(ctx, source_sha, branch):
    head = get_commit_sha(ctx)
    if head != source_sha:
        raise RuntimeError("Checkout does not match the pull-request event revision")
    remote = _git(ctx, "ls-remote", "--exit-code", "origin", f"refs/heads/{branch}").split()[0]
    if remote != head:
        print("Branch advanced; leaving the new revision to its own workflow run")
        return None

    message = _git(ctx, "log", "-1", "--format=%B")
    # Only stage tracked files, as before; tool caches must not enter the commit.
    _git(ctx, "add", "--update")
    diff = _git(ctx, "diff", "--cached", "--raw", "--no-abbrev", "--no-renames", "-z")
    if not diff:
        print("No changes to commit")
        return None
    if re.search(r"^Deps-Tidy-Source: [0-9a-f]{40}$", message, re.MULTILINE):
        raise RuntimeError(
            "Dependency generation still changes this workflow's own output. "
            "Refusing another automated commit; fix the generator instead of retrying."
        )
    return TidyUpdate(file_changes(diff), _git(ctx, "write-tree"))
