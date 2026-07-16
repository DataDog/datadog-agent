"""Evaluation helpers for anomaly detection dev tasks.

Contains non-@task symbols: constants, S3 helpers, StepLogger, combo helpers,
Optuna config builders, aggregation helpers, and summary printers used by the
@task entry points in tasks/anomalydetection.py.

Note: SCENARIOS and SCENARIO_EPISODE_NAMES are loaded from
q_branch/gensim-eval-scenarios.json at import time. That file is not
committed to this repo — it is expected to be present in developer environments
where the q_branch/ directory has been checked out separately, or produced by
dda inv anomalydetection.download-scenarios.
"""

import json
import os
import random
import shlex
import shutil
import tempfile
import zipfile

from tasks.libs.common.color import Color, color_message

# --- Constants ---

_MANIFEST_PATH = os.path.join(os.path.dirname(__file__), "..", "..", "..", "q_branch", "gensim-eval-scenarios.json")

try:
    with open(_MANIFEST_PATH) as _f:
        _EVAL_MANIFEST = json.load(_f)
    SCENARIO_EPISODE_NAMES = {entry["scenario"]: entry["episode"] for entry in _EVAL_MANIFEST}
    SCENARIOS = list(SCENARIO_EPISODE_NAMES.keys())
except (OSError, json.JSONDecodeError, KeyError):
    # Manifest not present in this environment; eval tasks will require --scenarios.
    SCENARIO_EPISODE_NAMES = {}
    SCENARIOS = []

S3_BUCKET = "qbranch-gensim-recordings"
AWS_PROFILE = "sso-agent-sandbox-account-admin"

# Components included in ablation / combination search.
# passthrough is intentionally excluded: it is designed for TP scoring (eval_tp),
# not for Gaussian F1 eval (eval_scenarios / eval_combinations). This study
# evaluates scorer-produced correlation periods only.
DETECTORS = ["bocpd", "cusum", "holt_residual", "rrcf", "scanmw", "scanwelch", "tukey_biweight"]
ABLATION_CORRELATORS = ["anomaly_scorer"]
SUPPORTED_CORRELATORS = ["anomaly_scorer", "cross_signal", "time_cluster"]

# Correlators always represented in generated configs. time_cluster defaults on
# in the testbench, so scorer-only trials must explicitly disable it.
CONFIGURED_CORRELATORS = ["anomaly_scorer", "time_cluster"]

# Log metrics extractors. Not part of the random ablation grid: eval_combinations
# always enables all of them unless force-disabled.
EXTRACTORS = [
    "log_metrics_extractor",
    "connection_error_extractor",
    "log_pattern_extractor",
]

# Fixed anchor subsets used by eval_component to anchor the evaluation at known
# reference configurations regardless of the random seed.
ANCHOR_COMBOS = [
    {"detectors": ["bocpd"], "correlators": ["anomaly_scorer"]},
    {"detectors": ["bocpd", "rrcf"], "correlators": ["anomaly_scorer"]},
]


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


# --- S3 parquet helpers ---


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

        # Find the latest successful entry matching this episode.
        # parquet_count < 50 indicates a failed or truncated run. Known failures are 0 or 2.
        # Post trace-removal, legitimate recordings on short scenarios produce ~60-90 parquets.
        matches = [
            entry for entry in lines if entry.get("episode") == episode_name and entry.get("parquet_count", 0) >= 50
        ]
        if not matches:
            return None

        # Walk newest-first, return the first zip that actually exists in S3.
        for entry in reversed(matches):
            zip_key = entry.get("zip")
            if not zip_key:
                continue
            check = ctx.run(
                f"aws-vault exec {AWS_PROFILE} -- aws s3api head-object --bucket {S3_BUCKET} --key {shlex.quote(zip_key)}",
                warn=True,
                hide=True,
            )
            if check is None or check.failed:
                print(color_message(f"  '{zip_key}' not found in S3, trying previous run...", Color.BLUE))
                continue
            print(
                color_message(
                    f"Resolved '{name}' from runs.jsonl: {zip_key} "
                    f"(image: {entry.get('image', '?')}, {entry.get('timestamp', '?')})",
                    Color.BLUE,
                )
            )
            return zip_key
        return None
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


