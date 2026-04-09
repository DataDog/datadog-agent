"""Evaluation library for the observer eval tasks (extracted from tasks/q.py).

Contains all non-@task symbols: constants, helpers, and extracted logic blocks.
The @task-decorated entry points remain in tasks/q.py and delegate here.
"""

import json
import os
import random
import re
import shlex
import shutil
import tempfile
import zipfile

from tasks.libs.common.color import Color, color_message

# --- Constants ---

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

# Fixed anchor subsets used by eval_component to anchor the evaluation at known
# reference configurations regardless of the random seed.
#   [0] minimal: smallest meaningful stack (1 detector + 1 correlator)
#   [1] medium:  a richer mid-complexity stack for a second anchor point
# Components missing from the pool (because they are force-disabled or are the
# evaluated component) are stripped; an anchor is omitted entirely if it ends up
# with no detectors or no correlators after filtering.
ANCHOR_COMBOS = [
    {"detectors": ["bocpd"], "correlators": ["time_cluster"]},
    {"detectors": ["bocpd", "rrcf"], "correlators": ["cross_signal", "time_cluster"]},
]

_BENCH_FILTER = "BenchmarkDetection|BenchmarkIngestion|BenchmarkLogExtraction"


# --- StepLogger ---


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


# --- Phase 1: Helper functions ---


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
    """Index of the run with highest best_score among non-failed runs (first on ties).

    Returns -1 if there are no runs or all runs failed.
    """
    valid = [i for i, r in enumerate(runs) if not r.get("failed") and r.get("best_score") is not None]
    if not valid:
        return -1
    return max(valid, key=lambda i: runs[i]["best_score"])


def _full_stack_combo(force_disable: list | None = None) -> dict:
    """All detectors and correlators not in force_disable (for eval baseline)."""
    fd = set(force_disable or [])
    return {
        "detectors": sorted(d for d in DETECTORS if d not in fd),
        "correlators": sorted(c for c in CORRELATORS if c not in fd),
    }


def _anchor_combos(force_disable: list | None = None, force_enable: list | None = None) -> list[dict]:
    """Fixed anchor subsets derived from ANCHOR_COMBOS after filtering force_disable.

    Each anchor that ends up with at least one detector and one correlator is
    included.  Anchors whose key components are all removed are silently skipped
    so that evaluating a component that appears in an anchor does not break the
    run.  force_enable components are unconditionally added to every anchor.
    """
    fd = set(force_disable or [])
    fe_dets = sorted(d for d in (force_enable or []) if d in DETECTORS and d not in fd)
    fe_cors = sorted(c for c in (force_enable or []) if c in CORRELATORS and c not in fd)
    anchors = []
    seen_keys: set = set()
    for combo in ANCHOR_COMBOS:
        dets = sorted({d for d in combo["detectors"] if d not in fd} | set(fe_dets))
        cors = sorted({c for c in combo["correlators"] if c not in fd} | set(fe_cors))
        if not dets or not cors:
            continue
        key = (tuple(dets), tuple(cors))
        if key in seen_keys:
            continue
        seen_keys.add(key)
        anchors.append({"detectors": dets, "correlators": cors})
    return anchors


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


