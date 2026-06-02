"""
Renovate maintenance tasks.

This module exposes a check that verifies every native dep declared via
``http_archive`` or ``http_file`` in any ``*.MODULE.bazel`` file under ``deps/``
is either tracked by a Renovate ``customManager`` in ``renovate.json`` or listed
in ``deps/.renovate-untracked.json`` with a rationale. It runs in CI via
``.github/workflows/validate-renovate-deps.yml``.

The task  ``refresh_archive_hashes``, is the companion to the
Renovate auto-bump flow for native deps pinned via ``http_archive(...)`` in
``deps/repos.MODULE.bazel``. Renovate updates the version literal but cannot
recompute the ``sha256`` field; this task downloads the new tarball, hashes
it, and rewrites the source. It is invoked from
``.github/workflows/bazel-native-tidy.yml``.

Design mirrors ``tasks/python_version.py::_prepare_bazel_update``: regex-based
edits with strict match-count validation so partial/silent failures abort the
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import urllib.error
import urllib.request
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

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
# Capture plain URL string literals (no template placeholders).
URL_LITERAL_RE = re.compile(r'"(https?://[^"\s{}]+)"')
# Match top-level scalar assignments: `name = <expr>` not inside any block.
# Captures the variable name and the right-hand side expression.
_TOP_LEVEL_ASSIGN_RE = re.compile(r'^(\w+)\s*=\s*(.+)$', re.MULTILINE)
# Locate the start of a `url =` or `urls =` field within a block body.
_URL_FIELD_RE = re.compile(r'^\s*urls?\s*=\s*', re.MULTILINE)

# Safe builtins allowed when evaluating top-level Starlark scalar expressions.
# Starlark is a Python subset; these cover all constructs used in the file.
_SAFE_BUILTINS: dict = {"__builtins__": {}}


def _parse_top_level_namespace(text: str) -> dict:
    """Evaluate top-level scalar assignments from a MODULE.bazel file.

    Walks the assignments in source order so later variables can reference
    earlier ones (e.g. ``sqlite_amalgamation`` references ``sqlite_ver``).
    Only simple expressions are evaluated — anything that raises is silently
    skipped so a complex assignment never blocks resolution of simpler ones.

    The returned namespace can be passed to ``_extract_urls`` to resolve URL
    expressions that reference these variables.
    """
    namespace: dict = {}
    # Only consider lines that appear before the first http_archive/http_file
    # call so we don't accidentally pick up block-internal assignments.
    preamble_end = min(
        (text.index(marker) for marker in ("http_archive(", "http_file(") if marker in text),
        default=len(text),
    )
    preamble = text[:preamble_end]
    for m in _TOP_LEVEL_ASSIGN_RE.finditer(preamble):
        name, expr = m.group(1), m.group(2).strip()
        try:
            namespace[name] = eval(expr, {"__builtins__": {}}, namespace)  # noqa: S307 — restricted namespace, preamble-only expressions
        except Exception:
            pass
    return namespace


def main():
    import sys

    # The canonical entry points are `bazel run //tasks:check_renovate_bazel_coverage`
    # (used by CI) and `dda inv renovate.check-bazel-coverage`. BUILD_WORKSPACE_DIRECTORY
    # is set by `bazel run`; the REPO_ROOT fallback only exists so the `dda inv`
    # path (which doesn't set it) still resolves the workspace correctly.
    _workspace = os.environ.get("BUILD_WORKSPACE_DIRECTORY")
    _root = _workspace if _workspace else str(REPO_ROOT)
    # Translate invoke's Exit to sys.exit so `bazel run` / direct python
    # invocation produce the same stderr report + exit code as `dda inv`,
    # whose runner catches Exit internally.
    try:
        check_bazel_coverage(Context(), _root)
    except Exit as e:
        if e.message:
            print(e.message, file=sys.stderr)
        sys.exit(e.code)


@task
def check_bazel_coverage(_: Context, root: str | None = None) -> None:
    """
    Fail if any http_archive or http_file in deps/ lacks a Renovate customManager,
    or if the allowlist contains stale entries that no longer match any dep.

    Scans every ``*.MODULE.bazel`` file under ``deps/`` for ``http_archive``
    and ``http_file`` calls. A dep is considered covered when either:
      * its name appears as ``depNameTemplate`` in one of ``renovate.json``'s customManagers, or
      * it is listed in ``deps/.renovate-untracked.json`` with a non-empty rationale.

    Also fails when ``deps/.renovate-untracked.json`` lists a dep that no longer
    exists in ``deps/**/*.MODULE.bazel`` — without this, renamed or removed deps
    leave dead entries behind and the file rots.

    Writes a markdown report to ``$GITHUB_STEP_SUMMARY`` when running in GitHub Actions.
    """
    root_path = Path(root) if root is not None else Path(REPO_ROOT)
    dep_names = _parse_deps_dir(root_path / "deps")
    tracked_names = _parse_renovate_json(root_path / "renovate.json")
    allowlist = _parse_allowlist(root_path / "deps" / ".renovate-untracked.json")
    allowlist_keys = set(allowlist)

    untracked = dep_names - tracked_names - allowlist_keys
    stale = allowlist_keys - dep_names
    # A dep listed in BOTH renovate.json AND the allowlist means a tracking PR
    # forgot to delete the allowlist entry. Left alone, the dead entry would
    # silently mask a later removal of the Renovate manager — defeating the
    # phase-out workflow this check enforces.
    double_classified = tracked_names & allowlist_keys

    if untracked or stale or double_classified:
        report = _emit_failure_report(untracked, stale, double_classified, allowlist)
        summary_path = os.environ.get("GITHUB_STEP_SUMMARY")
        if summary_path:
            # GITHUB_STEP_SUMMARY is a shared file for the whole step; append
            # rather than overwrite to play nice with any other writes.
            with open(summary_path, "a", encoding="utf-8") as fh:
                fh.write(report + "\n")
        raise Exit(report, code=1)

    print(
        f"OK: {len(dep_names)} native deps (http_archive + http_file), "
        f"{len(dep_names) - len(allowlist)} tracked by Renovate, "
        f"{len(allowlist)} intentionally untracked."
    )


def _parse_deps_dir(deps_dir: Path) -> set[str]:
    names: set[str] = set()
    for path in deps_dir.rglob("*.MODULE.bazel"):
        text = path.read_text()
        for call in ("http_archive", "http_file"):
            names |= _extract_call_names(text, call, path)
    return names


def _extract_call_names(text: str, call_name: str, path: Path) -> set[str]:
    """Extract name = "..." from all call_name(...) blocks, regardless of arg order or comments.

    When the name is a Starlark template (e.g. `name = "{}_win".format(name)`),
    resolve it against the enclosing list comprehension and the dict literal it
    iterates. Fails loudly on any template we can't resolve — silent skips
    would let loop-emitted deps escape the coverage check.
    """
    names: set[str] = set()
    # Starlark accepts both single- and double-quoted string literals; capture either.
    name_re = re.compile(r"""\bname\s*=\s*(?:"([^"]+)"|'([^']+)')""")
    marker = call_name + "("
    start = 0
    while True:
        pos = text.find(marker, start)
        if pos == -1:
            break
        depth = 1
        i = pos + len(marker)
        while i < len(text) and depth:
            c = text[i]
            if c == "(":
                depth += 1
            elif c == ")":
                depth -= 1
            elif c in ('"', "'"):
                # Skip the matching string literal so parens inside it don't
                # throw off the depth count. Handles both quote styles.
                quote = c
                i += 1
                while i < len(text) and text[i] != quote:
                    if text[i] == "\\":
                        i += 1
                    i += 1
            i += 1
        block = text[pos + len(marker) : i - 1]
        uncommented = "\n".join(line for line in block.splitlines() if not line.lstrip().startswith("#"))
        m = name_re.search(uncommented)
        if m:
            literal = m.group(1) or m.group(2)
            if "{" in literal:
                names |= _resolve_templated_name(text, pos, literal, path)
            else:
                names.add(literal)
        start = i
    return names


