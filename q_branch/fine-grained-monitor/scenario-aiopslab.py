#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     # Using fork with --context fix for parallel cluster support
#     # TODO: Switch to upstream once PR is merged: https://github.com/microsoft/AIOpsLab
#     "aiopslab @ git+https://github.com/scottopell/AIOpsLab.git@parallel-cluster-support",
# ]
# ///
"""
AIOpsLab scenario runner for fine-grained-monitor clusters.

This script runs AIOpsLab problems against the fgm Kind cluster to collect
telemetry data for analysis. It's designed to work with the per-worktree
cluster isolation provided by dev.py.

Usage:
    ./scenario-aiopslab.py list                    # List available problems
    ./scenario-aiopslab.py list --type detection   # Filter by task type
    ./scenario-aiopslab.py info <problem_id>       # Show problem details
    ./scenario-aiopslab.py start <problem_id>      # Start problem in background
    ./scenario-aiopslab.py wait <guid>             # Wait for run to complete
    ./scenario-aiopslab.py runs                    # List recent runs

Prerequisites:
    - Cluster must be running: ./dev.py cluster deploy
    - AIOpsLab applications: ~/dev/AIOpsLab/aiopslab-applications
"""

import argparse
import base64
import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import time
import uuid
from datetime import datetime, timedelta
from pathlib import Path

# Project root is where this script lives
PROJECT_ROOT = Path(__file__).parent.resolve()
DEV_DIR = PROJECT_ROOT / ".dev"
AIOPS_DIR = DEV_DIR / "aiops"
AIOPS_RETENTION_DAYS = 7

# Lima VM name (shared with dev.py)
LIMA_VM = "gadget-k8s-host"


# --- Worktree identification functions ---


def get_worktree_id() -> str:
    """Get worktree identifier from directory basename.

    Uses the parent directory of the fine-grained-monitor project root,
    e.g., /Users/scott/dev/beta-datadog-agent/q_branch/fine-grained-monitor
    -> "beta-datadog-agent"
    """
    # PROJECT_ROOT is fine-grained-monitor, parent is q_branch, grandparent is worktree
    return PROJECT_ROOT.parent.parent.name


def get_cluster_name() -> str:
    """Get Kind cluster name for this worktree."""
    return f"fgm-{get_worktree_id()}"


def get_kube_context() -> str:
    """Get kubectl context name for this worktree's cluster."""
    return f"kind-{get_cluster_name()}"


def is_process_running(pid: int) -> bool:
    """Check if a process with given PID is running."""
    try:
        os.kill(pid, 0)
        return True
    except (OSError, ProcessLookupError):
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


def ensure_dev_dir():
    """Create .dev directory if needed."""
    DEV_DIR.mkdir(exist_ok=True)


def calculate_port() -> int:
    """Calculate unique viewer port based on checkout path for worktree support."""
    path_hash = hashlib.md5(str(PROJECT_ROOT).encode()).hexdigest()
    offset = int(path_hash[:8], 16) % 1000
    return 8050 + offset


# Problem ID prefix -> Kubernetes namespace mapping
# Based on AIOpsLab service/metadata/*.json files
PROBLEM_NAMESPACE_MAP = {
    "astronomy_shop": "astronomy-shop",
    "hotel_res": "test-hotel-reservation",
    "social_net": "test-social-network",
    "flower": "docker",
    "train_ticket": "train-ticket",
    "flight_ticket": "openwhisk",
    "tidb": "tidb-cluster",
    # Chaos mesh problems (hotel reservation based)
    "container_kill": "test-hotel-reservation",
    "pod_failure": "test-hotel-reservation",
    "pod_kill": "test-hotel-reservation",
    "network_loss": "test-hotel-reservation",
    "network_delay": "test-hotel-reservation",
    "noop_detection_hotel": "test-hotel-reservation",
    "noop_detection_social": "test-social-network",
    "noop_detection_astronomy": "astronomy-shop",
    # K8s problems (social network based)
    "k8s_target_port": "test-social-network",
    "auth_miss_mongodb": "test-social-network",
    "revoke_auth_mongodb": "test-social-network",
    "user_unregistered_mongodb": "test-social-network",
    "misconfig_app": "test-hotel-reservation",
    "scale_pod": "test-social-network",
    "assign_to_non_existent_node": "test-social-network",
    "redeploy_without_PV": "test-hotel-reservation",
    "wrong_bin_usage": "test-hotel-reservation",
}