def _sample_component_params(trial, component: str) -> dict:
    """Sample Optuna hyperparameters for a named component that supports parseJSON.

    Each component's params are wrapped in a lambda so that suggest_* calls are
    only executed for the active component. Returns an empty dict for components
    with no tunable hyperparameters (scanmw, scanwelch, log_metrics_extractor,
    connection_error_extractor).
    """
    # Commented hyperparameters are the ones that have the less impact, this is used
    # to reduce the search space and speed up the evaluation
    space = {
        "bocpd": lambda: {
            # "warmup_points": trial.suggest_int("bocpd.warmup_points", 40, 300),
            "hazard": trial.suggest_float("bocpd.hazard", 1e-3, 0.2, log=True),
            "cp_threshold": trial.suggest_float("bocpd.cp_threshold", 0.35, 0.9),
            # "short_run_length": trial.suggest_int("bocpd.short_run_length", 2, 20),
            # "cp_mass_threshold": trial.suggest_float("bocpd.cp_mass_threshold", 0.4, 0.95),
            # "max_run_length": trial.suggest_int("bocpd.max_run_length", 50, 400),
            # "prior_variance_scale": trial.suggest_float("bocpd.prior_variance_scale", 1.0, 50.0),
            # "min_variance": trial.suggest_float("bocpd.min_variance", 0.01, 5.0, log=True),
            # "recovery_points": trial.suggest_int("bocpd.recovery_points", 3, 40),
        },
        "cusum": lambda: {
            # "min_points": trial.suggest_int("cusum.min_points", 3, 30),
            # "baseline_fraction": trial.suggest_float("cusum.baseline_fraction", 0.05, 0.5),
            # "slack_factor": trial.suggest_float("cusum.slack_factor", 0.1, 2.0),
            "threshold_factor": trial.suggest_float("cusum.threshold_factor", 2.0, 10.0),
        },
        "rrcf": lambda: {
            # "num_trees": trial.suggest_int("rrcf.num_trees", 20, 200),
            # "tree_size": trial.suggest_int("rrcf.tree_size", 64, 512),
            "shingle_size": trial.suggest_int("rrcf.shingle_size", 1, 16),
            "threshold_sigma": trial.suggest_float("rrcf.threshold_sigma", 0.5, 6.0),
        },
        "cross_signal": lambda: {
            # "window_seconds": trial.suggest_int("cross_signal.window_seconds", 5, 180),
        },
        "time_cluster": lambda: {
            # "proximity_seconds": trial.suggest_int("time_cluster.proximity_seconds", 2, 60),
            # "window_seconds": trial.suggest_int("time_cluster.window_seconds", 30, 600),
            # "min_cluster_size": trial.suggest_int("time_cluster.min_cluster_size", 1, 8),
        },
        "log_pattern_extractor": lambda: {
            # "disable_optimizations": trial.suggest_categorical(
            #     "log_pattern_extractor.disable_optimizations", [True, False]
            # ),
            # "min_cluster_size_before_emit": trial.suggest_int(
            #     "log_pattern_extractor.min_cluster_size_before_emit", 1, 30
            # ),
            # "max_tokenized_string_length": trial.suggest_int(
            #     "log_pattern_extractor.max_tokenized_string_length", 2000, 16000
            # ),
            # "max_num_tokens": trial.suggest_int("log_pattern_extractor.max_num_tokens", 32, 512),
            # "parse_hex_dump": trial.suggest_categorical("log_pattern_extractor.parse_hex_dump", [True, False]),
            "min_token_match_ratio": trial.suggest_float("log_pattern_extractor.min_token_match_ratio", 0.2, 0.95),
            # "cluster_time_to_live_sec": trial.suggest_int("log_pattern_extractor.cluster_time_to_live_sec", 600, 86400),
            # "garbage_collection_interval_sec": trial.suggest_int(
            #     "log_pattern_extractor.garbage_collection_interval_sec", 60, 7200
            # ),
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
            print(f"  {'-' * 54}")
            for param, ns_op, b_op, allocs in rows:
                print(f"  {param:<22}  {_fmt_ns(ns_op):>10}  {b_op:>8.0f}  {allocs:>7.0f}")
        else:
            print(f"  {'time/op':>10}  {'B/op':>8}  {'allocs':>7}")
            print(f"  {'-' * 30}")
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


# --- Phase 2: Extracted logic blocks ---


def _fmt_wall_dur(s: float) -> str:
    """Format a duration in seconds into a human-readable h/m/s string."""
    if s >= 3600:
        return f"{int(s // 3600)}h {int((s % 3600) // 60)}m {s % 60:.1f}s"
    if s >= 60:
        return f"{int(s // 60)}m {s % 60:.1f}s"
    return f"{s:.1f}s"


def aggregate_eval_component_results(
    variant_results: dict,
    n_subsets: int,
) -> dict:
    """Aggregate per-subset and per-scenario comparison between without/with variants.

    Returns a dict with keys:
        per_subset, per_subset_scenario, per_scenario_summary,
        valid_deltas_max, valid_deltas_mean,
        n_failed_subsets, total_failed_runs,
        delta_max, delta_mean,
        avg_max_without, avg_max_with, avg_mean_without, avg_mean_with,
        recommendation, rec_clr
    """
    # --- aggregate per-subset comparison ---
    per_subset = []
    for si in range(n_subsets):
        wo = variant_results["without"][si]
        wi = variant_results["with"][si]
        wo_max = wo["max_score"]
        wi_max = wi["max_score"]
        wo_mean = wo["mean_score"]
        wi_mean = wi["mean_score"]
        delta_max_ps = (wi_max - wo_max) if wo_max is not None and wi_max is not None else None
        delta_mean_ps = (wi_mean - wo_mean) if wo_mean is not None and wi_mean is not None else None
        failed_runs_total = wo["failed_runs"] + wi["failed_runs"]
        per_subset.append(
            {
                "subset": wo["subset"],
                "detectors": wo["detectors"],
                "correlators": wo["correlators"],
                "without_max": wo_max,
                "with_max": wi_max,
                "delta_max": delta_max_ps,
                "without_mean": wo_mean,
                "with_mean": wi_mean,
                "delta_mean": delta_mean_ps,
                "failed_runs": failed_runs_total,
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

    # Only include subsets where both variants produced at least one successful run.
    valid_deltas_max = [ps["delta_max"] for ps in per_subset if ps["delta_max"] is not None]
    valid_deltas_mean = [ps["delta_mean"] for ps in per_subset if ps["delta_mean"] is not None]
    n_failed_subsets = sum(1 for ps in per_subset if ps["delta_max"] is None)
    total_failed_runs = sum(ps["failed_runs"] for ps in per_subset)

    if n_failed_subsets > 0:
        print(
            color_message(
                f"Warning: {n_failed_subsets}/{n_subsets} subsets had fully failed runs and are excluded from the final delta.",
                Color.ORANGE,
            )
        )
    if total_failed_runs > 0 and n_failed_subsets == 0:
        print(
            color_message(
                f"Warning: {total_failed_runs} individual run(s) failed across subsets; "
                "partial results were excluded from per-subset scores.",
                Color.ORANGE,
            )
        )

    delta_max = sum(valid_deltas_max) / len(valid_deltas_max) if valid_deltas_max else None
    delta_mean = sum(valid_deltas_mean) / len(valid_deltas_mean) if valid_deltas_mean else None

    avg_max_without_vals = [ps["without_max"] for ps in per_subset if ps["without_max"] is not None]
    avg_max_with_vals = [ps["with_max"] for ps in per_subset if ps["with_max"] is not None]
    avg_mean_without_vals = [ps["without_mean"] for ps in per_subset if ps["without_mean"] is not None]
    avg_mean_with_vals = [ps["with_mean"] for ps in per_subset if ps["with_mean"] is not None]

    avg_max_without = sum(avg_max_without_vals) / len(avg_max_without_vals) if avg_max_without_vals else None
    avg_max_with = sum(avg_max_with_vals) / len(avg_max_with_vals) if avg_max_with_vals else None
    avg_mean_without = sum(avg_mean_without_vals) / len(avg_mean_without_vals) if avg_mean_without_vals else None
    avg_mean_with = sum(avg_mean_with_vals) / len(avg_mean_with_vals) if avg_mean_with_vals else None

    if delta_max is None:
        recommendation = "inconclusive"
        rec_clr = Color.ORANGE
    elif delta_max > 0:
        recommendation = "keep"
        rec_clr = Color.GREEN
    else:
        recommendation = "discard"
        rec_clr = Color.RED

    return {
        "per_subset": per_subset,
        "per_subset_scenario": per_subset_scenario,
        "per_scenario_summary": per_scenario_summary,
        "valid_deltas_max": valid_deltas_max,
        "valid_deltas_mean": valid_deltas_mean,
        "n_failed_subsets": n_failed_subsets,
        "total_failed_runs": total_failed_runs,
        "delta_max": delta_max,
        "delta_mean": delta_mean,
        "avg_max_without": avg_max_without,
        "avg_max_with": avg_max_with,
        "avg_mean_without": avg_mean_without,
        "avg_mean_with": avg_mean_with,
        "recommendation": recommendation,
        "rec_clr": rec_clr,
    }


def _scenario_sort_key(item: tuple[str, dict]) -> tuple:
    m = item[1].get("mean_delta_f1")
    if m is None:
        return (1, 0.0)
    return (0, -m)


def _fmt(v: float | None, signed: bool = False) -> str:
    if v is None:
        return "n/a"
    return f"{v:+.4f}" if signed else f"{v:.4f}"


def print_eval_component_summary(
    component: str,
    per_subset: list,
    per_scenario_summary: dict,
    aggregated: dict,
    wall_str: str,
) -> None:
    """Print the full component evaluation summary to stdout."""
    delta_max = aggregated["delta_max"]
    delta_mean = aggregated["delta_mean"]
    avg_max_without = aggregated["avg_max_without"]
    avg_max_with = aggregated["avg_max_with"]
    avg_mean_without = aggregated["avg_mean_without"]
    avg_mean_with = aggregated["avg_mean_with"]
    recommendation = aggregated["recommendation"]
    rec_clr = aggregated["rec_clr"]

    print(color_message(f"\n{'=' * 70}", Color.GREEN))
    print(color_message("  Component Evaluation Summary", Color.GREEN))
    print(color_message(f"{'=' * 70}\n", Color.GREEN))
    print(color_message(f"  Component: {component}\n", Color.GREEN))

    header = f"  {'Subset':<12}  {'Without':>8}  {'With':>8}  {'Delta':>8}"
    print(header)
    print(f"  {'-' * 44}")
    for ps in per_subset:
        wo_str = f"{ps['without_max']:>8.4f}" if ps["without_max"] is not None else f"{'FAILED':>8}"
        wi_str = f"{ps['with_max']:>8.4f}" if ps["with_max"] is not None else f"{'FAILED':>8}"
        if ps["delta_max"] is None:
            delta_str = color_message(f"{'n/a':>8}", Color.ORANGE)
        else:
            delta_clr = Color.GREEN if ps["delta_max"] > 0 else Color.RED
            delta_str = color_message(f"{ps['delta_max']:>+8.4f}", delta_clr)
        fail_note = f"  [{ps['failed_runs']} run(s) failed]" if ps["failed_runs"] > 0 else ""
        print(f"  {ps['subset']:<12}  {wo_str}  {wi_str}  {delta_str}{fail_note}")

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
    print(color_message(f"  Avg mean score without: {_fmt(avg_mean_without)}", Color.BLUE))
    print(color_message(f"  Avg mean score with:    {_fmt(avg_mean_with)}", Color.BLUE))
    if delta_mean is None:
        print(color_message("  Delta (mean):           n/a (no valid subsets)", Color.ORANGE))
    else:
        delta_clr = Color.GREEN if delta_mean > 0 else Color.RED
        print(color_message(f"  Delta (mean):           {delta_mean:+.4f}", delta_clr))

    print()
    print(color_message(f"  {'-' * 66}", Color.GREEN))
    print(color_message("  Primary metric — max best-trial score (avg across subsets)", Color.GREEN))
    print(color_message(f"  {'-' * 66}", Color.GREEN))
    print(color_message(f"  Avg max score without:  {_fmt(avg_max_without)}", Color.GREEN))
    print(color_message(f"  Avg max score with:     {_fmt(avg_max_with)}", Color.GREEN))
    if delta_max is None:
        print(color_message("  Delta (max):            n/a (no valid subsets)", Color.ORANGE))
    else:
        delta_clr = Color.GREEN if delta_max > 0 else Color.RED
        print(color_message(f"  Delta (max):            {delta_max:+.4f}", delta_clr))
    print()
    print(color_message(f"  Recommendation: {recommendation.upper()} {component}", rec_clr))
    if recommendation == "inconclusive":
        print(color_message("  All subsets failed — cannot make a reliable recommendation.", Color.ORANGE))
    elif recommendation == "keep":
        print(color_message("  Next step: update your config, then tune the new component using:", Color.BOLD))
        print(color_message(f"    dda inv q.eval-bayesian --only {component}", Color.BOLD))
    print(color_message(f"  Full eval wall time: {wall_str}", Color.GREEN))
    print(color_message(f"{'=' * 70}", Color.GREEN))


def print_eval_scenarios_summary(results: list, sigma: float) -> None:
    """Print the Observer Eval Summary table to stdout."""
    if not results:
        return

    print(color_message(f"\n{'=' * 60}", Color.GREEN))
    print(color_message("  Observer Eval Summary", Color.GREEN))
    print(color_message(f"{'=' * 60}\n", Color.GREEN))

    header = f"{'Scenario':<25}  {'F1':>6}  {'Precision':>9}  {'Recall':>6}  {'Alpha':>7}  {'Scored':>6}  {'Baseline FPs':>12}  {'Warmup (excl)':>13}  {'Cascading (excl)':>16}"
    print(header)
    print("-" * len(header))

    total_baseline_fps = 0
    total_baseline_duration = 0
    for r in results:
        alpha = r.get("alpha", -1)
        alpha_str = f"{alpha:.4f}" if alpha >= 0 else "  n/a"
        timed_out_suffix = "  [TIMEOUT]" if r.get("timed_out") else ""
        print(
            f"{r['name']:<25}  {r['f1']:>6.4f}  {r['precision']:>9.4f}  {r['recall']:>6.4f}"
            f"  {alpha_str:>7}  {r['num_predictions']:>6}  {r['num_baseline_fps']:>12}  {r['num_filtered_warmup']:>13}  {r['num_filtered_cascading']:>16}"
            f"{timed_out_suffix}"
        )
        duration = r.get("baseline_duration_seconds", 0)
        if duration > 0:
            total_baseline_fps += r["num_baseline_fps"]
            total_baseline_duration += duration

    if total_baseline_duration > 0:
        pooled_alpha = total_baseline_fps / total_baseline_duration
        print(f"\n  Pooled α: {pooled_alpha:.4f}  ({total_baseline_fps} FPs over {total_baseline_duration}s baseline)")

    print(f"\nOutput JSONs: /tmp/observer-eval-*.json (sigma={sigma}s)")


def print_eval_bayesian_summary(
    completed_trials: list,
    best: dict | None,
    max_score: float,
    avg_score: float,
    output_dir: str,
    study_path: str,
) -> None:
    """Print the Bayesian Optimization Summary table and final report paths."""
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

    report_path = os.path.join(output_dir, "report.json")
    print(color_message(f"\n{'=' * 70}", Color.GREEN))
    print(color_message(f"  Report: {report_path}", Color.GREEN))
    print(color_message(f"  score (best):    {max_score:.4f}", Color.GREEN))
    print(color_message(f"  avg_eval_score:  {avg_score:.4f}", Color.GREEN))
    print(color_message(f"  study:           {study_path}", Color.GREEN))
    print(color_message(f"  Per-trial:       {output_dir}/trial_*/report.json", Color.GREEN))
    print(color_message(f"{'=' * 70}", Color.GREEN))
