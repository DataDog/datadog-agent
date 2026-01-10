#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# ///
"""
Scenario runner for fine-grained-monitor incident reproduction.

Scenarios are reproducible Kubernetes workloads that exhibit specific
behaviors (crashes, resource spikes, etc.) for investigation with fgm-viewer.

Usage:
    ./scenario.py list                    # List available scenarios
    ./scenario.py run <name>              # Deploy scenario, return run_id
    ./scenario.py status [run_id]         # Show pod status (latest if omitted)
    ./scenario.py stop <run_id>           # Stop and clean up scenario
    ./scenario.py logs [run_id]           # Show scenario pod logs
    ./scenario.py export [run_id]         # Export as self-contained HTML

Examples:
    ./scenario.py run sigpipe-crash       # Deploy the SIGPIPE crash scenario
    ./scenario.py status                  # Check status of latest run
    ./scenario.py logs -c victim-app      # Show logs from victim-app container
    ./scenario.py stop a1b2c3d4           # Stop scenario run a1b2c3d4
    ./scenario.py export                  # Export latest run for offline viewing
"""

import argparse
import base64
import json
import re
import subprocess
import sys
import time
import uuid
from datetime import datetime, timedelta
from pathlib import Path
from urllib.error import URLError
from urllib.parse import urlencode
from urllib.request import urlopen

# Project root is where this script lives
PROJECT_ROOT = Path(__file__).parent.resolve()
DEV_DIR = PROJECT_ROOT / ".dev"

# Scenario configuration
SCENARIOS_DIR = PROJECT_ROOT / "scenarios"
SCENARIO_STATE_DIR = DEV_DIR / "scenarios"
SCENARIO_RETENTION_DAYS = 7

# Cluster deployment config
LIMA_VM = "gadget-k8s-host"
DEFAULT_NAMESPACE = "default"  # For legacy scenarios without namespace isolation


# === Cluster Utilities (duplicated from dev.py for standalone operation) ===


def get_worktree_id() -> str:
    """Get worktree identifier from directory basename.

    Uses the parent directory of the fine-grained-monitor project root,
    e.g., /Users/scott/dev/beta-datadog-agent/q_branch/fine-grained-monitor
    -> "beta-datadog-agent"
    """
    return PROJECT_ROOT.parent.parent.name


def get_cluster_name() -> str:
    """Get Kind cluster name for this worktree."""
    return f"fgm-{get_worktree_id()}"


def get_kube_context() -> str:
    """Get kubectl context name for this worktree's cluster."""
    return f"kind-{get_cluster_name()}"


def get_image_tag() -> str:
    """Get Docker image tag for this worktree."""
    return get_worktree_id()


def ensure_dev_dir():
    """Create .dev directory if needed."""
    DEV_DIR.mkdir(exist_ok=True)
    SCENARIO_STATE_DIR.mkdir(parents=True, exist_ok=True)


def check_lima_vm() -> bool:
    """Check if Lima VM is running."""
    result = subprocess.run(
        ["limactl", "list", "--format", "{{.Name}}:{{.Status}}"],
        capture_output=True,
        text=True,
    )
    for line in result.stdout.strip().split("\n"):
        if line.startswith(f"{LIMA_VM}:"):
            status = line.split(":")[1]
            return status == "Running"
    return False


def cluster_exists() -> bool:
    """Check if this worktree's Kind cluster exists."""
    cluster_name = get_cluster_name()
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--", "kind", "get", "clusters"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return False
    return cluster_name in result.stdout.strip().split("\n")


def run_cmd(cmd: list[str], description: str, capture: bool = False) -> tuple[bool, str]:
    """Run a command with status output. Returns (success, output)."""
    print(f"{description}...", end=" ", flush=True)
    start = time.time()

    result = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        cwd=PROJECT_ROOT,
    )

    elapsed = time.time() - start

    if result.returncode != 0:
        print("FAILED")
        if result.stderr:
            print(f"  Error: {result.stderr.strip()}")
        return False, result.stderr

    print(f"done ({elapsed:.1f}s)")
    return True, result.stdout


# === Scenario Helper Functions ===


def get_available_scenarios() -> list[str]:
    """Get list of available scenarios (dirs with deploy.yaml)."""
    if not SCENARIOS_DIR.exists():
        return []
    scenarios = []
    for d in SCENARIOS_DIR.iterdir():
        if d.is_dir() and (d / "deploy.yaml").exists():
            scenarios.append(d.name)
    return sorted(scenarios)