def get_namespace_for_problem(problem_id: str) -> str | None:
    """Get the Kubernetes namespace for a given problem ID.

    Returns None if the namespace cannot be determined.
    """
    # Check each prefix in order of specificity (longer prefixes first)
    for prefix in sorted(PROBLEM_NAMESPACE_MAP.keys(), key=len, reverse=True):
        if problem_id.startswith(prefix):
            return PROBLEM_NAMESPACE_MAP[prefix]
    return None


def generate_dashboard_config(problem_id: str, namespace: str) -> dict:
    """Generate a dashboard configuration for viewing the scenario."""
    # Extract a readable name from problem_id
    # e.g., "astronomy_shop_ad_service_failure-detection-1" -> "Ad Service Failure"
    name_part = problem_id.split("-")[0]  # Get part before task type
    name_part = name_part.replace("astronomy_shop_", "").replace("_", " ").title()

    return {
        "schema_version": 1,
        "name": f"AIOpsLab: {name_part}",
        "description": f"AIOpsLab scenario {problem_id}",
        "containers": {
            "namespace": namespace,
        },
        "time_range": {
            "mode": "from_containers",
            "padding_seconds": 60,
        },
        "panels": [
            {"metric": "cpu_percentage", "title": "CPU Usage %"},
            {"metric": "cgroup.v2.memory.current", "title": "Memory"},
            {"metric": "cgroup.v2.pids.current", "title": "PIDs"},
            {"metric": "cgroup.v2.io.stat.rbytes", "title": "Disk Read"},
            {"metric": "cgroup.v2.io.stat.wbytes", "title": "Disk Write"},
        ],
    }


def get_viewer_url(problem_id: str, namespace: str) -> str | None:
    """Generate viewer URL with inline dashboard config for the scenario."""
    if not namespace:
        return None

    dashboard = generate_dashboard_config(problem_id, namespace)
    dashboard_json = json.dumps(dashboard)
    dashboard_b64 = base64.b64encode(dashboard_json.encode()).decode()

    port = calculate_port()
    return f"http://localhost:{port}/?dashboard_inline={dashboard_b64}"


# --- AIOps helper functions ---


def get_aiops_cluster_env() -> dict:
    """Get environment variables for AIOpsLab to use this worktree's cluster."""
    return {
        **os.environ,
        "AIOPSLAB_CLUSTER": get_cluster_name(),
        # Point to local aiopslab-applications submodule (not included in pip/uv git installs)
        "AIOPSLAB_APPLICATIONS": os.path.expanduser("~/dev/AIOpsLab/aiopslab-applications"),
    }


def ensure_aiopslab_config():
    """Ensure aiopslab config.yml exists in the installed package location.

    AIOpsLab requires a config.yml file but only ships config.yml.example.
    This creates the config file if missing before any aiopslab imports.
    """
    # Find aiopslab package location
    spec = importlib.util.find_spec("aiopslab")
    if spec is None or spec.origin is None:
        return  # Package not installed, let import fail naturally

    pkg_dir = Path(spec.origin).parent
    config_path = pkg_dir / "config.yml"

    if config_path.exists():
        return  # Already configured

    # Create minimal config for Kind clusters
    config_content = """\
# Auto-generated config for Kind cluster usage
k8s_host: kind
k8s_user: ""
ssh_key_path: ~/.ssh/id_rsa
data_dir: data
qualitative_eval: false
print_session: false
"""
    config_path.write_text(config_content)


