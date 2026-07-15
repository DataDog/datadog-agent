"""
Invoke tasks for anomaly detection dev tooling (not part of agent build).
"""

import glob
import json
import os
import random
import re
import shlex
import shutil
import time
from dataclasses import dataclass

from invoke import Exit, task

from tasks.libs.anomalydetection.eval import (
    CORRELATORS,
    DETECTORS,
    EXTRACTORS,
    SCENARIOS,
    StepLogger,
    _anchor_combos,
    _build_optuna_config,
    _combo_to_config,
    _ensure_parquets,
    _fmt_wall_dur,
    _full_stack_combo,
    _prepare_eval_output_dir,
    _scenario_f1_from_bayesian_report,
    aggregate_eval_component_results,
    print_eval_bayesian_summary,
    print_eval_component_summary,
    print_eval_scenarios_summary,
    print_eval_tp_summary,
    random_component_combinations,
)
from tasks.libs.common.color import Color, color_message


@dataclass(frozen=True)
class _DDEvalOptions:
    config_template: str
    ddsource_dir: str
    command: str
    service: str
    project: str
    dataset: str
    env: str
    test_drive: str
    jobs: int
    max_attempts: int
    limit: int
    where_in: str
    testbench_binary_s3_uri: str
    scorer_binary_s3_uri: str


# --- Build ---


@task
def build_scorer(ctx):
    """
    Builds the anomalydetection-scorer binary to bin/anomalydetection-scorer.
    """
    ctx.run("go build -C internal/qbranch/anomalydetection-scorer -o ../../../bin/anomalydetection-scorer .")


@task
def build_testbench(ctx):
    """
    Builds the anomalydetection-testbench binary to bin/anomalydetection-testbench.
    """
    ctx.run(
        "go build -C internal/qbranch/anomalydetection-testbench -tags python -o ../../../bin/anomalydetection-testbench ."
    )


# --- Run ---


@task
def launch_testbench(
    ctx,
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    build: bool = False,
    headless_scenario: str = "",
    headless_output: str = "",
    profile: bool = False,
    open_pprof: bool = False,
    verbose: bool = False,
    profile_path: str = "",
    config: str = "",
    enable: str = "",
    disable: str = "",
    timeout: int = 0,
    logs_only: bool = False,
):
    """
    Launches the anomalydetection-testbench backend (and UI in interactive mode).

    Args:
        scenarios_dir: Directory containing the scenarios to load.
        build: Whether to build the binary before launching.
        profile: Whether to capture a heap profile (headless mode only).
        open_pprof: Open pprof UI after headless run (requires --profile).
        verbose: Pass --verbose to the testbench.
        profile_path: Override the default heap-profile output path.
        config: JSON params file; overrides --enable/--disable when set.
        enable: Comma-separated components to enable (passed as --enable).
        disable: Comma-separated components to disable (passed as --disable).
        timeout: Kill the headless process after this many seconds (0 = no limit).
        logs_only: Pass --logs-only (skip parquet metrics and trace stats).
    """
    if build:
        print("Building anomalydetection-testbench...")
        build_testbench(ctx)

    flags = ""
    if verbose:
        flags += " --verbose"
    if logs_only:
        flags += " --logs-only"
    if config:
        flags += f" --config {shlex.quote(config)}"
    else:
        if enable:
            flags += f" --enable {shlex.quote(enable)}"
        if disable:
            flags += f" --disable {shlex.quote(disable)}"

    if headless_scenario:
        if not headless_output:
            headless_output = f"/tmp/anomalydetection-testbench-headless-{headless_scenario}.json"
        if profile:
            if not profile_path:
                profile_path = f"/tmp/anomalydetection-testbench-headless-{headless_scenario}.prof"
            flags += f" --memprofile {profile_path}"
        print(
            f"Launching anomalydetection-testbench in headless mode for scenario {headless_scenario}, output to {headless_output}"
        )
        try:
            ctx.run(
                f"bin/anomalydetection-testbench --headless {headless_scenario} --scenarios-dir {scenarios_dir} --output {headless_output}{flags}",
                timeout=None if timeout == 0 else timeout,
            )
        except Exception as e:
            if type(e).__name__ == "CommandTimedOut":
                print(color_message(f"testbench timed out after {timeout}s", Color.ORANGE))
            else:
                raise
        if profile:
            if open_pprof:
                print("Running pprof...")
                ctx.run(f"go tool pprof -http=:8081 {profile_path}")
            else:
                print(f"To profile, run: go tool pprof -http=:8081 {profile_path}")
    else:
        if not config and not enable and not disable:
            flags += " --only scanmw,scanwelch,bocpd"
        print("Launching anomalydetection-testbench backend and UI, use ^C to exit")
        print(
            "To profile, run: go tool pprof -http=:8081 http://localhost:8080/debug/pprof/heap (8080 is the testbench API port)"
        )
        ctx.run(
            f"bin/anomalydetection-testbench --scenarios-dir {scenarios_dir}{flags} & ( cd internal/qbranch/anomalydetection-testbench/ui && npm install && npm run dev ) &"
        )


# --- Eval ---


@task
def eval_scenarios(
    ctx,
    scenario: str = "",
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    sigma: float = 30.0,
    only: str = "",
    build: bool = True,
    main_report_path: str = "/tmp/observer-eval-main-report.json",
    config: str = "",
    scenario_output_dir: str = "/tmp",
    timeout: int = 0,
    scenarios: str = "",
    _logger: StepLogger | None = None,
) -> dict:
    """
    Runs the observer F1 eval: replays scenarios, scores Gaussian F1.

    The main score is a metric between 0 and 1, 1 being the best. This is the
    source of truth for anomaly detection accuracy.

    Uses testbench --only to control which components are active.
    Default (no --only): uses testbench defaults (bocpd,rrcf,time_cluster + other default-enabled components).
    With --only: enables ONLY listed components + extractors, disables everything else.
      time_cluster is auto-added if not specified.
    With --config: JSON params file for testbench; overrides --only when both are set.

    Examples:
        dda inv anomalydetection.eval-scenarios                            # defaults
        dda inv anomalydetection.eval-scenarios --only scanmw              # scanmw + time_cluster (auto)
        dda inv anomalydetection.eval-scenarios --only bocpd,time_cluster  # explicit
        dda inv anomalydetection.eval-scenarios --config /tmp/params.json  # custom params

    Args:
        scenario: Run a single scenario (e.g. "213_pagerduty"). Default: all scenarios.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for scoring.
        only: Comma-separated components to enable (passed as --only to testbench). Auto-adds time_cluster.
        build: Whether to build the anomalydetection-testbench and anomalydetection-scorer binaries.
        main_report_path: Path for the aggregated JSON report.
        config: Path to anomalydetection-testbench JSON params file (--config). Empty: omit flag.
        scenario_output_dir: Directory where per-scenario testbench JSON outputs are written.
        timeout: Per-scenario time budget in seconds (rolling: unused time rolls over). 0 = no limit.
        scenarios: Comma-separated scenario names to run (default: all SCENARIOS).

    Returns:
        Main report dict with ``score`` and per-scenario ``metadata``.
    """
    only_flag = ""
    if only:
        components = {name.strip() for name in only.split(",") if name.strip()}
        components.add("time_cluster")
        only_flag = ",".join(sorted(components))
        print(color_message(f"Only: {only_flag}", Color.BLUE))

    config_obj = None
    if config:
        print(color_message(f"Config: {config}", Color.BLUE))
        with open(config) as f:
            config_obj = json.load(f)

    if build:
        build_testbench(ctx)
        build_scorer(ctx)

    scenarios_list = [s.strip() for s in scenarios.split(",") if s.strip()] if scenarios else SCENARIOS
    scenarios_to_run = [scenario] if scenario else scenarios_list
    scenario_logger = _logger or StepLogger(len(scenarios_to_run), "Scenario")

    results = []
    # Rolling budget: total = timeout * #scenarios. Each scenario gets whatever
    # time remains; unused time rolls over to subsequent scenarios.
    budget_remaining = timeout * len(scenarios_to_run) if timeout else 0
    for name in scenarios_to_run:
        parquet_dir = os.path.join(scenarios_dir, name, "parquet")
        scenario_root = os.path.join(scenarios_dir, name)
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            _ensure_parquets(ctx, name, parquet_dir)
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            # Fallback: check for *.parquet files directly in scenario root
            if not glob.glob(os.path.join(scenario_root, "*.parquet")):
                scenario_logger.detail(f"skipping {name} — no parquet data at {parquet_dir}", Color.ORANGE)
                continue

        output_path = os.path.join(scenario_output_dir, f"observer-eval-{name}.json")
        scenario_logger.step(name)

        only_part = f" --only {shlex.quote(only_flag)}" if only_flag else ""
        config_part = f" --config {shlex.quote(config)}" if config else ""
        scenario_start = time.monotonic()
        try:
            ctx.run(
                f"bin/anomalydetection-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)}{only_part}{config_part}",
                timeout=None if timeout == 0 else max(1, int(budget_remaining)),
            )
        except Exception as e:
            if type(e).__name__ == "CommandTimedOut":
                scenario_logger.detail(
                    f"testbench timed out (budget {budget_remaining:.0f}s remaining) — scoring as zero",
                    Color.ORANGE,
                )
                results.append(
                    {
                        "name": name,
                        "f1": 0.0,
                        "precision": 0.0,
                        "recall": 0.0,
                        "alpha": -1,
                        "num_predictions": 0,
                        "num_baseline_fps": 0,
                        "num_filtered_warmup": 0,
                        "num_filtered_cascading": 0,
                        "timed_out": True,
                    }
                )
                continue
            raise
        finally:
            if timeout:
                budget_remaining -= time.monotonic() - scenario_start

        if not os.path.isfile(output_path):
            scenario_logger.detail(f"testbench did not produce output at {output_path}", Color.RED)
            continue
        try:
            with open(output_path) as f:
                json.load(f)
        except (json.JSONDecodeError, OSError) as e:
            scenario_logger.detail(f"testbench output not valid JSON: {e}", Color.RED)
            continue

        scorer_result = ctx.run(
            f"bin/anomalydetection-scorer --input {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)} --sigma {sigma} --json",
            hide=True,
            warn=True,
        )

        if scorer_result.failed:
            scenario_logger.detail(f"scorer failed:\n{scorer_result.stderr}", Color.RED)
            continue

        try:
            score = json.loads(scorer_result.stdout.strip())
        except json.JSONDecodeError:
            scenario_logger.detail(f"scorer returned invalid JSON:\n{scorer_result.stdout}", Color.RED)
            continue

        scenario_logger.detail(
            f"F1={score.get('f1', 0):.4f}  prec={score.get('precision', 0):.4f}  rec={score.get('recall', 0):.4f}"
        )
        results.append({"name": name, **score})

    print_eval_scenarios_summary(results, sigma)

    f1_scores: list[float] = [float(r["f1"]) for r in results]
    main_score = sum(f1_scores) / len(f1_scores) if f1_scores else 0.0
    main_report = {"score": main_score, "metadata": {r["name"]: r for r in results}, "component_configs": config_obj}
    with open(main_report_path, "w") as f:
        json.dump(main_report, f, indent=4)
    print(f"Saved main report to {main_report_path}")
    print(color_message(f"Main score: {main_score * 100:.1f}%", Color.GREEN))

    return main_report


