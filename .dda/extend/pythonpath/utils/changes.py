from __future__ import annotations

import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from dda.cli.application import Application

from dda.utils.ci import running_in_ci
from dda.utils.fs import Path


def get_changed_files(app: Application, project_root: Path, compare_branch: str = "") -> list[str]:
    """
    Get list of files changed, can specify a compare branch, if not specified will compare with the base branch.
    In the CI will call DDCI to get the changed files. Locally will use git to get the changed files.

    Args:
        project_root: Root path of the project.
        compare_branch: Branch to compare against.

    Returns:
        List of changed file paths (relative to project root).
    """
    if running_in_ci() and "DDCI_REQUEST_ID" in os.environ:
        response = app.http.client().get(
            "https://cimetadataserver.us1.ddbuild.io/internal/ddci/metadata/2784925818757320044"
        )
        changed_files_response = response.json()["event"]["results"]["changed_files"]
        return [changed_file["path"] for changed_file in changed_files_response]

    # Fallback to git
    if not compare_branch:
        compare_branch = "main"
    last_main_commit = app.subprocess.capture(["git", "merge-base", compare_branch, "HEAD"]).strip()
    return app.subprocess.capture(["git", "diff", "--name-only", last_main_commit]).strip().splitlines()