def cleanup_old_aiops_runs():
    """Remove aiops runs older than AIOPS_RETENTION_DAYS."""
    if not AIOPS_DIR.exists():
        return

    cutoff = datetime.now() - timedelta(days=AIOPS_RETENTION_DAYS)
    removed = 0

    for run_dir in AIOPS_DIR.iterdir():
        if not run_dir.is_dir():
            continue

        try:
            mtime = datetime.fromtimestamp(run_dir.stat().st_mtime)
            if mtime < cutoff:
                import shutil

                shutil.rmtree(run_dir)
                removed += 1
        except (OSError, ValueError):
            continue

    if removed > 0:
        print(f"Cleaned up {removed} aiops run(s) older than {AIOPS_RETENTION_DAYS} days")


def get_aiops_run_dir(guid: str) -> Path:
    """Get the directory for an aiops run."""
    return AIOPS_DIR / guid


def get_aiops_pid(guid: str) -> int | None:
    """Get PID of a running aiops session, handling stale PIDs."""
    run_dir = get_aiops_run_dir(guid)
    pid_file = run_dir / "aiops.pid"

    if not pid_file.exists():
        return None

    try:
        pid = int(pid_file.read_text().strip())
    except (ValueError, OSError):
        return None

    if is_process_running(pid):
        return pid

    return None


def is_aiops_complete(guid: str) -> bool:
    """Check if an aiops run has completed."""
    run_dir = get_aiops_run_dir(guid)

    pid_file = run_dir / "aiops.pid"
    if not pid_file.exists():
        return (run_dir / "run_info.json").exists()

    pid = get_aiops_pid(guid)
    return pid is None


# --- AIOps commands ---


def cmd_aiops_list(task_type: str | None):
    """List available AIOpsLab problems."""
    ensure_aiopslab_config()
    from aiopslab.orchestrator.problems.registry import ProblemRegistry

    registry = ProblemRegistry()
    problems = registry.get_problem_ids(task_type)

    # Group by prefix for display
    grouped: dict[str, list[str]] = {}
    for pid in sorted(problems):
        prefix = pid.rsplit("-", 2)[0] if "-" in pid else pid
        grouped.setdefault(prefix, []).append(pid)

    print(f"Available AIOpsLab problems ({len(problems)} total):\n")

    if task_type:
        print(f"  Filtered by task type: {task_type}\n")

    for prefix, pids in sorted(grouped.items()):
        print(f"  {prefix}:")
        for pid in pids:
            # Extract task type from pid
            task = (
                "detection"
                if "detection" in pid
                else "localization"
                if "localization" in pid
                else "analysis"
                if "analysis" in pid
                else "mitigation"
                if "mitigation" in pid
                else "other"
            )
            print(f"    - {pid} [{task}]")
        print()

    print("Task type counts:")
    print(f"  detection:    {len([p for p in problems if 'detection' in p])}")
    print(f"  localization: {len([p for p in problems if 'localization' in p])}")
    print(f"  analysis:     {len([p for p in problems if 'analysis' in p])}")
    print(f"  mitigation:   {len([p for p in problems if 'mitigation' in p])}")

    return 0


def cmd_aiops_info(problem_id: str):
    """Show info about a specific AIOpsLab problem."""
    ensure_aiopslab_config()
    from aiopslab.orchestrator.problems.registry import ProblemRegistry

    registry = ProblemRegistry()

    if problem_id not in registry.get_problem_ids():
        print(f"Error: Problem '{problem_id}' not found")
        print("\nUse './scenario-aiopslab.py list' to see available problems")
        return 1

    # Extract task type from problem_id
    task_type = (
        "detection"
        if "detection" in problem_id
        else "localization"
        if "localization" in problem_id
        else "analysis"
        if "analysis" in problem_id
        else "mitigation"
        if "mitigation" in problem_id
        else "unknown"
    )

    namespace = get_namespace_for_problem(problem_id)

    print(f"Problem: {problem_id}")
    print(f"Task type: {task_type}")
    print(f"Deployment: {registry.get_problem_deployment(problem_id)}")
    print(f"Namespace: {namespace or 'unknown'}")
    print()

    print("Note: Use './scenario-aiopslab.py start <problem_id>' to deploy and run")
    print("      The application will be deployed to the cluster at start time.")

    # Show viewer URL preview
    viewer_url = get_viewer_url(problem_id, namespace)
    if viewer_url:
        print("\nViewer URL (after deployment):")
        print(f"  {viewer_url}")

    return 0