@task
def eval_tp(
    ctx,
    scenario: str = "",
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    sigma: float = 30.0,
    only: str = "",
    build: bool = True,
):
    """
    Runs TP metric scoring: replays scenarios with passthrough correlator and scores
    each detected anomaly against ground truth metric labels in ground_truth.json.

    passthrough correlator is auto-added if not specified (required for TP scoring).

    Examples:
        dda inv anomalydetection.eval-tp --only scanmw              # scanmw + passthrough (auto)
        dda inv anomalydetection.eval-tp --only bocpd,passthrough    # explicit

    Args:
        scenario: Run a single scenario (e.g. "213_pagerduty"). Default: all scenarios.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for scoring.
        only: Comma-separated components to enable (passed as --only to testbench). Auto-adds passthrough.
    """
    if not only:
        print(color_message("--only is required (e.g. --only scanmw)", Color.RED))
        return

    components = {name.strip() for name in only.split(",") if name.strip()}
    components.add("passthrough")
    only_flag = ",".join(sorted(components))

    print(color_message(f"Only: {only_flag}", Color.BLUE))

    if build:
        build_testbench(ctx)
        build_scorer(ctx)

    scenarios_to_run = [scenario] if scenario else SCENARIOS

    results = []
    for name in scenarios_to_run:
        parquet_dir = os.path.join(scenarios_dir, name, "parquet")
        scenario_root = os.path.join(scenarios_dir, name)
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            _ensure_parquets(ctx, name, parquet_dir)
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            if not glob.glob(os.path.join(scenario_root, "*.parquet")):
                print(color_message(f"Skipping {name} — no parquet data at {parquet_dir}", Color.ORANGE))
                continue

        output_path = f"/tmp/observer-eval-{name}-tp.json"
        print(color_message(f"\n{'=' * 60}", Color.BLUE))
        print(color_message(f"  {name}", Color.BLUE))
        print(color_message(f"{'=' * 60}", Color.BLUE))

        ctx.run(
            f"bin/anomalydetection-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)}"
            f" --scenarios-dir {shlex.quote(scenarios_dir)}"
            f" --only {shlex.quote(only_flag)}"
            f" --verbose"
        )

        if not os.path.isfile(output_path):
            print(color_message(f"Testbench did not produce output at {output_path}", Color.RED))
            continue

        scorer_result = ctx.run(
            f"bin/anomalydetection-scorer --input {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)} --sigma {sigma} --score-tp --json",
            hide=True,
            warn=True,
        )

        if scorer_result.failed:
            print(color_message(f"Scorer failed for {name}:\n{scorer_result.stderr}", Color.RED))
            continue

        try:
            score = json.loads(scorer_result.stdout.strip())
        except json.JSONDecodeError:
            print(color_message(f"Scorer returned invalid JSON for {name}:\n{scorer_result.stdout}", Color.RED))
            continue

        results.append({"name": name, **score})

    print_eval_tp_summary(results)


# --- Scenario management ---


@task
def download_scenarios(
    ctx,
    scenario: str = "",
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    skip_existing: bool = False,
):
    """
    Download scenario parquet data from S3.

    Resolves the latest recording for each scenario from runs.jsonl in the S3 bucket.

    Args:
        scenario: Download a single scenario (e.g. "food_delivery_redis"). Default: all.
        scenarios_dir: Directory containing scenario subdirectories.
        skip_existing: If True, skip scenarios whose parquet directory already contains files.

    Examples:
        dda inv anomalydetection.download-scenarios
        dda inv anomalydetection.download-scenarios --scenario=food_delivery_redis
    """
    scenarios_to_download = [scenario] if scenario else SCENARIOS
    for name in scenarios_to_download:
        parquet_dir = os.path.join(scenarios_dir, name, "parquet")
        if skip_existing and os.path.isdir(parquet_dir) and os.listdir(parquet_dir):
            print(color_message(f"Skipping download for '{name}' — parquet data already present", Color.BLUE))
            continue
        # Download to a temp dir first, then swap — preserves existing data if download fails.
        tmp_parquet_dir = parquet_dir + ".new"
        if os.path.isdir(tmp_parquet_dir):
            shutil.rmtree(tmp_parquet_dir)
        _ensure_parquets(ctx, name, tmp_parquet_dir)
        if os.path.isdir(tmp_parquet_dir) and os.listdir(tmp_parquet_dir):
            if os.path.isdir(parquet_dir):
                shutil.rmtree(parquet_dir)
            os.rename(tmp_parquet_dir, parquet_dir)
        else:
            shutil.rmtree(tmp_parquet_dir, ignore_errors=True)
            print(color_message(f"Download failed for '{name}', keeping existing data", Color.ORANGE))


# --- Combination search ---


@task
def eval_combinations(
    ctx,
    n: int = 10,
    output_dir: str = "/tmp/observer-eval-combinations",
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    sigma: float = 30.0,
    seed: int = None,
    build: bool = True,
    overwrite: bool = False,
    force_enable: str = "",
    force_disable: str = "",
):
    """
    Run Gaussian F1 eval on n component combinations and rank them.

    The first combination (combo_000) always enables every detector and
    correlator not listed in --force-disable (full stack). Remaining
    combinations are random: each has at least 1 detector and 1 correlator.
    All EXTRACTORS are enabled in each combo unless named in --force-disable.
    A JSON config file is written per combination so enabled/disabled state is
    precise (no auto-add side effects from --only).

    Output layout:
        <output_dir>/combo_NNN/config.json   - exact component config used
        <output_dir>/combo_NNN/report.json   - per-scenario F1 scores
        <output_dir>/report.json             - all combos ranked by score, plus best_combination

    Args:
        n: Total combinations to evaluate: one full-stack plus (n - 1) random
            (default: 10). Use n=1 for only the full-stack baseline.
        output_dir: Root directory for per-combo results and summary. If this
            path already exists and contains report.json, the task aborts unless
            overwrite is True; otherwise the directory is removed first.
        overwrite: Allow replacing an existing output_dir that contains report.json.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for F1 scoring.
        seed: Random seed for reproducibility (default: None = random).
        build: Whether to build anomalydetection-testbench and anomalydetection-scorer first.
        force_enable: Comma-separated components always present in every combination.
        force_disable: Comma-separated components never included in any combination.

    Examples:
        dda inv anomalydetection.eval-combinations --n 20 --seed 42
        dda inv anomalydetection.eval-combinations --n 5 --output-dir /tmp/ablation
        dda inv anomalydetection.eval-combinations --n 10 --force-enable bocpd --force-disable scanmw,scanwelch
    """
    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    if build:
        build_testbench(ctx)
        build_scorer(ctx)

    if seed is not None:
        seed = int(seed)
    else:
        seed = random.randint(0, 2**32 - 1)

    force_enable_list = [c.strip() for c in force_enable.split(",") if c.strip()]
    force_disable_list = [c.strip() for c in force_disable.split(",") if c.strip()]

    if force_enable_list:
        print(color_message(f"Force-enabled:  {', '.join(force_enable_list)}", Color.BLUE))
    if force_disable_list:
        print(color_message(f"Force-disabled: {', '.join(force_disable_list)}", Color.BLUE))

    full_combo = _full_stack_combo(force_disable_list)
    full_key = (tuple(full_combo["detectors"]), tuple(full_combo["correlators"]))
    random_count = max(0, n - 1)
    random_combos = random_component_combinations(
        random_count,
        seed=seed,
        force_enable=force_enable_list,
        force_disable=force_disable_list,
        exclude_combo_keys={full_key},
    )
    combos = [full_combo] + random_combos
    print(
        color_message(
            f"combo_000 = full stack; plus {len(random_combos)} random (seed={seed}, total={len(combos)})",
            Color.BLUE,
        )
    )

    combo_logger = StepLogger(len(combos), "Combo")
    summary_results = []
    for i, combo in enumerate(combos):
        combo_label = f"combo_{i:03d}"
        combo_dir = os.path.join(output_dir, combo_label)
        os.makedirs(combo_dir, exist_ok=True)

        config_data = _combo_to_config(combo["detectors"], combo["correlators"], force_disable=force_disable_list)
        config_path = os.path.join(combo_dir, "config.json")
        with open(config_path, "w") as f:
            json.dump(config_data, f, indent=4)

        combo_title = f"{combo_label} (full stack)" if i == 0 else combo_label
        combo_logger.step(combo_title)
        combo_logger.detail(f"detectors:   {', '.join(combo['detectors'])}")
        combo_logger.detail(f"correlators: {', '.join(combo['correlators'])}")
        ext_on = [e for e in EXTRACTORS if e not in force_disable_list]
        combo_logger.detail(f"extractors:  {', '.join(ext_on)}")

        scenario_output_dir = os.path.join(combo_dir, "scenarios")
        os.makedirs(scenario_output_dir, exist_ok=True)

        report_path = os.path.join(combo_dir, "report.json")
        try:
            report = eval_scenarios(
                ctx,
                scenarios_dir=scenarios_dir,
                sigma=sigma,
                config=config_path,
                build=False,
                main_report_path=report_path,
                scenario_output_dir=scenario_output_dir,
                _logger=combo_logger.child(len(SCENARIOS), "Scenario"),
            )
        except Exception as e:
            combo_logger.detail(f"eval_scenarios failed: {e}", Color.RED)
            report = None

        if report is not None:
            combo_logger.detail(f"score: {report.get('score', 0.0):.4f}")
            summary_results.append(
                {
                    "rank": 0,
                    "combo": combo_label,
                    "score": report.get("score", 0.0),
                    "detectors": combo["detectors"],
                    "correlators": combo["correlators"],
                    "report_path": report_path,
                    "config_path": config_path,
                }
            )

    summary_results.sort(key=lambda x: x["score"], reverse=True)
    for rank, r in enumerate(summary_results, 1):
        r["rank"] = rank

    if summary_results:
        print(color_message(f"\n{'=' * 70}", Color.GREEN))
        print(color_message("  Combinations Eval Summary", Color.GREEN))
        print(color_message(f"{'=' * 70}\n", Color.GREEN))
        header = f"{'Rank':<5}  {'Score':>6}  {'Detectors':<35}  Correlators"
        print(header)
        print("-" * 80)
        for r in summary_results:
            print(f"{r['rank']:<5}  {r['score']:>6.4f}  {', '.join(r['detectors']):<35}  {', '.join(r['correlators'])}")

        best = summary_results[0]
        print(color_message(f"\n{'=' * 70}", Color.GREEN))
        print(color_message(f"  Best combination: {best['combo']}  (score={best['score']:.4f})", Color.GREEN))
        print(color_message(f"    detectors:   {', '.join(best['detectors'])}", Color.GREEN))
        print(color_message(f"    correlators: {', '.join(best['correlators'])}", Color.GREEN))
        print(color_message(f"    config:      {best['config_path']}", Color.GREEN))
        print(color_message(f"    report:      {best['report_path']}", Color.GREEN))
        print(color_message(f"{'=' * 70}", Color.GREEN))

    scores = [r["score"] for r in summary_results]
    max_score = max(scores) if scores else 0.0
    avg_score = sum(scores) / len(scores) if scores else 0.0
    best_combination = summary_results[0] if summary_results else None

    report_path = os.path.join(output_dir, "report.json")
    with open(report_path, "w") as f:
        json.dump(
            {
                "score": max_score,
                "avg_eval_score": avg_score,
                "seed": seed,
                "force_enable": force_enable_list,
                "force_disable": force_disable_list,
                "combos": summary_results,
                "best_combination": best_combination,
            },
            f,
            indent=4,
        )
    print(color_message(f"\nReport: {report_path}", Color.GREEN))
    print(color_message(f"  score (max):      {max_score:.4f}", Color.GREEN))
    print(color_message(f"  avg_eval_score:   {avg_score:.4f}", Color.GREEN))
    print(color_message(f"Per-combo reports: {output_dir}/combo_*/report.json", Color.GREEN))

    return summary_results


