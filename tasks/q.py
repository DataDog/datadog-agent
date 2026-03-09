import json
import os
import sys

from invoke import task

from tasks.libs.common.color import Color, color_message

SCENARIOS = ["213_pagerduty", "353_postmark", "food_delivery_redis"]
DETECTORS = ["cusum", "bocpd", "rrcf", "pelt", "mannwhitney", "cusum_adaptive", "edivisive", "correlation", "topk", "ensemble", "cusum_hardened"]
CORRELATOR_PASSTHROUGH = "passthrough"
CORRELATOR_TIMECLUSTER = "time_cluster"


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
def eval(ctx, scenario: str = "", scenarios_dir: str = "./comp/observer/scenarios", sigma: float = 30.0):
    """
    Runs the observer eval: builds binaries, replays scenarios headless, scores against ground truth.

    Output JSONs are saved to /tmp/observer-eval-<scenario>.json for inspection.

    Args:
        scenario: Run a single scenario (e.g. "213_pagerduty"). Default: all scenarios.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for scoring.
    """
    print(color_message("Building observer-testbench...", Color.BLUE))
    ctx.run("go build -o bin/observer-testbench ./cmd/observer-testbench", hide=True)
    print(color_message("Building observer-scorer...", Color.BLUE))
    ctx.run("go build -o bin/observer-scorer ./cmd/observer-scorer", hide=True)

    scenarios_to_run = [scenario] if scenario else SCENARIOS

    results = []
    for name in scenarios_to_run:
        parquet_dir = os.path.join(scenarios_dir, name, "parquet")
        if not os.path.isdir(parquet_dir):
            print(color_message(f"Skipping {name} — no parquet data at {parquet_dir}", Color.ORANGE))
            continue

        output_path = f"/tmp/observer-eval-{name}.json"
        print(color_message(f"\n{'='*60}", Color.BLUE))
        print(color_message(f"  {name}", Color.BLUE))
        print(color_message(f"{'='*60}", Color.BLUE))

        ctx.run(f"bin/observer-testbench --headless {name} --output {output_path} --scenarios-dir {scenarios_dir}")

        scorer_result = ctx.run(
            f"bin/observer-scorer --output {output_path} --scenarios-dir {scenarios_dir} --sigma {sigma} --json",
            hide=True,
        )

        score = json.loads(scorer_result.stdout.strip())
        results.append({"name": name, **score})

    # Print summary table
    if results:
        print(color_message(f"\n{'='*60}", Color.GREEN))
        print(color_message("  Observer Eval Summary", Color.GREEN))
        print(color_message(f"{'='*60}\n", Color.GREEN))

        # Header
        header = f"{'Scenario':<25}  {'F1':>6}  {'Precision':>9}  {'Recall':>6}  {'Scored':>6}  {'Baseline FPs':>12}  {'Warmup (excl)':>13}  {'Cascading (excl)':>16}"
        print(header)
        print("-" * len(header))

        for r in results:
            # Count baseline FPs from the output JSON
            baseline_fps = _count_baseline_fps(
                f"/tmp/observer-eval-{r['name']}.json",
                os.path.join(scenarios_dir, r['name'], 'metadata.json'),
                sigma,
            )

            print(
                f"{r['name']:<25}  {r['f1']:>6.4f}  {r['precision']:>9.4f}  {r['recall']:>6.4f}"
                f"  {r['num_predictions']:>6}  {baseline_fps:>12}  {r['num_filtered_warmup']:>13}  {r['num_filtered_cascading']:>16}"
            )

        print(f"\nOutput JSONs: /tmp/observer-eval-*.json (sigma={sigma}s)")


