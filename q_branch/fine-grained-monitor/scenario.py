#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = ["pyyaml"]
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
    ./scenario.py export [run_id]         # Export as parquet and HTML files
    ./scenario.py import <name>           # Import scenario from gensim blueprint
    ./scenario.py import --list           # List available gensim blueprints

Examples:
    ./scenario.py run sigpipe-crash       # Deploy the SIGPIPE crash scenario
    ./scenario.py status                  # Check status of latest run
    ./scenario.py logs -c victim-app      # Show logs from victim-app container
    ./scenario.py stop a1b2c3d4           # Stop scenario run a1b2c3d4
    ./scenario.py export                  # Export latest run as .parquet and .html
    ./scenario.py import todo-app         # Import todo-app blueprint from gensim
"""

import argparse
import base64
import json
import re
import shutil
import subprocess
import sys
import tempfile
import time
import uuid
from datetime import datetime, timedelta
from pathlib import Path
from urllib.error import URLError
from urllib.parse import urlencode
from urllib.request import urlopen

import yaml

# Add q_branch to path for shared library imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from lib.k8s_backend import (
    Mode,
    VMBackend,
    create_backend,
    detect_environment,
    run_cmd,
)

import dev as q_branch_dev

# Project root is where this script lives
PROJECT_ROOT = Path(__file__).parent.resolve()
DEV_DIR = PROJECT_ROOT / ".dev"

# Scenario configuration
SCENARIOS_DIR = PROJECT_ROOT / "scenarios"
SCENARIO_STATE_DIR = DEV_DIR / "scenarios"
SCENARIO_RETENTION_DAYS = 7

# Cluster deployment config
LIMA_VM = q_branch_dev.LIMA_VM
DEFAULT_NAMESPACE = "default"  # For legacy scenarios without namespace isolation

# Gensim cache config (for importing blueprints)
GENSIM_CACHE_DIR = Path.home() / ".cache" / "fgm" / "gensim"
GENSIM_REPO_URL = "git@github.com:DataDog/gensim.git"
GENSIM_BRANCH = "sopell/k8s-adapter"  # TODO: Change to "main" once PR #106 is merged

# Environment and backend (lazy initialization)
_env = None
_backend = None


def _get_env():
    """Get or initialize environment."""
    global _env
    if _env is None:
        _env = detect_environment()
    return _env


def _get_backend():
    """Get or initialize backend."""
    global _backend
    if _backend is None:
        _backend = create_backend(_get_env(), LIMA_VM)
    return _backend


# === Cluster Utilities (wrappers around shared library) ===


def get_worktree_id() -> str:
    """Get worktree identifier from directory basename."""
    return q_branch_dev.get_worktree_id(PROJECT_ROOT)


def get_cluster_name() -> str:
    """Get Kind cluster name for this worktree."""
    return q_branch_dev.get_cluster_name("fgm", get_worktree_id())


def get_kube_context() -> str:
    """Get kubectl context name for this worktree's cluster."""
    return q_branch_dev.get_kube_context(get_cluster_name())


def get_image_tag() -> str:
    """Get Docker image tag for this worktree."""
    return q_branch_dev.get_image_tag(get_worktree_id())


def ensure_dev_dir():
    """Create .dev directory if needed."""
    DEV_DIR.mkdir(exist_ok=True)
    SCENARIO_STATE_DIR.mkdir(parents=True, exist_ok=True)


def check_lima_vm() -> bool:
    """Check if Lima VM is running (only relevant in VM mode)."""
    env = _get_env()
    if env.mode == Mode.DIRECT:
        return True  # No VM needed in direct mode
    backend = _get_backend()
    if isinstance(backend, VMBackend):
        return backend.status() == "Running"
    return True


def cluster_exists() -> bool:
    """Check if this worktree's Kind cluster exists."""
    backend = _get_backend()
    return q_branch_dev.cluster_exists(backend, get_cluster_name())


