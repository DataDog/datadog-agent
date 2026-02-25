#!/usr/bin/env python3
"""
Bayesian optimization harness for observer parameter tuning.

Uses Optuna for optimization and existing analyze_with_llm.py + evaluate_diagnosis.py
for fitness evaluation (GPT-5.2 diagnosis → GPT-5.2 scoring).

Usage:
    pip install optuna
    export OPENAI_API_KEY="sk-..."

    # Tune CUSUM + TimeCluster on memory leak
    python3 comp/observer/tuning/harness.py \
        --parquet memory-leak-export.parquet \
        --scenario memory-leak \
        --detector cusum \
        --correlator timecluster \
        --trials 30

    # Tune LightESD + GraphSketch on network latency
    python3 comp/observer/tuning/harness.py \
        --parquet demo-network-latency.parquet \
        --scenario network-latency \
        --detector lightesd \
        --correlator graphsketch \
        --trials 30
"""

import argparse
import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

try:
    import optuna
except ImportError:
    print("Error: optuna not installed. Run: pip install optuna")
    sys.exit(1)

# Default results directory (permanent location in repo)
RESULTS_DIR = Path(__file__).parent / "results"


class TuningHarness:
    """Bayesian optimization harness for observer parameter tuning."""

    def __init__(
        self,
        parquet: str,
        scenario: str,
        detector: str,
        correlator: str,
        model: str = "gpt-5.2-2025-12-11",
        verbose: bool = False,
        log_file: str = None,
        output_dir: str = None,
        timescale: float = 0.25,
        runs_per_trial: int = 1,
        param_ranges: dict = None,
        dedup: bool = False,
    ):
        self.parquet = parquet
        self.scenario = scenario
        self.detector = detector
        self.correlator = correlator
        self.model = model
        self.verbose = verbose
        self.trial_count = 0
        self.log_file = log_file  # JSONL file for incremental logging
        self.output_dir = output_dir  # Directory to save LLM outputs
        self.timescale = timescale  # Replay speed multiplier
        self.runs_per_trial = runs_per_trial  # Number of eval runs to average per param set
        self.dedup = dedup  # Enable anomaly deduplication
        # Custom parameter ranges (defaults if not provided)
        self.param_ranges = param_ranges or {
            "cusum_baseline": (0.1, 0.5),
            "cusum_slack": (0.1, 2.0),
            "cusum_threshold": (2.0, 8.0),
            "tc_slack": (1, 30),
        }

        # Paths to existing scripts (relative to repo root)
        self.repo_root = Path(__file__).parent.parent.parent.parent
        self.analyze_script = self.repo_root / "analyze_with_llm.py"
        self.evaluate_script = self.repo_root / "evaluate_diagnosis.py"
        self.demo_binary = self.repo_root / "bin" / "observer-demo-v2"

        # Validate paths
        if not self.analyze_script.exists():
            print(f"Warning: analyze_with_llm.py not found at {self.analyze_script}")
        if not self.evaluate_script.exists():
            print(f"Warning: evaluate_diagnosis.py not found at {self.evaluate_script}")

    def objective(self, trial: optuna.Trial) -> float:
        """
        Optimization objective: maximize GPT diagnosis score.

        Returns: score from 0-100 (higher is better)
        """
        self.trial_count += 1
        print(f"\n{'='*60}")
        print(f"Trial {self.trial_count}: {self.detector} + {self.correlator}")
        print(f"{'='*60}")

        # 1. Sample parameters based on detector/correlator choice
        params = self.sample_params(trial)
        if self.verbose:
            print(f"Parameters: {json.dumps(params, indent=2)}")

        # 2. Run replay with params (once per trial - deterministic)
        output_json = self.run_replay(params)
        if output_json is None:
            print("  ERROR: Replay failed")
            return 0.0

        # 3-4. Run diagnosis + grading N times and average
        scores = []
        all_diagnoses = []
        all_gradings = []

        for run in range(self.runs_per_trial):
            if self.runs_per_trial > 1:
                print(f"  Run {run + 1}/{self.runs_per_trial}...", end=" ", flush=True)

            # GPT diagnosis
            diagnosis = self.diagnose(output_json)
            if diagnosis is None:
                print("diagnosis failed")
                continue

            # GPT score
            score, grading_output = self.grade(diagnosis)
            scores.append(score)
            all_diagnoses.append(diagnosis)
            all_gradings.append(grading_output)

            if self.runs_per_trial > 1:
                print(f"score={score}")

        if not scores:
            print("  ERROR: All evaluation runs failed")
            return 0.0

        # Average score across runs
        avg_score = sum(scores) / len(scores)
        if self.runs_per_trial > 1:
            print(f"  Average Score: {avg_score:.1f}/100 (runs: {scores})")
        else:
            print(f"  Score: {avg_score}/100")

        # 5. Save LLM outputs for verification (save all runs)
        if self.output_dir:
            trial_dir = os.path.join(self.output_dir, f"trial_{self.trial_count:03d}")
            os.makedirs(trial_dir, exist_ok=True)

            # Save each run's outputs
            for run_idx, (diag, grad, sc) in enumerate(zip(all_diagnoses, all_gradings, scores, strict=False)):
                run_suffix = f"_run{run_idx + 1}" if self.runs_per_trial > 1 else ""

                with open(os.path.join(trial_dir, f"diagnosis{run_suffix}.txt"), "w") as f:
                    f.write(diag)

                with open(os.path.join(trial_dir, f"grading{run_suffix}.txt"), "w") as f:
                    f.write(grad)

            # Save params with all scores
            with open(os.path.join(trial_dir, "params.json"), "w") as f:
                json.dump(
                    {"params": params, "avg_score": avg_score, "scores": scores, "runs": self.runs_per_trial},
                    f,
                    indent=2,
                )

            # Copy observer output JSON
            import shutil

            shutil.copy(output_json, os.path.join(trial_dir, "observer_output.json"))

            print(f"  Outputs saved to: {trial_dir}")

        score = avg_score  # Use average for optimization

        # 6. Log trial result (append to JSONL for resumability)
        if self.log_file:
            trial_result = {
                "trial": self.trial_count,
                "params": params,
                "score": score,
                "detector": self.detector,
                "correlator": self.correlator,
                "scenario": self.scenario,
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
            }
            with open(self.log_file, "a") as f:
                f.write(json.dumps(trial_result) + "\n")

        return score

    def sample_params(self, trial: optuna.Trial) -> dict:
        """Sample parameters based on detector and correlator selection."""
        params = {}

        # Detector parameters (using custom ranges)
        if self.detector == "cusum":
            r = self.param_ranges
            params["cusum-baseline-fraction"] = trial.suggest_float(
                "cusum_baseline_fraction", r["cusum_baseline"][0], r["cusum_baseline"][1]
            )
            params["cusum-slack-factor"] = trial.suggest_float(
                "cusum_slack_factor", r["cusum_slack"][0], r["cusum_slack"][1]
            )
            params["cusum-threshold-factor"] = trial.suggest_float(
                "cusum_threshold_factor", r["cusum_threshold"][0], r["cusum_threshold"][1]
            )

        elif self.detector == "lightesd":
            params["lightesd-min-window-size"] = trial.suggest_int("lightesd_min_window", 20, 200)
            params["lightesd-alpha"] = trial.suggest_float("lightesd_alpha", 0.001, 0.1, log=True)
            params["lightesd-trend-window-fraction"] = trial.suggest_float("lightesd_trend_frac", 0.05, 0.3)
            params["lightesd-periodicity-significance"] = trial.suggest_float(
                "lightesd_period_sig", 0.001, 0.1, log=True
            )
            params["lightesd-max-periods"] = trial.suggest_int("lightesd_max_periods", 1, 4)

        # Correlator parameters
        if self.correlator == "graphsketch":
            params["graphsketch-cooccurrence-window"] = trial.suggest_int("gs_cooccurrence", 1, 60)
            params["graphsketch-decay-factor"] = trial.suggest_float("gs_decay", 0.5, 0.99)
            params["graphsketch-min-correlation"] = trial.suggest_float("gs_min_corr", 0.5, 10.0)
            params["graphsketch-edge-limit"] = trial.suggest_int("gs_edge_limit", 50, 1000)

        elif self.correlator == "timecluster":
            r = self.param_ranges
            params["timecluster-slack-seconds"] = trial.suggest_int(
                "tc_slack", int(r["tc_slack"][0]), int(r["tc_slack"][1])
            )

        elif self.correlator == "leadlag":
            params["leadlag-max-lag"] = trial.suggest_int("ll_max_lag", 10, 60)
            params["leadlag-min-obs"] = trial.suggest_int("ll_min_obs", 2, 10)
            params["leadlag-confidence"] = trial.suggest_float("ll_confidence", 0.4, 0.9)

        elif self.correlator == "surprise":
            params["surprise-window"] = trial.suggest_int("sp_window", 5, 30)
            params["surprise-min-lift"] = trial.suggest_float("sp_min_lift", 1.2, 5.0)
            params["surprise-min-support"] = trial.suggest_int("sp_min_support", 2, 10)

        return params

    def run_replay(self, params: dict) -> str:
        """Run observer-demo-v2 with given parameters. Returns output JSON path."""
        output_path = tempfile.mktemp(suffix=".json", prefix="observer_")

        # Build command
        cmd = [
            str(self.demo_binary),
            "--parquet",
            self.parquet,
            "--output",
            output_path,
            "--all",  # Process all data
            "--timescale",
            str(self.timescale),
        ]

        # Add detector flag
        if self.detector == "cusum":
            cmd.append("--cusum")
        elif self.detector == "lightesd":
            cmd.append("--lightesd")

        # Add correlator flag
        if self.correlator == "graphsketch":
            cmd.extend(["--graphsketch-correlator", "--time-cluster=false"])
        elif self.correlator == "timecluster":
            cmd.append("--time-cluster")
        elif self.correlator == "leadlag":
            cmd.extend(["--lead-lag", "--time-cluster=false"])
        elif self.correlator == "surprise":
            cmd.extend(["--surprise", "--time-cluster=false"])

        # Add dedup if enabled
        if self.dedup:
            cmd.append("--dedup")

        # Add tuning parameters
        for key, value in params.items():
            cmd.append(f"--{key}={value}")

        if self.verbose:
            print(f"  Command: {' '.join(cmd)}")

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=300,  # 5 minute timeout
                cwd=str(self.repo_root),
            )
            if result.returncode != 0:
                print(f"  Replay failed: {result.stderr[:500]}")
                return None
        except subprocess.TimeoutExpired:
            print("  Replay timed out")
            return None
        except Exception as e:
            print(f"  Replay error: {e}")
            return None

        if not os.path.exists(output_path):
            print("  No output file generated")
            print(f"  CMD: {' '.join(cmd)}")
            print(f"  stdout: {result.stdout[:300]}")
            print(f"  stderr: {result.stderr[:300]}")
            return None

        return output_path

    def diagnose(self, json_path: str) -> str:
        """Call analyze_with_llm.py to diagnose. Returns diagnosis text."""
        # Build context flags based on detector/correlator
        context_flags = []
        if self.detector == "cusum":
            context_flags.append("--cusum")
        elif self.detector == "lightesd":
            context_flags.append("--lightesd")

        if self.correlator == "graphsketch":
            context_flags.append("--graphsketch")
        elif self.correlator == "timecluster":
            context_flags.append("--timecluster")

        cmd = [
            sys.executable,
            str(self.analyze_script),
            json_path,
            "--model",
            self.model,
        ] + context_flags

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=120,  # 2 minute timeout
                cwd=str(self.repo_root),
            )
            if result.returncode != 0:
                print(f"  Diagnosis failed: {result.stderr[:500]}")
                return None
            return result.stdout
        except subprocess.TimeoutExpired:
            print("  Diagnosis timed out")
            return None
        except Exception as e:
            print(f"  Diagnosis error: {e}")
            return None

    def grade(self, diagnosis: str) -> tuple:
        """Call evaluate_diagnosis.py to grade. Returns (score, grading_output)."""
        # Write diagnosis to temp file
        with tempfile.NamedTemporaryFile(mode='w', suffix='.txt', delete=False) as f:
            f.write(diagnosis)
            diagnosis_path = f.name

        try:
            cmd = [
                sys.executable,
                str(self.evaluate_script),
                diagnosis_path,
                "--scenario",
                self.scenario,
            ]

            result = subprocess.run(cmd, capture_output=True, text=True, timeout=120, cwd=str(self.repo_root))

            grading_output = result.stdout

            if result.returncode != 0:
                print(f"  Grading failed: {result.stderr[:500]}")
                return 0.0, grading_output

            # Parse score from output
            # New format: "6. **Score**: 45/100" or "6. **Score**: [49/100]"
            import re

            for line in result.stdout.split("\n"):
                # Match pattern like "6. **Score**: 75" or "**Score**: 75"
                if re.match(r'^\d+\.\s*\*\*Score\*\*:', line) or line.strip().startswith("**Score**:"):
                    try:
                        # Extract everything after the last colon
                        score_str = line.split(":")[-1].strip()
                        # Remove markdown, brackets, etc.
                        score_str = score_str.replace("**", "").replace("[", "").replace("]", "").strip()
                        # Extract first number (handles "45/100", "45", etc.)
                        match = re.search(r'(\d+(?:\.\d+)?)', score_str)
                        if match:
                            return float(match.group(1)), grading_output
                    except (ValueError, IndexError):
                        continue

            print("  Could not parse score from output")
            if self.verbose:
                print(f"  Output: {result.stdout[:500]}")
            return 0.0, grading_output

        finally:
            os.unlink(diagnosis_path)


