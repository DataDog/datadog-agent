#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

"""
Compare two `dda inv schema.codegen` outputs and assert they bind the same settings.

`dda inv schema.codegen` and `dda inv schema.codegen --keep-orig-order` are meant to
produce the same configuration bindings, but distributed over a different set of Go
functions and files. The `--keep-orig-order` variant preserves the historical
hand-written layout (many small `agent()`, `dogstatsd()`, ... functions spread over
several files), while the plain variant groups everything into two big functions.

What matters is not which function a `config.BindEnvAndSetDefault(...)` call lives in,
but *which agents run it*. Every generated function belongs to one of three categories:

  * ``serverless``: initialized by the serverless agent. Serverless is a subset of the
    full agent, so the full agent runs these too.
  * ``system_probe``: initialized by system-probe, and reachable from serverless.
  * ``full_agent``: initialized by the full agent only, never by serverless.

The full agent therefore initializes ``serverless`` + ``full_agent``, while serverless
initializes only ``serverless``. Moving a setting between the two buckets changes
serverless behaviour in one direction or the other, which is exactly what this script
looks for.

Beware the names: despite it, ``initCoreAgentFull`` is the *full_agent* bucket, not a
serverless one, and ``initEverything`` is the *serverless* one. See ``InitConfig`` in
`pkg/config/setup/config.go`.

A fourth bucket, ``unreachable``, holds generated functions that nothing calls (today
just the ordered layout's ``otherSettings`` catch-all). It must stay empty: a setting
landing there is initialized by no agent at all.

This script parses both output directories, buckets every setting-registering call into
one of those categories, and verifies the categories hold the same calls on both sides.
Order inside a category is irrelevant and is expected to differ.

It never runs the code generator: point it at two already-generated directories.

Usage:
    python tasks/schema/compare_codegen.py <dir-a> <dir-b>
    python tasks/schema/compare_codegen.py /tmp/ordered /tmp/unordered

Exits 0 when both directories agree, 1 otherwise.
"""

from __future__ import annotations

import argparse
import bisect
import os
import re
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass

# Calls that actually register a setting: these are what must match between the two
# codegen variants. Anything else (helpers, platform switches, ...) only ever appears
# as an argument of one of these.
DEFAULT_CHECKED_METHODS = frozenset(
    {
        "BindEnvAndSetDefault",
        "SetDefault",
        "ParseEnvJSON",
        "ParseEnvJSONOrComma",
        "ParseEnvJSONOrSpace",
        "ParseEnvCSVSplit",
        "ParseEnvSplitComma",
        "ParseEnvSplitCommaAndSpace",
        "ParseEnvSplitCommaThenSpace",
        "ParseEnvTraceSpan",
    }
)

# Receivers whose statement-level calls are expected to register a setting. A statement
# on one of these calling an unlisted method means the code generator grew a new
# binding helper and the checked set needs updating -- the script warns about it.
SETTING_RECEIVERS = frozenset({"config", "pkgconfighelper"})

SERVERLESS = "serverless"
SYSTEM_PROBE = "system_probe"
FULL_AGENT = "full_agent"
UNREACHABLE = "unreachable"
CATEGORIES = (SERVERLESS, SYSTEM_PROBE, FULL_AGENT, UNREACHABLE)

CATEGORY_DESCRIPTION = {
    SERVERLESS: "initialized by serverless, and so by the full agent too",
    SYSTEM_PROBE: "initialized by system-probe, reachable from serverless",
    FULL_AGENT: "initialized by the full agent only, never by serverless",
    UNREACHABLE: "initialized by no agent -- must be empty",
}

