# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Bound dependency automation to one published update per input revision."""

import base64
import json
import os
import re
import subprocess
from pathlib import Path


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def validate_changes(raw_diff):
    """GitHub's signed-commit API publishes file contents, not Git file modes."""
    entries = raw_diff.rstrip("\0").split("\0")
    if len(entries) % 2:
        raise RuntimeError("Malformed staged Git diff")
    for index in range(0, len(entries), 2):
        header, path = entries[index : index + 2]
        old_mode, new_mode, *_ = header.removeprefix(":").split()
        if new_mode == "000000":  # Deletions do not need mode support.
            continue
        if (old_mode == "000000" and new_mode == "100644") or (
            old_mode == new_mode and new_mode in {"100644", "100755"}
        ):
            continue
        raise RuntimeError(
            f"Cannot publish the mode change {old_mode} -> {new_mode} for {path!r}. "
            "The signed-commit publisher cannot preserve it; fix the tracked mode explicitly."
        )


def publication_payload(source_sha, repository, branch, message, raw_diff):
    additions, deletions = [], []
    entries = raw_diff.rstrip("\0").split("\0")
    for index in range(0, len(entries), 2):
        header, path = entries[index : index + 2]
        if header.split()[1] == "000000":
            deletions.append({"path": path})
        else:
            contents = subprocess.check_output(["git", "show", f":{path}"])
            additions.append({"path": path, "contents": base64.b64encode(contents).decode("ascii")})
    return {
        "query": """mutation($input: CreateCommitOnBranchInput!) {
          createCommitOnBranch(input: $input) { commit { oid } }
        }""",
        "variables": {
            "input": {
                "branch": {"repositoryNameWithOwner": repository, "branchName": branch},
                "expectedHeadOid": source_sha,
                "message": {"headline": message, "body": f"Deps-Tidy-Source: {source_sha}"},
                "fileChanges": {"additions": additions, "deletions": deletions},
            }
        },
    }


def prepare(source_sha, repository, branch, commit_message, output, publication):
    head = git("rev-parse", "HEAD")
    if head != source_sha:
        raise RuntimeError("Checkout does not match the pull-request event revision")
    remote = git("ls-remote", "--exit-code", "origin", f"refs/heads/{branch}").split()[0]
    if remote != head:
        print("Branch advanced; leaving the new revision to its own workflow run")
        output.write("has_changes=false\n")
        return

    message = git("log", "-1", "--format=%B")
    # Only stage tracked files, as before; tool caches must not enter the commit.
    git("add", "--update")
    diff = git("diff", "--cached", "--raw", "--no-abbrev", "--no-renames", "-z")
    if not diff:
        print("No changes to commit")
        output.write("has_changes=false\n")
        return
    if re.search(r"^Deps-Tidy-Source: [0-9a-f]{40}$", message, re.MULTILINE):
        raise RuntimeError(
            "Dependency generation still changes this workflow's own output. "
            "Refusing another automated commit; fix the generator instead of retrying."
        )
    validate_changes(diff)
    json.dump(publication_payload(source_sha, repository, branch, commit_message, diff), publication)
    output.write(f"has_changes=true\nexpected_tree={git('write-tree')}\n")


if __name__ == "__main__":
    with (
        Path(os.environ["GITHUB_OUTPUT"]).open("a", encoding="utf-8") as outputs,
        Path(os.environ["PUBLICATION_FILE"]).open("w", encoding="utf-8") as publication,
    ):
        prepare(
            os.environ["SOURCE_SHA"],
            os.environ["REPOSITORY"],
            os.environ["HEAD_REF"],
            os.environ["COMMIT_MESSAGE"],
            outputs,
            publication,
        )