# --- Summary printers ---


def print_eval_scenarios_summary(results: list, sigma: float) -> None:
    """Print the Observer Eval Summary table to stdout."""
    if not results:
        return

    print(color_message(f"\n{'=' * 60}", Color.GREEN))
    print(color_message("  Observer Eval Summary", Color.GREEN))
    print(color_message(f"{'=' * 60}\n", Color.GREEN))

    header = f"{'Scenario':<25}  {'F1':>6}  {'Precision':>9}  {'Recall':>6}  {'Alpha':>7}  {'Detections':>10}  {'Baseline FPs':>12}  {'Warmup (ign)':>12}  {'Post-onset (ign)':>17}"
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
            f"  {alpha_str:>7}  {r['num_predictions']:>10}  {r['num_baseline_fps']:>12}  {r['num_filtered_warmup']:>13}  {r['num_filtered_cascading']:>18}"
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


def print_eval_tp_summary(results: list) -> None:
    """Print the Observer TP Eval Summary table to stdout."""
    if not results:
        return

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

    # Per-scenario TP details
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


# --- Output-dir helper ---


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


# --- Combo helpers ---


def _full_stack_combo(force_disable: list | None = None, force_enable: list | None = None) -> dict:
    """All ablation components plus force-enabled supported components."""
    fd = set(force_disable or [])
    fe_cors = {c for c in (force_enable or []) if c in SUPPORTED_CORRELATORS and c not in fd}
    return {
        "detectors": sorted(d for d in DETECTORS if d not in fd),
        "correlators": sorted({c for c in ABLATION_CORRELATORS if c not in fd} | fe_cors),
    }


def _anchor_combos(force_disable: list | None = None, force_enable: list | None = None) -> list[dict]:
    """Fixed anchor subsets derived from ANCHOR_COMBOS after filtering force_disable."""
    fd = set(force_disable or [])
    fe_dets = sorted(d for d in (force_enable or []) if d in DETECTORS and d not in fd)
    fe_cors = sorted(c for c in (force_enable or []) if c in SUPPORTED_CORRELATORS and c not in fd)
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
    """Generate up to n distinct random component combinations.

    Each combination is guaranteed to contain at least 1 detector (from DETECTORS)
    and 1 correlator (from ABLATION_CORRELATORS).
    """
    force_enable = set(force_enable or [])
    force_disable = set(force_disable or [])

    det_pool = [d for d in DETECTORS if d not in force_disable]
    cor_pool = [c for c in ABLATION_CORRELATORS if c not in force_disable]
    forced_dets = sorted(d for d in force_enable if d in DETECTORS and d not in force_disable)
    forced_cors = sorted(c for c in force_enable if c in SUPPORTED_CORRELATORS and c not in force_disable)

    rng = random.Random(seed)
    combos = []
    seen: set = set(exclude_combo_keys or [])
    max_attempts = n * 100
    attempts = 0
    while len(combos) < n and attempts < max_attempts:
        attempts += 1

        free_det_pool = [d for d in det_pool if d not in force_enable]
        if forced_dets:
            extra_dets = sorted(rng.sample(free_det_pool, rng.randint(0, len(free_det_pool)))) if free_det_pool else []
        else:
            if not free_det_pool:
                break
            extra_dets = sorted(rng.sample(free_det_pool, rng.randint(1, len(free_det_pool))))
        dets = sorted(set(forced_dets + extra_dets))

        free_cor_pool = [c for c in cor_pool if c not in force_enable]
        if forced_cors:
            extra_cors = sorted(rng.sample(free_cor_pool, rng.randint(0, len(free_cor_pool)))) if free_cor_pool else []
        else:
            if not free_cor_pool:
                break
            extra_cors = sorted(rng.sample(free_cor_pool, rng.randint(1, len(free_cor_pool))))
        cors = sorted(set(forced_cors + extra_cors))

        key = (tuple(dets), tuple(cors))
        if key in seen:
            continue
        seen.add(key)
        combos.append({"detectors": dets, "correlators": cors})
    if n > 0 and attempts >= max_attempts:
        print(
            color_message(
                f"Warning: Only generated {len(combos)} unique combinations (max attempts={max_attempts})",
                Color.ORANGE,
            )
        )
    return combos