# --- Bayesian Optimization ---


def _run_bayesian_runs(
    ctx,
    components_list: list,
    m_runs: int,
    n_trials: int,
    seeds: list,
    output_dir: str,
    scenarios_dir: str,
    sigma: float,
    timeout: int,
    scenarios: str,
    lock: str = "",
    eval_backend: str = "local",
    ddeval_options: _DDEvalOptions | None = None,
    run_logger: StepLogger | None = None,
    step_label_prefix: str = "",
) -> dict:
    """Run M independent Bayesian optimisations on a fixed component set.

    Each run gets its own subdirectory (output_dir/run_NNN) and seed.
    Returns a dict with keys: run_scores, run_details, max_score, mean_score, failed_runs.
    """
    run_scores: list[float] = []
    run_details: list[dict] = []

    for ri in range(m_runs):
        run_label = f"run_{ri:03d}"
        run_dir = os.path.join(output_dir, run_label)
        run_seed = seeds[ri]

        if run_logger:
            step_title = (
                f"{step_label_prefix} / {run_label}  (seed={run_seed})"
                if step_label_prefix
                else f"{run_label}  (seed={run_seed})"
            )
            run_logger.step(step_title)
            run_logger.detail(f"components: {', '.join(components_list)}")

        trial_logger = run_logger.child(n_trials, "Trial") if run_logger else None
        report = eval_bayesian(
            ctx,
            components=",".join(components_list),
            lock=lock,
            n_trials=n_trials,
            output_dir=run_dir,
            scenarios_dir=scenarios_dir,
            sigma=sigma,
            seed=run_seed,
            build=False,
            overwrite=True,
            timeout=timeout,
            scenarios=scenarios,
            eval_backend=eval_backend,
            **_ddeval_options_kwargs(ddeval_options),
            _logger=trial_logger,
        )

        run_failed = report is None or report.get("completed_trials", 0) == 0
        if run_failed and run_logger:
            if report is None:
                reason = "eval_bayesian returned None (aborted before producing a report)"
            else:
                n_ft = report.get("failed_trials", 0)
                reason = f"all {n_ft} trials failed — no completed trials in report"
            run_logger.detail(f"Warning: {run_label} failed: {reason}", Color.RED)

        best_score = report.get("score") if report and not run_failed else None
        avg_score_val = report.get("avg_eval_score") if report and not run_failed else None
        if not run_failed:
            run_scores.append(best_score)
        run_details.append(
            {
                "run": run_label,
                "seed": run_seed,
                "failed": run_failed,
                "best_score": best_score,
                "avg_score": avg_score_val,
                "report_path": os.path.join(run_dir, "report.json"),
                "scenario_f1": _scenario_f1_from_bayesian_report(report) if not run_failed else {},
            }
        )

    return {
        "run_scores": run_scores,
        "run_details": run_details,
        "max_score": max(run_scores) if run_scores else None,
        "mean_score": sum(run_scores) / len(run_scores) if run_scores else None,
        "failed_runs": sum(1 for r in run_details if r["failed"]),
    }


