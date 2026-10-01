#!/usr/bin/env python3
"""Best-effort generation of report.v1.json from an unversioned report.json.

Both files are emitted by consignor-report from the same Builder state:
- report.json:     Builder::into_json     (services/consignor/src/report.rs)
- report.v1.json:  Builder::into_json_v1 (services/consignor/src/report/json_v1.rs)
                   contract: libs/report/src/v1.rs

What is derivable from report.json:
- job block (id, shas, tolerances from job_info; query window from time_range)
- experiments:
  - optimization_goal (goal token, trial_count = trial_index + 1, result;
    is_regression maps to is_significant_change)
  - bounds_checks, comparison variant only, aggregated per check name
    (pass_count / total_count / min_observed / max_observed)
    Empty checks are retained with zero counts so Agent quality gates fail
    when the legacy report has no comparison data.
  - erratic (goal row first, else first bounds check, else False)
- failed_replicates (from failed_replicates.executions; buckets sorted by
  replicate index; list stable-sorted by experiment then variant)

What report.json does NOT contain (placeholders emitted):
- bounds_checks[].series -> the check name as a placeholder (the metric series
  name is dropped by the unversioned serializer; the v1 schema requires a
  string, so null would not parse)
- bounds_checks[].unit -> guessed by running the Rust detect_units heuristic
  on the check name; can be wrong when name and series differ
- quantile_checks -> [] (cairn quantile results are not serialized at all)
- analysis_errors -> []
- experiments whose only data was a no-optimization-goal row, quantile rows,
  or analysis errors: they never appear in report.json and are omitted.

Usage:
  convert_old_report_to_v1.py report.json [--output PATH] [--force]
  convert_old_report_to_v1.py DIR_OR_REPORT... --compare

--compare generates in memory and diffs against the existing report.v1.json
next to each input, classifying differences into known gaps vs unexpected.

Exit status: 0 on success; in --compare mode, 1 if any unexpected difference.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

SCHEMA_ID = "tag:datadoghq.dev,2026-09-10:single-machine-performance/schemas/report/v1"

# OptimizationGoal serializes as PascalCase in report.json, as a snake_case
# token in v1 (json_v1.rs goal_token).
GOAL_TOKENS = {
    "Cpu": "cpu",
    "Memory": "memory",
    "IngressThroughput": "ingress_throughput",
    "EgressThroughput": "egress_throughput",
}

# Mirror of DimensionedCheckValue::detect_units (report.rs), applied to the
# check name instead of the (unavailable) series name.
BYTES_REGEX = re.compile(r".*bytes.*(?:\['.*'\])?")
THROUGHPUT_REGEX = re.compile(r"rate(.+)")

# Check names whose underlying series is byte-dimensioned even though the
# name itself does not contain "bytes". The unversioned report.json drops
# the series name, so these cannot be derived; they are asserted from
# observed name/series pairs instead (e.g. memory_usage -> total_pss_bytes).
CHECK_NAME_UNIT_OVERRIDES = {
    "memory_usage": "bytes",
}


VARIANT_ORDER = {"baseline": 0, "comparison": 1}


def detect_unit(name: str) -> str:
    """Guess v1 Unit for a check, using the check name as series proxy."""
    if name in CHECK_NAME_UNIT_OVERRIDES:
        return CHECK_NAME_UNIT_OVERRIDES[name]
    if BYTES_REGEX.search(name):
        return "bytes"
    if THROUGHPUT_REGEX.search(name):
        return "bytes_per_second"
    return "none"


def goal_token(goal: str) -> str:
    if goal in GOAL_TOKENS:
        return GOAL_TOKENS[goal]
    # fallback: CamelCase -> snake_case
    token = "@" + re.sub(r"(?<!^)(?=[A-Z])", "_", goal).lower()
    print(f"warning: unknown optimization goal {goal!r}, token {token!r}", file=sys.stderr)
    return token


def build_optimization_goal(raw: dict) -> dict:
    ran = raw.get("analysis_ran_successfully", False)
    if ran:
        goal = goal_token(raw["optimization_goal"])
        result = {
            "percent_change": raw["percent_change"],
            "confidence_interval": {
                "lower": raw["percent_change_ci_lb"],
                "upper": raw["percent_change_ci_ub"],
            },
            "confidence_percent": raw["confidence_percent"],
            "is_significant_change": raw["is_regression"],
            "is_improvement": raw["is_improvement"],
            "is_erratic": raw["is_erratic"],
        }
    else:
        goal = None
        result = None
    return {
        "goal": goal,
        "trial_count": raw.get("trial_index", 0) + 1,
        "max_trial_count": raw.get("max_trial_count", 0),
        "result": result,
    }


def build_bounds_check(name: str, raw: dict) -> dict | None:
    """Aggregate one report.json bounds check into a v1 BoundsCheck.

    Retain empty comparison results with zero counts. The native v1 serializer
    omits these checks, but keeping them lets Agent CI detect missing gate data.
    """
    if raw.get("bounds_check_type") != "bounds":
        print(f"warning: skipping non-bounds check {name!r}", file=sys.stderr)
        return None
    replicates = (raw.get("results") or {}).get("comparison") or []

    mins = [r["min_observed"] for r in replicates if r.get("min_observed") is not None]
    maxs = [r["max_observed"] for r in replicates if r.get("max_observed") is not None]

    return {
        "name": name,
        # known gap: the real series name is not present in report.json;
        # the check name is used as a placeholder so the file still parses
        # against the v1 schema (series is a required String)
        "series": name,
        "unit": detect_unit(name),  # heuristic guess (known gap)
        "lower_bound": raw.get("lower_bound"),
        "upper_bound": raw.get("upper_bound"),
        "pass_count": sum(1 for r in replicates if r.get("passed")),
        "total_count": len(replicates),
        "min_observed": min(mins) if mins else None,
        "max_observed": max(maxs) if maxs else None,
    }


def build_experiment(name: str, raw: dict) -> dict:
    goal_raw = raw.get("optimization_goal")
    checks_raw = raw.get("bounds_checks") or {}

    if goal_raw is not None:
        erratic = goal_raw.get("reported_erratic", False)
        goal = build_optimization_goal(goal_raw)
    else:
        # json_v1.rs: erratic set from first-seen row; all rows of an
        # experiment share the same config value, so take any check's.
        first = next(iter(checks_raw.values()), None)
        erratic = first.get("reported_erratic", False) if first else False
        goal = None

    bounds_checks = []
    for check_name in sorted(checks_raw):
        check = build_bounds_check(check_name, checks_raw[check_name])
        if check is not None:
            bounds_checks.append(check)

    return {
        "name": name,
        "erratic": erratic,
        "optimization_goal": goal,
        "bounds_checks": bounds_checks,
        "quantile_checks": [],  # not present in report.json (known gap)
    }


def build_failed_replicates(raw: dict) -> list:
    rows = []
    for exe in (raw or {}).get("executions", []):
        variant = exe.get("variant")
        if variant not in VARIANT_ORDER:
            print(
                f"warning: unsupported variant {variant!r}, dropping failed "
                f"replicate row for {exe.get('experiment')!r}",
                file=sys.stderr,
            )
            continue
        reps = [{"replicate_index": r["replicate_idx"], "count": r["count"]} for r in exe.get("replicates", [])]
        reps.sort(key=lambda r: r["replicate_index"])
        rows.append(
            {
                "experiment": exe["experiment"],
                "variant": variant,
                "replicate_type": exe["replicate_type"],
                "failure": exe["failure"],
                "is_dead": exe["is_dead"],
                "replicates": reps,
            }
        )
    # json_v1.rs: stable sort by (experiment, variant); ties keep row order.
    rows.sort(key=lambda r: (r["experiment"], VARIANT_ORDER[r["variant"]]))
    return rows


def convert(report: dict) -> dict:
    job_info = report.get("job_info")
    time_range = report.get("time_range")
    if not job_info or not time_range:
        raise ValueError("report.json is missing job_info or time_range")

    experiments = [
        build_experiment(name, report["experiments"][name]) for name in sorted(report.get("experiments") or {})
    ]

    return {
        "$schema": SCHEMA_ID,
        "job": {
            "id": str(report["job_id"]),
            "baseline_sha": job_info["baseline_sha"],
            "comparison_sha": job_info["comparison_sha"],
            "metrics_query_start": time_range["metrics_query_start"],
            "metrics_query_end": time_range["metrics_query_end"],
            "tolerances": {
                "p_value": job_info["p_value"],
                "effect_size": job_info["effect_size"],
                "coefficient_of_variation_limit": job_info["coefficient_of_variation_limit"],
            },
        },
        "experiments": experiments,
        "analysis_errors": [],  # not present in report.json (known gap)
        "failed_replicates": build_failed_replicates(report.get("failed_replicates")),
    }


# ---------------------------------------------------------------------------
# --compare support
# ---------------------------------------------------------------------------


def diff_v1(generated: dict, expected: dict) -> tuple[list, list]:
    """Return (known_diffs, unexpected_diffs) between two v1 reports.

    Matching is done by experiment / check name, not list index.
    """
    known: list[str] = []
    unexpected: list[str] = []

    if generated["job"] != expected["job"]:
        unexpected.append("job block differs")

    gen_exps = {e["name"]: e for e in generated["experiments"]}
    exp_exps = {e["name"]: e for e in expected["experiments"]}
    for name in sorted(set(exp_exps) - set(gen_exps)):
        known.append(f"experiments/{name}: absent from report.json (no-goal or quantile-only experiment)")
    for name in sorted(set(gen_exps) - set(exp_exps)):
        unexpected.append(f"experiments/{name}: generated but not in reference")
    for name in sorted(set(gen_exps) & set(exp_exps)):
        g, e = gen_exps[name], exp_exps[name]
        if g["erratic"] != e["erratic"]:
            unexpected.append(f"experiments/{name}/erratic: {g['erratic']} != {e['erratic']}")
        if g["optimization_goal"] != e["optimization_goal"]:
            unexpected.append(
                f"experiments/{name}/optimization_goal: "
                f"{json.dumps(g['optimization_goal'])} != {json.dumps(e['optimization_goal'])}"
            )
        g_checks = {c["name"]: c for c in g["bounds_checks"]}
        e_checks = {c["name"]: c for c in e["bounds_checks"]}
        for cname in sorted(set(g_checks) - set(e_checks)):
            unexpected.append(f"experiments/{name}/bounds_checks/{cname}: generated but not in reference")
        for cname in sorted(set(e_checks) - set(g_checks)):
            unexpected.append(f"experiments/{name}/bounds_checks/{cname}: in reference but not generated")
        for cname in sorted(set(g_checks) & set(e_checks)):
            gk, ek = g_checks[cname], e_checks[cname]
            if gk["series"] != ek["series"]:
                known.append(f"experiments/{name}/bounds_checks/{cname}/series: {gk['series']!r} != {ek['series']!r}")
            if gk["unit"] != ek["unit"]:
                known.append(f"experiments/{name}/bounds_checks/{cname}/unit: {gk['unit']!r} != {ek['unit']!r}")
            for field in ("lower_bound", "upper_bound", "pass_count", "total_count", "min_observed", "max_observed"):
                if gk[field] != ek[field]:
                    unexpected.append(
                        f"experiments/{name}/bounds_checks/{cname}/{field}: {gk[field]!r} != {ek[field]!r}"
                    )
        if g["quantile_checks"] != e["quantile_checks"]:
            known.append(f"experiments/{name}/quantile_checks: not present in report.json")

    if generated["analysis_errors"] != expected["analysis_errors"]:
        known.append("analysis_errors: not present in report.json")
    if generated["failed_replicates"] != expected["failed_replicates"]:
        unexpected.append("failed_replicates differ")

    return known, unexpected


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def resolve_inputs(paths: list[str]) -> list[Path]:
    out = []
    for p in paths:
        path = Path(p)
        if path.is_dir():
            candidate = path / "report.json"
            if candidate.is_file():
                out.append(candidate)
            else:
                print(f"error: {path} has no report.json", file=sys.stderr)
                sys.exit(2)
        else:
            out.append(path)
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument(
        "inputs",
        nargs="+",
        help="report.json paths, or directories containing a report.json",
    )
    parser.add_argument(
        "-o",
        "--output",
        help="output path (default: report.v1.json next to the input)",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="overwrite an existing output file",
    )
    parser.add_argument(
        "--compare",
        action="store_true",
        help="diff the generated report against the existing report.v1.json "
        "instead of writing; exit 1 on unexpected differences",
    )
    args = parser.parse_args()

    inputs = resolve_inputs(args.inputs)
    if args.output and (len(inputs) > 1 or args.compare):
        print("error: --output only supports a single input in write mode", file=sys.stderr)
        return 2

    rc = 0
    for report_path in inputs:
        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
            generated = convert(report)
        except (OSError, ValueError, KeyError) as e:
            print(f"error: {report_path}: {e}", file=sys.stderr)
            rc = 1
            continue

        payload = json.dumps(generated, separators=(",", ":")) + "\n"

        if args.compare:
            v1_path = report_path.parent / "report.v1.json"
            try:
                with open(v1_path, encoding="utf-8") as f:
                    expected = json.load(f)
            except OSError as e:
                print(f"error: {v1_path}: {e}", file=sys.stderr)
                rc = 1
                continue
            known, unexpected = diff_v1(generated, expected)
            status = "OK" if not known and not unexpected else "DIFF"
            print(f"{status} {report_path.parent}")
            for d in known:
                print(f"  known: {d}")
            for d in unexpected:
                print(f"  UNEXPECTED: {d}")
            if unexpected:
                rc = 1
            continue

        out_path = Path(args.output) if args.output else report_path.parent / "report.v1.json"
        if out_path.exists() and not args.force:
            print(f"error: {out_path} exists (use --force)", file=sys.stderr)
            rc = 1
            continue
        with open(out_path, "w", encoding="utf-8") as f:
            f.write(payload)
        print(f"wrote {out_path}")

    return rc


if __name__ == "__main__":
    sys.exit(main())