# Which generated function goes in which category.
#
# Kept hardcoded on purpose: this is the contract the comparison is checking, so it must
# be stated here rather than inferred from the code under test. Any function missing
# from this table is a hard error, not a guess -- if the code generator grows a new
# function, wire it into `pkg/config/setup/config.go` and add it here.
#
# Cross-check against `InitConfig` in `pkg/config/setup/config.go`:
#   - `serverless`  <- the `commonConfigComponents` list
#   - `full_agent`  <- `initCoreAgentFull` plus the `comps` list of
#                      `initFullAgentOnlyComponents`
FUNC_CATEGORY = {
    # --- serverless: the commonConfigComponents list ---------------------------------
    "agent": SERVERLESS,
    "fips": SERVERLESS,
    "dogstatsd": SERVERLESS,
    "forwarder": SERVERLESS,
    "aggregator": SERVERLESS,
    "serializer": SERVERLESS,
    "serverless": SERVERLESS,
    "setupAPM": SERVERLESS,
    "OTLP": SERVERLESS,
    "setupMultiRegionFailover": SERVERLESS,
    "telemetry": SERVERLESS,
    "autoconfig": SERVERLESS,
    "remoteconfig": SERVERLESS,
    "logsagent": SERVERLESS,
    "containerSyspath": SERVERLESS,
    "containerd": SERVERLESS,
    "cri": SERVERLESS,
    "kubernetes": SERVERLESS,
    "cloudfoundry": SERVERLESS,
    "debugging": SERVERLESS,
    "vector": SERVERLESS,
    "podman": SERVERLESS,
    "fleet": SERVERLESS,
    "autoscaling": SERVERLESS,
    # The grouped layout emits the whole list above as a single function.
    "initEverything": SERVERLESS,
    # --- full_agent: initCoreAgentFull + initFullAgentOnlyComponents -----------------
    # Emitted by both layouts. Named "CoreAgentFull" for the *full agent*, not for
    # serverless.
    "initCoreAgentFull": FULL_AGENT,
    "setupProcesses": FULL_AGENT,
    "setupPrivateActionRunner": FULL_AGENT,
    "remoteflags": FULL_AGENT,
    "anomalyDetection": FULL_AGENT,
    # --- system_probe: system_probe_settings.go --------------------------------------
    "initMainSystemProbeConfig": SYSTEM_PROBE,
    "initCWSSystemProbeConfig": SYSTEM_PROBE,
    "initUSMSystemProbeConfig": SYSTEM_PROBE,
    # The grouped layout emits the three above as a single function.
    "initSystemProbeConfig": SYSTEM_PROBE,
    # --- unreachable -----------------------------------------------------------------
    # The ordered layout's catch-all for settings it found no hint function for. Nothing
    # in config.go calls it, so anything landing in it is dead config.
    "otherSettings": UNREACHABLE,
}

# `system_probe` populates the `system-probe` config object, every other category
# populates `datadog`. The two are separate namespaces, so the same setting path
# legitimately exists in both (`log_level`, `secret_backend_command`, ...) and only a
# repeat within one object is a real duplicate.
CONFIG_OBJECT = {
    SERVERLESS: "datadog",
    FULL_AGENT: "datadog",
    UNREACHABLE: "datadog",
    SYSTEM_PROBE: "system-probe",
}

FUNC_RE = re.compile(r'^func\s+(?:\([^)]*\)\s*)?(\w+)\s*\(', re.MULTILINE)
CALL_RE = re.compile(r'(?<![\w.])(\w+)\.(\w+)\s*\(')


class ComparisonError(Exception):
    """A problem that prevents the comparison from running at all."""


def _is_word_char(char: str) -> bool:
    return char.isalnum() or char == '_'


def lex(text: str) -> tuple[str, list[bool]]:
    """Blank out Go comments and flag the offsets that sit inside a literal.

    Returns ``(code, in_literal)`` where ``code`` is ``text`` with every comment
    character replaced by a space (newlines kept, so offsets and line numbers stay
    valid) and ``in_literal[i]`` is True when ``text[i]`` belongs to a string or rune
    literal, quotes included. Both are needed to brace-match without being fooled by a
    parenthesis inside a comment or a string.
    """
    code = list(text)
    in_literal = [False] * len(text)
    i, n = 0, len(text)

    while i < n:
        char = text[i]

        if char == '/' and i + 1 < n and text[i + 1] == '/':
            end = text.find('\n', i)
            end = n if end == -1 else end
            code[i:end] = ' ' * (end - i)
            i = end
        elif char == '/' and i + 1 < n and text[i + 1] == '*':
            end = text.find('*/', i + 2)
            end = n if end == -1 else end + 2
            for offset in range(i, end):
                if code[offset] != '\n':
                    code[offset] = ' '
            i = end
        elif char in '"`\'':
            quote = char
            in_literal[i] = True
            i += 1
            while i < n:
                char = text[i]
                in_literal[i] = True
                if quote != '`' and char == '\\':
                    if i + 1 < n:
                        in_literal[i + 1] = True
                    i += 2
                    continue
                i += 1
                if char == quote:
                    break
                if quote != '`' and char == '\n':
                    # Unterminated literal: stop here rather than swallowing the file.
                    break
        else:
            i += 1

    return ''.join(code), in_literal