@task
def eval_detectors(
    ctx,
    scenario: str = "",
    detector: str = "",
    scenarios_dir: str = "./comp/observer/scenarios",
    sigma: float = 30.0,
):
    """
    Runs per-detector eval: each detector x each scenario x both correlators (passthrough + time_cluster).

    Produces a comparison matrix showing Level 1 (passthrough) and Level 2 (time_cluster) scores.

    Args:
        scenario: Run a single scenario. Default: all.
        detector: Run a single detector. Default: all.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for scoring.
    """
    print(color_message("Building observer-testbench...", Color.BLUE))
    ctx.run("go build -o bin/observer-testbench ./cmd/observer-testbench", hide=True)
    print(color_message("Building observer-scorer...", Color.BLUE))
    ctx.run("go build -o bin/observer-scorer ./cmd/observer-scorer", hide=True)

    scenarios_to_run = [scenario] if scenario else SCENARIOS
    detectors_to_run = [detector] if detector else DETECTORS

    # Validate scenarios have parquet data
    valid_scenarios = []
    for name in scenarios_to_run:
        parquet_dir = os.path.join(scenarios_dir, name, "parquet")
        if not os.path.isdir(parquet_dir):
            print(color_message(f"Skipping {name} — no parquet data at {parquet_dir}", Color.ORANGE))
            continue
        valid_scenarios.append(name)

    if not valid_scenarios:
        print(color_message("No scenarios with parquet data found.", Color.RED))
        sys.exit(1)

    results = []  # list of dicts: {scenario, detector, level, f1, precision, recall, scored, warmup, cascading}

    for scn in valid_scenarios:
        for det in detectors_to_run:
            for correlator, level_name in [
                (CORRELATOR_PASSTHROUGH, "L1"),
                (CORRELATOR_TIMECLUSTER, "L2"),
            ]:
                output_path = f"/tmp/observer-eval-{scn}-{det}-{correlator}.json"

                # Build enable/disable flags: enable only this detector + this correlator
                # Disable all other detectors and all other correlators
                all_detectors = set(DETECTORS)
                disable_others = (all_detectors - {det})
                # Enable the target detector + target correlator; disable everything else
                enable_list = f"{det},{correlator}"
                disable_list = ",".join(disable_others)
                # Also disable correlators we don't want
                other_correlators = {CORRELATOR_PASSTHROUGH, CORRELATOR_TIMECLUSTER} - {correlator}
                disable_list += "," + ",".join(other_correlators) if other_correlators else ""

                label = f"{scn} / {det} / {level_name}({correlator})"
                print(color_message(f"  Running {label}...", Color.BLUE))

                # Use --verbose for passthrough so anomaly Source is in the JSON
                verbose_flag = " --verbose" if level_name == "L1" else ""

                try:
                    ctx.run(
                        f"bin/observer-testbench --headless {scn}"
                        f" --output {output_path}"
                        f" --scenarios-dir {scenarios_dir}"
                        f" --enable {enable_list}"
                        f" --disable {disable_list}"
                        f"{verbose_flag}",
                        hide=True,
                    )
                except Exception as e:
                    print(color_message(f"    FAILED: {e}", Color.RED))
                    continue

                # Score: always do timestamp scoring; add --score-metrics for L1
                score_metrics_flag = " --score-metrics" if level_name == "L1" else ""
                try:
                    scorer_result = ctx.run(
                        f"bin/observer-scorer --output {output_path}"
                        f" --scenarios-dir {scenarios_dir} --sigma {sigma} --json"
                        f"{score_metrics_flag}",
                        hide=True,
                    )
                    score = json.loads(scorer_result.stdout.strip())
                except Exception as e:
                    print(color_message(f"    Scoring failed: {e}", Color.RED))
                    continue

                metrics = score.get("metrics") or {}
                results.append({
                    "scenario": scn,
                    "detector": det,
                    "level": level_name,
                    "correlator": correlator,
                    "f1": score.get("f1", 0),
                    "precision": score.get("precision", 0),
                    "recall": score.get("recall", 0),
                    "scored": score.get("num_predictions", 0),
                    "warmup": score.get("num_filtered_warmup", 0),
                    "cascading": score.get("num_filtered_cascading", 0),
                    "m_tp": metrics.get("tp_count", ""),
                    "m_fp": metrics.get("fp_count", ""),
                    "m_unk": metrics.get("unknown_count", ""),
                    "m_prec": metrics.get("metric_precision", ""),
                    "m_rec": metrics.get("metric_recall", ""),
                })

    # Print comparison matrix
    if not results:
        print(color_message("No results collected.", Color.RED))
        return

    print(color_message(f"\n{'='*90}", Color.GREEN))
    print(color_message("  Detector Eval Matrix", Color.GREEN))
    print(color_message(f"{'='*90}\n", Color.GREEN))

    header = (
        f"{'Scenario':<22} {'Detector':<10} {'Level':<5}"
        f" {'F1':>6} {'Prec':>6} {'Rec':>6} {'Scored':>6}"
        f" {'mTP':>5} {'mFP':>5} {'mUnk':>5} {'mPrec':>6} {'mRec':>6}"
    )
    print(header)
    print("-" * len(header))

    for r in results:
        # Metric columns: show values for L1, blank for L2
        m_tp = f"{r['m_tp']:>5}" if r['m_tp'] != "" else "    -"
        m_fp = f"{r['m_fp']:>5}" if r['m_fp'] != "" else "    -"
        m_unk = f"{r['m_unk']:>5}" if r['m_unk'] != "" else "    -"
        m_prec = f"{r['m_prec']:>6.4f}" if r['m_prec'] != "" else "     -"
        m_rec = f"{r['m_rec']:>6.4f}" if r['m_rec'] != "" else "     -"

        print(
            f"{r['scenario']:<22} {r['detector']:<10} {r['level']:<5}"
            f" {r['f1']:>6.4f} {r['precision']:>6.4f} {r['recall']:>6.4f}"
            f" {r['scored']:>6}"
            f" {m_tp} {m_fp} {m_unk} {m_prec} {m_rec}"
        )

    print(f"\nOutput JSONs: /tmp/observer-eval-*-*.json (sigma={sigma}s)")

    # Write results as JSON for programmatic consumption
    results_path = "/tmp/observer-eval-detectors-matrix.json"
    with open(results_path, "w") as f:
        json.dump(results, f, indent=2)
    print(f"Full results: {results_path}")