def scenario_run_cmd(cmd: list[str], description: str, capture: bool = False) -> tuple[bool, str]:
    """Run a command with status output (scenario-specific: uses PROJECT_ROOT as cwd)."""
    return run_cmd(cmd, description, capture=capture, cwd=PROJECT_ROOT)


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


# === Gensim Import Utilities ===


def ensure_gensim_cache() -> Path:
    """Clone gensim repo to cache if not present, return cache path."""
    if GENSIM_CACHE_DIR.exists():
        return GENSIM_CACHE_DIR

    print("Cloning gensim (first-time setup)...")
    GENSIM_CACHE_DIR.parent.mkdir(parents=True, exist_ok=True)

    result = subprocess.run(
        [
            "git",
            "clone",
            "--depth",
            "1",
            "--branch",
            GENSIM_BRANCH,
            GENSIM_REPO_URL,
            str(GENSIM_CACHE_DIR),
        ],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0:
        print(f"Failed to clone gensim: {result.stderr}")
        raise RuntimeError("Failed to clone gensim repository")

    print(f"Cloned gensim to {GENSIM_CACHE_DIR}")
    return GENSIM_CACHE_DIR


def update_gensim_cache():
    """Update the cached gensim repo to latest."""
    if not GENSIM_CACHE_DIR.exists():
        ensure_gensim_cache()
        return

    print(f"Updating gensim cache from {GENSIM_BRANCH}...")

    # Fetch and reset to the branch
    fetch_result = subprocess.run(
        ["git", "-C", str(GENSIM_CACHE_DIR), "fetch", "origin", GENSIM_BRANCH],
        capture_output=True,
        text=True,
    )

    if fetch_result.returncode != 0:
        print(f"Failed to fetch: {fetch_result.stderr}")
        return

    reset_result = subprocess.run(
        ["git", "-C", str(GENSIM_CACHE_DIR), "reset", "--hard", f"origin/{GENSIM_BRANCH}"],
        capture_output=True,
        text=True,
    )

    if reset_result.returncode != 0:
        print(f"Failed to reset: {reset_result.stderr}")
        return

    print("Gensim cache updated")


def list_gensim_blueprints() -> str:
    """List available blueprints from cached gensim repo."""
    cache = ensure_gensim_cache()
    adapter = cache / "src" / "k8s-adapter" / "adapter.py"

    if not adapter.exists():
        raise FileNotFoundError(f"k8s-adapter not found at {adapter}")

    result = subprocess.run(
        ["uv", "run", str(adapter), "list-blueprints"],
        capture_output=True,
        text=True,
        cwd=cache / "src" / "k8s-adapter",
    )

    if result.returncode != 0:
        raise RuntimeError(f"Failed to list blueprints: {result.stderr}")

    return result.stdout


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


def cmd_run(scenario_name: str, duration_minutes: int = 30):
    """Build images, deploy scenario to cluster, return run ID."""
    kube_context = get_kube_context()
    image_tag = get_image_tag()
    env = _get_env()
    backend = _get_backend()

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

    # Check environment is ready
    if env.mode == Mode.VM:
        if not check_lima_vm():
            print(f"Error: Lima VM '{LIMA_VM}' not running")
            print("  Start with: limactl start gadget-k8s-host")
            return 1
    else:
        if not env.has_docker:
            print("Error: Docker is not available")
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
        success, _ = scenario_run_cmd(
            ["docker", "build", "-t", image_name, str(component_dir)],
            f"Building {component}",
        )
        if not success:
            print(f"Failed to build {component}")
            return 1

        # Load image into Kind cluster (handles VM vs Direct mode)
        if not q_branch_dev.load_image(backend, cluster_name, image_name, env.mode):
            print(f"Failed to load {component}")
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

    # Deploy cleanup job for namespace-isolated scenarios
    # This job sleeps for the duration, then deletes the namespace
    if uses_namespace:
        print("Deploying cleanup job...", end=" ", flush=True)
        cleanup_manifest = f"""---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: scenario-cleanup
  namespace: {namespace}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: scenario-cleanup-{run_id}
rules:
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: scenario-cleanup-{run_id}
subjects:
- kind: ServiceAccount
  name: scenario-cleanup
  namespace: {namespace}
roleRef:
  kind: ClusterRole
  name: scenario-cleanup-{run_id}
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: batch/v1
kind: Job
metadata:
  name: scenario-cleanup
  namespace: {namespace}
  labels:
    fgm-run: "{run_id}"
spec:
  ttlSecondsAfterFinished: 10
  template:
    spec:
      serviceAccountName: scenario-cleanup
      restartPolicy: Never
      containers:
      - name: cleanup
        image: bitnami/kubectl:latest
        command:
        - /bin/sh
        - -c
        - |
          echo "Scenario cleanup job started at $(date)"
          echo "Will delete namespace {namespace} after {duration_minutes} minutes"
          sleep {duration_minutes * 60}
          echo "Duration expired. Deleting namespace {namespace}..."
          kubectl delete namespace {namespace}
          echo "Cleanup complete at $(date)"
"""
        result = subprocess.run(
            ["kubectl", "apply", "-f", "-", "--context", kube_context],
            input=cleanup_manifest,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            print("FAILED")
            print(f"  Warning: Cleanup job failed to deploy: {result.stderr}")
            print("  Scenario will run indefinitely until manually stopped")
        else:
            print("done")

    # Save metadata
    metadata = {
        "run_id": run_id,
        "scenario": scenario_name,
        "started_at": datetime.now().isoformat(),
        "duration_minutes": duration_minutes,
        "components": components,
        "cluster": get_cluster_name(),
        "namespace": namespace,
        "is_gensim": is_gensim,
        "uses_namespace_isolation": uses_namespace,
    }
    (run_dir / "metadata.json").write_text(json.dumps(metadata, indent=2))

    # Wait for pods to be ready
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

    if uses_namespace:
        print(f"\nScenario will auto-stop after {duration_minutes} minutes (via cleanup job)")
    else:
        print("\nScenario will run indefinitely until manually stopped")
        print("  (Auto-cleanup only works for namespace-isolated scenarios)")

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
    """Export scenario data as both raw parquet file and HTML viewer with embedded parquet."""
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

    # Export all available metrics (not just dashboard panels)

    # Use scenario time range with padding (not 'all' which loads days of data)
    # Parse scenario start time (treat as UTC)
    from datetime import datetime, timedelta, timezone

    start_dt = datetime.fromisoformat(metadata["started_at"]).replace(tzinfo=timezone.utc)

    # Get duration from metadata (defaults to 30 minutes for old runs)
    duration_minutes = metadata.get("duration_minutes", 30)

    # Add 60s padding before start, use metadata duration after start
    time_from = start_dt - timedelta(seconds=60)
    time_to = start_dt + timedelta(minutes=duration_minutes)

    # Convert to epoch milliseconds
    params["time_from_ms"] = str(int(time_from.timestamp() * 1000))
    params["time_to_ms"] = str(int(time_to.timestamp() * 1000))

    print(f"  Namespace: {params.get('namespace', '(all)')}")
    print(f"  Labels: {params.get('labels', '(none)')}")
    print(f"  Duration: {duration_minutes} minutes")
    print(f"  Time range: {time_from.strftime('%H:%M:%S')} to {time_to.strftime('%H:%M:%S')}")
    print("  Metrics: (all)")

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
        # Fetch all available metrics (for the picker dropdown)
        metrics_url = f"http://localhost:{viewer_port}/api/metrics"
        print("Fetching metric list...", end=" ", flush=True)
        try:
            response = urlopen(metrics_url, timeout=30)
            metrics_data = json.loads(response.read().decode())
            all_metrics = [m["name"] for m in metrics_data.get("metrics", [])]
            print(f"done ({len(all_metrics)} metrics)")
        except URLError as e:
            print(f"FAILED ({e}), continuing with dashboard metrics only")
            all_metrics = None

        # Call export API with all metrics (labels filter still applied)
        url = f"http://localhost:{viewer_port}/api/export?{urlencode(params)}"
        print(f"Fetching data from {url}...", end=" ", flush=True)

        try:
            response = urlopen(url, timeout=300)  # 5 minutes for streaming export
            parquet_data = response.read()
            print(f"done ({len(parquet_data)} bytes)")
        except URLError as e:
            print("FAILED")
            print(f"  Error: {e}")
            return 1

        # Determine output paths
        base_name = output or f"scenario-results-{run_id}"
        # Strip extension if provided
        if base_name.endswith(".html") or base_name.endswith(".parquet"):
            base_name = base_name.rsplit(".", 1)[0]
        parquet_path = f"{base_name}.parquet"
        html_path = f"{base_name}.html"

        # Write raw parquet file
        print(f"Writing parquet to {parquet_path}...", end=" ", flush=True)
        Path(parquet_path).write_bytes(parquet_data)
        parquet_size_mb = len(parquet_data) / (1024 * 1024)
        print(f"done ({parquet_size_mb:.1f} MB)")

        # Generate and write HTML with embedded parquet
        print("Generating HTML...", end=" ", flush=True)
        html = generate_export_html(parquet_data, dashboard, metadata, all_metrics)
        print("done")

        print(f"Writing HTML to {html_path}...", end=" ", flush=True)
        Path(html_path).write_text(html)
        html_size_mb = len(html) / (1024 * 1024)
        print(f"done ({html_size_mb:.1f} MB)")

        print("\nExported:")
        print(f"  Parquet: {parquet_path} ({parquet_size_mb:.1f} MB)")
        print(f"  HTML:    {html_path} ({html_size_mb:.1f} MB)")
        print("  Open HTML in browser to view (works offline)")

        return 0

    finally:
        # Stop port-forward
        pf_proc.terminate()
        pf_proc.wait()


def cmd_import(blueprint_name: str | None, list_only: bool, update: bool):
    """Import a scenario from a gensim blueprint."""
    # Handle --update flag
    if update:
        update_gensim_cache()
        return 0

    # Handle --list flag
    if list_only:
        try:
            output = list_gensim_blueprints()
            print(output)
            return 0
        except Exception as e:
            print(f"Error listing blueprints: {e}")
            return 1

    # Import specific blueprint
    if blueprint_name is None:
        print("Error: Blueprint name required")
        print("  Usage: ./scenario.py import <blueprint-name>")
        print("  List available: ./scenario.py import --list")
        return 1

    try:
        cache = ensure_gensim_cache()
    except RuntimeError as e:
        print(f"Error: {e}")
        return 1

    # Validate blueprint exists
    blueprint_dir = cache / "blueprints" / blueprint_name
    blueprint_path = blueprint_dir / f"{blueprint_name}.spec.yaml"
    if not blueprint_path.exists():
        print(f"Error: Blueprint '{blueprint_name}' not found")
        print(f"  Expected: {blueprint_path}")
        print("  List available: ./scenario.py import --list")
        return 1

    adapter = cache / "src" / "k8s-adapter" / "adapter.py"
    if not adapter.exists():
        print(f"Error: k8s-adapter not found at {adapter}")
        return 1

    # Find all disruption files for this blueprint
    disruption_files = list(blueprint_dir.glob(f"{blueprint_name}.disruption-*.yaml"))

    scenarios_to_generate = []

    # Base scenario (no disruption)
    scenarios_to_generate.append(
        {
            "name": blueprint_name,
            "blueprint": blueprint_path,
            "disruption": None,
        }
    )

    # Disruption scenarios
    for disruption_file in disruption_files:
        # Read the disruption name from the YAML file
        try:
            with open(disruption_file) as f:
                disruption_data = yaml.safe_load(f)
                # Use the 'name' field from the disruption YAML
                # This is the authoritative name that k8s-adapter uses
                scenario_name = disruption_data.get("name", disruption_file.stem)
        except Exception as e:
            print(f"Warning: Could not read disruption name from {disruption_file}: {e}")
            # Fallback to filename-based naming
            scenario_name = disruption_file.stem.replace(f"{blueprint_name}.disruption-", "")

        scenarios_to_generate.append(
            {
                "name": scenario_name,
                "blueprint": blueprint_path,
                "disruption": disruption_file,
            }
        )

    print(f"Importing blueprint '{blueprint_name}' with {len(disruption_files)} disruption(s)...")
    imported_scenarios = []

    for scenario in scenarios_to_generate:
        scenario_name = scenario["name"]
        output_dir = SCENARIOS_DIR / scenario_name

        disruption_label = ""
        if scenario["disruption"]:
            disruption_label = f" (with disruption: {scenario['disruption'].name})"

        print(f"\n  Generating scenario '{scenario_name}'{disruption_label}...")

        # Run k8s-adapter generate in a temp directory first
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp_output = Path(tmp_dir) / scenario_name

            cmd = [
                "uv",
                "run",
                str(adapter),
                "generate",
                "--blueprint",
                str(scenario["blueprint"]),
                "--output",
                str(tmp_output),
            ]

            if scenario["disruption"]:
                cmd.extend(["--disruption", str(scenario["disruption"])])

            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                cwd=cache / "src" / "k8s-adapter",
            )

            if result.returncode != 0:
                print(f"    FAILED to generate scenario '{scenario_name}'")
                print(f"      Error: {result.stderr}")
                if result.stdout:
                    print(f"      Output: {result.stdout}")
                continue

            # Copy to scenarios dir (overwrite if exists)
            if output_dir.exists():
                shutil.rmtree(output_dir)

            shutil.copytree(tmp_output, output_dir)
            imported_scenarios.append(scenario_name)
            print(f"    ✓ Generated scenarios/{scenario_name}/")

    if not imported_scenarios:
        print("\nNo scenarios were successfully imported")
        return 1

    print(f"\n✓ Successfully imported {len(imported_scenarios)} scenario(s):")
    for name in imported_scenarios:
        print(f"  - {name}")

    print("\nDeploy with:")
    print(f"  ./scenario.py run {imported_scenarios[0]}  # Base scenario")
    if len(imported_scenarios) > 1:
        print(f"  ./scenario.py run {imported_scenarios[1]}  # Example disruption")

    return 0


def generate_export_html(parquet_data: bytes, dashboard: dict, metadata: dict, all_metrics: list = None) -> str:
    """Generate a self-contained HTML file with embedded parquet data.

    Uses the main viewer (index.html) as base and bundles all JS modules
    using esbuild for a truly self-contained export that reuses all the
    existing viewer code.

    Args:
        parquet_data: The parquet file data to embed
        dashboard: Dashboard configuration dict
        metadata: Scenario run metadata
        all_metrics: Optional list of all available metric names (for picker dropdown)
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

    # Add all available metrics list (for picker dropdown even if data not exported)
    if all_metrics:
        metrics_script = f'<script id="all-metrics" type="application/json">{json.dumps(all_metrics)}</script>'
        html = html.replace("</body>", f"    {metrics_script}\n</body>")

    return html


def cmd_disrupt(disruption_type: str, run_id: str | None):
    """Apply a disruption to a running scenario."""
    if run_id is None:
        run_id = get_latest_scenario_run()
        if run_id is None:
            print("No running scenarios found")
            return 1

    namespace = f"fgm-run-{run_id}"

    # Map disruption types to implementation functions
    disruption_map = {
        "network-latency": apply_network_latency,
        "cpu-stress": apply_cpu_stress,
        "memory-pressure": apply_memory_pressure,
    }

    if disruption_type not in disruption_map:
        print(f"Error: Unknown disruption type '{disruption_type}'")
        print(f"Available types: {', '.join(disruption_map.keys())}")
        return 1

    print(f"Applying '{disruption_type}' disruption to run {run_id}...")
    try:
        result = disruption_map[disruption_type](namespace)
        if result == 0:
            print("✓ Disruption applied successfully")
        return result
    except Exception as e:
        print(f"Failed to apply disruption: {e}")
        return 1


def apply_network_latency(namespace: str):
    """Apply network latency to Redis pod using tc (traffic control)."""
    # Find Redis deployment
    print("  Adding NET_ADMIN capability to Redis deployment...")

    # Patch deployment to add NET_ADMIN capability
    patch = '''
    {
      "spec": {
        "template": {
          "spec": {
            "containers": [{
              "name": "todo-redis",
              "securityContext": {
                "capabilities": {
                  "add": ["NET_ADMIN"]
                }
              }
            }]
          }
        }
      }
    }
    '''

    patch_result = subprocess.run(
        ["kubectl", "patch", "deployment", "todo-redis", "-n", namespace, "--type=strategic", "-p", patch],
        capture_output=True,
        text=True,
    )

    if patch_result.returncode != 0:
        print(f"  Error patching deployment: {patch_result.stderr}")
        return 1

    # Wait for new pod to be ready
    print("  Waiting for pod to restart with NET_ADMIN capability...")
    subprocess.run(
        ["kubectl", "rollout", "status", "deployment/todo-redis", "-n", namespace, "--timeout=60s"],
        capture_output=True,
    )

    # Find the new Redis pod
    result = subprocess.run(
        ["kubectl", "get", "pods", "-n", namespace, "-l", "app=todo-redis", "-o", "jsonpath={.items[0].metadata.name}"],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0 or not result.stdout:
        print(f"Error: Could not find Redis pod in namespace {namespace}")
        return 1

    pod_name = result.stdout.strip()
    print(f"  Target: {pod_name}")

    # Install iproute2 if needed (has tc command)
    print("  Installing iproute2...")
    subprocess.run(
        [
            "kubectl",
            "exec",
            "-n",
            namespace,
            pod_name,
            "--",
            "sh",
            "-c",
            "apk add --no-cache iproute2 2>/dev/null || true",
        ],
        capture_output=True,
        text=True,
    )

    # Apply network latency: 200ms ± 50ms
    print("  Applying 200ms ± 50ms latency...")
    tc_result = subprocess.run(
        [
            "kubectl",
            "exec",
            "-n",
            namespace,
            pod_name,
            "--",
            "tc",
            "qdisc",
            "add",
            "dev",
            "eth0",
            "root",
            "netem",
            "delay",
            "200ms",
            "50ms",
        ],
        capture_output=True,
        text=True,
    )

    if tc_result.returncode != 0:
        print(f"  Error applying tc: {tc_result.stderr}")
        return 1

    print(f"  Network latency active on {pod_name}")
    return 0


def apply_cpu_stress(namespace: str):
    """Apply CPU stress to backend pod."""
    # Find backend pod
    result = subprocess.run(
        [
            "kubectl",
            "get",
            "pods",
            "-n",
            namespace,
            "-l",
            "app=todo-backend",
            "-o",
            "jsonpath={.items[0].metadata.name}",
        ],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0 or not result.stdout:
        print(f"Error: Could not find backend pod in namespace {namespace}")
        return 1

    pod_name = result.stdout.strip()
    print(f"  Target: {pod_name}")

    # Inject CPU stress via a background process
    print("  Starting CPU stress (2 cores)...")
    stress_result = subprocess.run(
        ["kubectl", "exec", "-n", namespace, pod_name, "--", "sh", "-c", "nohup sh -c 'while :; do :; done' &"],
        capture_output=True,
        text=True,
    )

    if stress_result.returncode != 0:
        print(f"  Error starting stress: {stress_result.stderr}")
        return 1

    print(f"  CPU stress active on {pod_name}")
    return 0


def apply_memory_pressure(namespace: str):
    """Apply memory pressure to Redis pod."""
    # Reduce memory limit on Redis deployment
    print("  Reducing Redis memory limit to 32Mi...")
    patch_result = subprocess.run(
        [
            "kubectl",
            "patch",
            "deployment",
            "todo-redis",
            "-n",
            namespace,
            "--type=json",
            "-p",
            '[{"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/memory", "value": "32Mi"}]',
        ],
        capture_output=True,
        text=True,
    )

    if patch_result.returncode != 0:
        print(f"  Error patching deployment: {patch_result.stderr}")
        return 1

    print("  Memory limit reduced, pod will restart")
    return 0


def main():
    parser = argparse.ArgumentParser(
        description="Scenario runner for fine-grained-monitor incident reproduction",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  ./scenario.py list                   # List available scenarios
  ./scenario.py run sigpipe-crash      # Run scenario for 30 min (auto-stops)
  ./scenario.py run todo-app --duration 60  # Run scenario for 60 min (auto-stops)
  ./scenario.py status                 # Check status of latest run
  ./scenario.py status a1b2c3d4        # Check status of specific run
  ./scenario.py logs                   # Show logs from latest run
  ./scenario.py logs -c victim-app     # Show logs from specific container
  ./scenario.py stop a1b2c3d4          # Stop and clean up scenario
  ./scenario.py disrupt network-latency  # Apply network latency to latest run
  ./scenario.py disrupt cpu-stress a1b2c3d4  # Apply CPU stress to specific run
  ./scenario.py export                 # Export latest run as self-contained HTML
  ./scenario.py export a1b2c3d4 -o results.html  # Export specific run
  ./scenario.py import --list          # List blueprints available for import
  ./scenario.py import todo-app        # Import todo-app blueprint from gensim

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
    run_parser.add_argument(
        "--duration", type=int, default=30, help="Expected scenario duration in minutes (default: 30)"
    )

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

    # disrupt <type> [run_id]
    disrupt_parser = subparsers.add_parser("disrupt", help="Apply disruption to running scenario")
    disrupt_parser.add_argument("type", type=str, help="Disruption type (network-latency, cpu-stress, memory-pressure)")
    disrupt_parser.add_argument("run_id", type=str, nargs="?", default=None, help="Run ID (default: latest)")

    # export [run_id]
    export_parser = subparsers.add_parser("export", help="Export scenario as parquet and HTML files")
    export_parser.add_argument("run_id", type=str, nargs="?", default=None, help="Run ID (default: latest)")
    export_parser.add_argument(
        "-o", "--output", type=str, default=None, help="Output base name (creates .parquet and .html)"
    )

    # import <name> | --list | --update
    import_parser = subparsers.add_parser("import", help="Import scenario from gensim blueprint")
    import_parser.add_argument("name", type=str, nargs="?", default=None, help="Blueprint name to import")
    import_parser.add_argument("--list", action="store_true", dest="list_blueprints", help="List available blueprints")
    import_parser.add_argument("--update", action="store_true", help="Update gensim cache")

    args = parser.parse_args()

    if args.command == "list":
        sys.exit(cmd_list())
    elif args.command == "run":
        sys.exit(cmd_run(args.name, args.duration))
    elif args.command == "status":
        sys.exit(cmd_status(args.run_id))
    elif args.command == "stop":
        sys.exit(cmd_stop(args.run_id))
    elif args.command == "logs":
        sys.exit(cmd_logs(args.run_id, args.container))
    elif args.command == "disrupt":
        sys.exit(cmd_disrupt(args.type, args.run_id))
    elif args.command == "export":
        sys.exit(cmd_export(args.run_id, args.output))
    elif args.command == "import":
        sys.exit(cmd_import(args.name, args.list_blueprints, args.update))


if __name__ == "__main__":
    main()