def match_delimiter(code: str, in_literal: list[bool], start: int, opening: str, closing: str) -> int:
    """Return the offset of the delimiter closing the one at ``start``."""
    depth = 0
    for i in range(start, len(code)):
        if in_literal[i]:
            continue
        if code[i] == opening:
            depth += 1
        elif code[i] == closing:
            depth -= 1
            if depth == 0:
                return i
    raise ComparisonError(f"unbalanced {opening}{closing} starting at offset {start}")


def normalize(snippet: str) -> str:
    """Collapse a call to a whitespace- and comment-insensitive canonical form.

    Both variants are gofmt'd, but the same call can be laid out differently (a
    composite literal split over several lines in one variant, inline in the other) and
    can carry different doc comments. Only the code itself matters here.
    """
    code, in_literal = lex(snippet)
    out: list[str] = []
    out_literal: list[bool] = []
    pending_space = False

    def append(char: str, is_literal: bool) -> None:
        out.append(char)
        out_literal.append(is_literal)

    for char, is_literal in zip(code, in_literal, strict=False):
        if is_literal:
            append(char, True)
            pending_space = False
            continue
        if char.isspace():
            pending_space = True
            continue
        # gofmt requires a trailing comma when a composite literal is split over several
        # lines and forbids it inline, so `{"a", "b",}` and `{"a", "b"}` are the same
        # code laid out differently.
        if char in ')]}' and out and out[-1] == ',' and not out_literal[-1]:
            out.pop()
            out_literal.pop()
        # Keep a single space only where dropping it would glue two words together:
        # `map[string]any` is safe to tighten, `struct{a int}` is not.
        if pending_space and out and _is_word_char(out[-1]) and _is_word_char(char):
            append(' ', False)
        pending_space = False
        append(char, is_literal)

    return ''.join(out)


def first_string_argument(snippet: str) -> str | None:
    """Return the raw first string literal of a call, i.e. the setting path."""
    code, in_literal = lex(snippet)
    open_paren = code.index('(')
    for i in range(open_paren + 1, len(code)):
        if in_literal[i]:
            end = i + 1
            while end < len(code) and in_literal[end]:
                end += 1
            return snippet[i:end]
        if code[i] == ',':
            break
    return None


def _line_starts(text: str) -> list[int]:
    starts = [0]
    for i, char in enumerate(text):
        if char == '\n':
            starts.append(i + 1)
    return starts


def _find_functions(code: str, in_literal: list[bool]) -> list[tuple[str, int, int]]:
    """Return ``(name, body_start, body_end)`` for every function in the file."""
    functions = []
    for match in FUNC_RE.finditer(code):
        if in_literal[match.start()]:
            continue
        params_end = match_delimiter(code, in_literal, match.end() - 1, '(', ')')
        body_start = code.index('{', params_end)
        body_end = match_delimiter(code, in_literal, body_start, '{', '}')
        functions.append((match.group(1), body_start, body_end))
    return functions


@dataclass(frozen=True)
class Call:
    method: str
    setting: str | None
    normalized: str
    path: str
    line: int
    func: str
    category: str

    @property
    def key(self) -> tuple[str, str]:
        """Identity of the binding, independent of its arguments."""
        return (self.method, self.setting if self.setting is not None else self.normalized)

    @property
    def location(self) -> str:
        return f"{self.path}:{self.line} ({self.func})"


def categorize(path: str, func: str) -> str:
    try:
        return FUNC_CATEGORY[func]
    except KeyError:
        raise ComparisonError(
            f"{path}: unknown generated function {func}(). Determine which agents "
            f"initialize it (see InitConfig in pkg/config/setup/config.go) and add it to "
            f"FUNC_CATEGORY in this script."
        ) from None


