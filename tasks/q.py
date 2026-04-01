import glob
import json
import os
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
):
    """
    Runs the observer F1 eval: replays scenarios, scores Gaussian F1.

    Uses testbench --only to control which components are active.
    Default (no --only): uses testbench defaults (bocpd,rrcf,time_cluster + other default-enabled components).
    With --only: enables ONLY listed components + extractors, disables everything else.
      time_cluster is auto-added if not specified.

    Examples:
        dda inv q.eval-scenarios                            # defaults
        dda inv q.eval-scenarios --only scanmw              # scanmw + time_cluster (auto)
        dda inv q.eval-scenarios --only bocpd,time_cluster  # explicit

    Args:
        scenario: Run a single scenario (e.g. "213_pagerduty"). Default: all scenarios.
        scenarios_dir: Directory containing scenario subdirectories.
        sigma: Gaussian width in seconds for scoring.
        only: Comma-separated components to enable (passed as --only to testbench). Auto-adds time_cluster.
    """
    only_flag = ""
    if only:
        components = {name.strip() for name in only.split(",") if name.strip()}
        components.add("time_cluster")
        only_flag = ",".join(sorted(components))
        print(color_message(f"Only: {only_flag}", Color.BLUE))

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

        only_part = f" --only {shlex.quote(only_flag)}" if only_flag else ""
        ctx.run(
            f"bin/observer-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)} --scenarios-dir {shlex.quote(scenarios_dir)}{only_part}"
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


@task
def eval_tp(
    ctx, scenario: str = "", scenarios_dir: str = "./comp/observer/scenarios", sigma: float = 30.0, only: str = ""
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
            if not glob.glob(os.path.join(scenario_root, "*.parquet")):
                print(color_message(f"Skipping {name} — no parquet data at {parquet_dir}", Color.ORANGE))
                continue

        output_path = f"/tmp/observer-eval-{name}-tp.json"
        print(color_message(f"\n{'='*60}", Color.BLUE))
        print(color_message(f"  {name}", Color.BLUE))
        print(color_message(f"{'='*60}", Color.BLUE))

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
        print(color_message(f"\n{'='*60}", Color.GREEN))
        print(color_message("  Observer TP Eval Summary", Color.GREEN))
        print(color_message(f"{'='*60}\n", Color.GREEN))

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
                f"No recording found for '{name}' in runs.jsonl. " f"Run a gensim-eks episode to produce one.",
                Color.RED,
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


# --- Local Episode Runner ---

_DEFAULT_GENSIM_PATH = os.path.expanduser("~/dd/gensim-episodes")
_DEFAULT_CLUSTER_NAME = "observer-local"

# Maps short episode names to their directory under gensim-episodes
_EPISODE_PATHS = {
    "food-delivery-redis": "synthetics/food-delivery-redis-cpu-saturation",
}

# Maps short episode names to their default scenario file (without .yaml)
_EPISODE_DEFAULT_SCENARIOS = {
    "food-delivery-redis": "redis-cpu-saturation",
}


_LOCAL_EPISODE_LOG = "/tmp/local-episode-runner.log"


def _update_run_status(ctx, kube_ctx, run_id, image, episodes, started_at=None, completed_at=None):
    """Creates or updates the gensim-run-status ConfigMap with the same schema
    as the EKS orchestrator, so gensim-status.py can display local runs."""
    import datetime as _dt

    status = {
        "runId": run_id,
        "image": image,
        "startedAt": started_at or _dt.datetime.now(_dt.timezone.utc).isoformat(),
        "episodes": episodes,
    }
    if completed_at:
        status["completedAt"] = completed_at

    status_json = json.dumps(status).replace('"', '\\"')
    ctx.run(
        f'kubectl --context {kube_ctx} create configmap gensim-run-status '
        f'--from-literal=status="{status_json}" '
        f'--dry-run=client -o yaml | kubectl --context {kube_ctx} apply -f -',
        hide=True,
        warn=True,
    )


def _ensure_kind_cluster(ctx, cluster_name):
    """Creates a Kind cluster if it doesn't already exist."""
    result = ctx.run(f"kind get clusters 2>/dev/null | grep -q '^{cluster_name}$'", warn=True, hide=True)
    if result.ok:
        print(color_message(f"Kind cluster '{cluster_name}' already exists", Color.GREEN))
        return

    print(color_message(f"Creating Kind cluster '{cluster_name}'...", Color.BLUE))
    kind_config = """
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
"""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        f.write(kind_config)
        f.flush()
        ctx.run(f"kind create cluster --name {cluster_name} --config {f.name}")
    os.unlink(f.name)
    print(color_message(f"Kind cluster '{cluster_name}' created", Color.GREEN))


def _build_and_load_episode_images(ctx, episode_dir, cluster_name):
    """Builds episode service images and loads them into Kind."""
    compose_file = os.path.join(episode_dir, "docker-compose.yaml")
    if not os.path.exists(compose_file):
        print(color_message("No docker-compose.yaml, skipping service image build", Color.BLUE))
        return

    print(color_message("Building episode service images...", Color.BLUE))
    ctx.run(f"docker compose -f {compose_file} build", env={"DOCKER_BUILDKIT": "1"})

    # Parse image names from docker-compose.yaml and load each into Kind
    import yaml as pyyaml

    with open(compose_file) as f:
        compose = pyyaml.safe_load(f)

    images = [svc.get("image") for svc in compose.get("services", {}).values() if svc.get("image")]
    for img in images:
        print(color_message(f"  Loading {img} into Kind...", Color.BLUE))
        ctx.run(f"kind load docker-image {img} --name {cluster_name}")

    print(color_message(f"Loaded {len(images)} service images into Kind", Color.GREEN))


def _load_agent_image(ctx, image, cluster_name):
    """Pulls (if needed) and loads the agent image into Kind."""
    # Pull if not already present
    result = ctx.run(f"docker image inspect {image} >/dev/null 2>&1", warn=True, hide=True)
    if not result.ok:
        print(color_message(f"Pulling {image}...", Color.BLUE))
        ctx.run(f"docker pull {image}")

    print(color_message("Loading agent image into Kind...", Color.BLUE))
    ctx.run(f"kind load docker-image {image} --name {cluster_name}")


def _generate_agent_values(image, mode, cluster_name):
    """Generates Helm values YAML for the Datadog Agent with observer config."""
    repo, tag = image.rsplit(":", 1) if ":" in image else (image, "latest")

    env_vars = [
        ("DD_OBSERVER_RECORDING_ENABLED", "true"),
        ("DD_OBSERVER_RECORDING_PARQUET_OUTPUT_DIR", "/tmp/observer-parquet"),
        ("DD_OBSERVER_RECORDING_PARQUET_FLUSH_INTERVAL", "30s"),
        ("DD_OBSERVER_RECORDING_PARQUET_RETENTION", "24h"),
        ("DD_OBSERVER_HIGH_FREQUENCY_SYSTEM_CHECKS_ENABLED", "true"),
        ("DD_OBSERVER_HIGH_FREQUENCY_CONTAINER_CHECKS_ENABLED", "true"),
    ]

    if mode in ("live-anomaly-detection", "live-and-record"):
        env_vars.append(("DD_OBSERVER_ANALYSIS_ENABLED", "true"))
        env_vars.append(("DD_OBSERVER_EVENT_REPORTER_SENDING_ENABLED", "true"))

    if mode in ("live-anomaly-detection",):
        # Disable recording for live-only mode
        env_vars = [(k, v) for k, v in env_vars if "RECORDING" not in k]

    env_block = "\n".join(f"  - name: {k}\n    value: \"{v}\"" for k, v in env_vars)

    return f"""datadog:
  apiKeyExistingSecret: "datadog-secret"
  clusterName: "{cluster_name}"
  logLevel: "INFO"
  apm:
    instrumentation:
      enabled: true
  logs:
    enabled: true
    containerCollectAll: true
  processAgent:
    enabled: true
    processCollection: true
  kubelet:
    tlsVerify: false
  clusterChecks:
    enabled: true
  env:
{env_block}

agents:
  image:
    pullPolicy: "IfNotPresent"
    repository: "{repo}"
    tag: "{tag}"
    doNotCheckTag: true

clusterAgent:
  enabled: true
  replicas: 1
"""


@task
def run_local_episode(
    ctx,
    episode: str = "food-delivery-redis",
    scenario: str = "",
    image: str = "datadog/agent-dev:sopell-hf-container-checks-full",
    mode: str = "live-and-record",
    gensim_path: str = "",
    cluster_name: str = "",
    output_dir: str = "",
    skip_build: bool = False,
    skip_teardown: bool = False,
):
    """
    Runs a gensim episode locally on a Kind cluster with the observer agent.

    This is the local equivalent of `dda inv aws.eks.gensim.submit` — it builds
    episode service images, deploys them to Kind alongside the observer agent,
    executes the episode's play-episode.sh, collects parquet recordings, and
    optionally tears down. Output is compatible with q.eval-scenarios.

    Example:
        dda inv q.run-local-episode \\
            --episode=food-delivery-redis \\
            --image=datadog/agent-dev:sopell-hf-container-checks-full \\
            --mode=live-and-record
    """
    # Resolve paths and defaults
    gensim_path = gensim_path or os.environ.get("GENSIM_REPO_PATH", _DEFAULT_GENSIM_PATH)
    cluster_name = cluster_name or _DEFAULT_CLUSTER_NAME

    if episode not in _EPISODE_PATHS:
        raise ValueError(f"Unknown episode '{episode}'. Known: {list(_EPISODE_PATHS.keys())}")

    episode_dir = os.path.join(gensim_path, _EPISODE_PATHS[episode])
    if not os.path.isdir(episode_dir):
        raise FileNotFoundError(f"Episode directory not found: {episode_dir}")

    scenario = scenario or _EPISODE_DEFAULT_SCENARIOS.get(episode, "")
    if not scenario:
        raise ValueError(f"No default scenario for episode '{episode}', specify --scenario")

    scenario_file = os.path.join(episode_dir, "episodes", f"{scenario}.yaml")
    if not os.path.exists(scenario_file):
        raise FileNotFoundError(f"Scenario file not found: {scenario_file}")

    if not output_dir:
        # Map to scenario directory compatible with q.eval-scenarios
        short_name = {v: k for k, v in SCENARIO_EPISODE_NAMES.items()}.get(os.path.basename(episode_dir), episode)
        output_dir = os.path.join("comp", "observer", "scenarios", short_name, "parquet")

    release_name = f"gensim-{episode}"
    namespace = "default"
    dd_env = f"local-{episode}-{os.getpid()}"
    run_id = f"local-{episode}-{os.getpid()}"

    # Validate credentials
    api_key = os.environ.get("DDDEV_API_KEY") or os.environ.get("DD_API_KEY", "")
    app_key = os.environ.get("DDDEV_APP_KEY") or os.environ.get("DD_APP_KEY", "")
    if not api_key:
        raise RuntimeError("DDDEV_API_KEY or DD_API_KEY must be set")

    print(color_message(f"{'=' * 60}", Color.BLUE))
    print(color_message("Local Episode Runner", Color.BLUE))
    print(color_message(f"  Episode:  {episode} ({scenario})", Color.BLUE))
    print(color_message(f"  Image:    {image}", Color.BLUE))
    print(color_message(f"  Mode:     {mode}", Color.BLUE))
    print(color_message(f"  Cluster:  {cluster_name}", Color.BLUE))
    print(color_message(f"  Output:   {output_dir}", Color.BLUE))
    print(color_message(f"{'=' * 60}", Color.BLUE))

    kube_ctx = f"kind-{cluster_name}"

    # Episode status tracking (mirrors gensim-run-status ConfigMap schema)
    import datetime as _dt

    ep_status = [{"episode": episode, "scenario": scenario, "status": "queued", "phase": ""}]
    started_at = _dt.datetime.now(_dt.timezone.utc).isoformat()

    def _set_phase(status, phase=""):
        ep_status[0]["status"] = status
        ep_status[0]["phase"] = phase
        _update_run_status(ctx, kube_ctx, run_id, image, ep_status, started_at)

    # Phase 1: Ensure Kind cluster
    print(color_message("\n[Phase 1/7] Ensuring Kind cluster...", Color.BLUE))
    _ensure_kind_cluster(ctx, cluster_name)

    _set_phase("running", "cluster-setup")

    # Phase 2: Build and load images
    print(color_message("\n[Phase 2/7] Building and loading images...", Color.BLUE))
    _set_phase("running", "image-build")
    if not skip_build:
        _build_and_load_episode_images(ctx, episode_dir, cluster_name)
    _load_agent_image(ctx, image, cluster_name)

    # Phase 3: Create secrets
    print(color_message("\n[Phase 3/7] Creating K8s secrets...", Color.BLUE))
    ctx.run(
        f"kubectl --context {kube_ctx} delete secret datadog-secret --ignore-not-found",
        hide=True,
        warn=True,
    )
    ctx.run(
        f"kubectl --context {kube_ctx} create secret generic datadog-secret "
        f"--from-literal api-key={api_key} --from-literal app-key={app_key}",
    )

    # Phase 4: Install episode chart
    _set_phase("running", "episode-install")
    print(color_message("\n[Phase 4/7] Installing episode chart...", Color.BLUE))
    chart_dir = os.path.join(episode_dir, "chart")
    ctx.run(f"helm --kube-context {kube_ctx} uninstall {release_name} --wait 2>/dev/null || true", warn=True, hide=True)
    ctx.run(
        f"helm --kube-context {kube_ctx} install {release_name} {chart_dir} "
        f"--set agent.enabled=false "
        f"--set namespace={namespace} "
        f"--set datadog.env={dd_env} "
        f"--set datadog.apiKey={api_key} "
        f"--set datadog.appKey={app_key} "
        f"--timeout 5m --wait"
    )

    # Phase 5: Install Datadog Agent
    _set_phase("running", "agent-install")
    print(color_message("\n[Phase 5/7] Installing Datadog Agent...", Color.BLUE))
    values_yaml = _generate_agent_values(image, mode, cluster_name)
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        f.write(values_yaml)
        values_path = f.name

    try:
        ctx.run("helm repo add datadog https://helm.datadoghq.com 2>/dev/null || true", hide=True, warn=True)
        ctx.run("helm repo update datadog", hide=True, warn=True)
        ctx.run(
            f"helm --kube-context {kube_ctx} uninstall datadog-agent --wait 2>/dev/null || true", warn=True, hide=True
        )
        ctx.run(
            f"helm --kube-context {kube_ctx} install datadog-agent datadog/datadog "
            f"-f {values_path} --timeout 5m --wait"
        )
    finally:
        os.unlink(values_path)

    # Wait for agent pod to be ready
    print(color_message("  Waiting for agent pod readiness...", Color.BLUE))
    ctx.run(
        f"kubectl --context {kube_ctx} wait --for=condition=ready pod " f"-l app=datadog-agent --timeout=120s",
        warn=True,
    )

    # Phase 6: Run episode
    print(color_message(f"\n[Phase 6/7] Running episode ({scenario})...", Color.BLUE))
    print(color_message("  This will take ~34 minutes (warmup + baseline + disruption + cooldown)", Color.BLUE))

    episode_env = {
        "DD_API_KEY": api_key,
        "DD_APP_KEY": app_key,
        "DD_ENV": dd_env,
        "DD_SITE": "datadoghq.com",
        "KUBE_NAMESPACE": namespace,
        "KUBECONFIG": os.path.expanduser("~/.kube/config"),
        "KUBECTL_CONTEXT": kube_ctx,
    }

    # Pre-flight: verify API credentials work before starting the long episode.
    print(color_message("  Validating Datadog API credentials...", Color.BLUE))
    preflight = ctx.run(
        f'curl -s -o /dev/null -w "%{{http_code}}" '
        f'-X GET "https://api.datadoghq.com/api/v1/validate" '
        f'-H "DD-API-KEY: {api_key}" '
        f'-H "DD-APPLICATION-KEY: {app_key}"',
        hide=True,
    )
    if preflight.stdout.strip() != "200":
        raise RuntimeError(
            f"Datadog API credential validation failed (HTTP {preflight.stdout.strip()}). "
            f"Check that DDDEV_API_KEY and DDDEV_APP_KEY are correct."
        )
    print(color_message("  API credentials validated OK", Color.GREEN))

    _set_phase("running", "episode-running")

    play_script = os.path.join(episode_dir, "play-episode.sh")
    # Pipe episode output to a log file so gensim-status.py can tail it.
    ctx.run(
        f"bash {shlex.quote(play_script)} run-episode {shlex.quote(scenario)} " f"2>&1 | tee {_LOCAL_EPISODE_LOG}",
        env=episode_env,
    )

    # Phase 7: Collect parquet
    _set_phase("running", "collecting-parquet")
    print(color_message("\n[Phase 7/7] Collecting parquet recordings...", Color.BLUE))
    os.makedirs(output_dir, exist_ok=True)

    agent_pod = ctx.run(
        f"kubectl --context {kube_ctx} get pod -l app=datadog-agent " f"-o jsonpath='{{.items[0].metadata.name}}'",
        hide=True,
    ).stdout.strip()

    if agent_pod:
        ctx.run(f"kubectl --context {kube_ctx} cp {agent_pod}:/tmp/observer-parquet/ {output_dir}/", warn=True)
        parquet_count = len(glob.glob(os.path.join(output_dir, "*.parquet")))
        print(color_message(f"  Collected {parquet_count} parquet files to {output_dir}", Color.GREEN))
    else:
        print(color_message("  WARNING: Agent pod not found, could not collect parquets", Color.RED))

    # Teardown (episode + agent, but leave cluster)
    if not skip_teardown:
        print(color_message("\nTearing down episode and agent...", Color.BLUE))
        ctx.run(
            f"helm --kube-context {kube_ctx} uninstall {release_name} --wait 2>/dev/null || true", warn=True, hide=True
        )
        ctx.run(
            f"helm --kube-context {kube_ctx} uninstall datadog-agent --wait 2>/dev/null || true", warn=True, hide=True
        )

    ep_status[0]["parquetFiles"] = len(glob.glob(os.path.join(output_dir, "*.parquet")))
    _set_phase("done", "")
    completed_at = _dt.datetime.now(_dt.timezone.utc).isoformat()
    _update_run_status(ctx, kube_ctx, run_id, image, ep_status, started_at, completed_at)

    print(color_message(f"\n{'=' * 60}", Color.GREEN))
    print(color_message("Episode complete!", Color.GREEN))
    print(color_message(f"  Parquets: {output_dir}", Color.GREEN))
    print(color_message("", Color.GREEN))
    print(color_message("  Next steps:", Color.GREEN))
    print(color_message("    dda inv q.eval-scenarios --scenario=food_delivery_redis", Color.GREEN))
    print(color_message(f"{'=' * 60}", Color.GREEN))


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

    print(color_message(f"\n{'='*65}", Color.GREEN))
    print(color_message("  Observer Benchmark Summary", Color.GREEN))
    print(color_message(f"{'='*65}\n", Color.GREEN))

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
