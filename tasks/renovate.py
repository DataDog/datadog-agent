"""
Renovate maintenance tasks.

This module exposes a check that verifies every native dep declared via
``http_archive`` or ``http_file`` in any ``*.MODULE.bazel`` file under ``deps/``
is either tracked by a Renovate ``customManager`` in ``renovate.json`` or listed
in ``deps/.renovate-untracked.json`` with a rationale. It runs in CI via
``.github/workflows/validate-renovate-deps.yml``.

The task  ``refresh_archive_hashes``, is the companion to the Renovate
auto-bump flow for native deps pinned via ``http_archive(...)`` or
``http_file(...)`` in ``deps/**/*.MODULE.bazel``. Renovate updates the version
literal but cannot recompute the ``sha256`` field; this task downloads the new
artifact, hashes it, and rewrites the source. It is invoked from
``.github/workflows/bazel-native-tidy.yml``.

Design mirrors ``tasks/python_version.py::_prepare_bazel_update``: Starlark
evaluation for dependency discovery, then scoped regex edits with strict
match-count validation so partial/silent failures abort the run.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import shlex
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

REPO_ROOT = Path(__file__).resolve().parent.parent

# Match each literal `http_archive(...)` / `http_file(...)` block. Captures the
# block body so the sha256 line can be replaced after Starlark evaluation has
# identified the dep. Anchored on a closing `)` with the same indentation as the
# call, which covers both top-level calls and calls inside comprehensions.
REPOSITORY_RULE_BLOCK_RE = re.compile(
    r"^(?P<indent>[ \t]*)(?P<kind>http_archive|http_file)\(\n(?P<body>.*?)(?P=indent)\)",
    re.MULTILINE | re.DOTALL,
)
NAME_RE = re.compile(r"""^\s*name\s*=\s*(?P<quote>["'])(?P<name>[^"']+)(?P=quote)""", re.MULTILINE)
SHA256_RE = re.compile(
    r'^(?P<indent>\s*)sha256\s*=\s*"(?P<sha>[0-9a-fA-F]{64})"(?P<comma>,?)',
    re.MULTILINE,
)
USE_REPO_RULE_ASSIGN_RE = re.compile(r"^[A-Za-z_]\w*\s*=\s*use_repo_rule\(", re.MULTILINE)

STUBBED_MODULE_FUNCTIONS = (
    "archive_override",
    "bazel_dep",
    "constants_repo",
    "git_override",
    "include",
    "register_toolchains",
    "use_extension",
    "use_repo",
    "use_repo_rule",
)


@dataclass(frozen=True)
class RepositoryRuleCall:
    kind: str
    name: str
    path: Path
    relative_path: str
    sha256: str | None
    urls: tuple[str, ...]
    identity: tuple


def _starlark():
    import starlark as sl

    return sl


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
        "base_ref": "Git ref to compare against to detect changed http_archive/http_file calls. "
        "Defaults to origin/main, which suits the bazel-native-tidy workflow. "
        "For local testing pass HEAD~1 or any other ref."
    }
)
def refresh_archive_hashes(ctx: Context, base_ref: str = "origin/main") -> None:
    """
    Recompute sha256 for changed Bazel native deps.

    The implementation depends on the Bazel Python toolchain's starlark-pyo3
    wheel, which is not installed in dda's legacy invoke environment. Keep this
    invoke entry point as a stable wrapper and run the real implementation via
    Bazel.
    """
    ctx.run(
        "bazel run //tasks:refresh_renovate_archive_hashes -- " f"--base-ref={shlex.quote(base_ref)}",
    )