def parse_file(path: str, checked_methods: frozenset[str]) -> tuple[list[Call], list[str]]:
    """Extract every setting-registering call of a Go file, plus helper warnings."""
    with open(path, encoding='utf-8') as file:
        text = file.read()

    code, in_literal = lex(text)
    functions = _find_functions(code, in_literal)
    func_starts = [start for _, start, _ in functions]
    line_starts = _line_starts(text)

    def enclosing_func(offset: int) -> str:
        index = bisect.bisect_right(func_starts, offset) - 1
        if index < 0:
            raise ComparisonError(f"{path}: call at offset {offset} is outside any function")
        name, _, end = functions[index]
        if offset >= end:
            raise ComparisonError(f"{path}: call at offset {offset} is outside any function")
        return name

    calls: list[Call] = []
    warnings: list[str] = []
    consumed_until = -1

    for match in CALL_RE.finditer(code):
        start = match.start()
        if in_literal[start]:
            continue

        receiver, method = match.group(1), match.group(2)
        line = bisect.bisect_right(line_starts, start)

        if method not in checked_methods:
            statement_level = not code[line_starts[line - 1] : start].strip()
            if statement_level and receiver in SETTING_RECEIVERS:
                warnings.append(
                    f"{path}:{line}: unchecked statement `{receiver}.{method}(...)` -- "
                    f"add it to the checked methods if it registers a setting"
                )
            continue

        # A checked call nested inside another checked call is already covered by the
        # enclosing snippet; don't count it twice.
        if start <= consumed_until:
            continue

        end = match_delimiter(code, in_literal, match.end() - 1, '(', ')')
        consumed_until = end
        snippet = text[start : end + 1]
        func = enclosing_func(start)
        calls.append(
            Call(
                method=method,
                setting=first_string_argument(snippet),
                normalized=normalize(snippet),
                path=path,
                line=line,
                func=func,
                category=categorize(path, func),
            )
        )

    return calls, warnings


def parse_dir(directory: str, checked_methods: frozenset[str]) -> tuple[list[Call], list[str]]:
    if not os.path.isdir(directory):
        raise ComparisonError(f"{directory} is not a directory")

    files = sorted(name for name in os.listdir(directory) if name.endswith('.go'))
    if not files:
        raise ComparisonError(f"no .go file found in {directory}")

    calls: list[Call] = []
    warnings: list[str] = []
    for name in files:
        file_calls, file_warnings = parse_file(os.path.join(directory, name), checked_methods)
        calls.extend(file_calls)
        warnings.extend(file_warnings)

    if not calls:
        raise ComparisonError(f"no setting-registering call found in {directory}")
    return calls, warnings


def by_category(calls: list[Call]) -> dict[str, list[Call]]:
    grouped: dict[str, list[Call]] = {category: [] for category in CATEGORIES}
    for call in calls:
        grouped[call.category].append(call)
    return grouped


def _index(calls: list[Call]) -> dict[tuple[str, str], list[Call]]:
    index: dict[tuple[str, str], list[Call]] = defaultdict(list)
    for call in calls:
        index[call.key].append(call)
    return index


def _describe(key: tuple[str, str]) -> str:
    method, setting = key
    return f"{method}({setting})"


@dataclass
class Divergence:
    """One way in which the two directories disagree about a setting."""

    kind: str
    key: tuple[str, str]
    detail: str
    lines: list[str]


ONLY_IN_A = "only on one side"
RECATEGORIZED = "different category"
CODE_DIFFERS = "different generated code"
DIVERGENCE_KINDS = (ONLY_IN_A, RECATEGORIZED, CODE_DIFFERS)

KIND_EXPLANATION = {
    ONLY_IN_A: "one directory registers the setting and the other does not at all",
    RECATEGORIZED: (
        f"the setting is registered by a different set of agents; "
        f"'{FULL_AGENT} -> {SERVERLESS}' means serverless starts initializing it, "
        f"'{SERVERLESS} -> {FULL_AGENT}' means it stops"
    ),
    CODE_DIFFERS: "same category, but the emitted call or its multiplicity differs",
}


def _placement(calls: list[Call]) -> str:
    """Describe which categories register a setting, and how many times each."""
    counts = Counter(call.category for call in calls)
    return ', '.join(
        f"{category}" + (f" x{counts[category]}" if counts[category] > 1 else "")
        for category in CATEGORIES
        if counts[category]
    )


def _locations(calls: list[Call]) -> str:
    return ', '.join(call.location for call in calls)


