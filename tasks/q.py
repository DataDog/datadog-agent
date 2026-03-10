import glob
import json
import os
import shlex
import shutil
import sys
import tempfile
import zipfile

from invoke import task

from tasks.libs.common.color import Color, color_message

SCENARIOS = ["213_pagerduty", "353_postmark", "food_delivery_redis"]
DETECTORS = ["cusum", "bocpd", "rrcf", "mannwhitney", "correlation", "topk"]
CORRELATOR_PASSTHROUGH = "passthrough"
CORRELATOR_TIMECLUSTER = "time_cluster"

# S3 zip key for each scenario. Update when re-recording.
SCENARIO_ZIPS = {
    "213_pagerduty": "gensim-results-213_PagerDuty_June_2014_Outage-20260303-1309-78229d.zip",
    "353_postmark": "gensim-results-353_postmark_upstream_cloud_provider_outage-20260303-1333-ad0bba.zip",
    "food_delivery_redis": "gensim-results-food-delivery-redis-cpu-saturation-20260303-1314-5f7194.zip",
}

S3_BUCKET = "qbranch-gensim-recordings"
AWS_PROFILE = "sso-agent-sandbox-account-admin"


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
def eval_scenarios(ctx, scenario: str = "", scenarios_dir: str = "./comp/observer/scenarios", sigma: float = 30.0):
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
        scenario_root = os.path.join(scenarios_dir, name)
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            _ensure_parquets(ctx, name, parquet_dir)
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            # Fallback: check for *.parquet files directly in scenario root
            if not glob.glob(os.path.join(scenario_root, "*.parquet")):
                print(color_message(f"Skipping {name} — no parquet data at {parquet_dir}", Color.ORANGE))
                continue

        output_path = f"/tmp/observer-eval-{name}.json"
        print(color_message(f"\n{'='*60}", Color.BLUE))
        print(color_message(f"  {name}", Color.BLUE))
        print(color_message(f"{'='*60}", Color.BLUE))

        ctx.run(
            f"bin/observer-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)}"
        )

        if not os.path.isfile(output_path):
            print(color_message(f"Testbench did not produce output at {output_path}", Color.RED))
            continue
        try:
            with open(output_path) as f:
                json.load(f)
        except (json.JSONDecodeError, OSError) as e:
            print(color_message(f"Testbench output at {output_path} is not valid JSON: {e}", Color.RED))
            continue

        scorer_result = ctx.run(
            f"bin/observer-scorer --input {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)} --sigma {sigma} --json",
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
            print(
                f"{r['name']:<25}  {r['f1']:>6.4f}  {r['precision']:>9.4f}  {r['recall']:>6.4f}"
                f"  {r['num_predictions']:>6}  {r['num_baseline_fps']:>12}  {r['num_filtered_warmup']:>13}  {r['num_filtered_cascading']:>16}"
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
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            _ensure_parquets(ctx, name, parquet_dir)
        if not os.path.isdir(parquet_dir) or not os.listdir(parquet_dir):
            scenario_root = os.path.join(scenarios_dir, name)
            if not glob.glob(os.path.join(scenario_root, "*.parquet")):
                print(color_message(f"Skipping {name} — no parquet data at {parquet_dir}", Color.ORANGE))
                continue
        valid_scenarios.append(name)

    if not valid_scenarios:
        print(color_message("No scenarios with parquet data found.", Color.RED))
        sys.exit(1)

    results = []

    for scn in valid_scenarios:
        for det in detectors_to_run:
            for correlator, level_name in [
                (CORRELATOR_PASSTHROUGH, "L1"),
                (CORRELATOR_TIMECLUSTER, "L2"),
            ]:
                output_path = f"/tmp/observer-eval-{scn}-{det}-{correlator}.json"

                all_detectors = set(DETECTORS)
                disable_others = (all_detectors - {det})
                enable_list = f"{det},{correlator}"
                disable_list = ",".join(disable_others)
                other_correlators = {CORRELATOR_PASSTHROUGH, CORRELATOR_TIMECLUSTER} - {correlator}
                disable_list += "," + ",".join(other_correlators) if other_correlators else ""

                label = f"{scn} / {det} / {level_name}({correlator})"
                print(color_message(f"  Running {label}...", Color.BLUE))

                # Use --verbose for passthrough so anomaly Source is in the JSON
                verbose_flag = " --verbose" if level_name == "L1" else ""

                try:
                    ctx.run(
                        f"bin/observer-testbench --headless {shlex.quote(scn)}"
                        f" --output {shlex.quote(output_path)}"
                        f" --scenarios-dir {shlex.quote(scenarios_dir)}"
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
                        f"bin/observer-scorer --input {shlex.quote(output_path)}"
                        f" --scenarios-dir {shlex.quote(scenarios_dir)} --sigma {sigma} --json"
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
                    "detections": metrics.get("detections", []),
                    "unknown_metric_count": metrics.get("unknown_metric_count", 0),
                    "unknown_detection_count": metrics.get("unknown_detection_count", 0),
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

    # Per-metric detection detail for L1 results
    l1_results = [r for r in results if r["level"] == "L1" and r.get("detections")]
    if l1_results:
        print(color_message(f"\n{'='*90}", Color.GREEN))
        print(color_message("  Per-Metric Detection Detail (L1 only)", Color.GREEN))
        print(color_message(f"{'='*90}", Color.GREEN))

        for r in l1_results:
            print(f"\n  {r['scenario']} / {r['detector']}:")
            tp_dets = [d for d in r["detections"] if d["classification"] == "tp"]
            fp_dets = [d for d in r["detections"] if d["classification"] == "fp"]

            if tp_dets:
                print("    TP Metrics:")
                for d in tp_dets:
                    if d["detected"]:
                        delta = ""
                        if d.get("delta_from_disruption_sec"):
                            delta = f", delta={d['delta_from_disruption_sec']:+.0f}s"
                        print(f"      {d['service']} / {d['metric']} — first at {d['first_seen_unix']} ({d['count']} total{delta})")
                    else:
                        print(f"      {d['service']} / {d['metric']} — NOT DETECTED")

            if fp_dets:
                print("    FP Metrics:")
                for d in fp_dets:
                    if d["detected"]:
                        print(f"      {d['service']} / {d['metric']} — fired at {d['first_seen_unix']} ({d['count']} total)")
                    else:
                        print(f"      {d['service']} / {d['metric']} — not fired (good)")

            unk_m = r.get("unknown_metric_count", 0)
            unk_d = r.get("unknown_detection_count", 0)
            if unk_d > 0:
                print(f"    Unknown: {unk_m} metrics ({unk_d} detections)")

    print(f"\nOutput JSONs: /tmp/observer-eval-*-*.json (sigma={sigma}s)")

    results_path = "/tmp/observer-eval-detectors-matrix.json"
    with open(results_path, "w") as f:
        json.dump(results, f, indent=2)
    print(f"Full results: {results_path}")


def _ensure_parquets(ctx, name, parquet_dir):
    """Download and extract parquet files from S3 if not present locally."""
    zip_key = SCENARIO_ZIPS.get(name)
    if not zip_key:
        print(
            color_message(
                f"No S3 zip configured for '{name}' — add it to SCENARIO_ZIPS to enable auto-download", Color.ORANGE
            )
        )
        return

    print(color_message(f"Downloading {zip_key} from S3...", Color.BLUE))
    with tempfile.NamedTemporaryFile(suffix=".zip", delete=False) as tmp:
        tmp_path = tmp.name

    try:
        result = ctx.run(
            f"aws-vault exec {AWS_PROFILE} -- aws s3 cp " f"s3://{S3_BUCKET}/{zip_key} {shlex.quote(tmp_path)}",
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
                        with zf.open(member) as src, open(os.path.join(scenario_dir, "metadata.json"), "wb") as dst:
                            dst.write(src.read())
        except (zipfile.BadZipFile, OSError) as e:
            print(color_message(f"Failed to extract {zip_key}: {e}", Color.RED))
            shutil.rmtree(parquet_dir, ignore_errors=True)
            return

        print(color_message(f"Extracted parquet files to {parquet_dir}", Color.GREEN))
    finally:
        os.unlink(tmp_path)


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