def refresh_archive_hashes_impl(ctx: Context, root: Path, base_ref: str = "origin/main") -> None:
    """
    Recompute sha256 for any http_archive/http_file in deps/**/*.MODULE.bazel
    whose resolved non-sha fields differ from ``base_ref``.

    Used by ``.github/workflows/bazel-native-tidy.yml`` after Renovate bumps a
    version. Renovate cannot refresh ``sha256`` itself; this task downloads the
    new artifact from the first reachable URL, hashes it, and rewrites the
    literal ``sha256`` field so the next Bazel build verifies cleanly.
    """
    base_check = ctx.run(
        f"git -C {shlex.quote(str(root))} rev-parse --verify {shlex.quote(base_ref)}",
        hide=True,
        warn=True,
    )
    if not base_check.ok:
        raise Exit(f"Could not resolve base ref {base_ref!r}: {base_check.stderr.strip()}")

    current_calls = _parse_repository_rule_calls_in_deps(root / "deps", root)
    previous_calls = _parse_repository_rule_calls_at_ref(ctx, root, base_ref, current_calls)

    needs_refresh: list[RepositoryRuleCall] = []
    for key, current in current_calls.items():
        previous = previous_calls.get(key)
        if previous is None:
            # New repository rule added in this PR — initial sha256 is the human's job.
            continue
        if current.identity != previous.identity:
            needs_refresh.append(current)

    if not needs_refresh:
        print("No http_archive/http_file blocks need sha256 refresh.")
        return

    needs_refresh.sort(key=lambda call: (call.relative_path, call.name))
    print(f"Refreshing sha256 for {len(needs_refresh)} block(s): {', '.join(call.name for call in needs_refresh)}")

    updated_texts: dict[Path, str] = {}
    for call in needs_refresh:
        if not call.urls:
            raise Exit(f"{call.relative_path}: {call.name} changed but has no resolved URL to download")
        if call.sha256 is None:
            raise Exit(f"{call.relative_path}: {call.name} changed but has no resolved sha256 field")

        print(f"  → {call.name}: downloading from {call.urls[0]}")
        new_sha = _download_and_hash(list(call.urls))
        if new_sha == call.sha256:
            print(f"    sha256 unchanged ({call.sha256[:12]}...)")
            continue

        text = updated_texts.get(call.path)
        if text is None:
            text = call.path.read_text()
        updated_texts[call.path] = _replace_sha256_in_rule_block(text, call, new_sha)
        print(f"    sha256 {call.sha256[:12]}... -> {new_sha[:12]}...")

    if updated_texts:
        for path, text in sorted(updated_texts.items(), key=lambda item: item[0].as_posix()):
            path.write_text(text)
            print(f"Updated {path.relative_to(root)}.")
    else:
        print("No sha256 values changed.")


def _parse_repository_rule_calls_in_deps(deps_dir: Path, root: Path) -> dict[tuple[str, str], RepositoryRuleCall]:
    calls: dict[tuple[str, str], RepositoryRuleCall] = {}
    for path in deps_dir.rglob("*.MODULE.bazel"):
        text = path.read_text()
        if "http_archive(" not in text and "http_file(" not in text:
            continue
        calls.update(_parse_repository_rule_calls_from_text(path, text, root))
    return calls


def _parse_repository_rule_calls_at_ref(
    ctx: Context,
    root: Path,
    base_ref: str,
    current_calls: dict[tuple[str, str], RepositoryRuleCall],
) -> dict[tuple[str, str], RepositoryRuleCall]:
    calls: dict[tuple[str, str], RepositoryRuleCall] = {}
    for relative_path in sorted({call.relative_path for call in current_calls.values()}):
        result = ctx.run(
            f"git -C {shlex.quote(str(root))} show {shlex.quote(f'{base_ref}:{relative_path}')}",
            hide=True,
            warn=True,
        )
        if not result.ok:
            continue
        calls.update(_parse_repository_rule_calls_from_text(root / relative_path, result.stdout, root))
    return calls


def _parse_repository_rule_calls_from_text(
    path: Path, text: str, root: Path
) -> dict[tuple[str, str], RepositoryRuleCall]:
    """Evaluate a MODULE.bazel file and return resolved http_archive/http_file calls."""
    calls: list[RepositoryRuleCall] = []
    relative_path = path.relative_to(root).as_posix()

    def record(kind: str):
        def _record(*args: object, **kwargs: object) -> None:
            if args:
                raise ValueError(f"{kind} in {relative_path} used positional args; only keyword args are supported")
            calls.append(_repository_rule_call_from_kwargs(kind, path, relative_path, kwargs))

        return _record

    def noop(*_args: object, **_kwargs: object) -> None:
        return None

    sl = _starlark()
    module = sl.Module()
    module.add_callable("http_archive", record("http_archive"))
    module.add_callable("http_file", record("http_file"))
    for name in STUBBED_MODULE_FUNCTIONS:
        module.add_callable(name, noop)

    stripped_text = _strip_use_repo_rule_assignments(text)
    try:
        ast = sl.parse(relative_path, stripped_text)
        sl.eval(module, ast, sl.Globals.standard())
    except Exception as exc:
        raise Exit(f"Could not evaluate {relative_path} while resolving repository rules: {exc}") from exc

    result: dict[tuple[str, str], RepositoryRuleCall] = {}
    for call in calls:
        key = (call.relative_path, call.name)
        if key in result:
            raise Exit(f"{relative_path}: duplicate repository rule name {call.name!r}")
        result[key] = call
    return result