def compare(
    a_calls: list[Call],
    b_calls: list[Call],
    label_a: str,
    label_b: str,
) -> list[Divergence]:
    """Compare the two sides setting by setting, reporting each disagreement once.

    Categories are compared per setting rather than per bucket: reporting a moved
    setting as "missing here, extra there" would double every recategorization and hide
    what actually changed, which is the set of agents that initialize it.
    """
    a_index, b_index = _index(a_calls), _index(b_calls)
    divergences: list[Divergence] = []

    for key in sorted(a_index.keys() | b_index.keys()):
        a_group, b_group = a_index.get(key, []), b_index.get(key, [])

        if not b_group or not a_group:
            present, label = (a_group, label_a) if a_group else (b_group, label_b)
            missing = label_b if a_group else label_a
            divergences.append(
                Divergence(
                    kind=ONLY_IN_A,
                    key=key,
                    detail=f"only in {label}, absent from {missing}",
                    lines=[f"{label}: {_locations(present)}"],
                )
            )
            continue

        a_places = Counter(call.category for call in a_group)
        b_places = Counter(call.category for call in b_group)
        if a_places != b_places:
            divergences.append(
                Divergence(
                    kind=RECATEGORIZED,
                    key=key,
                    detail=f"{_placement(a_group)} -> {_placement(b_group)}",
                    lines=[
                        f"{label_a}: {_locations(a_group)}",
                        f"{label_b}: {_locations(b_group)}",
                    ],
                )
            )
            continue

        # Same categories on both sides: the generated call itself must match, including
        # how many times it is emitted.
        a_forms = Counter(call.normalized for call in a_group)
        b_forms = Counter(call.normalized for call in b_group)
        if a_forms == b_forms:
            continue

        lines = [
            f"{label_a} x{a_forms[form]}, {label_b} x{b_forms[form]}: {form}"
            for form in sorted(set(a_forms) | set(b_forms))
        ]
        lines.append(f"{label_a}: {_locations(a_group)}")
        lines.append(f"{label_b}: {_locations(b_group)}")
        divergences.append(Divergence(kind=CODE_DIFFERS, key=key, detail="", lines=lines))

    return divergences


def find_duplicates(calls: list[Call]) -> dict[tuple[str, str], list[Call]]:
    """Return the settings registered twice within the same config object."""
    duplicates = {}
    for key, group in sorted(_index(calls).items()):
        for _, repeated in sorted(_group_by_config_object(group).items()):
            if len(repeated) > 1:
                duplicates.setdefault(key, []).extend(repeated)
    return duplicates


def _group_by_config_object(calls: list[Call]) -> dict[str, list[Call]]:
    grouped: dict[str, list[Call]] = defaultdict(list)
    for call in calls:
        grouped[CONFIG_OBJECT[call.category]].append(call)
    return grouped


def print_summary(calls: list[Call], label: str) -> None:
    grouped = by_category(calls)
    print(f"{label}:")
    for category in CATEGORIES:
        bucket = grouped[category]
        print(f"  {category:<18} {len(bucket):>5} calls   {CATEGORY_DESCRIPTION[category]}")
        if not bucket:
            continue
        methods = Counter(call.method for call in bucket)
        print(f"  {'':<18}       in: {', '.join(sorted({call.func for call in bucket}))}")
        print(f"  {'':<18}       {', '.join(f'{name}={count}' for name, count in sorted(methods.items()))}")

    # Spell out the subset relation: serverless runs a strict subset of what the full
    # agent runs, so the interesting number is how much each agent ends up with.
    serverless_total = len(grouped[SERVERLESS])
    print(f"  => serverless initializes {serverless_total} datadog settings")
    print(f"  => full agent initializes {serverless_total + len(grouped[FULL_AGENT])} datadog settings")
    print(f"  => system-probe initializes {len(grouped[SYSTEM_PROBE])} system-probe settings")


def print_duplicates(calls: list[Call], label: str, max_examples: int) -> int:
    """Report settings registered twice in one config object. Returns the count."""
    duplicates = find_duplicates(calls)
    if not duplicates:
        print(f"OK   {label}: no setting registered twice in the same config object")
        return 0

    print(f"FAIL {label}: {len(duplicates)} setting(s) registered twice in the same config object")
    for key, group in list(duplicates.items())[: max_examples or None]:
        print(f"  {_describe(key)} x{len(group)} [{_placement(group)}]")
        for call in group:
            print(f"      {call.location}")
    if max_examples and len(duplicates) > max_examples:
        print(f"  ... and {len(duplicates) - max_examples} more")
    return len(duplicates)


