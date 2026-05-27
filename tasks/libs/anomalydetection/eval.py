"""Evaluation helpers for anomaly detection dev tasks.

Contains non-@task symbols: constants, S3 helpers, StepLogger, and summary
printers used by eval_scenarios / eval_tp in tasks/anomalydetection.py.

Note: SCENARIOS and SCENARIO_EPISODE_NAMES are loaded from
q_branch/gensim-eval-scenarios.json at import time. That file is not
committed to this repo — it is expected to be present in developer environments
where the q_branch/ directory has been checked out separately, or produced by
dda inv anomalydetection.download-scenarios (a future Wave-4 task).
"""

import json
import os
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
    SCENARIO_EPISODE_NAMES = {entry["short"]: entry["episode"] for entry in _EVAL_MANIFEST}
    SCENARIOS = list(SCENARIO_EPISODE_NAMES.keys())
except (OSError, json.JSONDecodeError, KeyError):
    # Manifest not present in this environment; eval tasks will require --scenarios.
    SCENARIO_EPISODE_NAMES = {}
    SCENARIOS = []

S3_BUCKET = "qbranch-gensim-recordings"
AWS_PROFILE = "sso-agent-sandbox-account-admin"


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