@task
def eval_bayesian(
    ctx,
    components: str = "",
    lock: str = "",
    only: str = "",
    n_trials: int = 10,
    output_dir: str = "/tmp/observer-optuna-eval",
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    sigma: float = 30.0,
    seed: int = None,
    build: bool = True,
    overwrite: bool = False,
    timeout: int = 0,
    scenarios: str = "",
    eval_backend: str = "local",
    ddeval_config_template: str = "",
    ddeval_ddsource_dir: str = "",
    ddeval_command: str = "",
    ddeval_service: str = "eval_worker_observer_log_ad",
    ddeval_project: str = "observer-log-ad",
    ddeval_dataset: str = "observer-log-ad-gensim-store-working",
    ddeval_env: str = "staging",
    ddeval_test_drive: str = "observer-log-ad-ddeval-worker",
    ddeval_jobs: int = 6,
    ddeval_max_attempts: int = 1,
    ddeval_limit: int = 0,
    ddeval_where_in: str = "",
    ddeval_testbench_binary_s3_uri: str = "",
    ddeval_scorer_binary_s3_uri: str = "",
    _logger: StepLogger | None = None,
):
    """
    Run Bayesian hyperparameter optimization (Optuna TPE) for a fixed set of observer components.

    Each trial enables the specified components, sampling hyperparameters and scoring via
    eval_scenarios (mean F1). Locked components (--lock) are enabled but use Go defaults (not tuned).

    Requires optuna: dda inv --dep optuna anomalydetection.eval-bayesian ...

    Output layout:
        <output_dir>/trial_NNN/config.json     - sampled component config for this trial
        <output_dir>/trial_NNN/report.json     - eval_scenarios output for this trial
        <output_dir>/trial_NNN/scenarios/      - per-scenario testbench outputs
        <output_dir>/study.pkl                 - serialized Optuna study
        <output_dir>/report.json               - summary: best score, best params, avg_eval_score
        <output_dir>/best_config.json          - best config used

    Args:
        components: Comma-separated component names to enable (default: all).
        lock: Comma-separated components to enable but not tune (keep at Go defaults).
        only: Tune only the listed components; lock everything else. Mutually exclusive with --lock.
        n_trials: Number of Optuna trials (default: 10).
        output_dir: Root output directory.
        overwrite: Allow replacing an existing output_dir that contains report.json.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for F1 scoring.
        seed: Random seed for TPE sampler reproducibility.
        build: Whether to build testbench and scorer first.
        timeout: Per-scenario time budget in seconds (rolling). 0 = no limit.
        scenarios: Comma-separated scenario names to run (default: all SCENARIOS).
        eval_backend: Evaluation backend. "local" runs eval_scenarios; "ddeval" submits
            each trial to the remote ddeval workflow and optimizes the returned mean F1.
        ddeval_config_template: Optional JSON experiment config template for ddeval. When omitted, the task
            builds a minimal config from the generated trial config and binary artifact URIs.
        ddeval_ddsource_dir: dd-source checkout containing the ddeval Bazel target.
            Defaults to $DDSOURCE_DIR or $DD_SOURCE_DIR when --ddeval-command is not set.
        ddeval_command: Installed ddeval command or wrapper. When set, this is used
            instead of running the ddeval Bazel target from dd-source.
        ddeval_service: ddeval executor service name.
        ddeval_project: LLMObs/ddEval project name.
        ddeval_dataset: ddEval dataset name.
        ddeval_env: ddEval environment.
        ddeval_test_drive: Rapid Test Drive name for the Atlas worker.
        ddeval_jobs: Scenario concurrency passed to ddeval (-j).
        ddeval_max_attempts: Max attempts per scenario.
        ddeval_limit: Optional dataset limit for smoke tests.
        ddeval_where_in: Optional ddeval --where-in filter, e.g. metadata.record_id=a,b.
        ddeval_testbench_binary_s3_uri: S3 URI for the anomalydetection-testbench binary.
            Defaults to $OBSERVER_LOG_AD_DDEVAL_TESTBENCH_BINARY_S3_URI.
        ddeval_scorer_binary_s3_uri: S3 URI for the anomalydetection-scorer binary.
            Defaults to $OBSERVER_LOG_AD_DDEVAL_SCORER_BINARY_S3_URI.

    Examples:
        dda inv --dep optuna anomalydetection.eval-bayesian
        dda inv --dep optuna anomalydetection.eval-bayesian --components bocpd,rrcf,time_cluster
        dda inv --dep optuna anomalydetection.eval-bayesian --only bocpd
        dda inv --dep optuna anomalydetection.eval-bayesian --n-trials 100 --seed 42
        dda inv --dep optuna anomalydetection.eval-bayesian --eval-backend ddeval \
            --ddeval-command ddeval \
            --ddeval-testbench-binary-s3-uri s3://.../anomalydetection-testbench \
            --ddeval-scorer-binary-s3-uri s3://.../anomalydetection-scorer \
            --n-trials 3
    """
    import pickle

    try:
        import optuna
    except Exception:
        import sys

        print(color_message('Please use dda inv --dep optuna ... to run this task', Color.RED), file=sys.stderr)
        raise Exit from None

    only_list = [c.strip() for c in only.split(",") if c.strip()]
    if only_list and lock:
        print(color_message("Error: --only and --lock are mutually exclusive", Color.RED))
        return

    components_list = [c.strip() for c in components.split(",") if c.strip()]

    if only_list:
        all_components = DETECTORS + CORRELATORS + EXTRACTORS
        unknown_only = set(only_list) - set(all_components)
        if unknown_only:
            print(color_message(f"Error: unknown components in --only: {', '.join(sorted(unknown_only))}", Color.RED))
            return
        if not components_list:
            components_list = all_components
        else:
            unknown_only_in_subset = set(only_list) - set(components_list)
            if unknown_only_in_subset:
                print(
                    color_message(
                        f"Error: --only targets not in --components: {', '.join(sorted(unknown_only_in_subset))}",
                        Color.RED,
                    )
                )
                return
        locked_set = {c for c in components_list if c not in set(only_list)}
    else:
        if not components_list:
            components_list = DETECTORS + CORRELATORS + EXTRACTORS
        locked_set = {c.strip() for c in lock.split(",") if c.strip()}

    if not components_list:
        print(color_message("Error: at least one component is required (--components)", Color.RED))
        return

    unknown = (set(components_list) | locked_set) - set(DETECTORS + CORRELATORS + EXTRACTORS)
    if unknown:
        print(color_message(f"Error: unknown components: {', '.join(sorted(unknown))}", Color.RED))
        return

    not_active = locked_set - set(components_list)
    if not_active:
        print(color_message(f"Error: locked components not in active set: {', '.join(sorted(not_active))}", Color.RED))
        return

    if seed is not None:
        seed = int(seed)
    else:
        seed = random.randint(0, 2**32 - 1)

    try:
        eval_backend, ddeval_options = _resolve_ddeval_options(
            eval_backend=eval_backend,
            ddeval_config_template=ddeval_config_template,
            ddeval_ddsource_dir=ddeval_ddsource_dir,
            ddeval_command=ddeval_command,
            ddeval_service=ddeval_service,
            ddeval_project=ddeval_project,
            ddeval_dataset=ddeval_dataset,
            ddeval_env=ddeval_env,
            ddeval_test_drive=ddeval_test_drive,
            ddeval_jobs=ddeval_jobs,
            ddeval_max_attempts=ddeval_max_attempts,
            ddeval_limit=ddeval_limit,
            ddeval_where_in=ddeval_where_in,
            ddeval_testbench_binary_s3_uri=ddeval_testbench_binary_s3_uri,
            ddeval_scorer_binary_s3_uri=ddeval_scorer_binary_s3_uri,
        )
    except ValueError as e:
        print(color_message(f"Error: {e}", Color.RED))
        return

    if not _validate_ddeval_scenario_filter(eval_backend, scenarios):
        return

    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    if build and eval_backend == "local":
        build_testbench(ctx)
        build_scorer(ctx)

    tuned = [c for c in components_list if c not in locked_set]
    trial_logger = _logger or StepLogger(n_trials, "Trial")

    if not _logger:
        print(color_message(f"\n{'=' * 60}", Color.BLUE))
        print(color_message("  Observer Bayesian Optimization", Color.BLUE))
        print(color_message(f"{'=' * 60}", Color.BLUE))
        print(color_message(f"  components:  {', '.join(components_list)}", Color.BLUE))
        if locked_set:
            print(color_message(f"  locked:      {', '.join(sorted(locked_set))} (not tuned)", Color.BLUE))
        print(color_message(f"  tuned:       {', '.join(tuned)}", Color.BLUE))
        print(color_message(f"  n_trials:    {n_trials}", Color.BLUE))
        print(color_message(f"  output_dir:  {output_dir}", Color.BLUE))
        print(color_message(f"  seed:        {seed}", Color.BLUE))
        print(color_message(f"  backend:     {eval_backend}", Color.BLUE))
        if ddeval_options:
            print(color_message(f"  ddeval data: {ddeval_options.project}/{ddeval_options.dataset}", Color.BLUE))
            print(color_message(f"  ddeval jobs: {ddeval_options.jobs}", Color.BLUE))

    completed_trials: list[dict] = []
    failed_trials: list[dict] = []

    def objective(trial):
        trial_label = f"trial_{trial.number:03d}"
        trial_dir = os.path.join(output_dir, trial_label)
        os.makedirs(trial_dir, exist_ok=True)

        config_data = _build_optuna_config(trial, components_list, locked_set)
        config_path = os.path.join(trial_dir, "config.json")
        with open(config_path, "w") as f:
            json.dump(config_data, f, indent=4)

        trial_logger.step(trial_label)
        for key, val in sorted(trial.params.items()):
            trial_logger.detail(f"{key}: {val}")
        _log_trial_config(trial_logger, config_data)

        report_path = os.path.join(trial_dir, "report.json")
        scenario_output_dir = os.path.join(trial_dir, "scenarios")
        os.makedirs(scenario_output_dir, exist_ok=True)

        failure_reason: str | None = None
        report = None
        try:
            if eval_backend == "ddeval":
                report = _run_ddeval_trial(
                    ctx,
                    trial_config_path=config_path,
                    report_path=report_path,
                    trial_dir=trial_dir,
                    options=ddeval_options,
                    sigma=sigma,
                    logger=trial_logger,
                )
            else:
                scenarios_list = [s.strip() for s in scenarios.split(",") if s.strip()] if scenarios else SCENARIOS
                report = eval_scenarios(
                    ctx,
                    scenarios_dir=scenarios_dir,
                    sigma=sigma,
                    config=config_path,
                    build=False,
                    main_report_path=report_path,
                    scenario_output_dir=scenario_output_dir,
                    timeout=timeout,
                    scenarios=scenarios,
                    _logger=trial_logger.child(len(scenarios_list), "Scenario"),
                )
        except Exception as e:
            failure_reason = f"{eval_backend} eval raised {type(e).__name__}: {e}"

        if failure_reason is None and report is None:
            failure_reason = f"{eval_backend} eval returned None (no report produced)"

        if failure_reason is not None:
            trial_logger.detail(f"Trial failed: {failure_reason}", Color.RED)
            failed_trials.append(
                {"trial": trial.number, "label": trial_label, "reason": failure_reason, "config_path": config_path}
            )
            return float("-inf")

        score = report.get("score", 0.0)
        trial_logger.score(score)
        completed_trials.append(
            {
                "trial": trial.number,
                "label": trial_label,
                "score": score,
                "params": dict(trial.params),
                "config_path": config_path,
                "report_path": report_path,
                "experiment_url": report.get("experiment_url") or report.get("experimentUrl"),
                "workflow_id": report.get("workflow_id"),
                "workflow_run_id": report.get("workflow_run_id"),
                "duration_s": report.get("duration_s"),
                "metrics": report.get("metrics"),
                "experiment_config_path": report.get("experiment_config_path"),
                "workflow_log_path": report.get("workflow_log_path"),
            }
        )
        return score

    optuna.logging.set_verbosity(optuna.logging.WARNING)
    sampler = optuna.samplers.TPESampler(seed=seed)
    study = optuna.create_study(direction="maximize", sampler=sampler)
    study.optimize(objective, n_trials=n_trials)

    study_path = os.path.join(output_dir, "study.pkl")
    with open(study_path, "wb") as f:
        pickle.dump(study, f)

    completed_trials.sort(key=lambda x: x["score"], reverse=True)
    scores = [t["score"] for t in completed_trials]
    max_score = max(scores) if scores else 0.0
    avg_score = sum(scores) / len(scores) if scores else 0.0
    best = completed_trials[0] if completed_trials else None

    n_failed = len(failed_trials)
    if n_failed > 0:
        fail_pct = 100 * n_failed / n_trials
        clr = Color.RED if not completed_trials else Color.ORANGE
        print(
            color_message(
                f"Warning: {n_failed}/{n_trials} trials failed ({fail_pct:.0f}%).  "
                "Failed trials are excluded from scoring.",
                clr,
            )
        )
        for ft in failed_trials:
            print(color_message(f"  [{ft['label']}] {ft['reason']}", clr))
        if not completed_trials:
            print(color_message("Error: all trials failed — scores are meaningless.", Color.RED))

    final_report = {
        "score": max_score,
        "avg_eval_score": avg_score,
        "n_trials": n_trials,
        "completed_trials": len(completed_trials),
        "failed_trials": n_failed,
        "seed": seed,
        "components": components_list,
        "locked": sorted(locked_set),
        "eval_backend": eval_backend,
        "best_combination": best,
        "trials": completed_trials,
        "failures": failed_trials,
    }
    report_path = os.path.join(output_dir, "report.json")
    with open(report_path, "w") as f:
        json.dump(final_report, f, indent=4)

    if best and best.get("config_path") and os.path.exists(best["config_path"]):
        with open(best["config_path"]) as f:
            best_config = json.load(f)
        best_config_path = os.path.join(output_dir, "best_config.json")
        with open(best_config_path, "w") as f:
            json.dump(best_config, f, indent=4)

    print_eval_bayesian_summary(completed_trials, best, max_score, avg_score, output_dir, study_path)

    return final_report


def _resolve_ddeval_options(
    *,
    eval_backend: str,
    ddeval_config_template: str,
    ddeval_ddsource_dir: str,
    ddeval_command: str,
    ddeval_service: str,
    ddeval_project: str,
    ddeval_dataset: str,
    ddeval_env: str,
    ddeval_test_drive: str,
    ddeval_jobs: int,
    ddeval_max_attempts: int,
    ddeval_limit: int,
    ddeval_where_in: str,
    ddeval_testbench_binary_s3_uri: str,
    ddeval_scorer_binary_s3_uri: str,
) -> tuple[str, _DDEvalOptions | None]:
    eval_backend = eval_backend.strip().lower()
    if eval_backend not in {"local", "ddeval"}:
        raise ValueError(f"unknown eval backend: {eval_backend}")
    if eval_backend == "local":
        return eval_backend, None

    config_template = (ddeval_config_template or os.environ.get("OBSERVER_LOG_AD_DDEVAL_CONFIG_TEMPLATE", "")).strip()
    testbench_binary_s3_uri = (
        ddeval_testbench_binary_s3_uri or os.environ.get("OBSERVER_LOG_AD_DDEVAL_TESTBENCH_BINARY_S3_URI", "")
    ).strip()
    scorer_binary_s3_uri = (
        ddeval_scorer_binary_s3_uri or os.environ.get("OBSERVER_LOG_AD_DDEVAL_SCORER_BINARY_S3_URI", "")
    ).strip()
    command = (ddeval_command or os.environ.get("DDEVAL_COMMAND", "")).strip()
    if command:
        ddsource_dir = os.path.abspath(ddeval_ddsource_dir) if ddeval_ddsource_dir else ""
    else:
        ddsource_dir = ddeval_ddsource_dir or os.environ.get("DDSOURCE_DIR") or os.environ.get("DD_SOURCE_DIR") or ""

    if not command and not ddsource_dir:
        raise ValueError(
            "--ddeval-command, $DDEVAL_COMMAND, --ddeval-ddsource-dir, or $DDSOURCE_DIR is required for "
            "--eval-backend=ddeval"
        )
    if ddsource_dir:
        ddsource_dir = os.path.abspath(ddsource_dir)
    if config_template:
        config_template = os.path.abspath(config_template)

    if ddsource_dir and not os.path.isdir(ddsource_dir):
        raise ValueError(f"dd-source directory not found: {ddsource_dir}")
    if config_template and not os.path.isfile(config_template):
        raise ValueError(f"ddeval config template not found: {config_template}")
    if not config_template and (not testbench_binary_s3_uri or not scorer_binary_s3_uri):
        raise ValueError(
            "--ddeval-testbench-binary-s3-uri and --ddeval-scorer-binary-s3-uri "
            "are required when --ddeval-config-template is not set"
        )

    return eval_backend, _DDEvalOptions(
        config_template=config_template,
        ddsource_dir=ddsource_dir,
        command=command,
        service=ddeval_service,
        project=ddeval_project,
        dataset=ddeval_dataset,
        env=ddeval_env,
        test_drive=ddeval_test_drive,
        jobs=ddeval_jobs,
        max_attempts=ddeval_max_attempts,
        limit=ddeval_limit,
        where_in=ddeval_where_in,
        testbench_binary_s3_uri=testbench_binary_s3_uri,
        scorer_binary_s3_uri=scorer_binary_s3_uri,
    )