def print_divergences(divergences: list[Divergence], max_examples: int) -> None:
    by_kind: dict[str, list[Divergence]] = defaultdict(list)
    for divergence in divergences:
        by_kind[divergence.kind].append(divergence)

    for kind in DIVERGENCE_KINDS:
        group = by_kind.get(kind, [])
        if not group:
            print(f"OK   {kind}: none")
            continue

        print(f"FAIL {kind}: {len(group)} setting(s) -- {KIND_EXPLANATION[kind]}")

        # A breakdown first: with ~1000 recategorized settings the shape of the change
        # is the useful part, the individual paths are just evidence.
        shapes = Counter(divergence.detail for divergence in group if divergence.detail)
        for shape, count in shapes.most_common():
            print(f"       {count:>5}  {shape}")

        for divergence in group[: max_examples or None]:
            print(f"  {_describe(divergence.key)}")
            for line in divergence.lines:
                print(f"      {line}")
        if max_examples and len(group) > max_examples:
            print(f"  ... and {len(group) - max_examples} more, re-run with --max-examples 0 for all")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument('dir_a', help="a codegen output directory, e.g. the --keep-orig-order one")
    parser.add_argument('dir_b', help="the other codegen output directory")
    parser.add_argument('--methods', help="comma-separated list of methods to check, replacing the default set")
    parser.add_argument(
        '--max-examples',
        type=int,
        default=10,
        help="maximum number of settings detailed per problem kind, 0 for no limit (default: 10)",
    )
    parser.add_argument('--quiet', action='store_true', help="skip the per-category summary")
    args = parser.parse_args(argv)

    checked_methods = DEFAULT_CHECKED_METHODS
    if args.methods:
        checked_methods = frozenset(method.strip() for method in args.methods.split(',') if method.strip())
        if not checked_methods:
            parser.error("--methods is empty")

    label_a = os.path.basename(args.dir_a.rstrip('/')) or args.dir_a
    label_b = os.path.basename(args.dir_b.rstrip('/')) or args.dir_b
    if label_a == label_b:
        label_a, label_b = args.dir_a, args.dir_b

    try:
        a_calls, a_warnings = parse_dir(args.dir_a, checked_methods)
        b_calls, b_warnings = parse_dir(args.dir_b, checked_methods)
    except ComparisonError as err:
        print(f"error: {err}", file=sys.stderr)
        return 2

    for warning in dict.fromkeys(a_warnings + b_warnings):
        print(f"warning: {warning}", file=sys.stderr)

    if not args.quiet:
        print_summary(a_calls, label_a)
        print()
        print_summary(b_calls, label_b)
        print()

    failures = 0
    for calls, label in ((a_calls, label_a), (b_calls, label_b)):
        failures += print_duplicates(calls, label, args.max_examples)

    # `unreachable` settings are left out of the pairwise comparison and get their own
    # check below: "no agent initializes this" is the actionable framing, and reporting
    # them here too would just say the same thing twice.
    reachable_a = [call for call in a_calls if call.category != UNREACHABLE]
    reachable_b = [call for call in b_calls if call.category != UNREACHABLE]

    divergences = compare(reachable_a, reachable_b, label_a, label_b)
    print_divergences(divergences, args.max_examples)
    failures += len(divergences)

    dead = [call for call in a_calls + b_calls if call.category == UNREACHABLE]
    if dead:
        failures += len(dead)
        print(f"FAIL {UNREACHABLE}: {len(dead)} setting(s) that no agent initializes")
        for call in dead[: args.max_examples or None]:
            print(f"  {_describe(call.key)}\n      {call.location}")
    else:
        print(f"OK   {UNREACHABLE}: empty")

    if failures:
        print(f"\n{label_a} and {label_b} do not initialize the same settings.")
        return 1

    print(f"\n{label_a} and {label_b} initialize the same settings in every category.")
    return 0


if __name__ == '__main__':
    sys.exit(main())
