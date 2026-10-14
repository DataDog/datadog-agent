"""
Comparison of the trace payload duplicates reports of two commits.

The reports are the JSON summaries written by TestNoAgentIntroducedDuplicates
(pkg/trace/testutil/dupcheck) with -dupcheck.json. An issue is identified by
its subject, the location the agent wrote: its title changes when it gets a
second partner. The report is deterministic, so every difference is reported,
including count changes: a new copy may only add occurrences and partners to
an existing issue.
"""

from __future__ import annotations

DUPCHECK_DIR = "pkg/trace/testutil/dupcheck"

# _MAX_PARTNER_COUNTS is the number of partner count changes listed per issue.
_MAX_PARTNER_COUNTS = 3

FRESH_REPORT_COMMAND = (
    f'dda inv test --targets=./{DUPCHECK_DIR} '
    '--test-args="-test.run=TestNoAgentIntroducedDuplicates -dupcheck.report -test.v" '
    '--bazel-args="--test_output=all"'
)

_INTRO = (
    "Values the agent writes to more than one location of the trace payloads it sends, "
    f"as reported by `TestNoAgentIntroducedDuplicates` in `{DUPCHECK_DIR}`. "
    "This report doesn't block the PR."
)


def compare_reports(base: dict, head: dict) -> list[dict]:
    """
    Return the issues that differ between the base and head reports, in head
    report order followed by the issues gone from head. Each change has a kind
    (new, changed or gone), the base and head issues (None when absent), the
    partners added and removed, and the partners kept whose count changed.
    """
    base_issues = {i["subject"]: i for i in base["issues"]}
    head_issues = {i["subject"]: i for i in head["issues"]}
    changes = []
    for subject, h in head_issues.items():
        b = base_issues.get(subject)
        if b is None:
            changes.append(_change("new", None, h))
            continue
        change = _change("changed", b, h)
        if b["count"] != h["count"] or b["partners"] != h["partners"]:
            changes.append(change)
    for subject, b in base_issues.items():
        if subject not in head_issues:
            changes.append(_change("gone", b, None))
    return changes


def _change(kind: str, base: dict | None, head: dict | None) -> dict:
    base_partners = set(base["partners"]) if base else set()
    head_partners = set(head["partners"]) if head else set()
    return {
        "kind": kind,
        "base": base,
        "head": head,
        "new_partners": sorted(head_partners - base_partners) if base else [],
        "removed_partners": sorted(base_partners - head_partners) if head else [],
        # A finding can repeat several partners, so the count of a partner can
        # change while the count of the issue and its partner names don't.
        "changed_partners": [
            (p, base["partners"][p], head["partners"][p])
            for p in sorted(base_partners & head_partners)
            if base["partners"][p] != head["partners"][p]
        ],
    }


def render_diff(base: dict, head: dict, baseline_ref: str, checker_changed: bool) -> str:
    """
    Return the Markdown comparison of the base and head reports, or an empty
    string when they have the same issues.
    """
    changes = compare_reports(base, head)
    if not changes:
        return ""
    lines = [
        f"{_INTRO} Compared with the merge base {_commit_link(baseline_ref)}.",
        "",
        f"**{_totals(base, head)}**",
        "",
        "| Change | Issue | Occurrences | Details |",
        "|---|---|---|---|",
    ]
    for c in changes:
        lines.append(_change_row(c))
    if checker_changed:
        lines += [
            "",
            f"> [!NOTE]\n> This PR changes `{DUPCHECK_DIR}`, so part of the difference may come from the checker itself.",
        ]
    lines += ["", *_full_report(head)]
    return "\n".join(lines) + "\n"


def render_without_baseline(head: dict, baseline_ref: str) -> str:
    """Return the Markdown report of head, for a merge base without a report."""
    lines = [
        f"{_INTRO} The merge base {_commit_link(baseline_ref)} has no report to compare with.",
        "",
        f"**{_plural(len(head['issues']), 'issue')}, {_plural(head['occurrences'], 'occurrence')}**",
        "",
        *_full_report(head),
    ]
    return "\n".join(lines) + "\n"


def _totals(base: dict, head: dict) -> str:
    issues = f"{_plural(len(head['issues']), 'issue')}{_delta(len(base['issues']), len(head['issues']))}"
    occurrences = f"{_plural(head['occurrences'], 'occurrence')}{_delta(base['occurrences'], head['occurrences'])}"
    return f"{issues}, {occurrences}"


def _change_row(c: dict) -> str:
    issue = c["head"] or c["base"]
    title = f"`{_escape(issue['title'])}`"
    details = []
    if c["kind"] == "new":
        occurrences = f"{c['head']['count']:,}"
        details.append("created by: " + ", ".join(c["head"]["created_by"]))
    elif c["kind"] == "gone":
        occurrences = f"{c['base']['count']:,} → 0"
    else:
        b, h = c["base"]["count"], c["head"]["count"]
        occurrences = f"{h:,}" if b == h else f"{b:,} → {h:,} ({h - b:+,})"
    if c["new_partners"]:
        details.append("new partners: " + ", ".join(f"`{_escape(p)}`" for p in c["new_partners"]))
    if c["removed_partners"]:
        details.append("removed partners: " + ", ".join(f"`{_escape(p)}`" for p in c["removed_partners"]))
    if c["changed_partners"]:
        # The largest changes first, at most _MAX_PARTNER_COUNTS of them, as an
        # issue can have dozens of partners.
        changed = sorted(c["changed_partners"], key=lambda x: (-abs(x[2] - x[1]), x[0]))
        items = [f"`{_escape(p)}` {b:,} → {h:,}" for p, b, h in changed[:_MAX_PARTNER_COUNTS]]
        if len(changed) > _MAX_PARTNER_COUNTS:
            items.append(f"+{len(changed) - _MAX_PARTNER_COUNTS:,} more")
        details.append("partner counts: " + ", ".join(items))
    return f"| {c['kind'].capitalize()} | {title} | {occurrences} | {'<br>'.join(details)} |"


def _full_report(head: dict) -> list[str]:
    return [
        "<details><summary>Full report</summary>",
        "",
        "```",
        head["text"],
        "```",
        "",
        "</details>",
        "",
        f"To print a fresh report locally: `{FRESH_REPORT_COMMAND}`",
    ]


def _commit_link(sha: str) -> str:
    return f"[{sha[:8]}](https://github.com/DataDog/datadog-agent/commit/{sha})"


def _delta(base: int, head: int) -> str:
    return "" if base == head else f" ({head - base:+,})"


def _plural(n: int, word: str) -> str:
    return f"{n:,} {word}" if n == 1 else f"{n:,} {word}s"


def _escape(s: str) -> str:
    return s.replace("|", "\\|")
