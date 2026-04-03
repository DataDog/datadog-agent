import glob
import json
import os
import random
import re
import shlex
import shutil
import tempfile
import zipfile

from invoke import task

from tasks.libs.common.color import Color, color_message

SCENARIOS = ["213_pagerduty", "353_postmark", "food_delivery_redis"]

# Maps short scenario names to episode names used in runs.jsonl
SCENARIO_EPISODE_NAMES = {
    "213_pagerduty": "213_PagerDuty_June_2014_Outage",
    "353_postmark": "353_postmark_upstream_cloud_provider_outage",
    "food_delivery_redis": "food-delivery-redis-cpu-saturation",
}

S3_BUCKET = "qbranch-gensim-recordings"
AWS_PROFILE = "sso-agent-sandbox-account-admin"

# All available detectors and correlators for ablation / combination search.
# passthrough is intentionally excluded: it is designed for TP scoring (eval_tp),
# not for Gaussian F1 eval (eval_scenarios / eval_combinations).
DETECTORS = ["bocpd", "cusum", "rrcf", "scanmw", "scanwelch"]
CORRELATORS = ["cross_signal", "time_cluster"]

# Log metrics extractors (component_catalog extractors). Not part of the random
# ablation grid: eval_combinations always enables all of them unless force-disabled.
EXTRACTORS = [
    "log_metrics_extractor",
    "connection_error_extractor",
    "log_pattern_extractor",
]


class StepLogger:
    """Hierarchical progress logger that always shows the full ancestor chain.

    Each step prints [root X/N > ... > this X/N]  title, so every line carries
    complete context regardless of nesting depth. Create a root logger with
    StepLogger(total, label), then nest via .child() which captures the parent.

    Example output (depth 0 → 1 → 2):
        [Bayesian run 1/30]  without/subset_000/run_000
          [Bayesian run 1/30 > Trial 2/5]  trial_001
            [Bayesian run 1/30 > Trial 2/5 > Scenario 1/3]  213_pagerduty
    """

    def __init__(self, total: int, label: str, depth: int = 0, parent: "StepLogger | None" = None):
        self.total = total
        self.label = label
        self.depth = depth
        self._parent = parent
        self._current = 0

    @property
    def _indent(self) -> str:
        return "  " * self.depth

    def _ancestor_tag(self) -> str:
        """Build 'root X/N > ... > this X/N' chain from the root down to this logger."""
        parts = []
        node: StepLogger | None = self
        while node is not None:
            parts.append(f"{node.label} {node._current}/{node.total}")
            node = node._parent
        return " > ".join(reversed(parts))

    def step(self, title: str = "", color: Color = Color.BLUE) -> "StepLogger":
        """Increment counter and print full-context progress line. Returns self for chaining."""
        self._current += 1
        tag = f"[{self._ancestor_tag()}]"
        msg = f"{self._indent}{tag}  {title}" if title else f"{self._indent}{tag}"
        print(color_message(msg, color))
        return self

    def detail(self, message: str, color: Color = Color.BLUE) -> None:
        """Print an indented detail line under the current step."""
        print(color_message(f"{self._indent}  {message}", color))

    def score(self, value: float) -> None:
        """Print a score line, green if positive, red otherwise."""
        clr = Color.GREEN if value > 0 else Color.RED
        print(color_message(f"{self._indent}  score: {value:.4f}", clr))

    def child(self, total: int, label: str) -> "StepLogger":
        """Create a nested logger one level deeper, capturing self as parent."""
        return StepLogger(total, label, depth=self.depth + 1, parent=self)


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


def _prepare_eval_output_dir(output_dir: str, *, overwrite: bool) -> bool:
    """Ensure ``output_dir`` is empty and ready for a fresh eval run.

    If the path exists and contains ``report.json``, the run is aborted unless
    ``overwrite`` is True (avoids silently deleting completed experiments).

    Returns True if the directory is ready, False if the caller should stop.
    """
    if os.path.isfile(output_dir):
        print(color_message(f"Error: output path is a file, not a directory: {output_dir}", Color.RED))
        return False
    report_path = os.path.join(output_dir, "report.json")
    if os.path.isfile(report_path) and not overwrite:
        print(
            color_message(
                f"Error: output directory already contains report.json: {report_path}\n"
                f"Use --overwrite to replace it, or choose a different --output-dir.",
                Color.RED,
            )
        )
        return False
    if os.path.exists(output_dir):
        shutil.rmtree(output_dir)
    os.makedirs(output_dir)
    return True


def _scenario_f1_from_bayesian_report(report: dict | None) -> dict[str, float]:
    """Per-scenario F1 from the best trial's eval_scenarios report (metadata)."""
    if not report:
        return {}
    best = report.get("best_combination")
    if not isinstance(best, dict):
        return {}
    path = best.get("report_path")
    if not path or not os.path.isfile(path):
        return {}
    try:
        with open(path) as f:
            main = json.load(f)
    except (OSError, json.JSONDecodeError, TypeError):
        return {}
    meta = main.get("metadata")
    if not isinstance(meta, dict):
        return {}
    out: dict[str, float] = {}
    for name, row in meta.items():
        if isinstance(row, dict) and "f1" in row:
            try:
                out[str(name)] = float(row["f1"])
            except (TypeError, ValueError):
                continue
    return out


