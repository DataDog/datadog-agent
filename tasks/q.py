import glob
import json
import os
import random
import shlex
import shutil
import time

from invoke import Exit, task

from tasks.libs.common.color import Color, color_message
from tasks.libs.q.eval import (
    _BENCH_FILTER,
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
    _print_benchmark_summary,
    _scenario_f1_from_bayesian_report,
    aggregate_eval_component_results,
    print_eval_bayesian_summary,
    print_eval_component_summary,
    print_eval_scenarios_summary,
    random_component_combinations,
)


# --- Build ---
@task
def build_testbench(ctx):
    """
    Builds the observer-testbench binary.
    """
    ctx.run("go build -o bin/observer-testbench ./cmd/observer-testbench")


@task
def build_scorer(ctx):
    """
    Builds the observer-scorer binary.
    """
    ctx.run("go build -o bin/observer-scorer ./cmd/observer-scorer")


# --- Eval ---
@task
def eval_scenarios(
    ctx,
    scenario: str = "",
    scenarios_dir: str = "./comp/observer/scenarios",
    sigma: float = 30.0,
    only: str = "",
    build: bool = True,
    main_report_path: str = "/tmp/observer-eval-main-report.json",
    config: str = "",
    scenario_output_dir: str = "/tmp",
    timeout: int = 0,
    scenarios: str = "",
    _logger: StepLogger | None = None,
) -> dict[str, object]:
    """
    Runs the observer F1 eval: replays scenarios, scores Gaussian F1.

    > The main score this function produces with the default parameters is the source of truth for our accuracy.
    > The main score is a metric between 0 and 1, 1 being the best.

    Uses testbench --only to control which components are active.
    Default (no --only): uses testbench defaults (bocpd,rrcf,time_cluster + other default-enabled components).
    With --only: enables ONLY listed components + extractors, disables everything else.
      time_cluster is auto-added if not specified.
    With --config: JSON params file for testbench; overrides --only when both are set.

    Examples:
        dda inv q.eval-scenarios                            # defaults
        dda inv q.eval-scenarios --only scanmw              # scanmw + time_cluster (auto)
        dda inv q.eval-scenarios --only bocpd,time_cluster  # explicit
        dda inv q.eval-scenarios --config /tmp/params.json  # Bayesian / custom params

    Args:
        scenario: Run a single scenario (e.g. "213_pagerduty"). Default: all scenarios.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for scoring.
        only: Comma-separated components to enable (passed as --only to testbench). Auto-adds time_cluster.
        build: Whether to build the observer-testbench and observer-scorer binaries.
        main_report_path: Path for the aggregated JSON report.
        config: Path to observer-testbench JSON params file (--config). Empty: omit flag.
        scenario_output_dir: Directory where per-scenario testbench JSON outputs are written.
            Defaults to /tmp. Set to a combo-specific folder to keep outputs co-located.
        timeout: Per-scenario time budget in seconds. A total budget of
            ``timeout × len(scenarios_to_run)`` is shared across all scenarios:
            unused time from fast scenarios rolls over to later ones. A scenario
            that exhausts the remaining budget is killed and skipped. 0 = no limit.
        scenarios: Comma-separated scenario names to run (default: all SCENARIOS).
            Overrides the global SCENARIOS list; ``scenario`` (singular) takes precedence
            when both are set.

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
                f"bin/observer-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)}{only_part}{config_part}",
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
            f"bin/observer-scorer --input {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)} --sigma {sigma} --json",
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

    # Create main report
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
    scenarios_dir: str = "./comp/observer/scenarios",
    sigma: float = 30.0,
    only: str = "",
    build: bool = True,
):
    """
    Runs TP metric scoring: replays scenarios with passthrough correlator and scores
    each detected anomaly against ground truth metric labels in ground_truth.json.

    Uses testbench --only to control which components are active.
    passthrough correlator is auto-added if not specified (required for TP scoring).

    Examples:
        dda inv q.eval-tp --only scanmw              # scanmw + passthrough (auto)
        dda inv q.eval-tp --only bocpd,passthrough    # explicit

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
            f"bin/observer-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)}"
            f" --scenarios-dir {shlex.quote(scenarios_dir)}"
            f" --only {shlex.quote(only_flag)}"
            f" --verbose"
        )

        if not os.path.isfile(output_path):
            print(color_message(f"Testbench did not produce output at {output_path}", Color.RED))
            continue

        scorer_result = ctx.run(
            f"bin/observer-scorer --input {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)} --sigma {sigma} --score-tp --json",
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

    if results:
        print(color_message(f"\n{'=' * 60}", Color.GREEN))
        print(color_message("  Observer TP Eval Summary", Color.GREEN))
        print(color_message(f"{'=' * 60}\n", Color.GREEN))

        header = f"{'Scenario':<25}  {'M F1':>6}  {'M Prec':>7}  {'M Rec':>6}  {'TP':>4}  {'Unk':>5}  {'Found':>5}  {'Missed':>6}"
        print(header)
        print("-" * len(header))

        for r in results:
            print(
                f"{r['name']:<25}"
                f"  {r.get('metric_f1', 0):>6.4f}  {r.get('metric_precision', 0):>7.4f}  {r.get('metric_recall', 0):>6.4f}"
                f"  {r.get('tp_count', 0):>4}  {r.get('unknown_count', 0):>5}"
                f"  {len(r.get('tp_metrics_found') or []):>5}  {len(r.get('tp_metrics_missed') or []):>6}"
            )

        # Print per-scenario TP details
        for r in results:
            detections = r.get("detections", [])
            if not detections:
                continue
            print(color_message(f"\n  {r['name']} detections:", Color.BLUE))
            for d in detections:
                if d.get("detected"):
                    status = (
                        f"HIT (count={d['count']}, first={d.get('delta_from_disruption_sec', 0):.0f}s after disruption)"
                    )
                else:
                    status = "MISS"
                print(f"    [{d['classification']}] {d['service']}/{d['metric']}: {status}")

        print("\nOutput JSONs: /tmp/observer-eval-*-tp.json")


# --- Combination search ---


@task
def eval_combinations(
    ctx,
    n: int = 10,
    output_dir: str = "/tmp/observer-eval-combinations",
    scenarios_dir: str = "./comp/observer/scenarios",
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
        build: Whether to build observer-testbench and observer-scorer first.
        force_enable: Comma-separated components always present in every combination.
        force_disable: Comma-separated components never included in any combination
            (detectors/correlators removed from the random pool; extractors in
            EXTRACTORS are disabled in the written config when listed here).

    Examples:
        dda inv q.eval-combinations --n 20 --seed 42
        dda inv q.eval-combinations --n 5 --output-dir /tmp/ablation
        dda inv q.eval-combinations --n 10 --force-enable bocpd --force-disable scanmw,scanwelch
    """
    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    if build:
        build_testbench(ctx)
        build_scorer(ctx)

    if seed is None:
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


@task
def eval_bayesian(
    ctx,
    components: str = ",".join(DETECTORS + CORRELATORS + EXTRACTORS),
    lock: str = "",
    only: str = "",
    n_trials: int = 10,
    output_dir: str = "/tmp/observer-optuna-eval",
    scenarios_dir: str = "./comp/observer/scenarios",
    sigma: float = 30.0,
    seed: int = None,
    build: bool = True,
    overwrite: bool = False,
    timeout: int = 0,
    scenarios: str = "",
    _logger: StepLogger | None = None,
):
    """
    Run Bayesian hyperparameter optimization (Optuna TPE) for a fixed set of observer components.

    Each trial enables the specified components, sampling hyperparameters and scoring via eval_scenarios (mean F1). Locked components (--lock) are enabled but use Go defaults (not tuned). Components with no tunable hyperparameters are always locked.

    Output layout:
        <output_dir>/trial_NNN/config.json     - sampled component config for this trial
        <output_dir>/trial_NNN/report.json     - eval_scenarios output for this trial
        <output_dir>/trial_NNN/scenarios/      - per-scenario testbench outputs
        <output_dir>/study.pkl                 - serialized Optuna study (for resuming / analysis)
        <output_dir>/report.json               - summary: best score, best params, avg_eval_score
        <output_dir>/best_config.json          - best config used

    Args:
        components: Comma-separated component names to enable (detectors, correlators, extractors; default: all).
        lock: Comma-separated components to enable but not tune (keep at Go defaults).
        only: Shorthand: enable all components but tune only the listed ones (locks everything else).
            Mutually exclusive with --lock.
        n_trials: Number of Optuna trials (default: 50).
        output_dir: Root output directory. If it already contains report.json,
            the task aborts unless overwrite is True; otherwise it is removed first.
        overwrite: Allow replacing an existing output_dir that contains report.json.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for F1 scoring.
        seed: Random seed for TPE sampler reproducibility (default: None = random).
        build: Whether to build observer-testbench and observer-scorer first.
        timeout: Per-scenario time budget in seconds, forwarded to eval_scenarios.
            Total budget per trial = ``timeout × #scenarios``; unused time rolls over.
            0 = no limit.
        scenarios: Comma-separated scenario names to run (default: all SCENARIOS).

    Examples:
        dda inv q.bayesian-eval                                                                       # all components
        dda inv q.bayesian-eval --components bocpd,rrcf,time_cluster,log_pattern_extractor            # fixed subset
        dda inv q.bayesian-eval --components bocpd,rrcf,time_cluster --lock time_cluster              # freeze one
        dda inv q.bayesian-eval --only bocpd                                                          # tune one, lock rest
        dda inv q.bayesian-eval --n-trials 100 --seed 42
    """
    import pickle

    try:
        import optuna
    except Exception:
        import sys

        print(color_message('Please use dda inv --dep optuna ... to run this task', Color.RED), file=sys.stderr)
        raise Exit(1) from None

    only_list = [c.strip() for c in only.split(",") if c.strip()]
    if only_list and lock:
        print(color_message("Error: --only and --lock are mutually exclusive", Color.RED))
        return

    components_list = [c.strip() for c in components.split(",") if c.strip()]

    if only_list:
        # Expand to full component set and lock everything except the --only targets.
        all_components = DETECTORS + CORRELATORS + EXTRACTORS
        unknown_only = set(only_list) - set(all_components)
        if unknown_only:
            print(color_message(f"Error: unknown components in --only: {', '.join(sorted(unknown_only))}", Color.RED))
            return
        components_list = all_components
        locked_set = {c for c in all_components if c not in set(only_list)}
    else:
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

    if seed is None:
        seed = random.randint(0, 2**32 - 1)

    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    if build:
        build_testbench(ctx)
        build_scorer(ctx)

    tuned = [c for c in components_list if c not in locked_set]
    trial_logger = _logger or StepLogger(n_trials, "Trial")

    if not _logger:
        # Only print the full header when running standalone (not nested inside eval_component)
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

        report_path = os.path.join(trial_dir, "report.json")
        scenario_output_dir = os.path.join(trial_dir, "scenarios")
        os.makedirs(scenario_output_dir, exist_ok=True)

        failure_reason: str | None = None
        report = None
        try:
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
            failure_reason = f"eval_scenarios raised {type(e).__name__}: {e}"

        if failure_reason is None and report is None:
            failure_reason = "eval_scenarios returned None (no report produced)"

        if failure_reason is not None:
            trial_logger.detail(f"Trial failed: {failure_reason}", Color.RED)
            failed_trials.append(
                {
                    "trial": trial.number,
                    "label": trial_label,
                    "reason": failure_reason,
                    "config_path": config_path,
                }
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
                f"Failed trials are excluded from scoring.",
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


# --- Component Evaluation ---


@task
def eval_component(
    ctx,
    component: str,
    n_subsets: int = 5,
    n_trials: int = 5,
    m_runs: int = 1,
    output_dir: str = "/tmp/observer-component-eval",
    scenarios_dir: str = "./comp/observer/scenarios",
    sigma: float = 30.0,
    seed: int = None,
    build: bool = True,
    overwrite: bool = False,
    tune_evaluated_component: bool = False,
    enable: str = "",
    # We currently disable cusum because it's slowing down the evaluation significantly
    disable: str = "cusum",
    lock: str = "",
    # By default, allow 5m x #scenarios per run
    timeout: int = 300,
    scenarios: str = "",
):
    """
    Evaluate whether adding a component improves observer accuracy via
    Bayesian optimization on random component subsets.

    For N random subsets of detectors/correlators (extractors stay fixed):
      - Run M independent Bayesian optimizations WITHOUT the target component.
      - Run M independent Bayesian optimizations WITH the target component.
      - Compare the max best-score across the M runs per subset, averaged
        over all subsets.

    By default the evaluated component is locked at Go defaults on the ``with``
    runs (``--lock`` in Bayesian eval), so the Optuna search space matches the
    ``without`` side and n_trials is a fair comparison. Pass
    ``--tune-evaluated-component`` to include the target in the HP search
    (larger space on ``with``; use a higher trial budget if you enable this).

    All seeds are derived deterministically from a single base seed so the
    entire experiment is fully reproducible.

    Output layout:
        <output_dir>/without/subset_NNN/run_NNN/  - bayesian_eval output
        <output_dir>/with/subset_NNN/run_NNN/      - bayesian_eval output
        <output_dir>/report.json                   - comparison report including
            full_eval_wall_time_seconds (build + runs + aggregation; stops before
            summary stdout), per_subset, per_subset_scenario (Δ F1 per scenario), and
            per_scenario_summary (mean/min/max Δ across subsets).

    Args:
        component: Component to evaluate (any detector, correlator, or extractor).
        n_subsets: Number of random detector/correlator subsets (default: 5).
        n_trials: Optuna trials per Bayesian run (default: 5).
        m_runs: Independent Bayesian optimisation runs per subset per variant (default: 1).
        output_dir: Root output directory. If it already contains report.json,
            the task aborts unless overwrite is True.
        overwrite: Allow replacing an existing output_dir that contains report.json.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for F1 scoring.
        seed: Base seed for deterministic reproducibility (default: random).
        build: Whether to build testbench and scorer first.
        tune_evaluated_component: If False (default), the target component is
            locked on ``with`` runs (same effective search complexity as ``without``).
            If True, Optuna also tunes the target component's hyperparameters.
        enable: Comma-separated components to force-enable in every subset
            (present in both ``without`` and ``with`` variants).
        disable: Comma-separated components to force-disable from every subset
            (absent in both variants; must not include the evaluated component).
        lock: Comma-separated components to lock at Go defaults in every
            Bayesian run (not tuned by Optuna, in addition to the evaluated
            component which is locked on ``with`` runs unless
            ``--tune-evaluated-component`` is set).
        timeout: Per-scenario time budget in seconds. Total budget per Bayesian run
            = ``timeout × #scenarios``; unused time from fast scenarios rolls over
            to later ones. 0 = no limit.
        scenarios: Comma-separated scenario names to evaluate (default: all SCENARIOS).
            Useful to focus on a subset and reduce wall-clock time.

    Examples:
        dda inv q.eval-component --component scanmw
        dda inv q.eval-component --component log_pattern_extractor --n-subsets 3
        dda inv q.eval-component --component cusum --seed 42 --n-trials 10
        dda inv q.eval-component --component scanmw --tune-evaluated-component
        dda inv q.eval-component --component bocpd --enable cusum --disable rrcf
        dda inv q.eval-component --component bocpd --lock time_cluster
        dda inv q.eval-component --component bocpd --timeout 120
        dda inv q.eval-component --component bocpd --scenarios 213_pagerduty,353_postmark
    """
    all_known = DETECTORS + CORRELATORS + EXTRACTORS
    if component not in all_known:
        print(color_message(f"Error: unknown component '{component}'. Known: {', '.join(all_known)}", Color.RED))
        return

    # --- parse and validate scenarios ---
    scenario_names = [s.strip() for s in scenarios.split(",") if s.strip()] if scenarios else list(SCENARIOS)
    unknown_scenarios = set(scenario_names) - set(SCENARIOS)
    if unknown_scenarios:
        print(
            color_message(
                f"Error: unknown scenarios: {', '.join(sorted(unknown_scenarios))}. Known: {', '.join(SCENARIOS)}",
                Color.RED,
            )
        )
        return

    # --- parse and validate enable / disable / lock ---
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

    if seed is None:
        seed = random.randint(0, 2**32 - 1)

    # --- deterministic seed derivation ---
    rng = random.Random(seed)
    subset_seed = rng.randint(0, 2**32 - 1)
    run_seeds: dict[tuple[str, int], list[int]] = {}
    for variant in ("without", "with"):
        for si in range(n_subsets):
            run_seeds[(variant, si)] = [rng.randint(0, 2**32 - 1) for _ in range(m_runs)]

    # --- generate subsets ---
    is_extractor = component in EXTRACTORS
    force_disable_subsets: list[str] = sorted(set(([] if is_extractor else [component]) + force_disable_extra_list))

    # Build fixed subsets: full stack, then anchors (minimal + medium), then random fill.
    # Anchors whose required components are disabled are silently skipped.
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
        # Fixed subsets (full stack + anchors) exceed the requested n_subsets; generate
        # run_seeds for the extra indices so the later loop doesn't raise KeyError.
        for variant in ("without", "with"):
            for si in range(n_subsets, len(subsets)):
                run_seeds[(variant, si)] = [rng.randint(0, 2**32 - 1) for _ in range(m_runs)]
        n_subsets = len(subsets)

    # --- compute totals ---
    n_scenarios = len(scenario_names)
    total_bayesian_runs = n_subsets * 2 * m_runs
    total_testbench_runs = total_bayesian_runs * n_trials * n_scenarios

    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    eval_start = time.perf_counter()

    # --- build once ---
    if build:
        build_testbench(ctx)
        build_scorer(ctx)

    # --- ensure scenarios are present ---
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

    # --- run evaluations ---
    variant_results: dict[str, list] = {"without": [], "with": []}
    run_logger = StepLogger(total_bayesian_runs, "Bayesian run")

    for variant in ("without", "with"):
        variant_dir = os.path.join(output_dir, variant)
        os.makedirs(variant_dir, exist_ok=True)

        for si, subset in enumerate(subsets):
            subset_label = f"subset_{si:03d}"
            subset_dir = os.path.join(variant_dir, subset_label)
            os.makedirs(subset_dir, exist_ok=True)

            # Build the full components list for this variant
            components_list = list(subset["detectors"]) + list(subset["correlators"])
            disabled_set = set(force_disable_extra_list)
            for ext in EXTRACTORS:
                if ext in disabled_set:
                    pass  # force-disabled extractor: skip in both variants
                elif ext == component:
                    if variant == "with":
                        components_list.append(ext)
                    # "without" → skip this extractor
                else:
                    components_list.append(ext)
            if not is_extractor and variant == "with":
                components_list.append(component)
            components_list = sorted(set(components_list))

            run_scores: list[float] = []
            run_details: list[dict] = []

            for ri in range(m_runs):
                run_label = f"run_{ri:03d}"
                run_dir = os.path.join(subset_dir, run_label)
                run_seed = run_seeds[(variant, si)][ri]

                run_logger.step(f"{variant} / {subset_label} / {run_label}  (seed={run_seed})")
                run_logger.detail(f"components: {', '.join(components_list)}")
                active_set = set(components_list)
                # Only lock components that are actually active in this run; extra_lock_list
                # entries absent from the subset would cause eval_bayesian to abort with
                # "locked components not in active set".
                lock_components = [c for c in extra_lock_list if c in active_set]
                if variant == "with" and not tune_evaluated_component:
                    lock_components.append(component)
                lock_for_run = ",".join(sorted(set(lock_components)))

                trial_logger = run_logger.child(n_trials, "Trial")
                report = eval_bayesian(
                    ctx,
                    components=",".join(components_list),
                    lock=lock_for_run,
                    n_trials=n_trials,
                    output_dir=run_dir,
                    scenarios_dir=scenarios_dir,
                    sigma=sigma,
                    seed=run_seed,
                    build=False,
                    overwrite=True,
                    timeout=timeout,
                    scenarios=scenarios,
                    _logger=trial_logger,
                )

                run_failed = report is None or report.get("completed_trials", 0) == 0
                if run_failed:
                    if report is None:
                        reason = "eval_bayesian returned None (aborted before producing a report)"
                    else:
                        n_ft = report.get("failed_trials", 0)
                        reason = f"all {n_ft} trials failed — no completed trials in report"
                    run_logger.detail(f"Warning: {variant}/{subset_label}/{run_label} failed: {reason}", Color.RED)

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

            max_score = max(run_scores) if run_scores else None
            mean_score = sum(run_scores) / len(run_scores) if run_scores else None
            n_failed_runs = sum(1 for r in run_details if r["failed"])

            variant_results[variant].append(
                {
                    "subset": subset_label,
                    "detectors": subset["detectors"],
                    "correlators": subset["correlators"],
                    "components": components_list,
                    "run_scores": run_scores,
                    "max_score": max_score,
                    "mean_score": mean_score,
                    "failed_runs": n_failed_runs,
                    "runs": run_details,
                }
            )

    # --- aggregate and print summary ---
    aggregated = aggregate_eval_component_results(variant_results, n_subsets)

    full_eval_wall_time_seconds = round(time.perf_counter() - eval_start, 3)
    _wall_str = f"{_fmt_wall_dur(full_eval_wall_time_seconds)} ({full_eval_wall_time_seconds:.3f}s)"

    print_eval_component_summary(
        component,
        aggregated["per_subset"],
        aggregated["per_scenario_summary"],
        aggregated,
        _wall_str,
    )

    # --- copy best "with" config to root ---
    all_with_runs = [run for subset in variant_results["with"] for run in subset["runs"] if not run.get("failed")]
    best_with_run = max(all_with_runs, key=lambda r: r["best_score"]) if all_with_runs else None
    best_config_path = None
    if best_with_run:
        src = os.path.join(os.path.dirname(best_with_run["report_path"]), "best_config.json")
        if os.path.isfile(src):
            best_config_path = os.path.join(output_dir, "best_config.json")
            shutil.copy2(src, best_config_path)

    # --- write report ---
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
    print(color_message(f"  Wall time:   {_wall_str}", Color.GREEN))
    if best_config_path:
        score_str = (
            f"{best_with_run['best_score']:.4f}" if best_with_run and best_with_run["best_score"] is not None else "n/a"
        )
        print(color_message(f"  Best config: {best_config_path}  (score={score_str})", Color.GREEN))

    return final_report


@task
def download_scenarios(
    ctx,
    scenario: str = "",
    scenarios_dir: str = "./comp/observer/scenarios",
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
        inv q.download-scenarios
        inv q.download-scenarios --scenario=food_delivery_redis
    """
    scenarios_to_download = [scenario] if scenario else SCENARIOS
    for name in scenarios_to_download:
        parquet_dir = os.path.join(scenarios_dir, name, "parquet")
        if skip_existing and os.path.isdir(parquet_dir) and os.listdir(parquet_dir):
            print(color_message(f"Skipping download for '{name}' — parquet data already present", Color.BLUE))
            continue
        # Download to a temp dir first, then swap -- preserves existing data if download fails.
        tmp_parquet_dir = parquet_dir + ".new"
        if os.path.isdir(tmp_parquet_dir):
            shutil.rmtree(tmp_parquet_dir)
        _ensure_parquets(ctx, name, tmp_parquet_dir)
        if os.path.isdir(tmp_parquet_dir) and os.listdir(tmp_parquet_dir):
            if os.path.isdir(parquet_dir):
                shutil.rmtree(parquet_dir)
            os.rename(tmp_parquet_dir, parquet_dir)
        else:
            # Download failed -- clean up temp dir, keep existing data
            shutil.rmtree(tmp_parquet_dir, ignore_errors=True)
            print(color_message(f"Download failed for '{name}', keeping existing data", Color.ORANGE))


@task
def launch_testbench(
    ctx,
    scenarios_dir: str = "./comp/observer/scenarios",
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
):
    """
    Will launch both the observer-testbench backend and UI.

    Args:
        scenarios_dir: The directory containing the scenarios to load.
        build: Whether to build the observer-testbench binary.
        profile: Whether to profile the observer-testbench binary (only in testbench headless mode).
        config: JSON params file; if set, overrides --enable/--disable/--only (testbench behavior).
        enable: Comma-separated components to enable (passed to testbench ``--enable``).
        disable: Comma-separated components to disable (passed to testbench ``--disable``).
        timeout: Seconds before the headless testbench process is killed (0 = no limit;
            ignored in interactive mode).
    """
    if build:
        print("Building observer-testbench...")
        build_testbench(ctx)

    flags = ""
    if verbose:
        flags += " --verbose"
    if config:
        flags += f" --config {shlex.quote(config)}"
    else:
        if enable:
            flags += f" --enable {shlex.quote(enable)}"
        if disable:
            flags += f" --disable {shlex.quote(disable)}"

    if headless_scenario:
        if not headless_output:
            headless_output = f"/tmp/observer-testbench-headless-{headless_scenario}.json"
        if profile:
            if not profile_path:
                profile_path = f"/tmp/observer-testbench-headless-{headless_scenario}.prof"
            flags += f" --memprofile {profile_path}"
        print(
            f"Launching observer-testbench in headless mode for scenario {headless_scenario}, output to {headless_output}"
        )
        try:
            ctx.run(
                f"bin/observer-testbench --headless {headless_scenario} --scenarios-dir {scenarios_dir} --output {headless_output} {flags}",
                timeout=None if timeout == 0 else timeout,
            )
        except Exception as e:
            if type(e).__name__ == "CommandTimedOut":
                print(color_message(f"testbench timed out after {timeout}s", Color.ORANGE))
            else:
                raise
        if profile:
            if open_pprof:
                print('Running pprof...')
                ctx.run(f"go tool pprof -http=:8081 {profile_path}")
            else:
                print(f"To profile, run: go tool pprof -http=:8081 {profile_path}")
    else:
        if not config and not enable and not disable:
            flags += " --only scanmw,scanwelch,bocpd"
        print("Launching observer-testbench backend and UI, use ^C to exit")
        print(
            "To profile, run: go tool pprof -http=:8081 http://localhost:8080/debug/pprof/heap (8080 is the testbench API port)"
        )
        ctx.run(
            f"bin/observer-testbench --scenarios-dir {scenarios_dir} {flags} & ( cd cmd/observer-testbench/ui && npm install && npm run dev ) &"
        )


# --- K8s ---
@task
def deploy_k8s_agent(ctx, cluster_name: str = ""):
    """
    Deploys the Datadog Agent to the Kind cluster on lima VM.

    See tasks/q/datadog-values.template.yaml for the values file used to deploy the agent.
    """

    if not cluster_name:
        cluster_name = os.getenv("USER", 'user') + '-observer-cluster'

    ctx.run(
        f"sed -e 's/$$CLUSTER_NAME/{cluster_name}/g' tasks/q/datadog-values.template.yaml > /tmp/datadog-values.yaml"
    )

    # Try to install, and if already exists, upgrade instead
    try:
        ctx.run('helm install datadog-agent -f /tmp/datadog-values.yaml datadog/datadog')
    except Exception:
        print("Restarting Datadog Agent...")
        uninstall_k8s_agent(ctx)
        ctx.run('helm install datadog-agent -f /tmp/datadog-values.yaml datadog/datadog')


@task
def uninstall_k8s_agent(ctx):
    ctx.run('helm uninstall datadog-agent')


@task
def build_k8s_image(ctx, devenv_id: str = ""):
    """
    Builds the datadog agent image and loads it into the Kind cluster on lima VM.
    """

    if devenv_id:
        devenv_id = f"--id {devenv_id}"

    print(color_message('Building observer-agent image...', Color.BLUE))
    ctx.run(
        f"dda env dev run {devenv_id} -- dda inv -- -e agent.hacky-dev-image-build --trace-agent --target-image observer-agent"
    )
    print(color_message('Saving image to /tmp/observer-agent_latest.tar...', Color.BLUE))
    ctx.run("docker image save observer-agent:latest -o /tmp/observer-agent_latest.tar")
    print(color_message('Copying image to VM...', Color.BLUE))
    ctx.run("limactl copy /tmp/observer-agent_latest.tar gadget-k8s-host:/home/lima.linux/observer-agent_latest.tar")
    print(color_message('Loading image into VM...', Color.BLUE))
    ctx.run("limactl shell --workdir '/home/lima.linux' gadget-k8s-host -- docker load -i observer-agent_latest.tar")
    print(color_message('Loading image into Kind...', Color.BLUE))
    ctx.run(
        "limactl shell --workdir '/home/lima.linux' gadget-k8s-host -- kind load docker-image observer-agent:latest --name gadget-dev"
    )
    print(color_message('Done!', Color.GREEN))


@task
def fetch_k8s_observer_parquet(ctx, dest: str = "/tmp/k8s-observer-metrics"):
    """
    Fetches the observer parquet files (logs / metrics) from the Datadog Agent pod and saves them to the specified destination.
    """

    datadog_agent_pod = ctx.run(
        "kubectl get pod | grep -oE 'datadog-agent-[a-z0-9A-Z]+ '", warn=True, hide=True
    ).stdout.strip()
    if not datadog_agent_pod:
        raise RuntimeError("Datadog Agent pod not found")

    ctx.run(f"kubectl cp {datadog_agent_pod}:/tmp/observer-metrics {dest}")

    print(color_message(f"Fetched observer parquet files to {dest}", Color.GREEN))


# --- Benchmarks ---


@task
def benchmark(ctx, bench=_BENCH_FILTER, benchtime="3s", count=1, only=""):
    """
    Runs the observer benchmark suite and prints a grouped summary.

    Runs ingestion, detection, and real-scenario benchmarks. Storage and
    profiling benchmarks (profile_test.go) are excluded by default.

    Args:
        bench: Benchmark filter regex (default: ingestion + detection + real-scenario families).
        benchtime: Time per benchmark (default: 3s).
        count: Number of runs per benchmark (default: 1).
        only: Comma-separated components to enable exclusively (e.g. bocpd,time_cluster).
              Extractors are always enabled. Default: use catalog defaults.

    Example:
        dda inv q.benchmark
        dda inv q.benchmark --bench BenchmarkDetection --benchtime 5s
        dda inv q.benchmark --only bocpd,time_cluster
    """
    only_args = f" -args -only={shlex.quote(only)}" if only else ""
    cmd = f"go test -run=^$ -bench='{bench}' -benchmem -benchtime={benchtime} -count={count} ./comp/observer/impl/{only_args}"
    print(color_message(f"\nRunning: {cmd}\n", Color.BLUE))
    result = ctx.run(cmd, warn=True)
    if result and result.failed:
        print(color_message("\nBenchmarks failed.", Color.RED))
        return
    _print_benchmark_summary(result.stdout if result else "")


@task
def eval_component_workspace_report(
    ctx,
    workspace_name: str,
    output_dir: str = "/tmp/observer-component-eval",
    local_dir: str = "",
    only_report: bool = False,
):
    """
    After ``q.eval-component`` on a remote dev workspace, copy results to your machine.

    SSH host is ``workspace-<workspace_name>`` (same as ``workspaces.create``). If
    ``report.json`` exists under ``output_dir``, copies the tree to ``local_dir``
    via ``scp -r`` (or only ``report.json`` with ``--only-report``). If the report
    is missing, eval may still be running — reattach to the tmux session on the host.

    Args:
        workspace_name: Workspace name (SSH: ``workspace-<name>``).
        output_dir:     Remote directory passed to ``q.eval-component`` (default: ``/tmp/observer-component-eval``).
        local_dir:      Local destination (default: ``./eval-results/<workspace_name>``).
        only_report:    Copy only ``report.json`` instead of the full output directory.

    Example:
        dda inv q.eval-component-workspace-report --workspace-name eval-bocpd
        dda inv q.eval-component-workspace-report --workspace-name eval-bocpd --only-report
    """
    ws_name = workspace_name.strip()
    if not ws_name:
        raise Exit("workspace_name is required")

    ssh_host = f"workspace-{ws_name}"
    remote_report = f"{output_dir}/report.json"

    # Check whether the report exists yet.
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
        # scp -r copies the remote directory tree into dest.
        ctx.run(f"scp -r {shlex.quote(f'{ssh_host}:{output_dir}/.')} {shlex.quote(dest)}/")
        print(color_message(f"Results copied to {dest}/", Color.GREEN))

    print(color_message(f"  report.json: {os.path.join(dest, 'report.json')}", Color.GREEN))