def _ddeval_options_kwargs(options: _DDEvalOptions | None) -> dict[str, object]:
    if options is None:
        return {}
    return {
        "ddeval_config_template": options.config_template,
        "ddeval_ddsource_dir": options.ddsource_dir,
        "ddeval_command": options.command,
        "ddeval_service": options.service,
        "ddeval_project": options.project,
        "ddeval_dataset": options.dataset,
        "ddeval_env": options.env,
        "ddeval_test_drive": options.test_drive,
        "ddeval_jobs": options.jobs,
        "ddeval_max_attempts": options.max_attempts,
        "ddeval_limit": options.limit,
        "ddeval_where_in": options.where_in,
        "ddeval_testbench_binary_s3_uri": options.testbench_binary_s3_uri,
        "ddeval_scorer_binary_s3_uri": options.scorer_binary_s3_uri,
    }


def _validate_ddeval_scenario_filter(eval_backend: str, scenarios: str) -> bool:
    if eval_backend != "ddeval" or not scenarios:
        return True
    print(
        color_message(
            "Error: --scenarios only filters local scenario directories. "
            "Use --ddeval-where-in to filter remote ddeval dataset records.",
            Color.RED,
        )
    )
    return False


def _log_trial_config(logger: StepLogger, config: dict) -> None:
    logger.detail("component config:")
    for line in json.dumps(config, indent=2, sort_keys=True).splitlines():
        logger.detail(f"  {line}")


def _ddeval_experiment_config(
    *,
    options: _DDEvalOptions,
    trial_config: dict,
    trial_config_path: str,
    sigma: float,
) -> dict:
    if options.config_template:
        with open(options.config_template) as f:
            experiment_config = json.load(f)
    else:
        experiment_config = {}

    input_parameters = dict(experiment_config.get("input_parameters") or {})
    input_parameters["component_config"] = trial_config
    input_parameters["trial_metadata"] = {
        **dict(input_parameters.get("trial_metadata") or {}),
        "trial_config_path": trial_config_path,
        "eval_source": "anomalydetection.eval-bayesian",
    }
    experiment_config["input_parameters"] = input_parameters

    executor_config = dict(experiment_config.get("executor_config") or {})
    if options.testbench_binary_s3_uri or options.scorer_binary_s3_uri:
        binary_artifacts = dict(executor_config.get("binary_artifacts") or {})
        if options.testbench_binary_s3_uri:
            binary_artifacts["testbench"] = {"s3_uri": options.testbench_binary_s3_uri}
        if options.scorer_binary_s3_uri:
            binary_artifacts["scorer"] = {"s3_uri": options.scorer_binary_s3_uri}
        executor_config["binary_artifacts"] = binary_artifacts
    executor_config["sigma"] = sigma
    experiment_config["executor_config"] = executor_config

    return experiment_config


def _run_ddeval_trial(
    ctx,
    *,
    trial_config_path: str,
    report_path: str,
    trial_dir: str,
    options: _DDEvalOptions | None,
    sigma: float,
    logger: StepLogger,
) -> dict[str, object]:
    """Run one Optuna trial through the remote ddeval workflow and return a local report."""
    if options is None:
        raise RuntimeError("ddeval options were not resolved")

    with open(trial_config_path) as f:
        trial_config = json.load(f)
    experiment_config = _ddeval_experiment_config(
        options=options,
        trial_config=trial_config,
        trial_config_path=trial_config_path,
        sigma=sigma,
    )

    trial_experiment_config_path = os.path.abspath(os.path.join(trial_dir, "ddeval-experiment-config.json"))
    with open(trial_experiment_config_path, "w") as f:
        json.dump(experiment_config, f, indent=4)

    result_log_path = os.path.abspath(os.path.join(trial_dir, "ddeval-workflow.log"))
    cmd = _ddeval_workflow_command(
        config_path=trial_experiment_config_path,
        options=options,
    )
    logger.detail(f"ddeval config: {trial_experiment_config_path}")
    logger.detail(f"ddeval log: {result_log_path}")

    started_at = time.monotonic()
    result = ctx.run(cmd, hide=True, warn=True)
    duration_s = time.monotonic() - started_at
    stdout = result.stdout or ""
    stderr = result.stderr or ""
    with open(result_log_path, "w") as f:
        f.write(stdout)
        if stderr:
            f.write("\n--- stderr ---\n")
            f.write(stderr)

    if result.failed:
        raise RuntimeError(f"ddeval workflow command failed; see {result_log_path}")

    workflow_result = _parse_ddeval_workflow_result(stdout)
    metrics_json = workflow_result.get("metricsJson") or workflow_result.get("metrics_json") or "{}"
    try:
        metrics = json.loads(metrics_json) if isinstance(metrics_json, str) else dict(metrics_json)
    except (TypeError, json.JSONDecodeError) as e:
        raise RuntimeError(f"ddeval metricsJson was not valid JSON: {e}") from e

    score = _ddeval_score(metrics)
    report = {
        "score": score,
        "metadata": {},
        "metrics": metrics,
        "experiment_url": workflow_result.get("experimentUrl") or workflow_result.get("experiment_url"),
        "workflow_id": _parse_ddeval_workflow_id(stdout),
        "workflow_run_id": _parse_ddeval_workflow_run_id(stdout),
        "duration_s": duration_s,
        "ddeval_result": workflow_result,
        "component_configs": trial_config,
        "experiment_config_path": trial_experiment_config_path,
        "workflow_log_path": result_log_path,
        "workflow_command": cmd,
    }
    with open(report_path, "w") as f:
        json.dump(report, f, indent=4)

    if report["experiment_url"]:
        logger.detail(f"experiment: {report['experiment_url']}")
    if report["workflow_id"]:
        run_id_part = f" run_id={report['workflow_run_id']}" if report["workflow_run_id"] else ""
        logger.detail(f"workflow: {report['workflow_id']}{run_id_part}")
    logger.detail(
        "F1={:.4f}  prec={:.4f}  rec={:.4f}  duration={}".format(
            score,
            float(metrics.get("precision", metrics.get("summary:mean_precision", 0.0)) or 0.0),
            float(metrics.get("recall", metrics.get("summary:mean_recall", 0.0)) or 0.0),
            _fmt_wall_dur(duration_s),
        )
    )
    return report


def _ddeval_workflow_command(
    *,
    config_path: str,
    options: _DDEvalOptions,
) -> str:
    # ddeval workflow run can prompt on test-drive drift and does not expose --yes
    # in the current dd-source CLI.
    if options.command:
        command = _quote_command(options.command)
        prefix = f"printf 'y\\n' | {command} workflow run"
        cd_part = ""
    else:
        prefix = "printf 'y\\n' | bzl run //domains/ai_platform/shared/libs/ddeval/cli:ddeval -- workflow run"
        cd_part = f"cd {shlex.quote(options.ddsource_dir)}"

    parts = [
        prefix,
        f"-s {shlex.quote(options.service)}",
        f"-p {shlex.quote(options.project)}",
        f"-d {shlex.quote(options.dataset)}",
        f"--env {shlex.quote(options.env)}",
        f"--workflow-test-drive {shlex.quote(options.test_drive)}",
        f"-f {shlex.quote(config_path)}",
        f"-j {int(options.jobs)}",
        f"--max-attempts {int(options.max_attempts)}",
    ]
    if options.limit:
        parts.append(f"--limit {int(options.limit)}")
    if options.where_in:
        parts.append(f"--where-in {shlex.quote(options.where_in)}")
    command = " ".join(parts)
    return " && ".join([cd_part, command]) if cd_part else command


def _quote_command(command: str) -> str:
    """Quote a command string while preserving intentional wrapper arguments."""
    return " ".join(shlex.quote(part) for part in shlex.split(command))


def _parse_ddeval_workflow_result(stdout: str) -> dict[str, object]:
    marker = "Result:"
    idx = stdout.rfind(marker)
    if idx < 0:
        raise RuntimeError("ddeval output did not include a Result block")
    payload = stdout[idx + len(marker) :].lstrip()
    try:
        parsed, _ = json.JSONDecoder().raw_decode(payload)
    except json.JSONDecodeError as e:
        raise RuntimeError(f"could not parse ddeval Result JSON: {e}") from e
    if not isinstance(parsed, dict):
        raise RuntimeError(f"ddeval Result JSON was not an object: {type(parsed).__name__}")
    return parsed


def _parse_ddeval_workflow_id(stdout: str) -> str | None:
    match = re.search(r"Workflow Id\s*:\s*([0-9a-fA-F-]{36})", stdout)
    if not match:
        match = re.search(r"wfId:\s*([0-9a-fA-F-]{36})", stdout)
    if not match:
        match = re.search(r"ID:\s*([0-9a-fA-F-]{36})", stdout)
    return match.group(1) if match else None


def _parse_ddeval_workflow_run_id(stdout: str) -> str | None:
    match = re.search(r"Workflow Run Id\s*:\s*([0-9a-fA-F-]{36})", stdout)
    if not match:
        match = re.search(r"runId:\s*([0-9a-fA-F-]{36})", stdout)
    return match.group(1) if match else None


def _ddeval_score(metrics: dict[str, object]) -> float:
    for key in ("summary:mean_f1", "f1"):
        if key not in metrics:
            continue
        try:
            return float(metrics[key])
        except (TypeError, ValueError):
            continue
    raise RuntimeError(f"ddeval metrics did not include a numeric F1 score: {metrics}")