def _best_run_index(runs: list[dict]) -> int:
    """Index of the run with highest best_score (first on ties)."""
    if not runs:
        return -1
    return max(range(len(runs)), key=lambda i: runs[i]["best_score"])


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

    scenarios_to_run = [scenario] if scenario else SCENARIOS
    scenario_logger = _logger or StepLogger(len(scenarios_to_run), "Scenario")

    results = []
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
        ctx.run(
            f"bin/observer-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)}{only_part}{config_part}"
        )

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

    # Print summary table
    if results:
        print(color_message(f"\n{'=' * 60}", Color.GREEN))
        print(color_message("  Observer Eval Summary", Color.GREEN))
        print(color_message(f"{'=' * 60}\n", Color.GREEN))

        # Header
        header = f"{'Scenario':<25}  {'F1':>6}  {'Precision':>9}  {'Recall':>6}  {'Alpha':>7}  {'Scored':>6}  {'Baseline FPs':>12}  {'Warmup (excl)':>13}  {'Cascading (excl)':>16}"
        print(header)
        print("-" * len(header))

        total_baseline_fps = 0
        total_baseline_duration = 0
        for r in results:
            alpha = r.get("alpha", -1)
            alpha_str = f"{alpha:.4f}" if alpha >= 0 else "  n/a"
            print(
                f"{r['name']:<25}  {r['f1']:>6.4f}  {r['precision']:>9.4f}  {r['recall']:>6.4f}"
                f"  {alpha_str:>7}  {r['num_predictions']:>6}  {r['num_baseline_fps']:>12}  {r['num_filtered_warmup']:>13}  {r['num_filtered_cascading']:>16}"
            )
            duration = r.get("baseline_duration_seconds", 0)
            if duration > 0:
                total_baseline_fps += r["num_baseline_fps"]
                total_baseline_duration += duration

        if total_baseline_duration > 0:
            pooled_alpha = total_baseline_fps / total_baseline_duration
            print(
                f"\n  Pooled α: {pooled_alpha:.4f}  ({total_baseline_fps} FPs over {total_baseline_duration}s baseline)"
            )

        print(f"\nOutput JSONs: /tmp/observer-eval-*.json (sigma={sigma}s)")

    # Create main report
    main_score = sum(r["f1"] for r in results) / len(results)
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


def _full_stack_combo(force_disable: list | None = None) -> dict:
    """All detectors and correlators not in force_disable (for eval baseline)."""
    fd = set(force_disable or [])
    return {
        "detectors": sorted(d for d in DETECTORS if d not in fd),
        "correlators": sorted(c for c in CORRELATORS if c not in fd),
    }


# TODO(celian): Add heuristics to prioritize combinations that are more likely to be useful.
def random_component_combinations(
    n: int,
    seed: int = None,
    force_enable: list = None,
    force_disable: list = None,
    exclude_combo_keys: set | None = None,
) -> list:
    """
    Generate up to n distinct random component combinations, each guaranteed to
    contain at least 1 detector (from DETECTORS) and 1 correlator (from CORRELATORS).

    Args:
        n: Target number of distinct combinations to generate.
        seed: Random seed for reproducibility (None = non-deterministic).
        force_enable: Components always present in every combination.
        force_disable: Components never present in any combination (removed from pool).
        exclude_combo_keys: Optional set of (tuple(detectors), tuple(correlators)) keys
            to skip (e.g. the full-stack combo reserved for combo_000).

    Returns:
        List of dicts: {"detectors": [...], "correlators": [...]}
        May be shorter than n if the combinatorial space is exhausted.
    """
    force_enable = set(force_enable or [])
    force_disable = set(force_disable or [])

    det_pool = [d for d in DETECTORS if d not in force_disable]
    cor_pool = [c for c in CORRELATORS if c not in force_disable]
    forced_dets = sorted(d for d in force_enable if d in DETECTORS)
    forced_cors = sorted(c for c in force_enable if c in CORRELATORS)

    # After forcing, we need at least one detector and one correlator in each combo.
    # If force_enable already covers a category, the random part for that category
    # can be empty; otherwise sample at least 1 from the remaining pool.
    rng = random.Random(seed)
    combos = []
    seen: set = set(exclude_combo_keys or [])
    max_attempts = n * 100
    attempts = 0
    while len(combos) < n and attempts < max_attempts:
        attempts += 1

        # Random detectors from the pool (excluding forced, which are added back below)
        free_det_pool = [d for d in det_pool if d not in force_enable]
        if forced_dets:
            # forced already satisfies the ≥1 detector requirement
            extra_dets = sorted(rng.sample(free_det_pool, rng.randint(0, len(free_det_pool)))) if free_det_pool else []
        else:
            if not free_det_pool:
                break  # no detectors available at all
            extra_dets = sorted(rng.sample(free_det_pool, rng.randint(1, len(free_det_pool))))
        dets = sorted(set(forced_dets + extra_dets))

        free_cor_pool = [c for c in cor_pool if c not in force_enable]
        if forced_cors:
            extra_cors = sorted(rng.sample(free_cor_pool, rng.randint(0, len(free_cor_pool)))) if free_cor_pool else []
        else:
            if not free_cor_pool:
                break  # no correlators available at all
            extra_cors = sorted(rng.sample(free_cor_pool, rng.randint(1, len(free_cor_pool))))
        cors = sorted(set(forced_cors + extra_cors))

        key = (tuple(dets), tuple(cors))
        if key in seen:
            continue
        seen.add(key)
        combos.append({"detectors": dets, "correlators": cors})
    if attempts >= max_attempts:
        print(
            color_message(
                f"Warning: Only generated {len(combos)} unique combinations (max attempts={max_attempts})", Color.ORANGE
            )
        )
    return combos