def get_scenario_run_dir(run_id: str) -> Path:
    """Get the directory for a scenario run."""
    return SCENARIO_STATE_DIR / run_id


def get_scenario_label(run_id: str, is_gensim: bool = False) -> str:
    """Get k8s label selector for a scenario run."""
    if is_gensim:
        return f"fgm-run={run_id}"
    return f"fgm-scenario={run_id}"


def get_run_namespace(run_id: str) -> str:
    """Get namespace for a scenario run (namespace-per-run isolation)."""
    return f"fgm-run-{run_id}"


def is_gensim_scenario(manifest: str) -> bool:
    """Detect if manifest was generated by gensim k8s-adapter."""
    return "Generated by k8s-adapter" in manifest or "gensim-category" in manifest


def scenario_uses_namespace_isolation(manifest: str) -> bool:
    """Check if scenario uses namespace-per-run isolation (has {{NAMESPACE}} placeholder)."""
    return "{{NAMESPACE}}" in manifest


def cleanup_old_scenario_runs():
    """Remove scenario runs older than SCENARIO_RETENTION_DAYS."""
    if not SCENARIO_STATE_DIR.exists():
        return

    cutoff = datetime.now() - timedelta(days=SCENARIO_RETENTION_DAYS)
    removed = 0

    for run_dir in SCENARIO_STATE_DIR.iterdir():
        if not run_dir.is_dir():
            continue

        try:
            mtime = datetime.fromtimestamp(run_dir.stat().st_mtime)
            if mtime < cutoff:
                # Remove the directory and its contents
                for f in run_dir.iterdir():
                    f.unlink()
                run_dir.rmdir()
                removed += 1
        except (OSError, ValueError):
            continue

    if removed > 0:
        print(f"Cleaned up {removed} scenario run(s) older than {SCENARIO_RETENTION_DAYS} days")


def get_scenario_components(scenario_name: str) -> list[str]:
    """Get list of buildable components (subdirs with Dockerfile) for a scenario."""
    scenario_dir = SCENARIOS_DIR / scenario_name
    components = []
    for d in scenario_dir.iterdir():
        if d.is_dir() and (d / "Dockerfile").exists():
            components.append(d.name)
    return sorted(components)


def get_latest_scenario_run() -> str | None:
    """Get the most recent scenario run ID."""
    if not SCENARIO_STATE_DIR.exists():
        return None

    runs = []
    for run_dir in SCENARIO_STATE_DIR.iterdir():
        if not run_dir.is_dir():
            continue
        metadata_file = run_dir / "metadata.json"
        if metadata_file.exists():
            try:
                metadata = json.loads(metadata_file.read_text())
                runs.append((metadata.get("started_at", ""), run_dir.name))
            except (json.JSONDecodeError, OSError):
                continue

    if not runs:
        return None

    # Sort by start time, return most recent
    runs.sort(reverse=True)
    return runs[0][1]


# === Scenario Commands ===


def cmd_list():
    """List available scenarios."""
    scenarios = get_available_scenarios()

    if not scenarios:
        print("No scenarios found")
        print(f"  Scenarios directory: {SCENARIOS_DIR}")
        print("  Each scenario needs a deploy.yaml file")
        return 0

    print("Available scenarios:\n")
    for name in scenarios:
        scenario_dir = SCENARIOS_DIR / name
        components = get_scenario_components(name)
        readme = scenario_dir / "README.md"

        # Get first line of README for description
        description = ""
        if readme.exists():
            first_line = readme.read_text().split("\n")[0].strip()
            if first_line.startswith("#"):
                description = first_line.lstrip("# ").strip()

        print(f"  {name}")
        if description:
            print(f"    {description}")
        if components:
            print(f"    Components: {', '.join(components)}")
        print()

    return 0