def _repository_rule_call_from_kwargs(
    kind: str,
    path: Path,
    relative_path: str,
    kwargs: dict[str, object],
) -> RepositoryRuleCall:
    name = kwargs.get("name")
    if not isinstance(name, str) or not name:
        raise ValueError(f"{kind} in {relative_path} has no string name")

    sha256 = kwargs.get("sha256")
    if sha256 is not None and not isinstance(sha256, str):
        raise ValueError(f"{kind}({name}) in {relative_path} has non-string sha256")

    return RepositoryRuleCall(
        kind=kind,
        name=name,
        path=path,
        relative_path=relative_path,
        sha256=sha256,
        urls=tuple(_extract_resolved_urls(kwargs)),
        identity=(kind, _stable_value({k: v for k, v in kwargs.items() if k != "sha256"})),
    )


def _extract_resolved_urls(kwargs: dict[str, object]) -> list[str]:
    urls = kwargs.get("urls")
    if urls is None:
        urls = kwargs.get("url")
    if urls is None:
        return []
    if isinstance(urls, str):
        return [urls]
    if isinstance(urls, list | tuple):
        return [url for url in urls if isinstance(url, str)]
    return []


def _stable_value(value: object) -> object:
    if isinstance(value, dict):
        return tuple(sorted((str(k), _stable_value(v)) for k, v in value.items()))
    if isinstance(value, list | tuple):
        return tuple(_stable_value(v) for v in value)
    return value


def _strip_use_repo_rule_assignments(text: str) -> str:
    """Remove `name = use_repo_rule(...)` assignments before starlark-pyo3 eval.

    Bazel's use_repo_rule returns a callable repository-rule proxy. starlark-pyo3
    cannot return Python callables into Starlark through normal value
    conversion, so we strip those assignments and inject Python recorder
    functions with the same names instead.
    """
    pieces: list[str] = []
    pos = 0
    for match in USE_REPO_RULE_ASSIGN_RE.finditer(text):
        start = match.start()
        end = _find_call_end(text, match.end(), start)
        pieces.append(text[pos:start])
        pos = end
    pieces.append(text[pos:])
    return "".join(pieces)


def _find_call_end(text: str, start: int, fallback: int) -> int:
    depth = 1
    i = start
    while i < len(text) and depth:
        c = text[i]
        if c in ('"', "'"):
            quote = c
            i += 1
            while i < len(text) and text[i] != quote:
                if text[i] == "\\":
                    i += 1
                i += 1
        elif c == "(":
            depth += 1
        elif c == ")":
            depth -= 1
        i += 1
    if depth != 0:
        raise Exit(f"Could not parse use_repo_rule assignment starting at byte {fallback}")
    while i < len(text) and text[i] in " \t":
        i += 1
    if i < len(text) and text[i] == "\n":
        i += 1
    return i


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


def _replace_sha256_in_rule_block(text: str, call: RepositoryRuleCall, new_sha: str) -> str:
    """Rewrite the sha256 line for the named repository rule block."""

    replacements = 0

    def repl(match: re.Match[str]) -> str:
        nonlocal replacements
        if match.group("kind") != call.kind:
            return match.group(0)
        body = match.group("body")
        name_match = NAME_RE.search(body)
        if not name_match or name_match.group("name") != call.name:
            return match.group(0)
        new_body, count = SHA256_RE.subn(
            lambda sha_match: f'{sha_match.group("indent")}sha256 = "{new_sha}"{sha_match.group("comma")}',
            body,
            count=1,
        )
        if count != 1:
            raise Exit(f"Expected 1 literal sha256 line in {call.kind}(name={call.name!r}), found {count}")
        replacements += 1
        return f"{match.group('indent')}{call.kind}(\n{new_body}{match.group('indent')})"

    new_text = REPOSITORY_RULE_BLOCK_RE.sub(repl, text)
    if replacements != 1:
        raise Exit(
            f"Expected to replace sha256 for exactly 1 {call.kind}(name={call.name!r}) block "
            f"in {call.relative_path}, found {replacements}. Templated names or sha256 values "
            "must be refreshed manually."
        )
    return new_text


if __name__ == "__main__":
    main()
