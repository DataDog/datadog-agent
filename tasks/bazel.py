"""
Bazel maintenance tasks.

The first task here, ``refresh_archive_hashes``, is the companion to the
Renovate auto-bump flow for native deps pinned via ``http_archive(...)`` in
``deps/repos.MODULE.bazel``. Renovate updates the version literal but cannot
recompute the ``sha256`` field; this task downloads the new tarball, hashes
it, and rewrites the source. It is invoked from
``.github/workflows/bazel-native-tidy.yml``.

Design mirrors ``tasks/python_version.py::_prepare_bazel_update``: regex-based
edits with strict match-count validation so partial/silent failures abort the
run.
"""

from __future__ import annotations

import hashlib
import re
import urllib.error
import urllib.request
from pathlib import Path

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

REPO_ROOT = Path(__file__).resolve().parent.parent
MODULE_FILE = REPO_ROOT / "deps" / "repos.MODULE.bazel"

# Match each `http_archive(...)` block. Captures the block body so individual
# fields can be extracted with separate regexes. Anchored on column-0 `)` —
# matches the formatting convention used throughout deps/repos.MODULE.bazel.
ARCHIVE_BLOCK_RE = re.compile(
    r"^http_archive\(\n(?P<body>.*?)^\)$",
    re.MULTILINE | re.DOTALL,
)
NAME_RE = re.compile(r'^\s*name\s*=\s*"(?P<name>[^"]+)"', re.MULTILINE)
SHA256_RE = re.compile(r'^\s*sha256\s*=\s*"(?P<sha>[0-9a-fA-F]{64})"', re.MULTILINE)
# Used by ``_block_signature`` to strip the sha256 line regardless of value validity,
# so a stale/wrong hash doesn't preserve the signature when the version literal moved.
SHA256_LINE_RE = re.compile(r'^\s*sha256\s*=\s*"[^"]*",?\s*\n', re.MULTILINE)
# Capture URL string literals inside `urls = [ ... ]`. Skips computed
# expressions (e.g. ``"...".format(var)``) — those won't match the simple
# ``"https?://..."`` literal pattern and are silently filtered.
URL_LITERAL_RE = re.compile(r'"(https?://[^"\s]+)"')


def _parse_archive_blocks(text: str) -> dict[str, str]:
    """Return a mapping of http_archive name -> raw block body."""
    blocks: dict[str, str] = {}
    for m in ARCHIVE_BLOCK_RE.finditer(text):
        body = m.group("body")
        name_match = NAME_RE.search(body)
        if name_match:
            blocks[name_match.group("name")] = body
    return blocks


def _block_signature(body: str) -> str:
    """Identity of a block excluding its sha256 — used to detect version bumps."""
    return SHA256_LINE_RE.sub("", body)


def _extract_urls(body: str) -> list[str]:
    """Return URL string literals found in the block, in order of appearance."""
    return URL_LITERAL_RE.findall(body)


def _extract_sha256(body: str) -> str | None:
    m = SHA256_RE.search(body)
    return m.group("sha") if m else None


def _download_and_hash(urls: list[str]) -> str:
    """Try each URL in order; return the sha256 of the first that downloads cleanly."""
    last_err: Exception | None = None
    for url in urls:
        try:
            with urllib.request.urlopen(url, timeout=120) as resp:  # noqa: S310 — URLs come from the source file we're checking
                hasher = hashlib.sha256()
                while chunk := resp.read(1 << 20):
                    hasher.update(chunk)
                return hasher.hexdigest()
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError) as exc:
            last_err = exc
            print(f"    fetch failed: {url} ({exc})")
            continue
    raise Exit(f"All URLs failed; last error: {last_err}")


def _replace_sha256_in_block(text: str, archive_name: str, new_sha: str) -> str:
    """Rewrite the sha256 line for the named http_archive block."""

    # Locate the block, then replace its sha256 in a single substitution scoped
    # to that block. Match-count validation guards against silent partial edits.
    def repl(match: re.Match[str]) -> str:
        body = match.group("body")
        if not NAME_RE.search(body) or NAME_RE.search(body).group("name") != archive_name:
            return match.group(0)
        new_body, count = SHA256_RE.subn(
            lambda _: f'    sha256 = "{new_sha}"',
            body,
            count=1,
        )
        if count != 1:
            raise Exit(f"Expected 1 sha256 line in http_archive(name={archive_name!r}), found {count}")
        return f"http_archive(\n{new_body})"

    new_text, count = ARCHIVE_BLOCK_RE.subn(repl, text, count=0)
    if count == 0:
        raise Exit(f"Could not locate http_archive block for {archive_name!r}")
    return new_text


@task(
    help={
        "base_ref": "Git ref to compare against to detect changed http_archive blocks. "
        "Defaults to origin/main, which suits the bazel-native-tidy workflow. "
        "For local testing pass HEAD~1 or any other ref."
    }
)
def refresh_archive_hashes(ctx: Context, base_ref: str = "origin/main") -> None:
    """
    Recompute sha256 for any http_archive in deps/repos.MODULE.bazel whose
    version literal differs from ``base_ref``.

    Used by ``.github/workflows/bazel-native-tidy.yml`` after Renovate bumps a
    version. Renovate cannot refresh ``sha256`` itself; this task downloads the
    new tarball from the first reachable URL, hashes it, and rewrites the
    source so the next Bazel build verifies cleanly.
    """
    current_text = MODULE_FILE.read_text()
    current_blocks = _parse_archive_blocks(current_text)

    result = ctx.run(f"git show {base_ref}:deps/repos.MODULE.bazel", hide=True, warn=True)
    if not result.ok:
        raise Exit(f"Could not read deps/repos.MODULE.bazel at {base_ref!r}: {result.stderr.strip()}")
    previous_blocks = _parse_archive_blocks(result.stdout)

    needs_refresh: list[str] = []
    for name, body in current_blocks.items():
        prev = previous_blocks.get(name)
        if prev is None:
            # New http_archive added in this PR — initial sha256 is the human's job.
            continue
        if _block_signature(body) != _block_signature(prev):
            needs_refresh.append(name)

    if not needs_refresh:
        print("No http_archive blocks need sha256 refresh.")
        return

    print(f"Refreshing sha256 for {len(needs_refresh)} block(s): {', '.join(needs_refresh)}")
    new_text = current_text
    for name in needs_refresh:
        body = _parse_archive_blocks(new_text)[name]
        urls = _extract_urls(body)
        if not urls:
            print(f"  ! {name}: skipping — no literal URL found in block")
            continue
        old_sha = _extract_sha256(body)
        print(f"  → {name}: downloading from {urls[0]}")
        new_sha = _download_and_hash(urls)
        if new_sha == old_sha:
            print(f"    sha256 unchanged ({old_sha[:12]}...)")
            continue
        new_text = _replace_sha256_in_block(new_text, name, new_sha)
        print(f"    sha256 {old_sha[:12]}... -> {new_sha[:12]}...")

    if new_text != current_text:
        MODULE_FILE.write_text(new_text)
        print(f"Updated {MODULE_FILE.relative_to(REPO_ROOT)}.")
    else:
        print("No sha256 values changed.")