def cmd_aiops_start(problem_id: str, max_steps: int):
    """Start an AIOpsLab problem in the background."""
    ensure_aiopslab_config()
    from aiopslab.orchestrator.problems.registry import ProblemRegistry

    # Verify problem exists
    registry = ProblemRegistry()
    if problem_id not in registry.get_problem_ids():
        print(f"Error: Problem '{problem_id}' not found")
        return 1

    # Verify cluster is ready
    if not cluster_exists():
        print(f"Error: Cluster '{get_cluster_name()}' not found")
        print("  Run: ./dev.py cluster deploy")
        return 1

    # Clean up old runs
    cleanup_old_aiops_runs()

    # Generate GUID for this run
    guid = str(uuid.uuid4())[:8]
    run_dir = get_aiops_run_dir(guid)
    run_dir.mkdir(parents=True, exist_ok=True)

    # Get namespace for this problem
    namespace = get_namespace_for_problem(problem_id)

    # Save run metadata
    metadata = {
        "guid": guid,
        "problem_id": problem_id,
        "max_steps": max_steps,
        "cluster": get_cluster_name(),
        "namespace": namespace,
        "started_at": datetime.now().isoformat(),
        "status": "starting",
    }
    (run_dir / "run_info.json").write_text(json.dumps(metadata, indent=2))

    # Start in background using subprocess (fork pattern like bench)
    stdout_file = run_dir / "logs.stdout"
    stderr_file = run_dir / "logs.stderr"

    stdout_handle = stdout_file.open("w")
    stderr_handle = stderr_file.open("w")

    # Run this same script with a hidden _run command
    proc = subprocess.Popen(
        [
            sys.executable,
            __file__,
            "_run",
            guid,
            problem_id,
            str(max_steps),
        ],
        stdout=stdout_handle,
        stderr=stderr_handle,
        cwd=PROJECT_ROOT,
        env=get_aiops_cluster_env(),
        start_new_session=True,
    )

    # Write PID file
    (run_dir / "aiops.pid").write_text(str(proc.pid))

    print(f"Starting AIOpsLab scenario {guid}")
    print(f"  Problem:   {problem_id}")
    print(f"  Cluster:   {get_cluster_name()}")
    print(f"  Namespace: {namespace or 'unknown'}")
    print(f"  Max steps: {max_steps}")
    print(f"  Logs:      {run_dir}/logs.{{stdout,stderr}}")
    print(f"  Wait:      ./scenario-aiopslab.py wait {guid}")
    print(f"  Results available for {AIOPS_RETENTION_DAYS} days")

    # Print viewer URL if namespace is known
    viewer_url = get_viewer_url(problem_id, namespace)
    if viewer_url:
        print("\nViewer (after scenario deploys):")
        print(f"  {viewer_url}")
    else:
        print("\nNote: Could not determine namespace for dashboard filtering")

    return 0