def _component_base_config(name: str, enabled: bool) -> dict:
    """Base JSON config for a component in eval-generated testbench params."""
    cfg: dict[str, object] = {"enabled": enabled}
    if enabled and name == "anomaly_scorer":
        # The scorer only contributes to Gaussian F1 when it emits Medium- or
        # High-severity episodes as anomaly_periods. Keep cooldown at zero so
        # those periods end on actual scorer de-escalation, not a delivery delay.
        cfg.update({"correlation_events": True, "cooldown_secs": 0})
    return cfg


def _combo_to_config(
    detectors: list,
    correlators: list,
    force_disable: list | None = None,
) -> dict:
    """Build a testbench JSON params config enabling exactly the listed detectors and correlators.

    All EXTRACTORS are enabled unless listed in force_disable.
    """
    force_disable_set = set(force_disable or [])
    enabled_set = set(detectors + correlators)
    components = {}
    for name in DETECTORS + CONFIGURED_CORRELATORS:
        components[name] = _component_base_config(name, name in enabled_set)
    for name in correlators:
        if name in SUPPORTED_CORRELATORS and name not in components:
            components[name] = _component_base_config(name, True)
    for name in EXTRACTORS:
        components[name] = _component_base_config(name, name not in force_disable_set)
    return {"components": components}


# --- Optuna helpers ---


def _sample_component_params(trial, component: str) -> dict:
    """Sample Optuna hyperparameters for a named component that supports parseJSON."""

    def sample_anomaly_scorer() -> dict:
        # A correlation episode begins at the selected Medium or High threshold.
        # Keep Low below High so the scorer's three-level state machine remains
        # well-formed while allowing Optuna to choose the emission boundary.
        low_threshold = trial.suggest_float("anomaly_scorer.low_threshold", 0.01, 0.2, log=True)
        high_threshold_gap = trial.suggest_float("anomaly_scorer.high_threshold_gap", 0.01, 0.3, log=True)
        high_threshold = min(0.45, low_threshold + high_threshold_gap)
        # The scorer applies HighThreshold * MarginPct to both downward
        # transitions. Keep the effective margin below both the Low boundary
        # and the Low-to-High gap so every severity transition remains reachable.
        max_margin = min(low_threshold, high_threshold - low_threshold)
        hysteresis_fraction = trial.suggest_float("anomaly_scorer.hysteresis_fraction", 0.05, 0.9)
        margin_pct = hysteresis_fraction * max_margin / high_threshold
        return {
            "correlation_events": True,
            "correlation_event_threshold": trial.suggest_categorical(
                "anomaly_scorer.correlation_event_threshold", ["medium", "high"]
            ),
            "cooldown_secs": 0,
            "alpha": trial.suggest_float("anomaly_scorer.alpha", 0.005, 0.08, log=True),
            "saturation_k": trial.suggest_float("anomaly_scorer.saturation_k", 2.0, 12.0),
            "window_secs": trial.suggest_int("anomaly_scorer.window_secs", 5, 60),
            "low_threshold": low_threshold,
            "high_threshold": high_threshold,
            "margin_pct": margin_pct,
        }

    def sample_holt_residual() -> dict:
        alpha = trial.suggest_float("holt_residual.alpha", 0.05, 0.5)
        beta_ratio = trial.suggest_float("holt_residual.beta_ratio", 0.05, 0.75)
        return {
            "alpha": alpha,
            "beta": alpha * beta_ratio,
            "residual_window": trial.suggest_int("holt_residual.residual_window", 20, 90),
            "z_threshold": trial.suggest_float("holt_residual.z_threshold", 2.5, 8.0),
            "confirm_m": trial.suggest_int("holt_residual.confirm_m", 1, 3),
            "min_deviation_mad": trial.suggest_float("holt_residual.min_deviation_mad", 1.5, 6.0),
            "refractory": trial.suggest_int("holt_residual.refractory", 5, 60),
        }

    def sample_tukey_biweight() -> dict:
        window_size = trial.suggest_int("tukey_biweight.window_size", 30, 120)
        return {
            "window_size": window_size,
            "min_points": window_size,
            "biweight_c": trial.suggest_float("tukey_biweight.biweight_c", 3.5, 6.0),
            "z_threshold": trial.suggest_float("tukey_biweight.z_threshold", 2.5, 10.0),
            "score_every": trial.suggest_int("tukey_biweight.score_every", 1, 6),
            "cooldown_points": trial.suggest_int("tukey_biweight.cooldown_points", 5, 60),
        }

    space = {
        "anomaly_scorer": sample_anomaly_scorer,
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
        "holt_residual": sample_holt_residual,
        "rrcf": lambda: {
            # "num_trees": trial.suggest_int("rrcf.num_trees", 20, 200),
            # "tree_size": trial.suggest_int("rrcf.tree_size", 64, 512),
            "shingle_size": trial.suggest_int("rrcf.shingle_size", 1, 16),
            "threshold_sigma": trial.suggest_float("rrcf.threshold_sigma", 0.5, 6.0),
        },
        "tukey_biweight": sample_tukey_biweight,
        "log_pattern_extractor": lambda: {
            # "disable_optimizations": trial.suggest_categorical(...),
            # "min_cluster_size_before_emit": trial.suggest_int(...),
            # "max_tokenized_string_length": trial.suggest_int(...),
            # "max_num_tokens": trial.suggest_int(...),
            # "parse_hex_dump": trial.suggest_categorical(...),
            "min_token_match_ratio": trial.suggest_float("log_pattern_extractor.min_token_match_ratio", 0.2, 0.95),
            # "cluster_time_to_live_sec": trial.suggest_int(...),
            # "garbage_collection_interval_sec": trial.suggest_int(...),
        },
    }
    fn = space.get(component)
    return fn() if fn else {}