def run_evaluate_mode(args):
    """Run evaluation with fixed params (no Optuna tuning)."""
    # Load params from file or use empty dict for defaults
    if args.params_file:
        params = load_params_from_file(args.params_file)
        param_source = os.path.basename(args.params_file)
    else:
        params = {}  # Empty = use binary defaults
        param_source = "defaults"

    print("=== EVALUATE MODE ===")
    print(f"Scenario: {args.scenario}")
    print(f"Detector: {args.detector}")
    print(f"Correlator: {args.correlator}")
    print(f"Params: {param_source}")
    print(f"Runs: {args.runs_per_trial}")
    print()

    # Create harness
    harness = TuningHarness(
        parquet=args.parquet,
        scenario=args.scenario,
        detector=args.detector,
        correlator=args.correlator,
        model=args.model,
        verbose=args.verbose,
        timescale=args.timescale,
        dedup=args.dedup,
    )

    # Run replay once
    print("Running observer...", end=" ", flush=True)
    output_json = harness.run_replay(params)
    if output_json is None:
        print("FAILED")
        sys.exit(1)
    print("done")

    # Run diagnosis + grading N times
    scores = []
    for run in range(args.runs_per_trial):
        print(f"  Run {run + 1}/{args.runs_per_trial}...", end=" ", flush=True)

        diagnosis = harness.diagnose(output_json)
        if diagnosis is None:
            print("diagnosis failed")
            continue

        score, _ = harness.grade(diagnosis)
        scores.append(score)
        print(f"score={score}")

    if not scores:
        print("ERROR: All runs failed")
        sys.exit(1)

    avg_score = sum(scores) / len(scores)
    print(f"\nResult: {avg_score:.1f} avg (scores: {scores})")

    # Append to CSV (use results directory by default)
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    csv_file = args.results_csv or str(RESULTS_DIR / "evaluation_results.csv")
    write_header = not os.path.exists(csv_file)
    with open(csv_file, "a") as f:
        if write_header:
            f.write("scenario,detector,correlator,param_source,avg_score,scores,runs\n")
        f.write(
            f"{args.scenario},{args.detector},{args.correlator},{param_source},{avg_score:.1f},\"{scores}\",{args.runs_per_trial}\n"
        )
    print(f"Appended to: {csv_file}")