def cmd_aiops_run_internal(guid: str, problem_id: str, max_steps: int):
    """Internal: Actually run the AIOpsLab problem (called by start)."""
    ensure_aiopslab_config()
    import asyncio

    from aiopslab.orchestrator import Orchestrator

    run_dir = get_aiops_run_dir(guid)

    # Update status
    metadata = json.loads((run_dir / "run_info.json").read_text())
    metadata["status"] = "running"
    (run_dir / "run_info.json").write_text(json.dumps(metadata, indent=2))

    class ObservationAgent:
        """Agent that observes system state and collects telemetry."""

        def __init__(self):
            self.step = 0
            self.observations = []

        async def get_action(self, state: str) -> str:
            self.step += 1
            print(f"[Step {self.step}] Received state ({len(state)} chars)")

            self.observations.append(
                {
                    "step": self.step,
                    "timestamp": datetime.now().isoformat(),
                    "state_preview": state[:500] if len(state) > 500 else state,
                }
            )

            # Save observations incrementally
            (run_dir / "observations.json").write_text(json.dumps(self.observations, indent=2))

            # Collect telemetry then submit
            if self.step == 1:
                return 'Action:```\nget_logs()\n```'
            elif self.step == 2:
                return 'Action:```\nget_metrics()\n```'
            elif self.step == 3:
                return 'Action:```\nget_traces()\n```'
            else:
                return 'Action:```\nsubmit("Observation complete")\n```'

    async def run():
        orch = Orchestrator(results_dir=str(run_dir))
        agent = ObservationAgent()
        orch.register_agent(agent, name="fgm-observer")

        print(f"Initializing problem: {problem_id}")
        problem_desc, instructions, apis = orch.init_problem(problem_id)

        # Save problem context
        context = {
            "problem_description": problem_desc,
            "instructions": instructions,
            "apis": {k: str(v) for k, v in apis.items()},
        }
        (run_dir / "problem_context.json").write_text(json.dumps(context, indent=2))

        print(f"Starting problem with max_steps={max_steps}")
        results = await orch.start_problem(max_steps=max_steps)

        return results

    try:
        results = asyncio.run(run())

        metadata["status"] = "completed"
        metadata["ended_at"] = datetime.now().isoformat()
        metadata["results"] = results.get("results", {})
        (run_dir / "run_info.json").write_text(json.dumps(metadata, indent=2))

        print(f"Problem completed: {results.get('results', {})}")
        return 0

    except Exception as e:
        metadata["status"] = "failed"
        metadata["ended_at"] = datetime.now().isoformat()
        metadata["error"] = str(e)
        (run_dir / "run_info.json").write_text(json.dumps(metadata, indent=2))

        print(f"Problem failed: {e}")
        import traceback

        traceback.print_exc()
        return 1

    finally:
        # Clean up PID file
        pid_file = run_dir / "aiops.pid"
        if pid_file.exists():
            pid_file.unlink()


def cmd_aiops_wait(guid: str):
    """Wait for an aiops run to complete and show results."""
    run_dir = get_aiops_run_dir(guid)

    if not run_dir.exists():
        print(f"Error: AIOps run '{guid}' not found")
        print("\nRecent runs:")
        cmd_aiops_runs()
        return 1

    pid_file = run_dir / "aiops.pid"
    stdout_file = run_dir / "logs.stdout"
    stderr_file = run_dir / "logs.stderr"

    if is_aiops_complete(guid):
        print(f"AIOps run {guid} already complete")
    else:
        pid = get_aiops_pid(guid)
        if pid:
            print(f"Waiting for aiops run {guid} (pid {pid})...")
            while is_process_running(pid):
                time.sleep(1)

        if pid_file.exists():
            pid_file.unlink()

        print(f"AIOps run {guid} complete")

    # Show results
    print("\n" + "=" * 60)
    print("Results:")
    print("=" * 60 + "\n")

    run_info_file = run_dir / "run_info.json"
    if run_info_file.exists():
        run_info = json.loads(run_info_file.read_text())
        print(f"Status:   {run_info.get('status', 'unknown')}")
        print(f"Problem:  {run_info.get('problem_id', 'unknown')}")
        print(f"Started:  {run_info.get('started_at', 'unknown')}")
        print(f"Ended:    {run_info.get('ended_at', 'unknown')}")
        if "results" in run_info:
            print(f"Results:  {json.dumps(run_info['results'], indent=2)}")
        if "error" in run_info:
            print(f"Error:    {run_info['error']}")

    print("\n" + "=" * 60)
    print("Logs (last 30 lines of stdout):")
    print("=" * 60 + "\n")

    if stdout_file.exists():
        lines = stdout_file.read_text().splitlines()
        for line in lines[-30:]:
            print(line)
    else:
        print("(no stdout)")

    if stderr_file.exists():
        stderr_content = stderr_file.read_text().strip()
        if stderr_content:
            print("\n" + "=" * 60)
            print("Stderr (last 20 lines):")
            print("=" * 60 + "\n")
            for line in stderr_content.splitlines()[-20:]:
                print(line)

    print(f"\nFull logs: {run_dir}/logs.{{stdout,stderr}}")
    return 0


