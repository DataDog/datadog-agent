from __future__ import annotations

import fnmatch
import subprocess
from collections import defaultdict
from pathlib import Path


def git(args: list[str], repo_root: Path) -> str:
    completed = subprocess.run(
        ["git", *args],
        cwd=repo_root,
        check=True,
        capture_output=True,
        text=True,
    )
    return completed.stdout


def get_changed_files(repo_root: Path, base_sha: str, head_sha: str) -> list[str]:
    output = git(["diff", "--name-only", base_sha, head_sha], repo_root)
    return [line.strip() for line in output.splitlines() if line.strip()]


def filter_paths(paths: list[str], globs: tuple[str, ...] | list[str]) -> list[str]:
    return [path for path in paths if any(fnmatch.fnmatch(path, pattern) for pattern in globs)]


def get_diff(repo_root: Path, base_sha: str, head_sha: str, paths: list[str]) -> str:
    args = ["diff", "--find-renames", "--unified=3", base_sha, head_sha, "--", *paths]
    return git(args, repo_root)


def short_sha(sha: str) -> str:
    return sha[:12]


def parse_changed_lines(diff_text: str) -> dict[str, set[int]]:
    changed: dict[str, set[int]] = defaultdict(set)
    current_file: str | None = None
    new_line = 0

    for raw_line in diff_text.splitlines():
        if raw_line.startswith("diff --git "):
            current_file = None
            continue
        if raw_line.startswith("+++ b/"):
            current_file = raw_line[6:]
            continue
        if raw_line.startswith("@@"):
            if current_file is None:
                continue
            hunk = raw_line.split("@@")[1].strip()
            _, _, new_range = hunk.partition("+")
            new_range = new_range.split(" ", 1)[0]
            start = new_range.split(",", 1)[0]
            new_line = int(start)
            continue
        if current_file is None:
            continue
        if not raw_line:
            continue
        prefix = raw_line[0]
        if prefix == "+" and not raw_line.startswith("+++"):
            changed[current_file].add(new_line)
            new_line += 1
        elif prefix == "-":
            continue
        else:
            new_line += 1
    return changed