@task
def eval_pipeline(
    ctx,
    n_combos: int = 10,
    n_trials_search: int = 5,
    n_trials_tune: int = 20,
    m_runs: int = 1,
    output_dir: str = "/tmp/observer-pipeline-eval",
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    sigma: float = 30.0,
    seed: int = None,
    build: bool = True,
    overwrite: bool = False,
    force_enable: str = "",
    force_disable: str = "",
    timeout: int = 0,
    scenarios: str = "",
    eval_backend: str = "local",
    ddeval_config_template: str = "",
    ddeval_ddsource_dir: str = "",
    ddeval_command: str = "",
    ddeval_service: str = "eval_worker_observer_log_ad",
    ddeval_project: str = "observer-log-ad",
    ddeval_dataset: str = "observer-log-ad-gensim-store-working",
    ddeval_env: str = "staging",
    ddeval_test_drive: str = "observer-log-ad-ddeval-worker",
    ddeval_jobs: int = 6,
    ddeval_max_attempts: int = 1,
    ddeval_limit: int = 0,
    ddeval_where_in: str = "",
    ddeval_testbench_binary_s3_uri: str = "",
    ddeval_scorer_binary_s3_uri: str = "",
):
    """
    Full pipeline fine-tuning: Bayesian search over component combinations, then deep tuning on the winner.

    Step 1 — Search: for each of n_combos combinations (full stack + anchor subsets + random),
        run Bayesian HP optimisation with n_trials_search trials. The combination with the
        highest optimised score wins.
    Step 2 — Tune: runs a deeper Bayesian optimisation (n_trials_tune trials) on the winner.

    Use --force-enable to pin a newly added component to every combination after
    eval_component recommends KEEP for it.

    Output layout:
        <output_dir>/search/combo_NNN/run_NNN/  - Bayesian search runs per combination
        <output_dir>/tune/                      - deep Bayesian tuning on the winner
        <output_dir>/report.json                - best combo, best HP config, final score

    Args:
        n_combos: Target number of combinations to evaluate in Step 1 (default: 10).
        n_trials_search: Optuna trials per combination during the search phase (default: 5).
        n_trials_tune: Optuna trials for the final fine-tuning pass on the winner (default: 20).
        m_runs: Independent Bayesian runs per combination (default: 1).
        output_dir: Root output directory.
        overwrite: Allow replacing an existing output_dir that contains report.json.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for F1 scoring.
        seed: Base seed for deterministic reproducibility.
        build: Whether to build testbench and scorer first.
        force_enable: Comma-separated components always present in every combination.
        force_disable: Comma-separated components never included.
        timeout: Per-scenario time budget in seconds (0 = no limit).
        scenarios: Comma-separated scenario names to run (default: all SCENARIOS).
        eval_backend: Evaluation backend for each Bayesian trial ("local" or "ddeval").
        ddeval_config_template: Optional JSON experiment config template for ddeval.
        ddeval_ddsource_dir: dd-source checkout containing the ddeval Bazel target.
        ddeval_command: Installed ddeval command or wrapper. When set, this is used
            instead of running the ddeval Bazel target from dd-source.
        ddeval_service: ddeval executor service name.
        ddeval_project: LLMObs/ddEval project name.
        ddeval_dataset: ddEval dataset name.
        ddeval_env: ddEval environment.
        ddeval_test_drive: Rapid Test Drive name for the Atlas worker.
        ddeval_jobs: Scenario concurrency passed to ddeval (-j).
        ddeval_max_attempts: Max attempts per scenario.
        ddeval_limit: Optional dataset limit for smoke tests.
        ddeval_where_in: Optional ddeval --where-in filter.
        ddeval_testbench_binary_s3_uri: S3 URI for the anomalydetection-testbench binary.
            Defaults to $OBSERVER_LOG_AD_DDEVAL_TESTBENCH_BINARY_S3_URI.
        ddeval_scorer_binary_s3_uri: S3 URI for the anomalydetection-scorer binary.
            Defaults to $OBSERVER_LOG_AD_DDEVAL_SCORER_BINARY_S3_URI.

    Examples:
        dda inv --dep optuna anomalydetection.eval-pipeline
        dda inv --dep optuna anomalydetection.eval-pipeline --n-combos 20 --n-trials-search 10 --n-trials-tune 50 --seed 42
        dda inv --dep optuna anomalydetection.eval-pipeline --force-enable scanmw
        dda inv --dep optuna anomalydetection.eval-pipeline --force-disable cusum,scanwelch
        dda inv --dep optuna anomalydetection.eval-pipeline --eval-backend ddeval \
            --ddeval-command ddeval \
            --ddeval-testbench-binary-s3-uri s3://.../anomalydetection-testbench \
            --ddeval-scorer-binary-s3-uri s3://.../anomalydetection-scorer \
            --n-combos 3 --n-trials-search 2 --n-trials-tune 3
    """
    try:
        eval_backend, ddeval_options = _resolve_ddeval_options(
            eval_backend=eval_backend,
            ddeval_config_template=ddeval_config_template,
            ddeval_ddsource_dir=ddeval_ddsource_dir,
            ddeval_command=ddeval_command,
            ddeval_service=ddeval_service,
            ddeval_project=ddeval_project,
            ddeval_dataset=ddeval_dataset,
            ddeval_env=ddeval_env,
            ddeval_test_drive=ddeval_test_drive,
            ddeval_jobs=ddeval_jobs,
            ddeval_max_attempts=ddeval_max_attempts,
            ddeval_limit=ddeval_limit,
            ddeval_where_in=ddeval_where_in,
            ddeval_testbench_binary_s3_uri=ddeval_testbench_binary_s3_uri,
            ddeval_scorer_binary_s3_uri=ddeval_scorer_binary_s3_uri,
        )
    except ValueError as e:
        print(color_message(f"Error: {e}", Color.RED))
        return

    if not _validate_ddeval_scenario_filter(eval_backend, scenarios):
        return

    if seed is not None:
        seed = int(seed)
    else:
        seed = random.randint(0, 2**32 - 1)

    rng = random.Random(seed)
    force_enable_list = [c.strip() for c in force_enable.split(",") if c.strip()]
    force_disable_list = [c.strip() for c in force_disable.split(",") if c.strip()]

    all_known = DETECTORS + CORRELATORS + EXTRACTORS
    unknown = set(force_enable_list + force_disable_list) - set(all_known)
    if unknown:
        print(color_message(f"Error: unknown components: {', '.join(sorted(unknown))}", Color.RED))
        return

    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    full_combo = _full_stack_combo(force_disable=force_disable_list)
    anchor_list = _anchor_combos(force_disable=force_disable_list, force_enable=force_enable_list)
    fixed_combos = [full_combo] + anchor_list
    fixed_keys = {(tuple(c["detectors"]), tuple(c["correlators"])) for c in fixed_combos}
    combo_seed = rng.randint(0, 2**32 - 1)
    random_count = max(0, n_combos - len(fixed_combos))
    random_combos = random_component_combinations(
        random_count,
        seed=combo_seed,
        force_enable=force_enable_list,
        force_disable=force_disable_list,
        exclude_combo_keys=fixed_keys,
    )
    combos = fixed_combos + random_combos
    actual_n_combos = len(combos)

    combo_run_seeds = [[rng.randint(0, 2**32 - 1) for _ in range(m_runs)] for _ in range(actual_n_combos)]
    tune_seed = rng.randint(0, 2**32 - 1)

    n_scenarios = len([s.strip() for s in scenarios.split(",") if s.strip()] if scenarios else SCENARIOS)
    total_search_runs = actual_n_combos * m_runs
    total_testbench_runs = total_search_runs * n_trials_search * n_scenarios

    if build and eval_backend == "local":
        build_testbench(ctx)
        build_scorer(ctx)

    if eval_backend == "local":
        download_scenarios(ctx, scenarios_dir=scenarios_dir, skip_existing=True)

    print(color_message(f"\n{'=' * 70}", Color.BLUE))
    print(color_message("  Observer Pipeline Eval", Color.BLUE))
    print(color_message(f"{'=' * 70}", Color.BLUE))
    print(color_message(f"  n_combos:            {actual_n_combos}", Color.BLUE))
    print(color_message(f"  n_trials_search:     {n_trials_search} (per combo)", Color.BLUE))
    print(color_message(f"  n_trials_tune:       {n_trials_tune} (winner fine-tune)", Color.BLUE))
    print(color_message(f"  m_runs:              {m_runs}", Color.BLUE))
    print(color_message(f"  total search runs:   {total_search_runs}", Color.BLUE))
    print(color_message(f"  total testbench:     {total_testbench_runs}", Color.BLUE))
    print(color_message(f"  seed:                {seed}", Color.BLUE))
    print(color_message(f"  output_dir:          {output_dir}", Color.BLUE))
    print(color_message(f"  backend:             {eval_backend}", Color.BLUE))
    if force_enable_list:
        print(color_message(f"  force-enabled:       {', '.join(force_enable_list)}", Color.BLUE))
    if force_disable_list:
        print(color_message(f"  force-disabled:      {', '.join(force_disable_list)}", Color.BLUE))
    print(color_message(f"{'=' * 70}\n", Color.BLUE))

    print(color_message(f"\n{'=' * 70}", Color.BLUE))
    print(color_message("  Pipeline Eval — Step 1/2: Combination Search", Color.BLUE))
    print(color_message(f"{'=' * 70}", Color.BLUE))

    search_dir = os.path.join(output_dir, "search")
    os.makedirs(search_dir, exist_ok=True)
    run_logger = StepLogger(total_search_runs, "Search run")

    combo_results = []
    for ci, combo in enumerate(combos):
        combo_label = f"combo_{ci:03d}"
        combo_dir = os.path.join(search_dir, combo_label)
        os.makedirs(combo_dir, exist_ok=True)

        components_list = sorted(
            set(
                list(combo["detectors"])
                + list(combo["correlators"])
                + [e for e in EXTRACTORS if e not in force_disable_list]
            )
        )

        combo_name = f"{combo_label} (full stack)" if ci == 0 else combo_label
        run_logger.detail(combo_name)
        run_logger.detail(f"detectors:   {', '.join(combo['detectors'])}")
        run_logger.detail(f"correlators: {', '.join(combo['correlators'])}")

        run_result = _run_bayesian_runs(
            ctx,
            components_list=components_list,
            m_runs=m_runs,
            n_trials=n_trials_search,
            seeds=combo_run_seeds[ci],
            output_dir=combo_dir,
            scenarios_dir=scenarios_dir,
            sigma=sigma,
            timeout=timeout,
            scenarios=scenarios,
            eval_backend=eval_backend,
            ddeval_options=ddeval_options,
            run_logger=run_logger,
            step_label_prefix=combo_label,
        )

        combo_results.append(
            {
                "combo": combo_label,
                "detectors": combo["detectors"],
                "correlators": combo["correlators"],
                "components": components_list,
                "max_score": run_result["max_score"],
                "mean_score": run_result["mean_score"],
                "failed_runs": run_result["failed_runs"],
                "runs": run_result["run_details"],
            }
        )

    valid_combos = [c for c in combo_results if c["max_score"] is not None]
    if not valid_combos:
        print(color_message("Error: all combinations failed — aborting.", Color.RED))
        return

    valid_combos.sort(key=lambda c: c["max_score"], reverse=True)
    best_combo = valid_combos[0]

    print(color_message(f"\n{'=' * 60}", Color.GREEN))
    print(color_message("  Combination Search Summary", Color.GREEN))
    print(color_message(f"{'=' * 60}\n", Color.GREEN))
    header = f"  {'Combo':<12}  {'Score':>6}  {'Detectors':<30}  Correlators"
    print(header)
    print(f"  {'-' * 70}")
    for c in valid_combos:
        print(
            f"  {c['combo']:<12}  {c['max_score']:>6.4f}"
            f"  {', '.join(c['detectors']):<30}  {', '.join(c['correlators'])}"
        )

    print(color_message(f"\n{'=' * 70}", Color.BLUE))
    print(color_message("  Pipeline Eval — Step 2/2: Fine-Tuning on Winner", Color.BLUE))
    print(color_message(f"{'=' * 70}", Color.BLUE))
    print(
        color_message(f"  Winner:     {best_combo['combo']}  (search score={best_combo['max_score']:.4f})", Color.BLUE)
    )
    print(color_message(f"  Components: {', '.join(best_combo['components'])}", Color.BLUE))

    tune_dir = os.path.join(output_dir, "tune")
    tune_result = eval_bayesian(
        ctx,
        components=",".join(best_combo["components"]),
        n_trials=n_trials_tune,
        output_dir=tune_dir,
        scenarios_dir=scenarios_dir,
        sigma=sigma,
        seed=tune_seed,
        build=False,
        overwrite=True,
        timeout=timeout,
        scenarios=scenarios,
        eval_backend=eval_backend,
        **_ddeval_options_kwargs(ddeval_options),
    )

    if not tune_result or tune_result.get("completed_trials", 0) == 0:
        print(color_message("Error: fine-tuning produced no results.", Color.RED))
        return

    final_score = tune_result.get("score", 0.0)
    best_config_path = os.path.join(tune_dir, "best_config.json")

    report = {
        "score": final_score,
        "seed": seed,
        "force_enable": force_enable_list,
        "force_disable": force_disable_list,
        "n_combos": actual_n_combos,
        "n_trials_search": n_trials_search,
        "n_trials_tune": n_trials_tune,
        "best_combo": best_combo,
        "tune": {
            "components": best_combo["components"],
            "score": final_score,
            "best_config_path": best_config_path,
            "report_path": os.path.join(tune_dir, "report.json"),
        },
        "combo_results": combo_results,
    }
    report_path = os.path.join(output_dir, "report.json")
    with open(report_path, "w") as f:
        json.dump(report, f, indent=4)

    print(color_message(f"\n{'=' * 70}", Color.GREEN))
    print(color_message("  Pipeline Eval — Final Summary", Color.GREEN))
    print(color_message(f"{'=' * 70}", Color.GREEN))
    print(
        color_message(
            f"  Winner:        {best_combo['combo']}  ({', '.join(best_combo['detectors'] + best_combo['correlators'])})",
            Color.GREEN,
        )
    )
    print(color_message(f"  Search score:  {best_combo['max_score']:.4f}", Color.GREEN))
    print(color_message(f"  Final score:   {final_score:.4f}", Color.GREEN))
    print(color_message(f"  Best config:   {best_config_path}", Color.GREEN))
    print(color_message(f"  Report:        {report_path}", Color.GREEN))
    print(color_message("\n  Verify with:", Color.BOLD))
    print(color_message(f"    dda inv anomalydetection.eval-scenarios --config {best_config_path}", Color.BOLD))
    print(color_message(f"{'=' * 70}", Color.GREEN))

    return report