_DICT_LITERAL_RE = re.compile(
    r'^(\w+)\s*=\s*\{(.*?)\n\}',
    re.DOTALL | re.MULTILINE,
)
_DICT_KEY_RE = re.compile(r"""^\s*(?:"([^"]+)"|'([^']+)')\s*:""", re.MULTILINE)
# `for <iter_var>, ... in <dict_var>.items()` — first target name, optional
# tuple-destructuring tail (which may contain parentheses for `(a, b)`), then
# the source dict. We anchor on the closing ` in ` token so the destructuring
# tail can be anything.
_COMPREHENSION_RE = re.compile(
    r'\bfor\s+(\w+)[^\n]*?\s+in\s+(\w+)\.items\(\)',
)


def _resolve_templated_name(text: str, call_pos: int, template: str, path: Path) -> set[str]:
    """Resolve a templated name like "{}_win" by finding the enclosing
    comprehension and the dict it iterates.

    Supported pattern (the only one in-tree today):

        SOME_DICT = {"keyA": (...), "keyB": (...)}
        [
            http_archive(
                name = "{}_suffix".format(VAR),
                ...
            )
            for VAR, (...) in SOME_DICT.items()
        ]

    Raises Exit if the template can't be resolved — a silent skip would defeat
    the coverage guarantee.
    """
    after = text[call_pos:]
    comp = _COMPREHENSION_RE.search(after)
    if not comp:
        raise Exit(
            f"{path}: templated name {template!r} has no enclosing "
            "`for VAR, ... in DICT.items()` comprehension. Refactor to literal "
            "names, or extend tasks/renovate.py to handle this pattern."
        )
    iter_var, dict_var = comp.group(1), comp.group(2)
    keys = _lookup_dict_keys(text, dict_var)
    if keys is None:
        raise Exit(
            f"{path}: templated name {template!r} iterates {dict_var}.items() "
            f"but no top-level `{dict_var} = {{...}}` literal was found. "
            "Refactor to literal names, or extend tasks/renovate.py."
        )
    # Substitute each key for the loop variable. Both `"{}".format(VAR)` and
    # `"{name}".format(name = VAR)` reduce to a single `{...}` placeholder
    # that we replace with the literal key.
    if "{}" in template:
        return {template.replace("{}", k) for k in keys}
    placeholder = "{" + iter_var + "}"
    if placeholder in template:
        return {template.replace(placeholder, k) for k in keys}
    raise Exit(
        f"{path}: templated name {template!r} doesn't reference loop var "
        f"{iter_var!r} via `{{}}` or `{{{iter_var}}}`. Refactor or extend the parser."
    )