def _combo_to_config(
    detectors: list,
    correlators: list,
    force_disable: list | None = None,
) -> dict:
    """
    Build a testbench JSON params config enabling exactly the listed detectors
    and correlators, explicitly disabling all other detectors/correlators.

    All EXTRACTORS are enabled unless listed in force_disable (explicit
    ``{"enabled": false}`` so ablation stays stable if catalog defaults change).

    The config follows the TestbenchParamsFile format consumed by --config:
        {"components": {"bocpd": {"enabled": true}, "rrcf": {"enabled": false}, ...}}
    """
    force_disable_set = set(force_disable or [])
    enabled_set = set(detectors + correlators)
    components = {}
    for name in DETECTORS + CORRELATORS:
        components[name] = {"enabled": name in enabled_set}
    for name in EXTRACTORS:
        components[name] = {"enabled": name not in force_disable_set}
    return {"components": components}


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


def _sample_component_params(trial, component: str) -> dict:
    """Sample Optuna hyperparameters for a named component that supports parseJSON.

    Each component's params are wrapped in a lambda so that suggest_* calls are
    only executed for the active component. Returns an empty dict for components
    with no tunable hyperparameters (scanmw, scanwelch, log_metrics_extractor,
    connection_error_extractor).
    """
    # TODO(celian): Reduce the search space, this contains every parameter
    space = {
        "bocpd": lambda: {
            "warmup_points": trial.suggest_int("bocpd.warmup_points", 40, 300),
            "hazard": trial.suggest_float("bocpd.hazard", 1e-3, 0.2, log=True),
            "cp_threshold": trial.suggest_float("bocpd.cp_threshold", 0.35, 0.9),
            "short_run_length": trial.suggest_int("bocpd.short_run_length", 2, 20),
            "cp_mass_threshold": trial.suggest_float("bocpd.cp_mass_threshold", 0.4, 0.95),
            "max_run_length": trial.suggest_int("bocpd.max_run_length", 50, 400),
            "prior_variance_scale": trial.suggest_float("bocpd.prior_variance_scale", 1.0, 50.0),
            "min_variance": trial.suggest_float("bocpd.min_variance", 0.01, 5.0, log=True),
            "recovery_points": trial.suggest_int("bocpd.recovery_points", 3, 40),
        },
        "cusum": lambda: {
            "min_points": trial.suggest_int("cusum.min_points", 3, 30),
            "baseline_fraction": trial.suggest_float("cusum.baseline_fraction", 0.05, 0.5),
            "slack_factor": trial.suggest_float("cusum.slack_factor", 0.1, 2.0),
            "threshold_factor": trial.suggest_float("cusum.threshold_factor", 2.0, 10.0),
        },
        "rrcf": lambda: {
            "num_trees": trial.suggest_int("rrcf.num_trees", 20, 200),
            "tree_size": trial.suggest_int("rrcf.tree_size", 64, 512),
            "shingle_size": trial.suggest_int("rrcf.shingle_size", 1, 16),
            "threshold_sigma": trial.suggest_float("rrcf.threshold_sigma", 0.5, 6.0),
        },
        "cross_signal": lambda: {
            "window_seconds": trial.suggest_int("cross_signal.window_seconds", 5, 180),
        },
        "time_cluster": lambda: {
            "proximity_seconds": trial.suggest_int("time_cluster.proximity_seconds", 2, 60),
            "window_seconds": trial.suggest_int("time_cluster.window_seconds", 30, 600),
            "min_cluster_size": trial.suggest_int("time_cluster.min_cluster_size", 0, 8),
        },
        "log_pattern_extractor": lambda: {
            "min_cluster_size_before_emit": trial.suggest_int(
                "log_pattern_extractor.min_cluster_size_before_emit", 1, 30
            ),
            "max_tokenized_string_length": trial.suggest_int(
                "log_pattern_extractor.max_tokenized_string_length", 2000, 16000
            ),
            "max_num_tokens": trial.suggest_int("log_pattern_extractor.max_num_tokens", 32, 512),
            "parse_hex_dump": trial.suggest_categorical("log_pattern_extractor.parse_hex_dump", [True, False]),
            "min_token_match_ratio": trial.suggest_float("log_pattern_extractor.min_token_match_ratio", 0.2, 0.95),
        },
    }
    fn = space.get(component)
    return fn() if fn else {}


