"""
Renovate maintenance tasks.

This module exposes a check that verifies every native dep declared via
``http_archive`` or ``http_file`` in any ``*.MODULE.bazel`` file under ``deps/``
is either tracked by a Renovate ``customManager`` in ``renovate.json`` or listed
in ``deps/.renovate-untracked.json`` with a rationale. It runs in CI via
``.github/workflows/validate-renovate-deps.yml``.
"""

from __future__ import annotations

import json
import os
import re
from pathlib import Path

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

REPO_ROOT = Path(__file__).resolve().parent.parent


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


if __name__ == "__main__":
    main()
