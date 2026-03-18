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

# S3 zip key for each scenario. Update when re-recording.
SCENARIO_ZIPS = {
    "213_pagerduty": "gensim-results-213_PagerDuty_June_2014_Outage-20260303-1309-78229d.zip",
    "353_postmark": "gensim-results-353_postmark_upstream_cloud_provider_outage-20260303-1333-ad0bba.zip",
    "food_delivery_redis": "gensim-results-food-delivery-redis-cpu-saturation-20260303-1314-5f7194.zip",
}

S3_BUCKET = "qbranch-gensim-recordings"
AWS_PROFILE = "sso-agent-sandbox-account-admin"
SCENARIOS_DIR = "./comp/observer/scenarios"


# --- Build ---
@task
def build_testbench(ctx):
    """
    Builds the observer-testbench binary.
    """
    ctx.run("go build -o bin/observer-testbench ./cmd/observer-testbench")


# --- Eval ---
@task
def eval_scenarios(ctx, scenario: str = "", sigma: float = 30.0):
    """
    Runs the observer eval: builds testbench, replays scenarios headless with scoring.

    Output JSONs are saved to /tmp/observer-eval-<scenario>.json for inspection.

    Args:
        scenario: Run a single scenario (e.g. "213_pagerduty"). Default: all scenarios.
        sigma: Gaussian width in seconds for scoring.
    """
    print(color_message("Building observer-testbench...", Color.BLUE))
    build_testbench(ctx)

    scenarios_to_run = [scenario] if scenario else SCENARIOS

    results = []
    for name in scenarios_to_run:
        parquet_dir = os.path.join(SCENARIOS_DIR, name, "parquet")
        scenario_root = os.path.join(SCENARIOS_DIR, name)
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
            f"bin/observer-testbench --headless {shlex.quote(name)} --output {shlex.quote(output_path)}"
            f" --scenarios-dir {shlex.quote(SCENARIOS_DIR)} --score --sigma {sigma}"
        )

        if not os.path.isfile(output_path):
            print(color_message(f"Testbench did not produce output at {output_path}", Color.RED))
            continue

        try:
            with open(output_path) as f:
                output = json.load(f)
        except (json.JSONDecodeError, OSError) as e:
            print(color_message(f"Testbench output at {output_path} is not valid JSON: {e}", Color.RED))
            continue

        score = output.get("score")
        if not score:
            print(color_message(f"No score in output for {name} (missing episode.json?)", Color.ORANGE))
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
def pull_scenarios(ctx, scenario: str = ""):
    """
    Downloads scenario data (parquet + episode.json) from S3.

    Args:
        scenario: Pull a single scenario (e.g. "213_pagerduty"). Default: all scenarios.
    """
    scenarios_to_pull = [scenario] if scenario else SCENARIOS
    for name in scenarios_to_pull:
        parquet_dir = os.path.join(SCENARIOS_DIR, name, "parquet")
        if os.path.isdir(parquet_dir) and os.listdir(parquet_dir):
            print(color_message(f"Skipping {name} — already present at {parquet_dir}", Color.GREEN))
            continue
        _ensure_parquets(ctx, name, parquet_dir)


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
def launch_testbench(ctx, build: bool = False):
    """
    Will launch both the observer-testbench backend and UI.

    Args:
        build: Whether to build the observer-testbench binary.
    """
    if build:
        print("Building observer-testbench...")
        build_testbench(ctx)

    print("Launching observer-testbench backend and UI, use ^C to exit")
    ctx.run(
        f"bin/observer-testbench --scenarios-dir {SCENARIOS_DIR} & ( cd cmd/observer-testbench/ui && npm install && npm run dev ) &"
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

BENCH_PACKAGE = "./comp/observer/impl/"

BENCH_PATTERNS = [
    "BenchmarkDetection_Isolated_Cardinality",
    "BenchmarkBOCPD_SteadyState",
    "BenchmarkRRCF_SteadyState_v2",
    "BenchmarkCorrelators",
    "BenchmarkLogIngestion_Real",
    "BenchmarkLogIngestionWithDetection_Real",
]


@task
def benchmark(ctx, quick=False, count=3, output=""):
    """
    Runs observer engine benchmarks and prints a summary table.

    Default (full) mode runs all benchmarks with 3 iterations each (~3-5 min).
    Quick mode runs 1 iteration — useful for fast sanity checks after a code change.

    Args:
        quick: Fast mode — 1 run per benchmark, lower benchtime.
        count: Number of independent runs per benchmark (default 3, ignored with --quick).
        output: If set, write raw parsed results to this JSON file.
    """
    run_count = 1 if quick else count
    benchtime = "1x" if quick else "5x"
    mode = "quick" if quick else "full"

    bench_regex = "|".join(BENCH_PATTERNS)
    cmd = (
        f"go test -run=^$ -bench='{bench_regex}' -benchmem"
        f" -count={run_count} -benchtime={benchtime}"
        f" {BENCH_PACKAGE}"
    )

    print(color_message(f"Running observer benchmarks ({mode} mode, count={run_count})...", Color.BLUE))
    result = ctx.run(cmd, hide=True, warn=True)

    if result is None or result.failed:
        print(color_message("Benchmark run failed:", Color.RED))
        if result and result.stderr:
            print(result.stderr[:2000])
        return

    parsed = _parse_bench_output(result.stdout.splitlines())

    if not parsed:
        print(color_message("No benchmark results parsed. Check that the package compiles.", Color.RED))
        return

    if output:
        with open(output, "w") as f:
            json.dump(parsed, f, indent=2)
        print(color_message(f"Raw results written to {output}", Color.GREEN))

    _print_bench_summary(parsed)


def _parse_bench_output(lines):
    """
    Parse go test -bench output into {benchmark_name: avg_ns_per_op}.
    Handles multiple -count runs by averaging.
    """
    pattern = re.compile(r'^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([\d.]+)\s+ns/op')
    raw = {}
    for line in lines:
        m = pattern.match(line)
        if not m:
            continue
        name = m.group(1)
        ns = float(m.group(2))
        raw.setdefault(name, []).append(ns)
    return {name: sum(vals) / len(vals) for name, vals in raw.items()}


def _fmt_ns(ns):
    """Format nanoseconds as a human-readable string."""
    if ns < 1_000:
        return f"{ns:.0f} ns"
    if ns < 1_000_000:
        return f"{ns / 1_000:.1f} µs"
    return f"{ns / 1_000_000:.1f} ms"


def _sub(results, prefix):
    """Return {sub_key: ns} for all results whose name starts with prefix."""
    return {k[len(prefix) :].lstrip("/"): v for k, v in results.items() if k.startswith(prefix)}


def _print_kv_table(rows, key_width=12):
    """Print a two-column key/value table."""
    for k, v in rows:
        print(f"  {k:<{key_width}} {v}")


def _print_bench_summary(results):
    SEP = "=" * 58

    print(color_message(f"\n{SEP}", Color.GREEN))
    print(color_message("  Observer Engine Benchmark Summary", Color.GREEN))
    print(color_message(SEP, Color.GREEN))

    # 1. Algorithm CPU cost in isolation (series=50 slice only)
    isolated = _sub(results, "BenchmarkDetection_Isolated_Cardinality")
    if isolated:
        print(color_message("\nAlgorithm CPU Cost in Isolation  (series=50, steady-state advance)", Color.BLUE))
        rows = []
        for detector in ("bocpd", "rrcf", "all"):
            key = f"detector={detector}/series=50"
            if key in isolated:
                rows.append((detector, _fmt_ns(isolated[key]) + "/advance"))
        _print_kv_table(rows)

    # 2. BOCPD steady-state scaling
    bocpd_card = _sub(results, "BenchmarkBOCPD_SteadyState_Cardinality")
    if bocpd_card:
        print(color_message("\nBOCPD Steady-State — Cardinality Scaling  (advance cost)", Color.BLUE))
        rows = sorted(bocpd_card.items(), key=lambda x: int(x[0].split("=")[1]))
        _print_kv_table([(k, _fmt_ns(v) + "/advance") for k, v in rows])

    # 3. BOCPD advance frequency
    bocpd_freq = _sub(results, "BenchmarkBOCPD_SteadyState_AdvanceFrequency")
    if bocpd_freq:
        print(color_message("\nBOCPD Advance Frequency  — cost when the scheduler stalls  (50 series)", Color.BLUE))
        rows = sorted(bocpd_freq.items(), key=lambda x: int(x[0].split("=")[1]))
        baseline = rows[0][1] if rows else 1
        formatted = []
        for k, v in rows:
            secs = int(k.split("=")[1])
            label = "normal cadence" if secs == 1 else f"{secs}s stall"
            formatted.append((k, f"{_fmt_ns(v):>10}  {v / baseline:.0f}x  — {label}"))
        _print_kv_table(formatted, key_width=12)

    # 4. RRCF steady-state scaling
    rrcf_card = _sub(results, "BenchmarkRRCF_SteadyState_v2_Cardinality")
    if rrcf_card:
        print(color_message("\nRRCF Steady-State — Cardinality Scaling  (advance cost)", Color.BLUE))
        rows = sorted(rrcf_card.items(), key=lambda x: int(x[0].split("=")[1]))
        _print_kv_table([(k, _fmt_ns(v) + "/advance") for k, v in rows])

    # 5. RRCF advance frequency
    rrcf_freq = _sub(results, "BenchmarkRRCF_SteadyState_v2_AdvanceFrequency")
    if rrcf_freq:
        print(color_message("\nRRCF Advance Frequency  — cost when the scheduler stalls  (20 metrics)", Color.BLUE))
        rows = sorted(rrcf_freq.items(), key=lambda x: int(x[0].split("=")[1]))
        baseline = rows[0][1] if rows else 1
        formatted = []
        for k, v in rows:
            secs = int(k.split("=")[1])
            label = "normal cadence" if secs == 1 else f"{secs}s stall"
            formatted.append((k, f"{_fmt_ns(v):>10}  {v / baseline:.0f}x  — {label}"))
        _print_kv_table(formatted, key_width=12)

    # 6. Per-correlator cost
    corr = _sub(results, "BenchmarkCorrelators_Isolated")
    if corr:
        print(color_message("\nCorrelator Cost  (BOCPD baseline + each correlator, 50 series)", Color.BLUE))
        baseline_ns = corr.get("correlator=none")
        order = ("none", "cross_signal", "time_cluster", "lead_lag", "surprise", "all")
        rows = []
        for name in order:
            key = f"correlator={name}"
            if key not in corr:
                continue
            ns = corr[key]
            if baseline_ns and name != "none":
                delta = ns - baseline_ns
                pct = delta / baseline_ns * 100
                rows.append((name, f"{_fmt_ns(ns)}/advance  (+{_fmt_ns(delta)}, +{pct:.0f}%)"))
            else:
                rows.append((name, f"{_fmt_ns(ns)}/advance  (baseline)"))
        _print_kv_table(rows, key_width=14)

    # 7. Log ingestion
    log_raw = _sub(results, "BenchmarkLogIngestion_Real_Cardinality")
    log_det = _sub(results, "BenchmarkLogIngestionWithDetection_Real_Cardinality")
    if log_raw:
        print(color_message("\nLog Ingestion  (real extractors)", Color.BLUE))
        rows = sorted(log_raw.items(), key=lambda x: int(x[0].split("=")[1]))
        for k, ns in rows:
            det_ns = log_det.get(k)
            det_str = f"  +detection: {_fmt_ns(det_ns)}" if det_ns else ""
            print(f"  {k:<12} {_fmt_ns(ns)}/log{det_str}")

    print()