def load_params_from_file(params_file: str) -> dict:
    """Load params from best_params_*.json and convert to CLI flag format."""
    with open(params_file) as f:
        data = json.load(f)

    name_map = {
        "cusum_baseline_fraction": "cusum-baseline-fraction",
        "cusum_slack_factor": "cusum-slack-factor",
        "cusum_threshold_factor": "cusum-threshold-factor",
        "lightesd_min_window": "lightesd-min-window-size",
        "lightesd_alpha": "lightesd-alpha",
        "lightesd_trend_frac": "lightesd-trend-window-fraction",
        "lightesd_period_sig": "lightesd-periodicity-significance",
        "lightesd_max_periods": "lightesd-max-periods",
        "gs_cooccurrence": "graphsketch-cooccurrence-window",
        "gs_decay": "graphsketch-decay-factor",
        "gs_min_corr": "graphsketch-min-correlation",
        "gs_edge_limit": "graphsketch-edge-limit",
        "tc_slack": "timecluster-slack-seconds",
    }

    params = {}
    for key, value in data["best_params"].items():
        if key in name_map:
            params[name_map[key]] = value
    return params


def main():
    parser = argparse.ArgumentParser(description="Bayesian optimization for observer parameter tuning")
    parser.add_argument("--parquet", required=True, help="Path to parquet file")
    parser.add_argument(
        "--scenario",
        required=True,
        choices=[
            "memory-leak",
            "network-latency",
            "crash-loop",
            "connection-timeout",
            "memory-exhaustion",
            "traffic-spike",
        ],
        help="Scenario name for grading",
    )
    parser.add_argument("--detector", required=True, choices=["cusum", "lightesd"], help="Anomaly detector to tune")
    parser.add_argument(
        "--correlator",
        required=True,
        choices=["graphsketch", "timecluster", "leadlag", "surprise"],
        help="Correlator to tune",
    )
    parser.add_argument("--trials", type=int, default=30, help="Number of optimization trials (default: 30)")
    parser.add_argument("--model", default="gpt-5.2-2025-12-11", help="OpenAI model for diagnosis/grading")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    parser.add_argument("--study-name", default=None, help="Optuna study name (for resuming)")
    parser.add_argument("--resume", action="store_true", help="Resume from existing log file")
    parser.add_argument("--output-dir", default=None, help="Directory to save LLM outputs for verification")
    parser.add_argument(
        "--timescale", type=float, default=0.25, help="Replay speed multiplier (default: 0.25 = 4x faster)"
    )
    parser.add_argument(
        "--runs-per-trial", type=int, default=1, help="Number of eval runs per param set to average (default: 1)"
    )
    # Custom parameter ranges (format: "min,max")
    parser.add_argument(
        "--cusum-baseline-range", default="0.1,0.5", help="CUSUM baseline_fraction range (default: 0.1,0.5)"
    )
    parser.add_argument("--cusum-slack-range", default="0.1,2.0", help="CUSUM slack_factor range (default: 0.1,2.0)")
    parser.add_argument(
        "--cusum-threshold-range", default="2.0,8.0", help="CUSUM threshold_factor range (default: 2.0,8.0)"
    )
    parser.add_argument("--tc-slack-range", default="1,30", help="TimeCluster slack_seconds range (default: 1,30)")
    parser.add_argument("--dedup", action="store_true", help="Enable anomaly deduplication before correlation")
    # Evaluate mode (fixed params, no tuning)
    parser.add_argument("--evaluate", action="store_true", help="Run with fixed params (no tuning)")
    parser.add_argument(
        "--params-file", default=None, help="JSON file with params (from best_params_*.json). Omit for defaults."
    )
    parser.add_argument(
        "--results-csv", default=None, help="CSV file to append results (default: results/evaluation_results.csv)"
    )
    args = parser.parse_args()

    # Check for API key
    if not os.environ.get("OPENAI_API_KEY"):
        print("Error: OPENAI_API_KEY environment variable not set")
        sys.exit(1)

    # === EVALUATE MODE ===
    if args.evaluate:
        run_evaluate_mode(args)
        sys.exit(0)

    # Ensure results directory exists
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)

    # Create study name and log file path (in results directory)
    study_name = args.study_name or f"{args.scenario}_{args.detector}_{args.correlator}"
    log_file = str(RESULTS_DIR / f"tuning_log_{study_name}.jsonl")

    # Set up output directory for LLM outputs (in results directory)
    output_dir = args.output_dir or str(RESULTS_DIR / f"tuning_outputs_{study_name}")
    os.makedirs(output_dir, exist_ok=True)

    # Check for existing progress
    completed_trials = 0
    if args.resume and os.path.exists(log_file):
        with open(log_file) as f:
            completed_trials = sum(1 for _ in f)
        print(f"Resuming: found {completed_trials} completed trials in {log_file}")

    # Parse custom parameter ranges
    def parse_range(s):
        parts = s.split(",")
        return (float(parts[0]), float(parts[1]))

    param_ranges = {
        "cusum_baseline": parse_range(args.cusum_baseline_range),
        "cusum_slack": parse_range(args.cusum_slack_range),
        "cusum_threshold": parse_range(args.cusum_threshold_range),
        "tc_slack": parse_range(args.tc_slack_range),
    }

    # Create harness with log file and output dir
    harness = TuningHarness(
        parquet=args.parquet,
        scenario=args.scenario,
        detector=args.detector,
        correlator=args.correlator,
        model=args.model,
        verbose=args.verbose,
        log_file=log_file,
        output_dir=output_dir,
        timescale=args.timescale,
        runs_per_trial=args.runs_per_trial,
        param_ranges=param_ranges,
        dedup=args.dedup,
    )
    harness.trial_count = completed_trials  # Continue numbering

    print(f"Starting tuning: {study_name}")
    print(f"  Parquet: {args.parquet}")
    print(f"  Scenario: {args.scenario}")
    print(f"  Detector: {args.detector}")
    print(f"  Correlator: {args.correlator}")
    print(f"  Dedup: {'Enabled' if args.dedup else 'Disabled'}")
    print(f"  Trials: {args.trials} (remaining: {args.trials - completed_trials})")
    print(f"  Model: {args.model}")
    print(f"  Log file: {log_file}")
    print(f"  LLM outputs: {output_dir}/")
    print(f"  Timescale: {args.timescale} ({1/args.timescale:.0f}x slower than realtime)")
    if args.runs_per_trial > 1:
        print(f"  Runs per trial: {args.runs_per_trial} (scores will be averaged)")
    if args.detector == "cusum":
        print(
            f"  CUSUM ranges: baseline={param_ranges['cusum_baseline']}, slack={param_ranges['cusum_slack']}, threshold={param_ranges['cusum_threshold']}"
        )
    if args.correlator == "timecluster":
        print(f"  TimeCluster range: tc_slack={param_ranges['tc_slack']}")

    # Calculate remaining trials
    remaining_trials = max(0, args.trials - completed_trials)
    if remaining_trials == 0:
        print("All trials already completed. Use --trials to run more.")
        sys.exit(0)

    # Create Optuna study
    study = optuna.create_study(
        study_name=study_name,
        direction="maximize",
        sampler=optuna.samplers.TPESampler(seed=42),
    )

    # Run optimization (only remaining trials)
    start_time = time.time()
    study.optimize(harness.objective, n_trials=remaining_trials)
    elapsed = time.time() - start_time

    # Find overall best (including previous runs from log file)
    best_score = study.best_value
    best_params = study.best_params
    total_trials = completed_trials + remaining_trials

    if os.path.exists(log_file):
        with open(log_file) as f:
            for line in f:
                trial_data = json.loads(line)
                if trial_data["score"] > best_score:
                    best_score = trial_data["score"]
                    best_params = trial_data["params"]

    # Print results
    print("\n" + "=" * 60)
    print("TUNING COMPLETE")
    print("=" * 60)
    print(f"Best score (overall): {best_score:.1f}/100")
    print("Best params:")
    for key, value in best_params.items():
        print(f"  {key}: {value}")
    print(f"Total trials: {total_trials}")
    print(f"This run: {remaining_trials} trials in {elapsed/60:.1f} minutes")

    # Save results to CSV (this run only) - in results directory
    output_csv = str(RESULTS_DIR / f"tuning_{study_name}.csv")
    study.trials_dataframe().to_csv(output_csv, index=False)
    print(f"\nThis run's results: {output_csv}")

    # Save best params to JSON (overall best) - in results directory
    output_json = str(RESULTS_DIR / f"best_params_{study_name}.json")
    with open(output_json, 'w') as f:
        json.dump(
            {
                "best_score": best_score,
                "best_params": best_params,
                "scenario": args.scenario,
                "detector": args.detector,
                "correlator": args.correlator,
                "total_trials": total_trials,
            },
            f,
            indent=2,
        )
    print(f"Best params saved to: {output_json}")
    print(f"Full trial log: {log_file}")


if __name__ == "__main__":
    main()
