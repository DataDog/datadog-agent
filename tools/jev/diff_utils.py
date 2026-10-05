"""Diff processing for the Jev e2e tooling: chunking, per-file modified
percentage annotations and size-capped output."""

from __future__ import annotations  # python 3.9 compat

from pr_context import git, truncate

import re

MAX_DIFF_BYTES = 40_000


MAX_DIFF_PER_FILE = 4_000


def _diff_chunk_stats(chunk: str) -> tuple[str, int, int]:
    """(path, added, deleted) for one per-file diff chunk."""
    header = chunk.splitlines()[0]
    m = re.match(r"diff --git a/(.*?) b/(.*)", header)
    path = (m.group(2) if m else header).strip()
    added = deleted = 0
    for line in chunk.splitlines():
        if (
            line.startswith("+++")
            or line.startswith("---")
            or line.startswith("diff --git ")
            or line.startswith("index ")
        ):
            continue
        if line.startswith("+"):
            added += 1
        elif line.startswith("-"):
            deleted += 1
    return path, added, deleted


def _chunk_annotation(chunk: str) -> str:
    """One-line `# <path>: X% of the file modified` header for a diff chunk."""
    path, added, deleted = _diff_chunk_stats(chunk)
    changed = added + deleted
    if "new file mode" in chunk:
        return f"# {path}: new file ({added} lines added)\n"
    if "Binary files" in chunk and changed == 0:
        return f"# {path}: binary file changed\n"
    try:
        total = len(open(path, encoding="utf-8", errors="ignore").read().splitlines())
    except OSError:
        return f"# {path}: file deleted ({deleted} lines removed)\n"
    if changed >= total:
        return f"# {path}: file rewritten ({changed} lines changed, was {total} lines)\n"
    pct = round(100 * changed / max(total, 1))
    return f"# {path}: {pct}% of the file modified ({changed} of {total} lines changed)\n"


def _split_diff_chunks(diff: str) -> list:
    """Split a unified diff into per-file chunks."""
    chunks, current = [], []
    for line in diff.splitlines(keepends=True):
        if line.startswith("diff --git "):
            if current:
                chunks.append("".join(current))
            current = [line]
        else:
            current.append(line)
    if current:
        chunks.append("".join(current))
    return chunks


def files_from_diff(diff: str) -> list:
    """(path, modification kind) pairs parsed from a unified diff, DDCI-style kinds."""
    out = []
    for chunk in _split_diff_chunks(diff):
        path, _, _ = _diff_chunk_stats(chunk)
        if "new file mode" in chunk:
            kind = "added"
        elif "deleted file mode" in chunk:
            kind = "deleted"
        else:
            kind = "edited"
        out.append((path, kind))
    return out


def _annotate_diff(diff: str, shortstat: str = "") -> str:
    """Annotated, truncated diff text: per-file modified-percentage headers,
    per-file and total size caps. File line counts are read from the CWD, so
    run this with the PR head checked out."""
    if not diff:
        return ""
    chunks = _split_diff_chunks(diff)
    parts = [f"# TOTAL: {shortstat.strip() or 'diff of the PR'}\n"]
    for chunk in chunks:
        parts.append(_chunk_annotation(chunk))
        if len(chunk.encode()) <= MAX_DIFF_PER_FILE:
            parts.append(chunk)
        else:
            path, _, _ = _diff_chunk_stats(chunk)
            parts.append(truncate(chunk, MAX_DIFF_PER_FILE, f"diff of {path}"))
            parts.append("\n")
    full = "".join(parts)
    if len(full.encode()) > MAX_DIFF_BYTES:
        return full[:MAX_DIFF_BYTES] + "\n[... diff truncated, see the file list above for the remaining files ...]"
    return full


def pr_diff(merge_base: str) -> str:
    """Annotated, truncated full diff of the working tree vs `merge_base`."""
    try:
        diff = git("diff", "--no-color", merge_base, "HEAD")
        shortstat = git("diff", "--no-color", "--shortstat", merge_base, "HEAD")
    except RuntimeError as e:
        print(f"[warn] could not compute the full PR diff: {e}")
        return ""
    return _annotate_diff(diff, shortstat)


# ---------------------------------------------------------------- test discovery