def _count_baseline_fps(output_path, metadata_path, sigma):
    """Count scored predictions that fired before ground truth onset."""
    try:
        with open(output_path) as f:
            output = json.load(f)
        with open(metadata_path) as f:
            meta = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return 0

    from datetime import datetime

    gt_str = meta.get("disruption", {}).get("start", "")
    bl_str = meta.get("baseline", {}).get("start", "")
    if not gt_str:
        return 0

    gt_ts = int(datetime.fromisoformat(gt_str.replace("Z", "+00:00")).timestamp())
    bl_ts = int(datetime.fromisoformat(bl_str.replace("Z", "+00:00")).timestamp()) if bl_str else 0

    count = 0
    cutoff = gt_ts + 2 * sigma
    for p in output.get("anomaly_periods", []):
        ts = p["period_start"]
        if bl_ts and ts < bl_ts:
            continue
        if ts > cutoff:
            continue
        if ts < gt_ts:
            count += 1
    return count


@task
def launch_testbench(ctx, scenarios_dir: str = "./comp/observer/scenarios", build: bool = False):
    """
    Will launch both the observer-testbench backend and UI.

    Args:
        scenarios_dir: The directory containing the scenarios to load.
        build: Whether to build the observer-testbench binary.
    """
    if build:
        print("Building observer-testbench...")
        build_testbench(ctx)

    print("Launching observer-testbench backend and UI, use ^C to exit")
    ctx.run(
        f"bin/observer-testbench --scenarios-dir {scenarios_dir} & ( cd cmd/observer-testbench/ui && npm install && npm run dev ) &"
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