def _build_optuna_config(
    trial,
    components: list,
    locked: set,
) -> dict:
    """Build a TestbenchParamsFile config dict for one Optuna trial.

    Enables exactly the listed components, disables all others in the catalog.
    Components in `locked` are enabled but their hyperparameters are not sampled
    (Go catalog defaults apply), effectively fixing them during the search.
    """
    active_set = set(components)
    result = {}

    for name in DETECTORS + CORRELATORS + EXTRACTORS:
        if name not in active_set:
            result[name] = {"enabled": False}

    for name in components:
        params = {"enabled": True}
        if name not in locked:
            params.update(_sample_component_params(trial, name))
        result[name] = params

    return {"components": result}


@task
def eval_bayesian(
    ctx,
    components: str = ",".join(DETECTORS + CORRELATORS + EXTRACTORS),
    lock: str = "",
    n_trials: int = 10,
    output_dir: str = "/tmp/observer-optuna-eval",
    scenarios_dir: str = "./comp/observer/scenarios",
    sigma: float = 30.0,
    seed: int = None,
    build: bool = True,
    overwrite: bool = False,
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
        n_trials: Number of Optuna trials (default: 50).
        output_dir: Root output directory. If it already contains report.json,
            the task aborts unless overwrite is True; otherwise it is removed first.
        overwrite: Allow replacing an existing output_dir that contains report.json.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for F1 scoring.
        seed: Random seed for TPE sampler reproducibility (default: None = random).
        build: Whether to build observer-testbench and observer-scorer first.

    Examples:
        dda inv q.bayesian-eval                                                                       # all components
        dda inv q.bayesian-eval --components bocpd,rrcf,time_cluster,log_pattern_extractor            # fixed subset
        dda inv q.bayesian-eval --components bocpd,rrcf,time_cluster --lock time_cluster              # freeze one
        dda inv q.bayesian-eval --n-trials 100 --seed 42
    """
    import pickle

    import optuna

    components_list = [c.strip() for c in components.split(",") if c.strip()]
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

    completed_trials = []

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

        try:
            report = eval_scenarios(
                ctx,
                scenarios_dir=scenarios_dir,
                sigma=sigma,
                config=config_path,
                build=False,
                main_report_path=report_path,
                scenario_output_dir=scenario_output_dir,
                _logger=trial_logger.child(len(SCENARIOS), "Scenario"),
            )
        except Exception as e:
            trial_logger.detail(f"eval_scenarios failed: {e}", Color.RED)
            return float("-inf")

        score = report.get("score", 0.0) if report else 0.0
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

    if completed_trials:
        print(color_message(f"\n{'=' * 70}", Color.GREEN))
        print(color_message("  Bayesian Optimization Summary", Color.GREEN))
        print(color_message(f"{'=' * 70}\n", Color.GREEN))
        header = f"{'Rank':<5}  {'Trial':<12}  {'Score':>6}"
        print(header)
        print("-" * 30)
        for rank, t in enumerate(completed_trials[:10], 1):
            print(f"{rank:<5}  {t['label']:<12}  {t['score']:>6.4f}")
        if len(completed_trials) > 10:
            print(f"  ... {len(completed_trials) - 10} more trials")

        if best:
            print(color_message(f"\n  Best: {best['label']}  (score={best['score']:.4f})", Color.GREEN))
            print(color_message("  Best parameters:", Color.GREEN))
            for key, val in sorted(best["params"].items()):
                print(color_message(f"    {key}: {val}", Color.GREEN))
            print(color_message(f"  config: {best['config_path']}", Color.GREEN))
            print(color_message(f"  report: {best['report_path']}", Color.GREEN))

    final_report = {
        "score": max_score,
        "avg_eval_score": avg_score,
        "n_trials": n_trials,
        "completed_trials": len(completed_trials),
        "seed": seed,
        "components": components_list,
        "locked": sorted(locked_set),
        "best_combination": best,
        "trials": completed_trials,
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

    print(color_message(f"\n{'=' * 70}", Color.GREEN))
    print(color_message(f"  Report: {report_path}", Color.GREEN))
    print(color_message(f"  score (best):    {max_score:.4f}", Color.GREEN))
    print(color_message(f"  avg_eval_score:  {avg_score:.4f}", Color.GREEN))
    print(color_message(f"  study:           {study_path}", Color.GREEN))
    print(color_message(f"  Per-trial:       {output_dir}/trial_*/report.json", Color.GREEN))
    print(color_message(f"{'=' * 70}", Color.GREEN))

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
):
    """
    Evaluate whether adding a component improves observer accuracy via
    Bayesian optimization on random component subsets.

    For N random subsets of detectors/correlators (extractors stay fixed):
      - Run M independent Bayesian optimizations WITHOUT the target component.
      - Run M independent Bayesian optimizations WITH the target component.
      - Compare the max best-score across the M runs per subset, averaged
        over all subsets.

    All seeds are derived deterministically from a single base seed so the
    entire experiment is fully reproducible.

    Output layout:
        <output_dir>/without/subset_NNN/run_NNN/  - bayesian_eval output
        <output_dir>/with/subset_NNN/run_NNN/      - bayesian_eval output
        <output_dir>/report.json                   - comparison report including
            per_subset, per_subset_scenario (Δ F1 per scenario), and
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

    Examples:
        dda inv q.eval-component --component scanmw
        dda inv q.eval-component --component log_pattern_extractor --n-subsets 3
        dda inv q.eval-component --component cusum --seed 42 --n-trials 10
    """
    all_known = DETECTORS + CORRELATORS + EXTRACTORS
    if component not in all_known:
        print(color_message(f"Error: unknown component '{component}'. Known: {', '.join(all_known)}", Color.RED))
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
    subsets = random_component_combinations(
        n_subsets,
        seed=subset_seed,
        force_disable=[] if is_extractor else [component],
    )
    if len(subsets) < n_subsets:
        print(
            color_message(
                f"Warning: only {len(subsets)} unique subsets generated (requested {n_subsets})",
                Color.ORANGE,
            )
        )
        n_subsets = len(subsets)

    # --- compute totals ---
    scenario_names = sorted(d for d in os.listdir(scenarios_dir) if os.path.isdir(os.path.join(scenarios_dir, d)))
    n_scenarios = len(scenario_names)
    total_bayesian_runs = n_subsets * 2 * m_runs
    total_testbench_runs = total_bayesian_runs * n_trials * n_scenarios

    if not _prepare_eval_output_dir(output_dir, overwrite=overwrite):
        return

    # --- build once ---
    if build:
        build_testbench(ctx)
        build_scorer(ctx)

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
            for ext in EXTRACTORS:
                if ext == component:
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

                trial_logger = run_logger.child(n_trials, "Trial")
                report = eval_bayesian(
                    ctx,
                    components=",".join(components_list),
                    n_trials=n_trials,
                    output_dir=run_dir,
                    scenarios_dir=scenarios_dir,
                    sigma=sigma,
                    seed=run_seed,
                    build=False,
                    overwrite=True,
                    _logger=trial_logger,
                )

                best_score = report.get("score", 0.0) if report else 0.0
                run_scores.append(best_score)
                run_details.append(
                    {
                        "run": run_label,
                        "seed": run_seed,
                        "best_score": best_score,
                        "avg_score": report.get("avg_eval_score", 0.0) if report else 0.0,
                        "report_path": os.path.join(run_dir, "report.json"),
                        "scenario_f1": _scenario_f1_from_bayesian_report(report),
                    }
                )

            max_score = max(run_scores) if run_scores else 0.0
            mean_score = sum(run_scores) / len(run_scores) if run_scores else 0.0

            variant_results[variant].append(
                {
                    "subset": subset_label,
                    "detectors": subset["detectors"],
                    "correlators": subset["correlators"],
                    "components": components_list,
                    "run_scores": run_scores,
                    "max_score": max_score,
                    "mean_score": mean_score,
                    "runs": run_details,
                }
            )

    # --- aggregate per-subset comparison ---
    per_subset = []
    for si in range(n_subsets):
        wo = variant_results["without"][si]
        wi = variant_results["with"][si]
        per_subset.append(
            {
                "subset": wo["subset"],
                "detectors": wo["detectors"],
                "correlators": wo["correlators"],
                "without_max": wo["max_score"],
                "with_max": wi["max_score"],
                "delta_max": wi["max_score"] - wo["max_score"],
                "without_mean": wo["mean_score"],
                "with_mean": wi["mean_score"],
                "delta_mean": wi["mean_score"] - wo["mean_score"],
                "without_run_scores": wo["run_scores"],
                "with_run_scores": wi["run_scores"],
            }
        )

    # --- per-scenario deltas (best run per variant per subset) ---
    per_subset_scenario: list[dict] = []
    for si in range(n_subsets):
        wo_runs = variant_results["without"][si]["runs"]
        wi_runs = variant_results["with"][si]["runs"]
        iwo = _best_run_index(wo_runs)
        iwi = _best_run_index(wi_runs)
        wo_f1 = wo_runs[iwo]["scenario_f1"] if iwo >= 0 else {}
        wi_f1 = wi_runs[iwi]["scenario_f1"] if iwi >= 0 else {}
        names = sorted(set(wo_f1.keys()) | set(wi_f1.keys()))
        by_scenario: dict[str, dict] = {}
        for s in names:
            a = wo_f1.get(s)
            b = wi_f1.get(s)
            delta = (b - a) if a is not None and b is not None else None
            by_scenario[s] = {
                "without_f1": a,
                "with_f1": b,
                "delta_f1": delta,
            }
        per_subset_scenario.append(
            {
                "subset": variant_results["without"][si]["subset"],
                "by_scenario": by_scenario,
            }
        )

    all_scenario_names: set[str] = set()
    for pss in per_subset_scenario:
        all_scenario_names.update(pss["by_scenario"].keys())

    per_scenario_summary: dict[str, dict] = {}
    for s in sorted(all_scenario_names):
        deltas = []
        for pss in per_subset_scenario:
            row = pss["by_scenario"].get(s)
            if row and row.get("delta_f1") is not None:
                deltas.append(row["delta_f1"])
        if not deltas:
            per_scenario_summary[s] = {
                "mean_delta_f1": None,
                "min_delta_f1": None,
                "max_delta_f1": None,
                "n_subsets": 0,
            }
        else:
            per_scenario_summary[s] = {
                "mean_delta_f1": sum(deltas) / len(deltas),
                "min_delta_f1": min(deltas),
                "max_delta_f1": max(deltas),
                "n_subsets": len(deltas),
            }

    without_maxs = [r["max_score"] for r in variant_results["without"]]
    with_maxs = [r["max_score"] for r in variant_results["with"]]
    without_means = [r["mean_score"] for r in variant_results["without"]]
    with_means = [r["mean_score"] for r in variant_results["with"]]

    avg_max_without = sum(without_maxs) / len(without_maxs) if without_maxs else 0.0
    avg_max_with = sum(with_maxs) / len(with_maxs) if with_maxs else 0.0
    avg_mean_without = sum(without_means) / len(without_means) if without_means else 0.0
    avg_mean_with = sum(with_means) / len(with_means) if with_means else 0.0
    delta_max = avg_max_with - avg_max_without
    delta_mean = avg_mean_with - avg_mean_without

    recommendation = "keep" if delta_max > 0 else "discard"
    rec_clr = Color.GREEN if delta_max > 0 else Color.RED

    # --- print summary (primary max-score block last) ---
    print(color_message(f"\n{'=' * 70}", Color.GREEN))
    print(color_message("  Component Evaluation Summary", Color.GREEN))
    print(color_message(f"{'=' * 70}\n", Color.GREEN))
    print(color_message(f"  Component: {component}\n", Color.GREEN))

    header = f"  {'Subset':<12}  {'Without':>8}  {'With':>8}  {'Delta':>8}"
    print(header)
    print(f"  {'-' * 44}")
    for ps in per_subset:
        delta_clr = Color.GREEN if ps["delta_max"] > 0 else Color.RED
        line = f"  {ps['subset']:<12}  {ps['without_max']:>8.4f}  {ps['with_max']:>8.4f}  "
        print(line + color_message(f"{ps['delta_max']:>+8.4f}", delta_clr))

    print()
    print(
        color_message(
            "  Per-scenario Δ F1 (best Bayesian trial per run; best run per variant per subset)",
            Color.BLUE,
        )
    )
    print(
        color_message(
            "  Columns: mean / min / max of Δ across subsets (helps spot inconsistent scenarios)",
            Color.BLUE,
        )
    )
    ps_header = f"  {'Scenario':<26}  {'mean Δ':>8}  {'min Δ':>8}  {'max Δ':>8}"
    print(ps_header)
    print(f"  {'-' * len(ps_header.strip())}")

    def _scenario_sort_key(item: tuple[str, dict]) -> tuple:
        m = item[1].get("mean_delta_f1")
        if m is None:
            return (1, 0.0)
        return (0, -m)

    for scen, row in sorted(per_scenario_summary.items(), key=_scenario_sort_key):
        mean_d = row["mean_delta_f1"]
        min_d = row["min_delta_f1"]
        max_d = row["max_delta_f1"]
        if mean_d is None:
            print(color_message(f"  {scen:<26}  {'n/a':>8}  {'n/a':>8}  {'n/a':>8}", Color.ORANGE))
        else:
            mean_clr = Color.GREEN if mean_d > 0 else Color.RED if mean_d < 0 else Color.BLUE
            msg = f"  {scen:<26}  {mean_d:>+8.4f}  {min_d:>+8.4f}  {max_d:>+8.4f}"
            print(color_message(msg, mean_clr))

    print()
    print(color_message("  Secondary: mean of trial-mean scores (across subsets)", Color.BLUE))
    print(color_message(f"  Avg mean score without: {avg_mean_without:.4f}", Color.BLUE))
    print(color_message(f"  Avg mean score with:    {avg_mean_with:.4f}", Color.BLUE))
    delta_clr = Color.GREEN if delta_mean > 0 else Color.RED
    print(color_message(f"  Delta (mean):           {delta_mean:+.4f}", delta_clr))

    print()
    print(color_message(f"  {'-' * 66}", Color.GREEN))
    print(color_message("  Primary metric — max best-trial score (avg across subsets)", Color.GREEN))
    print(color_message(f"  {'-' * 66}", Color.GREEN))
    print(color_message(f"  Avg max score without:  {avg_max_without:.4f}", Color.GREEN))
    print(color_message(f"  Avg max score with:     {avg_max_with:.4f}", Color.GREEN))
    delta_clr = Color.GREEN if delta_max > 0 else Color.RED
    print(color_message(f"  Delta (max):            {delta_max:+.4f}", delta_clr))
    print()
    print(color_message(f"  Recommendation: {recommendation.upper()} {component}", rec_clr))
    print(color_message(f"{'=' * 70}", Color.GREEN))

    # --- copy best "with" config to root ---
    all_with_runs = [run for subset in variant_results["with"] for run in subset["runs"]]
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
        "recommendation": recommendation,
        "seed": seed,
        "n_subsets": n_subsets,
        "m_runs": m_runs,
        "n_trials": n_trials,
        "total_bayesian_runs": total_bayesian_runs,
        "total_testbench_runs": total_testbench_runs,
        "best_config_path": best_config_path,
        "best_with_run": best_with_run,
        "summary": {
            "avg_max_score_without": avg_max_without,
            "avg_max_score_with": avg_max_with,
            "delta_max": delta_max,
            "avg_mean_score_without": avg_mean_without,
            "avg_mean_score_with": avg_mean_with,
            "delta_mean": delta_mean,
        },
        "per_subset": per_subset,
        "per_subset_scenario": per_subset_scenario,
        "per_scenario_summary": per_scenario_summary,
        "without": variant_results["without"],
        "with": variant_results["with"],
    }
    report_path = os.path.join(output_dir, "report.json")
    with open(report_path, "w") as f:
        json.dump(final_report, f, indent=4)

    print(color_message(f"\n  Report:      {report_path}", Color.GREEN))
    if best_config_path:
        print(
            color_message(f"  Best config: {best_config_path}  (score={best_with_run['best_score']:.4f})", Color.GREEN)
        )

    return final_report


def _resolve_zip_from_runs_jsonl(ctx, name):
    """Resolve the latest zip key for a scenario from runs.jsonl in S3."""
    episode_name = SCENARIO_EPISODE_NAMES.get(name)
    if not episode_name:
        return None

    with tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False, mode="w") as tmp:
        tmp_path = tmp.name

    try:
        result = ctx.run(
            f"aws-vault exec {AWS_PROFILE} -- aws s3 cp s3://{S3_BUCKET}/runs.jsonl {shlex.quote(tmp_path)}",
            warn=True,
            hide=True,
        )
        if result is None or result.failed:
            return None

        with open(tmp_path) as f:
            lines = []
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    lines.append(json.loads(line))
                except json.JSONDecodeError:
                    continue  # skip malformed records

        # Find the latest entry matching this episode
        matches = [entry for entry in lines if entry.get("episode") == episode_name]
        if not matches:
            return None

        latest = matches[-1]
        zip_key = latest.get("zip")
        if zip_key:
            print(
                color_message(
                    f"Resolved '{name}' from runs.jsonl: {zip_key} "
                    f"(image: {latest.get('image', '?')}, {latest.get('timestamp', '?')})",
                    Color.BLUE,
                )
            )
        return zip_key
    except (OSError, json.JSONDecodeError):
        return None
    finally:
        os.unlink(tmp_path)


def _ensure_parquets(ctx, name, parquet_dir):
    """Download and extract parquet files from S3 via runs.jsonl."""
    zip_key = _resolve_zip_from_runs_jsonl(ctx, name)
    if not zip_key:
        print(
            color_message(
                f"No recording found for '{name}' in runs.jsonl. Run a gensim-eks episode to produce one.",
                Color.RED,
            )
        )
        return

    print(color_message(f"Downloading {zip_key} from S3...", Color.BLUE))
    with tempfile.NamedTemporaryFile(suffix=".zip", delete=False) as tmp:
        tmp_path = tmp.name

    try:
        result = ctx.run(
            f"aws-vault exec {AWS_PROFILE} -- aws s3 cp s3://{S3_BUCKET}/{zip_key} {shlex.quote(tmp_path)}",
            warn=True,
        )
        if result is None or result.failed:
            print(color_message(f"Failed to download {zip_key} from S3", Color.RED))
            return

        scenario_dir = os.path.dirname(parquet_dir)
        os.makedirs(parquet_dir, exist_ok=True)
        try:
            with zipfile.ZipFile(tmp_path) as zf:
                for member in zf.namelist():
                    if member.startswith("tmp/gensim-archive/parquet/") and not member.endswith("/"):
                        filename = os.path.basename(member)
                        with zf.open(member) as src, open(os.path.join(parquet_dir, filename), "wb") as dst:
                            dst.write(src.read())
                    elif member.startswith("tmp/gensim-archive/results/") and member.endswith(".json"):
                        with zf.open(member) as src, open(os.path.join(scenario_dir, "episode.json"), "wb") as dst:
                            dst.write(src.read())
        except (zipfile.BadZipFile, OSError) as e:
            print(color_message(f"Failed to extract {zip_key}: {e}", Color.RED))
            shutil.rmtree(parquet_dir, ignore_errors=True)
            return

        print(color_message(f"Extracted parquet files to {parquet_dir}", Color.GREEN))
    finally:
        os.unlink(tmp_path)


@task
def download_scenarios(ctx, scenario: str = "", scenarios_dir: str = "./comp/observer/scenarios"):
    """
    Download scenario parquet data from S3.

    Resolves the latest recording for each scenario from runs.jsonl in the S3 bucket.

    Args:
        scenario: Download a single scenario (e.g. "food_delivery_redis"). Default: all.
        scenarios_dir: Directory containing scenario subdirectories.

    Examples:
        inv q.download-scenarios
        inv q.download-scenarios --scenario=food_delivery_redis
    """
    scenarios_to_download = [scenario] if scenario else SCENARIOS
    for name in scenarios_to_download:
        parquet_dir = os.path.join(scenarios_dir, name, "parquet")
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
):
    """
    Will launch both the observer-testbench backend and UI.

    Args:
        scenarios_dir: The directory containing the scenarios to load.
        build: Whether to build the observer-testbench binary.
        profile: Whether to profile the observer-testbench binary (only in testbench headless mode).
    """
    if build:
        print("Building observer-testbench...")
        build_testbench(ctx)

    flags = ""
    if verbose:
        flags += " --verbose"
    if config:
        flags += f" --config {config}"

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
        ctx.run(
            f"bin/observer-testbench --headless {headless_scenario} --scenarios-dir {scenarios_dir} --output {headless_output} {flags}"
        )
        if profile:
            if open_pprof:
                print('Running pprof...')
                ctx.run(f"go tool pprof -http=:8081 {profile_path}")
            else:
                print(f"To profile, run: go tool pprof -http=:8081 {profile_path}")
    else:
        print("Launching observer-testbench backend and UI, use ^C to exit")
        print(
            "To profile, run: go tool pprof -http=:8081 http://localhost:8080/debug/pprof/heap (8080 is the testbench API port)"
        )
        ctx.run(
            f"bin/observer-testbench --scenarios-dir {scenarios_dir} --only scanmw,scanwelch,bocpd {flags} & ( cd cmd/observer-testbench/ui && npm install && npm run dev ) &"
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

_BENCH_FILTER = "BenchmarkDetection|BenchmarkIngestion|BenchmarkLogExtraction"


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


def _print_benchmark_summary(output):
    """Parse go benchmark output and print a grouped readable summary."""
    # Matches: BenchmarkFoo/param=val-NCPU  count  ns/op  [B/op  allocs/op]
    line_re = re.compile(
        r'^(Benchmark[A-Za-z_]+)(?:/([\w=.,/-]+?))?-\d+\s+\d+\s+([\d.]+)\s+ns/op'
        r'(?:\s+([\d.]+)\s+B/op\s+([\d.]+)\s+allocs/op)?'
    )

    groups = {}  # family -> [(param, ns_op, b_op, allocs)]
    for line in output.splitlines():
        m = line_re.match(line)
        if not m:
            continue
        family, param, ns_op, b_op, allocs = m.groups()
        groups.setdefault(family, []).append(
            (
                param or "",
                float(ns_op),
                float(b_op or 0),
                float(allocs or 0),
            )
        )

    if not groups:
        return

    print(color_message(f"\n{'=' * 65}", Color.GREEN))
    print(color_message("  Observer Benchmark Summary", Color.GREEN))
    print(color_message(f"{'=' * 65}\n", Color.GREEN))

    for family, rows in groups.items():
        print(color_message(family, Color.BLUE))
        has_params = any(r[0] for r in rows)
        if has_params:
            print(f"  {'param':<22}  {'time/op':>10}  {'B/op':>8}  {'allocs':>7}")
            print("  " + "-" * 54)
            for param, ns_op, b_op, allocs in rows:
                print(f"  {param:<22}  {_fmt_ns(ns_op):>10}  {b_op:>8.0f}  {allocs:>7.0f}")
        else:
            print(f"  {'time/op':>10}  {'B/op':>8}  {'allocs':>7}")
            print("  " + "-" * 30)
            for _, ns_op, b_op, allocs in rows:
                print(f"  {_fmt_ns(ns_op):>10}  {b_op:>8.0f}  {allocs:>7.0f}")
        print()


def _fmt_ns(ns):
    """Format nanoseconds into a human-readable string."""
    if ns >= 1_000_000_000:
        return f"{ns / 1_000_000_000:.2f}s"
    if ns >= 1_000_000:
        return f"{ns / 1_000_000:.2f}ms"
    if ns >= 1_000:
        return f"{ns / 1_000:.2f}µs"
    return f"{ns:.0f}ns"