def _lookup_dict_keys(text: str, dict_var: str) -> list[str] | None:
    for m in _DICT_LITERAL_RE.finditer(text):
        if m.group(1) == dict_var:
            # _DICT_KEY_RE has alternation (double- vs single-quoted); findall
            # returns tuples — flatten to whichever group matched.
            return [dq or sq for dq, sq in _DICT_KEY_RE.findall(m.group(2))]
    return None


def _parse_renovate_json(path: Path) -> set[str]:
    # Assumption: renovate.json is plain JSON plus trailing commas only — no
    # // or /* */ comments, no single-quoted strings. Renovate accepts the full
    # JSON5 grammar but ours stays in this subset; if that changes, swap in a
    # real JSON5 parser (e.g. the `json5` package) instead of extending this regex.
    raw = path.read_text()
    stripped = re.sub(r",(\s*[}\]])", r"\1", raw)
    data = json.loads(stripped)
    return {cm["depNameTemplate"] for cm in data.get("customManagers", []) if "depNameTemplate" in cm}


def _parse_allowlist(path: Path) -> dict[str, str]:
    if not path.exists():
        return {}
    data = json.loads(path.read_text())
    entries = data.get("intentionally_untracked", {})
    bad = [k for k, v in entries.items() if not (isinstance(v, str) and v.strip())]
    if bad:
        raise Exit(
            f"Allowlist {path} has empty rationale for: {', '.join(bad)}. "
            "Every entry must include a non-empty justification string."
        )
    return entries