@task
def eval_component(
    ctx,
    component: str,
    n_subsets: int = 5,
    n_trials: int = 5,
    m_runs: int = 1,
    output_dir: str = "/tmp/observer-component-eval",
    scenarios_dir: str = "./comp/anomalydetection/observer/scenarios",
    sigma: float = 30.0,
    seed: int = None,
    build: bool = True,
    overwrite: bool = False,
    tune_evaluated_component: bool = False,
    enable: str = "",
    disable: str = "cusum",
    lock: str = "",
    timeout: int = 300,
    scenarios: str = "",
):
    """
    Evaluate whether adding a component improves observer accuracy via Bayesian optimization.

    For N random subsets of detectors/correlators:
      - Run M independent Bayesian optimizations WITHOUT the target component.
      - Run M independent Bayesian optimizations WITH the target component.
      - Compare max best-score across the M runs per subset, averaged over all subsets.

    Output layout:
        <output_dir>/without/subset_NNN/run_NNN/  - bayesian_eval output
        <output_dir>/with/subset_NNN/run_NNN/      - bayesian_eval output
        <output_dir>/report.json                   - comparison report with KEEP/DISCARD recommendation

    Args:
        component: Component to evaluate (any detector, correlator, or extractor).
        n_subsets: Number of random detector/correlator subsets (default: 5).
        n_trials: Optuna trials per Bayesian run (default: 5).
        m_runs: Independent Bayesian optimisation runs per subset per variant (default: 1).
        output_dir: Root output directory.
        overwrite: Allow replacing an existing output_dir that contains report.json.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for F1 scoring.
        seed: Base seed for deterministic reproducibility (default: random).
        build: Whether to build testbench and scorer first.
        tune_evaluated_component: If True, Optuna also tunes the target component's hyperparameters.
        enable: Comma-separated components to force-enable in every subset.
        disable: Comma-separated components to force-disable from every subset (default: cusum).
        lock: Comma-separated components to lock at Go defaults in every Bayesian run.
        timeout: Per-scenario time budget in seconds (default: 300).
        scenarios: Comma-separated scenario names (default: all SCENARIOS).

    Examples:
        dda inv --dep optuna anomalydetection.eval-component --component scanmw
        dda inv --dep optuna anomalydetection.eval-component --component bocpd --seed 42 --n-trials 10
        dda inv --dep optuna anomalydetection.eval-component --component bocpd --timeout 120
        dda inv --dep optuna anomalydetection.eval-component --component bocpd --scenarios food_delivery_redis
    """
    all_known = DETECTORS + CORRELATORS + EXTRACTORS
    if component not in all_known:
        print(color_message(f"Error: unknown component '{component}'. Known: {', '.join(all_known)}", Color.RED))
        return

    scenario_names = [s.strip() for s in scenarios.split(",") if s.strip()] if scenarios else list(SCENARIOS)
    unknown_scenarios = set(scenario_names) - set(SCENARIOS) if SCENARIOS else set()
    if unknown_scenarios:
        print(
            color_message(
                f"Error: unknown scenarios: {', '.join(sorted(unknown_scenarios))}. Known: {', '.join(SCENARIOS)}",
                Color.RED,
            )
        )
        return

    force_enable_list = [c.strip() for c in enable.split(",") if c.strip()]
    force_disable_extra_list = [c.strip() for c in disable.split(",") if c.strip()]
    extra_lock_list = [c.strip() for c in lock.split(",") if c.strip()]

    unknown = set(force_enable_list + force_disable_extra_list + extra_lock_list) - set(all_known)
    if unknown:
        print(color_message(f"Error: unknown components: {', '.join(sorted(unknown))}", Color.RED))
        return

    if component in force_disable_extra_list:
        print(color_message(f"Error: cannot force-disable the evaluated component '{component}'", Color.RED))
        return

    if seed is not None:
        seed = int(seed)
    else:
        seed = random.randint(0, 2**32 - 1)

    rng = random.Random(seed)
    subset_seed = rng.randint(0, 2**32 - 1)
    run_seeds: dict[tuple[str, int], list[int]] = {}
    for variant in ("without", "with"):
        for si in range(n_subsets):
            run_seeds[(variant, si)] = [rng.randint(0, 2**32 - 1) for _ in range(m_runs)]

    is_extractor = component in EXTRACTORS
    force_disable_subsets: list[str] = sorted(set(([] if is_extractor else [component]) + force_disable_extra_list))

    full_stack = _full_stack_combo(force_disable=force_disable_subsets)
    anchor_subsets = _anchor_combos(force_disable=force_disable_subsets, force_enable=force_enable_list)
    fixed_subsets = [full_stack] + anchor_subsets
    fixed_keys = {(tuple(s["detectors"]), tuple(s["correlators"])) for s in fixed_subsets}
    random_count = max(0, n_subsets - len(fixed_subsets))
    random_subsets = random_component_combinations(
        random_count,
        seed=subset_seed,
        force_enable=force_enable_list,
        force_disable=force_disable_subsets,
        exclude_combo_keys=fixed_keys,
    )
    subsets = fixed_subsets + random_subsets
    if len(subsets) < n_subsets:
        print(
            color_message(
                f"Warning: only {len(subsets)} unique subsets generated (requested {n_subsets})",
                Color.ORANGE,
            )
        )
        n_subsets = len(subsets)
    elif len(subsets) > n_subsets:
        for variant in ("without", "with"):
            for si in range(n_subsets, len(subsets)):
                run_seeds[(variant, si)] = [rng.randint(0, 2**32 - 1) for _ in range(m_runs)]
        n_subsets = len(subsets)

    n_scenarios = len(scenario_names)
    total_bayesian_runs = n_subsets * 2 * m_runs
    total_testbench_runs = total_bayesian_runs * n_trials * n_scenarios

    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    eval_start = time.perf_counter()

    if build:
        build_testbench(ctx)
        build_scorer(ctx)

    download_scenarios(ctx, scenarios_dir=scenarios_dir, skip_existing=True)

    print(color_message(f"\n{'=' * 70}", Color.BLUE))
    print(color_message("  Observer Component Evaluation", Color.BLUE))
    print(color_message(f"{'=' * 70}", Color.BLUE))
    print(color_message(f"  component:             {component}", Color.BLUE))
    print(color_message(f"  n_subsets:             {n_subsets}", Color.BLUE))
    print(color_message(f"  m_runs:                {m_runs}", Color.BLUE))
    print(color_message(f"  n_trials:              {n_trials}", Color.BLUE))
    print(color_message(f"  seed:                  {seed}", Color.BLUE))
    print(color_message(f"  scenarios:             {n_scenarios} ({', '.join(scenario_names)})", Color.BLUE))
    print(color_message(f"  total bayesian runs:   {total_bayesian_runs}", Color.BLUE))
    print(color_message(f"  total testbench runs:  {total_testbench_runs}", Color.BLUE))
    print(color_message(f"  output_dir:            {output_dir}", Color.BLUE))
    if force_enable_list:
        print(color_message(f"  force-enabled:         {', '.join(force_enable_list)}", Color.BLUE))
    if force_disable_extra_list:
        print(color_message(f"  force-disabled:        {', '.join(force_disable_extra_list)}", Color.BLUE))
    if extra_lock_list:
        print(color_message(f"  extra locked HPs:      {', '.join(extra_lock_list)}", Color.BLUE))
    if timeout:
        print(
            color_message(
                f"  timeout:               {timeout}s/scenario ({timeout * n_scenarios}s total budget per run)",
                Color.BLUE,
            )
        )
    if tune_evaluated_component:
        print(color_message("  target component HPs:  tuned (Optuna search on 'with')", Color.ORANGE))
    else:
        print(color_message("  target component HPs:  locked at defaults ('with' vs fair baseline)", Color.BLUE))
    print(color_message(f"{'=' * 70}\n", Color.BLUE))

    variant_results: dict[str, list] = {"without": [], "with": []}
    run_logger = StepLogger(total_bayesian_runs, "Bayesian run")

    for variant in ("without", "with"):
        variant_dir = os.path.join(output_dir, variant)
        os.makedirs(variant_dir, exist_ok=True)

        for si, subset in enumerate(subsets):
            subset_label = f"subset_{si:03d}"
            subset_dir = os.path.join(variant_dir, subset_label)
            os.makedirs(subset_dir, exist_ok=True)

            components_list = list(subset["detectors"]) + list(subset["correlators"])
            disabled_set = set(force_disable_extra_list)
            for ext in EXTRACTORS:
                if ext in disabled_set:
                    pass
                elif ext == component:
                    if variant == "with":
                        components_list.append(ext)
                else:
                    components_list.append(ext)
            if not is_extractor and variant == "with":
                components_list.append(component)
            components_list = sorted(set(components_list))

            active_set = set(components_list)
            lock_components = [c for c in extra_lock_list if c in active_set]
            if variant == "with" and not tune_evaluated_component:
                lock_components.append(component)
            lock_for_run = ",".join(sorted(set(lock_components)))

            run_result = _run_bayesian_runs(
                ctx,
                components_list=components_list,
                m_runs=m_runs,
                n_trials=n_trials,
                seeds=run_seeds[(variant, si)],
                output_dir=subset_dir,
                scenarios_dir=scenarios_dir,
                sigma=sigma,
                timeout=timeout,
                scenarios=scenarios,
                lock=lock_for_run,
                run_logger=run_logger,
                step_label_prefix=f"{variant} / {subset_label}",
            )

            variant_results[variant].append(
                {
                    "subset": subset_label,
                    "detectors": subset["detectors"],
                    "correlators": subset["correlators"],
                    "components": components_list,
                    "run_scores": run_result["run_scores"],
                    "max_score": run_result["max_score"],
                    "mean_score": run_result["mean_score"],
                    "failed_runs": run_result["failed_runs"],
                    "runs": run_result["run_details"],
                }
            )

    aggregated = aggregate_eval_component_results(variant_results, n_subsets)

    full_eval_wall_time_seconds = round(time.perf_counter() - eval_start, 3)
    wall_str = f"{_fmt_wall_dur(full_eval_wall_time_seconds)} ({full_eval_wall_time_seconds:.3f}s)"

    print_eval_component_summary(
        component,
        aggregated["per_subset"],
        aggregated["per_scenario_summary"],
        aggregated,
        wall_str,
    )

    all_with_runs = [run for subset in variant_results["with"] for run in subset["runs"] if not run.get("failed")]
    best_with_run = max(all_with_runs, key=lambda r: r["best_score"]) if all_with_runs else None
    best_config_path = None
    if best_with_run:
        src = os.path.join(os.path.dirname(best_with_run["report_path"]), "best_config.json")
        if os.path.isfile(src):
            best_config_path = os.path.join(output_dir, "best_config.json")
            shutil.copy2(src, best_config_path)

    final_report = {
        "component": component,
        "tune_evaluated_component": tune_evaluated_component,
        "recommendation": aggregated["recommendation"],
        "seed": seed,
        "n_subsets": n_subsets,
        "m_runs": m_runs,
        "n_trials": n_trials,
        "full_eval_wall_time_seconds": full_eval_wall_time_seconds,
        "total_bayesian_runs": total_bayesian_runs,
        "total_testbench_runs": total_testbench_runs,
        "best_config_path": best_config_path,
        "best_with_run": best_with_run,
        "summary": {
            "avg_max_score_without": aggregated["avg_max_without"],
            "avg_max_score_with": aggregated["avg_max_with"],
            "delta_max": aggregated["delta_max"],
            "avg_mean_score_without": aggregated["avg_mean_without"],
            "avg_mean_score_with": aggregated["avg_mean_with"],
            "delta_mean": aggregated["delta_mean"],
            "failed_subsets": aggregated["n_failed_subsets"],
            "total_failed_runs": aggregated["total_failed_runs"],
        },
        "per_subset": aggregated["per_subset"],
        "per_subset_scenario": aggregated["per_subset_scenario"],
        "per_scenario_summary": aggregated["per_scenario_summary"],
        "without": variant_results["without"],
        "with": variant_results["with"],
    }
    report_path = os.path.join(output_dir, "report.json")
    with open(report_path, "w") as f:
        json.dump(final_report, f, indent=4)

    print(color_message(f"\n  Report:      {report_path}", Color.GREEN))
    print(color_message(f"  Wall time:   {wall_str}", Color.GREEN))
    if best_config_path:
        score_str = (
            f"{best_with_run['best_score']:.4f}" if best_with_run and best_with_run["best_score"] is not None else "n/a"
        )
        print(color_message(f"  Best config: {best_config_path}  (score={score_str})", Color.GREEN))

    return final_report