def _build_optuna_config(
    trial,
    components: list,
    locked: set,
) -> dict:
    """Build a TestbenchParamsFile config dict for one Optuna trial."""
    active_set = set(components)
    result = {}

    for name in DETECTORS + CONFIGURED_CORRELATORS + EXTRACTORS:
        if name not in active_set:
            result[name] = _component_base_config(name, False)

    for name in components:
        params = _component_base_config(name, True)
        if name not in locked:
            params.update(_sample_component_params(trial, name))
        result[name] = params

    return {"components": result}


# --- Aggregation helpers ---


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
    """Index of the run with highest best_score among non-failed runs (first on ties)."""
    valid = [i for i, r in enumerate(runs) if not r.get("failed") and r.get("best_score") is not None]
    if not valid:
        return -1
    return max(valid, key=lambda i: runs[i]["best_score"])


def aggregate_eval_component_results(
    variant_results: dict,
    n_subsets: int,
) -> dict:
    """Aggregate per-subset and per-scenario comparison between without/with variants."""
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
            by_scenario[s] = {"without_f1": a, "with_f1": b, "delta_f1": delta}
        per_subset_scenario.append({"subset": variant_results["without"][si]["subset"], "by_scenario": by_scenario})

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


def _fmt_wall_dur(s: float) -> str:
    """Format a duration in seconds into a human-readable h/m/s string."""
    if s >= 3600:
        return f"{int(s // 3600)}h {int((s % 3600) // 60)}m {s % 60:.1f}s"
    if s >= 60:
        return f"{int(s // 60)}m {s % 60:.1f}s"
    return f"{s:.1f}s"


# --- Additional summary printers ---


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

    print(color_message(f"\n  score (max):      {max_score:.4f}", Color.GREEN))
    print(color_message(f"  avg_eval_score:   {avg_score:.4f}", Color.GREEN))
    print(color_message(f"  study:            {study_path}", Color.GREEN))
    print(color_message(f"  output_dir:       {output_dir}", Color.GREEN))


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
        print(color_message("  Next step: tune the full pipeline with the new component:", Color.BOLD))
        print(color_message(f"    dda inv anomalydetection.eval-pipeline --force-enable {component}", Color.BOLD))
        print(color_message("  Or tune only this component's hyperparameters:", Color.BOLD))
        print(color_message(f"    dda inv anomalydetection.eval-bayesian --only {component}", Color.BOLD))
    print(color_message(f"  Full eval wall time: {wall_str}", Color.GREEN))
    print(color_message(f"{'=' * 70}", Color.GREEN))