def _emit_failure_report(
    untracked: set[str],
    stale: set[str],
    double_classified: set[str],
    allowlist: dict[str, str],
) -> str:
    lines = ["## ❌ Renovate coverage check failed", ""]
    if untracked:
        lines += [
            "The following native deps (http_archive / http_file) in `deps/` have no "
            "matching `customManager` in `renovate.json`:",
            "",
            "| dep | suggested fix |",
            "|---|---|",
        ]
        for dep in sorted(untracked):
            lines.append(
                f"| `{dep}` | Add a `customManagers` entry with "
                f'`depNameTemplate: "{dep}"`, or add to '
                "`deps/.renovate-untracked.json` with a rationale. |"
            )
        lines += [
            "",
            "See `renovate.json` for existing patterns (linux-images, windows-images, ...).",
            "",
        ]
    if stale:
        lines += [
            "The following entries in `deps/.renovate-untracked.json` no longer match "
            "any `http_archive` / `http_file` name in `deps/**/*.MODULE.bazel` and "
            "must be removed:",
            "",
        ]
        for dep in sorted(stale):
            lines.append(f"- `{dep}`")
        lines += [
            "",
            "If the dep was renamed, replace the old entry with the new name. If it "
            "was removed, delete the entry. Stale allowlist entries silently weaken the check.",
            "",
        ]
    if double_classified:
        lines += [
            "The following deps are listed in BOTH `renovate.json` (as a "
            "`customManagers` entry) AND `deps/.renovate-untracked.json`. Once a dep is "
            "tracked by Renovate, its allowlist entry must be deleted in the same PR — "
            "leaving it behind silently masks future regressions of the Renovate manager:",
            "",
        ]
        for dep in sorted(double_classified):
            lines.append(f"- `{dep}` — remove the entry from `deps/.renovate-untracked.json`")
        lines.append("")
    lines.append(
        f"Currently allowlisted ({len(allowlist)}): "
        + (", ".join(f"`{k}`" for k in sorted(allowlist)) if allowlist else "_none_")
    )
    return "\n".join(lines)


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
    namespace = _parse_top_level_namespace(current_text)

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
        urls = _extract_urls(body, namespace)
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


def _extract_urls(body: str, namespace: dict | None = None) -> list[str]:
    """Return URLs found in the block, in order of appearance.

    First tries plain string literals (no ``{}`` placeholders). If none are
    found and a ``namespace`` is provided (populated by
    ``_parse_top_level_namespace``), locates each ``url``/``urls`` field,
    extracts its full right-hand-side expression using bracket-depth tracking,
    and evaluates it in the namespace to resolve variable references and
    ``.format()`` calls. This covers blocks whose URLs are constructed from
    top-level variables (e.g. sqlite3).
    """
    literals = URL_LITERAL_RE.findall(body)
    if literals:
        return literals
    if namespace is None:
        return []
    urls: list[str] = []
    for m in _URL_FIELD_RE.finditer(body):
        expr = _extract_balanced_expr(body, m.end())
        if expr is None:
            continue
        try:
            result = eval(expr, {"__builtins__": {}}, namespace)  # noqa: S307 — namespace is preamble-only scalars
        except Exception:
            continue
        if isinstance(result, str) and result.startswith("http"):
            urls.append(result)
        elif isinstance(result, list | tuple):
            urls.extend(u for u in result if isinstance(u, str) and u.startswith("http"))
    return urls


def _extract_balanced_expr(text: str, start: int) -> str | None:
    """Extract a Starlark expression starting at ``start``, respecting bracket depth.

    Reads until the expression ends: either at a top-level comma/newline (for
    simple scalar values) or after the matching closing bracket (for lists and
    tuples). Returns the stripped expression string, or ``None`` if ``start``
    is out of range.
    """
    if start >= len(text):
        return None
    depth = 0
    i = start
    while i < len(text):
        c = text[i]
        if c in "([":
            depth += 1
        elif c in ")]":
            depth -= 1
            if depth < 0:
                # Stepped outside the enclosing block — stop before this char.
                break
        elif c in ('"', "'"):
            # Skip string literals so brackets inside them don't affect depth.
            quote = c
            i += 1
            while i < len(text) and text[i] != quote:
                if text[i] == "\\":
                    i += 1
                i += 1
        elif c == "," and depth == 0:
            break
        elif c == "\n" and depth == 0:
            break
        i += 1
    return text[start:i].strip() or None


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


if __name__ == "__main__":
    main()