def cmd_aiops_runs():
    """List recent aiops runs."""
    if not AIOPS_DIR.exists():
        print("No aiops runs found")
        return 0

    runs = []
    for run_dir in AIOPS_DIR.iterdir():
        if not run_dir.is_dir():
            continue

        run_info_file = run_dir / "run_info.json"
        if not run_info_file.exists():
            continue

        try:
            run_info = json.loads(run_info_file.read_text())
            run_info["_complete"] = is_aiops_complete(run_info.get("guid", ""))
            runs.append(run_info)
        except (json.JSONDecodeError, KeyError):
            continue

    if not runs:
        print("No aiops runs found")
        return 0

    runs.sort(key=lambda r: r.get("started_at", ""), reverse=True)

    print("Recent AIOps runs:\n")
    for run in runs[:10]:
        guid = run.get("guid", "???")
        problem = run.get("problem_id", "???")[:40]
        started = run.get("started_at", "???")[:19]
        status = run.get("status", "unknown")

        print(f"  {guid}  {status:<12}  {problem:<40}  {started}")

    if len(runs) > 10:
        print(f"\n  ... and {len(runs) - 10} more")

    return 0


def main():
    parser = argparse.ArgumentParser(
        description="AIOpsLab scenario runner for fine-grained-monitor",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""\
Examples:
  ./scenario-aiopslab.py list                         # List all problems
  ./scenario-aiopslab.py list --type detection        # List detection problems
  ./scenario-aiopslab.py info sock-shop-detection-1   # Show problem info
  ./scenario-aiopslab.py start sock-shop-detection-1  # Start scenario
  ./scenario-aiopslab.py wait abc12345                # Wait for completion
  ./scenario-aiopslab.py runs                         # List recent runs

Prerequisites:
  - Cluster must exist: ./dev.py cluster deploy
  - AIOpsLab apps at: ~/dev/AIOpsLab/aiopslab-applications
""",
    )

    subparsers = parser.add_subparsers(dest="command", required=True)

    # list
    list_parser = subparsers.add_parser("list", help="List available AIOpsLab problems")
    list_parser.add_argument(
        "--type",
        dest="task_type",
        choices=["detection", "localization", "analysis", "mitigation"],
        help="Filter by task type",
    )

    # info
    info_parser = subparsers.add_parser("info", help="Show info about a problem")
    info_parser.add_argument("problem_id", help="Problem ID to show info for")

    # start
    start_parser = subparsers.add_parser("start", help="Start a problem in background")
    start_parser.add_argument("problem_id", help="Problem ID to start")
    start_parser.add_argument(
        "--max-steps",
        type=int,
        default=10,
        help="Maximum steps before termination (default: 10)",
    )

    # wait
    wait_parser = subparsers.add_parser("wait", help="Wait for run to complete")
    wait_parser.add_argument("guid", help="Run GUID to wait for")

    # runs
    subparsers.add_parser("runs", help="List recent aiops runs")

    # _run (internal, hidden)
    internal_parser = subparsers.add_parser("_run", help=argparse.SUPPRESS)
    internal_parser.add_argument("guid")
    internal_parser.add_argument("problem_id")
    internal_parser.add_argument("max_steps", type=int)

    args = parser.parse_args()

    # Ensure .dev directory exists
    ensure_dev_dir()

    # Dispatch
    if args.command == "list":
        sys.exit(cmd_aiops_list(args.task_type))
    elif args.command == "info":
        sys.exit(cmd_aiops_info(args.problem_id))
    elif args.command == "start":
        sys.exit(cmd_aiops_start(args.problem_id, args.max_steps))
    elif args.command == "wait":
        sys.exit(cmd_aiops_wait(args.guid))
    elif args.command == "runs":
        sys.exit(cmd_aiops_runs())
    elif args.command == "_run":
        sys.exit(cmd_aiops_run_internal(args.guid, args.problem_id, args.max_steps))
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
