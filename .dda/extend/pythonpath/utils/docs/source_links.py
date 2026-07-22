# SPDX-FileCopyrightText: 2026-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

import os
import re
import subprocess
from urllib.parse import unquote

from dda.utils.fs import Path

REPO_URL = "https://github.com/DataDog/datadog-agent"


def get_source_ref() -> str:
    """
    Return the git ref that source code links should point to.

    Mirrors the logic in `docs/public/.hooks/inject_variables.py`: an explicit
    `DOCS_SOURCE_REF` environment variable wins (CI sets it to the pull request
    branch, or to `main` when deploying), otherwise the current branch is used.
    """
    if ref := os.environ.get("DOCS_SOURCE_REF"):
        return ref

    try:
        process = subprocess.run(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"],
            capture_output=True,
            text=True,
            check=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return "main"

    branch = process.stdout.strip()
    return branch if branch and branch != "HEAD" else "main"


def source_link_exclusion_pattern(ref: str) -> str:
    """
    Return the regex for links that `validate_source_links` covers and that the
    external link checker must therefore skip.
    """
    refs = {"main", ref}
    return f"^{re.escape(REPO_URL)}/(blob|tree)/({'|'.join(sorted(re.escape(r) for r in refs))})/"


def validate_source_links(site_dir: Path, repo_root: Path, ref: str) -> list[str]:
    """
    Check links to repository source code against the local checkout.

    Links of the form `https://github.com/DataDog/datadog-agent/blob/<ref>/<path>`
    (or `tree/<ref>`) where `<ref>` is `main` or the current source ref are
    resolved against the working tree rather than GitHub. This allows pull
    requests that add or move source code to reference it from documentation,
    validates directory links (which external checkers cannot), and catches
    links that a rename in the same pull request would break after merging.

    Returns a list of human-readable errors, empty when every link resolves.
    """
    refs = {"main", ref}
    pattern = re.compile(
        f"{re.escape(REPO_URL)}/(?:blob|tree)/(?:{'|'.join(re.escape(r) for r in refs)})/([^\"'#?<>\\s\\\\]+)"
    )

    errors: list[str] = []
    seen: set[str] = set()
    for html_file in sorted(site_dir.rglob("*.html")):
        for match in pattern.finditer(html_file.read_text(encoding="utf-8")):
            path = unquote(match.group(1)).rstrip("/")
            if path in seen:
                continue
            seen.add(path)

            if not (repo_root / path).exists():
                page = html_file.relative_to(site_dir)
                errors.append(f"{page}: broken source link `{match.group(0)}` (no `{path}` in the repository)")

    return errors