def cmd_run(scenario_name: str):
    """Build images, deploy scenario to cluster, return run ID."""
    kube_context = get_kube_context()
    image_tag = get_image_tag()

    # Validate scenario exists
    scenario_dir = SCENARIOS_DIR / scenario_name
    if not scenario_dir.exists():
        print(f"Error: Scenario '{scenario_name}' not found")
        print(f"  Available: {', '.join(get_available_scenarios()) or 'none'}")
        return 1

    deploy_yaml = scenario_dir / "deploy.yaml"
    if not deploy_yaml.exists():
        print(f"Error: Scenario '{scenario_name}' missing deploy.yaml")
        return 1

    # Check Lima VM is running
    if not check_lima_vm():
        print(f"Error: Lima VM '{LIMA_VM}' not running")
        print("  Start with: limactl start gadget-k8s-host")
        return 1

    # Check cluster is available
    if not cluster_exists():
        print(f"Error: Cluster '{get_cluster_name()}' not found")
        print("  Run: ./dev.py cluster deploy")
        return 1

    # Clean up old runs
    cleanup_old_scenario_runs()

    # Generate run ID
    run_id = str(uuid.uuid4())[:8]
    ensure_dev_dir()
    run_dir = get_scenario_run_dir(run_id)
    run_dir.mkdir(parents=True, exist_ok=True)

    # Read manifest early to detect scenario type
    raw_manifest = deploy_yaml.read_text()
    is_gensim = is_gensim_scenario(raw_manifest)
    uses_namespace = scenario_uses_namespace_isolation(raw_manifest)

    # Determine namespace for this run
    if uses_namespace:
        namespace = get_run_namespace(run_id)
    else:
        namespace = DEFAULT_NAMESPACE

    print(f"Starting scenario '{scenario_name}' (run {run_id})")
    if uses_namespace:
        print(f"  Namespace: {namespace}")

    # Build component images
    components = get_scenario_components(scenario_name)
    cluster_name = get_cluster_name()

    for component in components:
        component_dir = scenario_dir / component
        image_name = f"fgm-scenario-{scenario_name}-{component}:{image_tag}"

        # Build image
        success, _ = run_cmd(
            ["docker", "build", "-t", image_name, str(component_dir)],
            f"Building {component}",
        )
        if not success:
            print(f"Failed to build {component}")
            return 1

        # Load into Lima VM
        print(f"Loading {component} into Lima VM...", end=" ", flush=True)
        start = time.time()

        save_proc = subprocess.Popen(
            ["docker", "save", image_name],
            stdout=subprocess.PIPE,
        )
        load_proc = subprocess.Popen(
            ["limactl", "shell", LIMA_VM, "--", "docker", "load"],
            stdin=save_proc.stdout,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        save_proc.stdout.close()
        stdout, stderr = load_proc.communicate()

        if load_proc.returncode != 0:
            print("FAILED")
            print(f"  Error: {stderr.decode()}")
            return 1
        print(f"done ({time.time() - start:.1f}s)")

        # Load into Kind cluster
        success, _ = run_cmd(
            ["limactl", "shell", LIMA_VM, "--", "kind", "load", "docker-image", image_name, "--name", cluster_name],
            f"Loading {component} into Kind",
        )
        if not success:
            return 1

    # Process manifest with placeholder substitution
    manifest = raw_manifest
    manifest = manifest.replace("{{RUN_ID}}", run_id)
    manifest = manifest.replace("{{IMAGE_TAG}}", image_tag)
    manifest = manifest.replace("{{SCENARIO_NAME}}", scenario_name)
    manifest = manifest.replace("{{NAMESPACE}}", namespace)

    # For legacy (non-gensim) scenarios, inject fgm-scenario label
    # Gensim scenarios already have proper labels from the template
    if not is_gensim:
        manifest = re.sub(
            r"(metadata:\s*\n\s*name:.*\n)(\s*labels:\s*\n)?",
            lambda m: m.group(1) + (m.group(2) or "  labels:\n") + f"    fgm-scenario: \"{run_id}\"\n",
            manifest,
        )

    # Save processed manifest for cleanup reference
    (run_dir / "manifests.yaml").write_text(manifest)

    # For namespace-isolated scenarios, create the namespace first
    if uses_namespace:
        print(f"Creating namespace {namespace}...", end=" ", flush=True)
        ns_manifest = f"""apiVersion: v1
kind: Namespace
metadata:
  name: {namespace}
  labels:
    fgm-run: "{run_id}"
    scenario: "{scenario_name}"
"""
        result = subprocess.run(
            ["kubectl", "apply", "-f", "-", "--context", kube_context],
            input=ns_manifest,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            print("FAILED")
            print(f"  Error: {result.stderr}")
            return 1
        print("done")

    # Apply manifest
    print("Applying manifests...", end=" ", flush=True)
    result = subprocess.run(
        ["kubectl", "apply", "-f", "-", "--context", kube_context],
        input=manifest,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print("FAILED")
        print(f"  Error: {result.stderr}")
        return 1
    print("done")

    # Save metadata
    metadata = {
        "run_id": run_id,
        "scenario": scenario_name,
        "started_at": datetime.now().isoformat(),
        "components": components,
        "cluster": get_cluster_name(),
        "namespace": namespace,
        "is_gensim": is_gensim,
        "uses_namespace_isolation": uses_namespace,
    }
    (run_dir / "metadata.json").write_text(json.dumps(metadata, indent=2))

    # Wait for pods to be ready
    label = get_scenario_label(run_id, is_gensim=is_gensim)
    print("Waiting for pods...", end=" ", flush=True)
    start = time.time()

    for _ in range(60):  # 60 second timeout
        result = subprocess.run(
            [
                "kubectl",
                "get",
                "pods",
                "-n",
                namespace,
                "--context",
                kube_context,
                "-o",
                "jsonpath={.items[*].status.phase}",
            ],
            capture_output=True,
            text=True,
        )
        phases = result.stdout.strip().split()
        if phases and all(p == "Running" for p in phases):
            break
        time.sleep(1)
    else:
        print(f"TIMEOUT ({time.time() - start:.0f}s)")
        print("  Pods may still be starting. Check with: ./scenario.py status")

    print(f"done ({time.time() - start:.1f}s)")

    print(f"\nScenario '{scenario_name}' running (run {run_id})")
    print(f"  Status: ./scenario.py status {run_id}")
    print(f"  Logs:   ./scenario.py logs {run_id}")
    print(f"  Stop:   ./scenario.py stop {run_id}")

    return 0


def cmd_status(run_id: str | None):
    """Show status of a scenario run."""
    kube_context = get_kube_context()

    # Use latest run if not specified
    if run_id is None:
        run_id = get_latest_scenario_run()
        if run_id is None:
            print("No scenario runs found")
            return 1

    run_dir = get_scenario_run_dir(run_id)
    if not run_dir.exists():
        print(f"Error: Run '{run_id}' not found")
        return 1

    # Load metadata
    metadata_file = run_dir / "metadata.json"
    metadata = {}
    if metadata_file.exists():
        metadata = json.loads(metadata_file.read_text())
        scenario = metadata.get("scenario", "unknown")
        started = metadata.get("started_at", "unknown")[:19]
        namespace = metadata.get("namespace", DEFAULT_NAMESPACE)
        print(f"Scenario: {scenario}")
        print(f"Run ID:   {run_id}")
        print(f"Started:  {started}")
        if metadata.get("uses_namespace_isolation"):
            print(f"Namespace: {namespace}")
        print()
    else:
        namespace = DEFAULT_NAMESPACE

    # Get appropriate label based on scenario type
    is_gensim = metadata.get("is_gensim", False)
    label = get_scenario_label(run_id, is_gensim=is_gensim)

    # Show pod status
    print(f"Pods (namespace: {namespace}):")

    result = subprocess.run(
        ["kubectl", "get", "pods", "-n", namespace, "--context", kube_context, "-o", "wide"],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0:
        print(f"  Error: {result.stderr}")
        return 1

    if result.stdout.strip():
        print(result.stdout)
    else:
        print("  No pods found (scenario may have been stopped)")

    return 0


def cmd_stop(run_id: str):
    """Stop and clean up a scenario run."""
    kube_context = get_kube_context()

    run_dir = get_scenario_run_dir(run_id)
    if not run_dir.exists():
        print(f"Error: Run '{run_id}' not found")
        return 1

    # Load metadata to determine cleanup strategy
    metadata_file = run_dir / "metadata.json"
    metadata = {}
    if metadata_file.exists():
        metadata = json.loads(metadata_file.read_text())

    uses_namespace = metadata.get("uses_namespace_isolation", False)
    namespace = metadata.get("namespace", DEFAULT_NAMESPACE)
    is_gensim = metadata.get("is_gensim", False)

    print(f"Stopping scenario run {run_id}...")

    if uses_namespace:
        # For namespace-isolated scenarios, delete the entire namespace (cascades to all resources)
        print(f"Deleting namespace {namespace}...", end=" ", flush=True)
        result = subprocess.run(
            ["kubectl", "delete", "namespace", namespace, "--context", kube_context, "--ignore-not-found"],
            capture_output=True,
            text=True,
        )

        if result.returncode != 0:
            print("FAILED")
            print(f"  Error: {result.stderr}")
            return 1
        print("done")
    else:
        # For legacy scenarios, delete by label
        label = get_scenario_label(run_id, is_gensim=is_gensim)
        print("Deleting k8s resources...", end=" ", flush=True)
        result = subprocess.run(
            ["kubectl", "delete", "all,configmap", "-l", label, "-n", namespace, "--context", kube_context],
            capture_output=True,
            text=True,
        )

        if result.returncode != 0 and "NotFound" not in result.stderr:
            print("FAILED")
            print(f"  Error: {result.stderr}")
            return 1
        print("done")

    # Clean up local state
    print("Cleaning up local state...", end=" ", flush=True)
    try:
        for f in run_dir.iterdir():
            f.unlink()
        run_dir.rmdir()
        print("done")
    except OSError as e:
        print(f"warning: {e}")

    print(f"Scenario run {run_id} stopped")
    return 0


def cmd_logs(run_id: str | None, container: str | None):
    """Show logs from scenario pods."""
    kube_context = get_kube_context()

    # Use latest run if not specified
    if run_id is None:
        run_id = get_latest_scenario_run()
        if run_id is None:
            print("No scenario runs found")
            return 1

    run_dir = get_scenario_run_dir(run_id)
    if not run_dir.exists():
        print(f"Error: Run '{run_id}' not found")
        return 1

    # Load metadata for namespace
    metadata_file = run_dir / "metadata.json"
    metadata = {}
    if metadata_file.exists():
        metadata = json.loads(metadata_file.read_text())

    namespace = metadata.get("namespace", DEFAULT_NAMESPACE)

    # Get pods (no label filter needed - namespace provides isolation)
    result = subprocess.run(
        [
            "kubectl",
            "get",
            "pods",
            "-n",
            namespace,
            "--context",
            kube_context,
            "-o",
            "jsonpath={.items[*].metadata.name}",
        ],
        capture_output=True,
        text=True,
    )

    pods = result.stdout.strip().split()
    if not pods or not pods[0]:
        print(f"No pods found for run {run_id}")
        return 1

    # Show logs for each pod
    for pod in pods:
        print(f"=== Logs from {pod} ===")

        cmd = ["kubectl", "logs", pod, "-n", namespace, "--context", kube_context, "--all-containers", "--tail=50"]
        if container:
            cmd = ["kubectl", "logs", pod, "-c", container, "-n", namespace, "--context", kube_context, "--tail=50"]

        result = subprocess.run(cmd, capture_output=True, text=True)

        if result.returncode != 0:
            print(f"  Error: {result.stderr}")
        else:
            print(result.stdout)
        print()

    return 0


def cmd_export(run_id: str | None, output: str | None):
    """Export scenario data as a self-contained HTML file with embedded parquet."""
    kube_context = get_kube_context()

    # Use latest run if not specified
    if run_id is None:
        run_id = get_latest_scenario_run()
        if run_id is None:
            print("No scenario runs found")
            return 1

    run_dir = get_scenario_run_dir(run_id)
    if not run_dir.exists():
        print(f"Error: Run '{run_id}' not found")
        return 1

    # Load metadata
    metadata_file = run_dir / "metadata.json"
    if not metadata_file.exists():
        print(f"Error: Metadata not found for run '{run_id}'")
        return 1

    metadata = json.loads(metadata_file.read_text())
    scenario_name = metadata["scenario"]
    namespace = metadata.get("namespace", DEFAULT_NAMESPACE)

    print(f"Exporting scenario '{scenario_name}' (run {run_id})")

    # Load dashboard from scenario directory
    dashboard_path = SCENARIOS_DIR / scenario_name / "dashboard.json"
    if not dashboard_path.exists():
        print(f"Error: Dashboard not found at {dashboard_path}")
        print("  Export requires a dashboard.json in the scenario directory")
        return 1

    dashboard = json.loads(dashboard_path.read_text())

    # Substitute {{RUN_ID}} placeholder in dashboard
    dashboard_str = json.dumps(dashboard).replace("{{RUN_ID}}", run_id)
    dashboard = json.loads(dashboard_str)

    # Build export URL params from dashboard config
    params = {}

    # Extract namespace filter
    containers_config = dashboard.get("containers", {})
    if "namespace" in containers_config:
        params["namespace"] = containers_config["namespace"]

    # Extract label selector
    if "label_selector" in containers_config:
        labels = containers_config["label_selector"]
        params["labels"] = ",".join(f"{k}:{v}" for k, v in labels.items())

    # Extract metrics from panels
    if dashboard.get("panels"):
        params["metrics"] = ",".join(p["metric"] for p in dashboard["panels"])

    # Use all available data
    params["range"] = "all"

    print(f"  Namespace: {params.get('namespace', '(all)')}")
    print(f"  Labels: {params.get('labels', '(none)')}")
    print(f"  Metrics: {params.get('metrics', '(all)')}")

    # Find the metrics-viewer pod
    result = subprocess.run(
        [
            "kubectl",
            "get",
            "pods",
            "-n",
            "fine-grained-monitor",
            "--context",
            kube_context,
            "-l",
            "app=fine-grained-monitor",
            "-o",
            "jsonpath={.items[0].metadata.name}",
        ],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0 or not result.stdout.strip():
        print("Error: Could not find fine-grained-monitor pod")
        print("  Ensure fgm is deployed: ./dev.py cluster deploy")
        return 1

    pod_name = result.stdout.strip()

    # Start port-forward in background
    viewer_port = 8399  # Use a distinct port for export
    print(f"Starting port-forward to {pod_name}...", end=" ", flush=True)

    pf_proc = subprocess.Popen(
        [
            "kubectl",
            "port-forward",
            pod_name,
            f"{viewer_port}:8050",
            "-n",
            "fine-grained-monitor",
            "--context",
            kube_context,
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )

    # Wait for port-forward to be ready
    time.sleep(2)

    if pf_proc.poll() is not None:
        print("FAILED")
        _, stderr = pf_proc.communicate()
        print(f"  Error: {stderr.decode()}")
        return 1
    print("done")

    try:
        # Call export API
        url = f"http://localhost:{viewer_port}/api/export?{urlencode(params)}"
        print(f"Fetching data from {url}...", end=" ", flush=True)

        try:
            response = urlopen(url, timeout=120)
            parquet_data = response.read()
            print(f"done ({len(parquet_data)} bytes)")
        except URLError as e:
            print("FAILED")
            print(f"  Error: {e}")
            return 1

        # Generate HTML with embedded parquet
        print("Generating HTML...", end=" ", flush=True)
        html = generate_export_html(parquet_data, dashboard, metadata)
        print("done")

        # Write output file
        output_path = output or f"scenario-results-{run_id}.html"
        Path(output_path).write_text(html)

        file_size_mb = len(html) / (1024 * 1024)
        print(f"\nExported to: {output_path} ({file_size_mb:.1f} MB)")
        print("  Open in browser to view (works offline)")

        return 0

    finally:
        # Stop port-forward
        pf_proc.terminate()
        pf_proc.wait()


def generate_export_html(parquet_data: bytes, dashboard: dict, metadata: dict) -> str:
    """Generate a self-contained HTML file with embedded parquet data.

    Uses the main viewer (index.html) as base and bundles all JS modules
    using esbuild for a truly self-contained export that reuses all the
    existing viewer code.
    """
    import re
    import tempfile

    # Base64 encode the parquet data
    parquet_b64 = base64.b64encode(parquet_data).decode("ascii")

    static_dir = PROJECT_ROOT / "src/metrics_viewer/static"
    index_path = static_dir / "index.html"
    if not index_path.exists():
        raise FileNotFoundError(f"Index file not found: {index_path}")

    # Bundle JS modules using esbuild
    print("  Bundling JS modules...")
    entry_point = static_dir / "js/ui.js"

    with tempfile.NamedTemporaryFile(mode="w", suffix=".js", delete=False) as f:
        bundle_path = f.name

    try:
        result = subprocess.run(
            [
                "npx",
                "esbuild",
                str(entry_point),
                "--bundle",
                "--format=iife",
                "--global-name=FGM",  # Expose exports as window.FGM
                f"--outfile={bundle_path}",
                "--minify",
            ],
            capture_output=True,
            text=True,
            cwd=static_dir,
        )
        if result.returncode != 0:
            raise RuntimeError(f"esbuild failed to bundle JS modules: {result.stderr}")

        bundled_js = Path(bundle_path).read_text()
    finally:
        Path(bundle_path).unlink(missing_ok=True)

    # Read index.html
    html = index_path.read_text()

    # Update title to show it's an export
    scenario_name = metadata.get("scenario", "unknown")
    run_id = metadata.get("run_id", "unknown")[:8]
    html = html.replace(
        "<title>Fine-Grained Monitor</title>", f"<title>FGM Export - {scenario_name} ({run_id})</title>"
    )

    # Replace the ES module script with bundled IIFE script
    # Original: <script type="module">import { initialize } from './static/js/ui.js'; ...
    module_script_pattern = r'<script type="module">.*?</script>'
    bundled_script = f'''<script>
{bundled_js}
// Initialize when DOM is ready (FGM is the global namespace from esbuild IIFE)
if (document.readyState === 'loading') {{
    document.addEventListener('DOMContentLoaded', function() {{ FGM.initialize(); }});
}} else {{
    FGM.initialize();
}}
</script>'''
    # Use lambda to avoid backslash interpretation in replacement string
    # (bundled JS contains patterns like \w that would be interpreted as backreferences)
    html = re.sub(module_script_pattern, lambda m: bundled_script, html, flags=re.DOTALL)

    # Add embedded parquet data before closing </body>
    parquet_script = f'<script id="parquet-data" type="application/base64">{parquet_b64}</script>'
    html = html.replace("</body>", f"    {parquet_script}\n</body>")

    # Add dashboard config if provided
    if dashboard:
        dashboard_script = f'<script id="dashboard-config" type="application/json">{json.dumps(dashboard)}</script>'
        html = html.replace("</body>", f"    {dashboard_script}\n</body>")

    return html


def main():
    parser = argparse.ArgumentParser(
        description="Scenario runner for fine-grained-monitor incident reproduction",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  ./scenario.py list                   # List available scenarios
  ./scenario.py run sigpipe-crash      # Deploy the SIGPIPE crash scenario
  ./scenario.py status                 # Check status of latest run
  ./scenario.py status a1b2c3d4        # Check status of specific run
  ./scenario.py logs                   # Show logs from latest run
  ./scenario.py logs -c victim-app     # Show logs from specific container
  ./scenario.py stop a1b2c3d4          # Stop and clean up scenario
  ./scenario.py export                 # Export latest run as self-contained HTML
  ./scenario.py export a1b2c3d4 -o results.html  # Export specific run

Scenarios are deployed to the Kind cluster for this worktree.
Use ./dev.py cluster deploy first to ensure the cluster exists.
""",
    )

    subparsers = parser.add_subparsers(dest="command", required=True)

    # list
    subparsers.add_parser("list", help="List available scenarios")

    # run <name>
    run_parser = subparsers.add_parser("run", help="Deploy scenario to cluster")
    run_parser.add_argument("name", type=str, help="Scenario name")

    # status [run_id]
    status_parser = subparsers.add_parser("status", help="Show scenario pod status")
    status_parser.add_argument("run_id", type=str, nargs="?", default=None, help="Run ID (default: latest)")

    # stop <run_id>
    stop_parser = subparsers.add_parser("stop", help="Stop and clean up scenario")
    stop_parser.add_argument("run_id", type=str, help="Run ID")

    # logs [run_id]
    logs_parser = subparsers.add_parser("logs", help="Show scenario pod logs")
    logs_parser.add_argument("run_id", type=str, nargs="?", default=None, help="Run ID (default: latest)")
    logs_parser.add_argument("-c", "--container", type=str, default=None, help="Container name")

    # export [run_id]
    export_parser = subparsers.add_parser("export", help="Export scenario as self-contained HTML")
    export_parser.add_argument("run_id", type=str, nargs="?", default=None, help="Run ID (default: latest)")
    export_parser.add_argument("-o", "--output", type=str, default=None, help="Output file path")

    args = parser.parse_args()

    if args.command == "list":
        sys.exit(cmd_list())
    elif args.command == "run":
        sys.exit(cmd_run(args.name))
    elif args.command == "status":
        sys.exit(cmd_status(args.run_id))
    elif args.command == "stop":
        sys.exit(cmd_stop(args.run_id))
    elif args.command == "logs":
        sys.exit(cmd_logs(args.run_id, args.container))
    elif args.command == "export":
        sys.exit(cmd_export(args.run_id, args.output))


if __name__ == "__main__":
    main()
