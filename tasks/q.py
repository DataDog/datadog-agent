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
