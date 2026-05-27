"""
Invoke tasks for anomaly detection dev tooling (not part of agent build).
"""

import glob
import json
import os
import random
import shlex
import shutil
import time

from invoke import Exit, task

from tasks.libs.anomalydetection.eval import (
    DETECTORS,
    CORRELATORS,
    EXTRACTORS,
    SCENARIOS,
    StepLogger,
    _anchor_combos,
    _best_run_index,
    _build_optuna_config,
    _combo_to_config,
    _ensure_parquets,
    _fmt,
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

# --- Build ---


@task
def build_scorer(ctx):
    """
    Builds the anomalydetection-scorer binary to bin/anomalydetection-scorer.
    """
    ctx.run("GOWORK=off go build -C internal/qbranch/anomalydetection-scorer -o ../../../bin/anomalydetection-scorer .")


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