# --- Workspace report retrieval ---


@task
def eval_component_workspace_report(
    ctx,
    workspace_name: str,
    output_dir: str = "/tmp/observer-component-eval",
    local_dir: str = "",
    only_report: bool = False,
):
    """
    After ``anomalydetection.eval-component`` on a remote dev workspace, copy results to your machine.

    SSH host is ``workspace-<workspace_name>`` (same as ``workspaces.create``). If
    ``report.json`` exists under ``output_dir``, copies the tree to ``local_dir``
    via ``scp -r`` (or only ``report.json`` with ``--only-report``). If the report
    is missing, eval may still be running — reattach to the tmux session on the host.

    Args:
        workspace_name: Workspace name (SSH: ``workspace-<name>``).
        output_dir:     Remote directory passed to ``anomalydetection.eval-component`` (default: ``/tmp/observer-component-eval``).
        local_dir:      Local destination (default: ``./eval-results/<workspace_name>``).
        only_report:    Copy only ``report.json`` instead of the full output directory.

    Examples:
        dda inv anomalydetection.eval-component-workspace-report --workspace-name eval-bocpd
        dda inv anomalydetection.eval-component-workspace-report --workspace-name eval-bocpd --only-report
    """
    ws_name = workspace_name.strip()
    if not ws_name:
        raise Exit("workspace_name is required")

    ssh_host = f"workspace-{ws_name}"
    remote_report = f"{output_dir}/report.json"

    check = ctx.run(
        f"ssh {shlex.quote(ssh_host)} test -f {shlex.quote(remote_report)}",
        warn=True,
        hide=True,
    )
    if check is None or check.failed:
        print(
            color_message(
                f"Report not found at {remote_report} on {ssh_host} — eval may still be running.",
                Color.ORANGE,
            )
        )
        print(color_message("  Reattach to the workspace tmux session to watch progress:", Color.BLUE))
        print(color_message(f"    dda inv workspaces.tmux-attach --name {ws_name}", Color.BLUE))
        return

    dest = local_dir.strip() or os.path.join("eval-results", ws_name)
    os.makedirs(dest, exist_ok=True)

    print(color_message(f"Copying report from {ssh_host}:{remote_report} to {dest}...", Color.BLUE))

    if only_report:
        ctx.run(f"scp {shlex.quote(f'{ssh_host}:{remote_report}')} {shlex.quote(dest)}/")
        print(color_message(f"report.json copied to {dest}/report.json", Color.GREEN))
    else:
        ctx.run(f"scp -r {shlex.quote(f'{ssh_host}:{output_dir}/.')} {shlex.quote(dest)}/")
        print(color_message(f"Results copied to {dest}/", Color.GREEN))

    print(color_message(f"  report.json: {os.path.join(dest, 'report.json')}", Color.GREEN))


@task
def eval_pipeline_workspace_report(
    ctx,
    workspace_name: str,
    output_dir: str = "/tmp/observer-pipeline-eval",
    local_dir: str = "",
    only_report: bool = False,
):
    """
    After ``anomalydetection.eval-pipeline`` on a remote dev workspace, copy results to your machine.

    Args:
        workspace_name: Workspace name (SSH: ``workspace-<name>``).
        output_dir:     Remote directory passed to ``anomalydetection.eval-pipeline`` (default: ``/tmp/observer-pipeline-eval``).
        local_dir:      Local destination (default: ``./eval-results/<workspace_name>``).
        only_report:    Copy only ``report.json`` instead of the full output directory.

    Examples:
        dda inv anomalydetection.eval-pipeline-workspace-report --workspace-name finetune
        dda inv anomalydetection.eval-pipeline-workspace-report --workspace-name finetune --only-report
    """
    ws_name = workspace_name.strip()
    if not ws_name:
        raise Exit("workspace_name is required")

    ssh_host = f"workspace-{ws_name}"
    remote_report = f"{output_dir}/report.json"

    check = ctx.run(
        f"ssh {shlex.quote(ssh_host)} test -f {shlex.quote(remote_report)}",
        warn=True,
        hide=True,
    )
    if check is None or check.failed:
        print(
            color_message(
                f"Report not found at {remote_report} on {ssh_host} — eval may still be running.",
                Color.ORANGE,
            )
        )
        print(color_message("  Reattach to the workspace tmux session to watch progress:", Color.BLUE))
        print(color_message(f"    dda inv workspaces.tmux-attach --name {ws_name}", Color.BLUE))
        return

    dest = local_dir.strip() or os.path.join("eval-results", ws_name)
    os.makedirs(dest, exist_ok=True)

    print(color_message(f"Copying report from {ssh_host}:{output_dir} to {dest}...", Color.BLUE))

    if only_report:
        ctx.run(f"scp {shlex.quote(f'{ssh_host}:{remote_report}')} {shlex.quote(dest)}/")
        print(color_message(f"report.json copied to {dest}/report.json", Color.GREEN))
    else:
        ctx.run(f"scp -r {shlex.quote(f'{ssh_host}:{output_dir}/.')} {shlex.quote(dest)}/")
        print(color_message(f"Results copied to {dest}/", Color.GREEN))

    print(color_message(f"  report.json: {os.path.join(dest, 'report.json')}", Color.GREEN))
