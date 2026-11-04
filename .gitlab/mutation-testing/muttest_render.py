#!/usr/bin/env python3
# ABOUTME: Renders gremlins JSON output into a markdown PR comment.
# ABOUTME: Reads one JSON file per package from --results-dir, writes a single report to --output.
from __future__ import annotations

import argparse
from collections import defaultdict
from io import StringIO
import json
from pathlib import Path
import sys

MAX_COMMENT_BYTES = 60_000

# Gremlins status values: KILLED, LIVED, NOT_COVERED, NOT_VIABLE, TIMED_OUT, RUN_ERROR.
# A mutant is "killed" if tests detected it, "survived" if tests failed to
# detect it, and "other" for non-actionable outcomes (non-viable, timeouts, errors).
KILLED = {"KILLED"}
SURVIVED = {"LIVED", "NOT_COVERED"}

LOCAL_CMD = ".gitlab/mutation-testing/muttest.sh"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--results-dir", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    mutants = _load_mutants(Path(args.results_dir))

    if not mutants:
        print("No mutants produced; no comment will be posted.")
        return 0

    report = _render_report(mutants, LOCAL_CMD)
    Path(args.output).write_text(report)
    return 0


def _load_mutants(results_dir: Path) -> list[dict]:
    """Load and flatten gremlins JSON mutation results from every *.json file in results_dir.

    Each returned dict has keys: file (str), line (int), column (int),
    status (str, uppercased), type (str), diff (str). Malformed JSON files are
    logged and skipped so one bad file does not blank the whole report.
    """
    mutants: list[dict] = []
    for json_file in sorted(results_dir.glob("*.json")):
        try:
            data = json.loads(json_file.read_text())
        except json.JSONDecodeError as e:
            print(f"Warning: skipping malformed JSON in {json_file}: {e}", file=sys.stderr)
            continue
        for m in data.get("files", []) or []:
            file_name = m.get("file_name", "")
            for mut in m.get("mutations", []) or []:
                mutants.append(
                    {
                        "file": file_name,
                        "line": mut.get("line", 0),
                        "column": mut.get("column", 0),
                        "status": (mut.get("status") or "").upper(),
                        "type": mut.get("type", ""),
                        "diff": mut.get("diff", ""),
                    }
                )
    return mutants


def _render_report(mutants: list[dict], local_cmd: str) -> str:
    survivors = [m for m in mutants if m["status"] in SURVIVED]

    # Shrink progressively until the body fits under the comment size cap.
    rendered = ""
    for truncate_survivors, max_table_rows in [
        (False, None),
        (True, None),
        (True, 50),
        (True, 10),
    ]:
        rendered = _render(
            mutants,
            survivors,
            local_cmd,
            truncate_survivors=truncate_survivors,
            max_table_rows=max_table_rows,
        )
        if len(rendered.encode("utf-8")) <= MAX_COMMENT_BYTES:
            return rendered
    # All truncation levels still exceed the cap; return the most-truncated
    # version rather than fail — a slightly-oversized comment beats no comment.
    return rendered


def _render(
    mutants: list[dict],
    survivors: list[dict],
    local_cmd: str,
    *,
    truncate_survivors: bool,
    max_table_rows: int | None = None,
) -> str:
    total = len(mutants)
    killed = sum(1 for m in mutants if m["status"] in KILLED)
    score = (killed / total) * 100 if total > 0 else 0.0

    out = StringIO()
    out.write("## Mutation Testing Results\n\n")
    out.write(f"**Score: {score:.1f}%** ({killed} killed / {total} total) — Advisory\n\n")

    _write_summary_table(out, mutants, max_rows=max_table_rows)

    if survivors:
        if truncate_survivors:
            out.write(
                f"\n> **{len(survivors)} surviving mutants** — per-mutant details omitted "
                f"to fit the PR comment size limit. Run locally (see footer) for the full list.\n"
            )
        else:
            _write_survivors(out, survivors)

    _write_footer(out, local_cmd)
    return out.getvalue()


def _write_summary_table(out: StringIO, mutants: list[dict], *, max_rows: int | None = None) -> None:
    by_file: dict[str, list[dict]] = defaultdict(list)
    for m in mutants:
        by_file[m["file"]].append(m)

    out.write("| File | Total | Killed | Survived | Other | Score |\n")
    out.write("|------|-------|--------|----------|-------|-------|\n")

    rows = []
    for file_path, file_mutants in by_file.items():
        total = len(file_mutants)
        killed = sum(1 for m in file_mutants if m["status"] in KILLED)
        survived = sum(1 for m in file_mutants if m["status"] in SURVIVED)
        other = total - killed - survived
        score = (killed / total) * 100 if total > 0 else 0.0
        rows.append((file_path, total, killed, survived, other, score))

    rows.sort(key=lambda row: row[5])  # sort by score, lowest first
    to_render = rows if max_rows is None else rows[:max_rows]
    for file_path, total, killed, survived, other, score in to_render:
        out.write(f"| {file_path} | {total} | {killed} | {survived} | {other} | {score:.1f}% |\n")
    if max_rows is not None and len(rows) > max_rows:
        out.write(f"| _...and {len(rows) - max_rows} more files (truncated)_ |  |  |  |  |  |\n")


def _write_survivors(out: StringIO, survivors: list[dict]) -> None:
    count = len(survivors)
    label = f"{count} surviving mutant{'s' if count != 1 else ''}"
    out.write(f"\n<details><summary>{label} (click to expand)</summary>\n\n")

    by_file: dict[str, list[dict]] = defaultdict(list)
    for s in survivors:
        by_file[s["file"]].append(s)

    for file_path, file_mutants in sorted(by_file.items()):
        out.write(f"### {file_path}\n\n")
        for m in sorted(file_mutants, key=lambda mut: (mut["line"], mut["column"])):
            out.write(f"- **Line {m['line']}** ({m['type']}) — status: `{m['status']}`\n")
            if m["diff"]:
                out.write(f"```diff\n{m['diff']}\n```\n")

    out.write("\n</details>\n")


def _write_footer(out: StringIO, local_cmd: str) -> None:
    out.write("\n---\n")
    out.write("> This check is **advisory** and does not block merge.\n")
    out.write(">\n")
    out.write(f"> **Run locally:** `{local_cmd}`\n")
    out.write(">\n")
    out.write("> To improve the score, write tests that catch the surviving mutants listed above.\n")
    out.write("> Each surviving mutant is a concrete code change that no test detects.\n")


if __name__ == "__main__":
    raise SystemExit(main())
