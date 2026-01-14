#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///
"""
Development workflow manager for fine-grained-monitor (fgm-*) binaries.

Binaries:
    fine-grained-monitor  - DaemonSet collecting 1Hz container metrics
    fgm-viewer            - Interactive web UI for viewing collected data
    fgm-consolidator      - Merges parquet files from multiple pods
    fgm-mcp-server        - MCP server for Claude Code / AI agent access

Local development:
    ./dev.py local build               # Build all release binaries
    ./dev.py local test                # Run tests
    ./dev.py local clippy              # Run clippy lints
    ./dev.py local viewer start        # Start fgm-viewer with data file
    ./dev.py local viewer stop         # Stop fgm-viewer
    ./dev.py local viewer status       # Check fgm-viewer status

Cluster deployment (Kind via Lima):
    ./dev.py cluster deploy            # Build image, load to Kind, restart pods
    ./dev.py cluster status            # Show cluster pod status
    ./dev.py cluster viewer            # Port-forward to viewer web UI
    ./dev.py cluster viewer stop       # Stop viewer port-forward
    ./dev.py cluster mcp setup         # One-time Claude Code integration setup
    ./dev.py cluster mcp start         # Start MCP port-forward
    ./dev.py cluster mcp stop          # Stop MCP port-forward

All resources deploy to the 'fine-grained-monitor' namespace.
"""

import argparse
import hashlib
import json
import os
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timedelta
from pathlib import Path

# Add q_branch to path for shared library imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from lib.k8s_backend import (
    Mode,
    Environment,
    VMBackend,
    DirectBackend,
    detect_environment,
    create_backend,
    is_process_running,
    check_health,
    format_uptime,
    run_cmd,
)
import dev as q_branch_dev

# Project root is where this script lives
PROJECT_ROOT = Path(__file__).parent.resolve()
DEV_DIR = PROJECT_ROOT / ".dev"
PID_FILE = DEV_DIR / "server.pid"
LOG_FILE = DEV_DIR / "server.log"
STATE_FILE = DEV_DIR / "state.json"
FORWARD_PID_FILE = DEV_DIR / "forward.pid"
FORWARD_LOG_FILE = DEV_DIR / "forward.log"
MCP_FORWARD_PID_FILE = DEV_DIR / "mcp_forward.pid"
MCP_FORWARD_LOG_FILE = DEV_DIR / "mcp_forward.log"
BENCH_DIR = DEV_DIR / "bench"

# Benchmark configuration
BENCH_DATA_DIR = PROJECT_ROOT / "testdata" / "bench" / "realistic1h"
BENCH_RETENTION_DAYS = 30

# Default data file (test data in testdata/)
DEFAULT_DATA = PROJECT_ROOT / "testdata" / "1hr.parquet"

# Binary path
BINARY = PROJECT_ROOT / "target" / "release" / "fgm-viewer"

# Cluster deployment config
IMAGE_NAME = "fine-grained-monitor"
LIMA_VM = q_branch_dev.LIMA_VM  # Use shared constant
POD_LABEL = "app=fine-grained-monitor"
NAMESPACE = "fine-grained-monitor"
# Note: IMAGE_TAG, KIND_CLUSTER, KUBE_CONTEXT are computed dynamically per-worktree

# Environment and backend are initialized lazily for cluster operations
_env: Environment | None = None
_backend: VMBackend | DirectBackend | None = None


def _get_env() -> Environment:
    """Get or initialize environment (lazy singleton)."""
    global _env
    if _env is None:
        _env = detect_environment()
    return _env


def _get_backend() -> VMBackend | DirectBackend:
    """Get or initialize backend (lazy singleton)."""
    global _backend
    if _backend is None:
        _backend = create_backend(_get_env(), LIMA_VM)
    return _backend


def _worktree_port_offset() -> int:
    """Calculate unique offset (0-499) based on checkout path for worktree support."""
    return q_branch_dev.calculate_port_offset(PROJECT_ROOT)


def calculate_local_viewer_port() -> int:
    """Calculate unique port for LOCAL viewer (range 8050-8549).

    This is for ./dev.py local viewer start - runs fgm-viewer binary directly.
    """
    return 8050 + _worktree_port_offset()


def calculate_cluster_viewer_port() -> int:
    """Calculate unique port for CLUSTER viewer port-forward (range 8550-9049).

    This is for ./dev.py cluster viewer start - forwards to pod in Kind cluster.
    Deliberately different from local viewer to avoid confusion.
    """
    return 8550 + _worktree_port_offset()


def calculate_mcp_port() -> int:
    """Calculate unique MCP port (range 9050-9549).

    This is for the MCP server port-forward from the cluster.
    """
    return 9050 + _worktree_port_offset()


# --- Worktree identification functions (FGM-specific wrappers) ---


def get_worktree_id() -> str:
    """Get worktree identifier from directory basename."""
    return q_branch_dev.get_worktree_id(PROJECT_ROOT)


def calculate_api_port() -> int:
    """Calculate unique API server port (6443-6447) based on worktree ID."""
    return q_branch_dev.calculate_api_port(get_worktree_id())


def get_cluster_name() -> str:
    """Get Kind cluster name for this worktree: fgm-{worktree_id}."""
    return q_branch_dev.get_cluster_name("fgm", get_worktree_id())


def get_kube_context() -> str:
    """Get kubectl context name for this worktree's cluster."""
    return q_branch_dev.get_kube_context(get_cluster_name())


def get_image_tag() -> str:
    """Get Docker image tag for this worktree."""
    return q_branch_dev.get_image_tag(get_worktree_id())


def get_data_dir() -> str:
    """Get data directory path inside containers for this worktree."""
    return q_branch_dev.get_data_dir("fine-grained-monitor", get_worktree_id())


def ensure_dev_dir():
    """Create .dev directory if needed."""
    DEV_DIR.mkdir(exist_ok=True)
    # Add to .gitignore if not present
    gitignore = PROJECT_ROOT / ".gitignore"
    if gitignore.exists():
        content = gitignore.read_text()
        if ".dev/" not in content and ".dev\n" not in content:
            with gitignore.open("a") as f:
                f.write("\n# dev.py state directory\n.dev/\n")


def read_state() -> dict | None:
    """Read state file if it exists."""
    if STATE_FILE.exists():
        try:
            return json.loads(STATE_FILE.read_text())
        except (OSError, json.JSONDecodeError):
            return None
    return None


def write_state(pid: int, port: int, data_file: str):
    """Write state file."""
    ensure_dev_dir()
    state = {
        "pid": pid,
        "port": port,
        "data_file": data_file,
        "start_time": time.time(),
    }
    STATE_FILE.write_text(json.dumps(state, indent=2))


def clear_state():
    """Remove state files."""
    for f in [PID_FILE, STATE_FILE]:
        if f.exists():
            f.unlink()


def is_fgm_viewer_process(pid: int) -> bool:
    """Check if PID is actually an fgm-viewer process."""
    try:
        # On macOS/Linux, check the process command
        result = subprocess.run(
            ["ps", "-p", str(pid), "-o", "comm="],
            capture_output=True,
            text=True,
        )
        comm = result.stdout.strip()
        return "fgm-viewer" in comm
    except Exception:
        return False


def get_running_pid() -> int | None:
    """Get PID of running server, handling stale PIDs."""
    state = read_state()
    if not state:
        return None

    pid = state.get("pid")
    if not pid:
        return None

    # Verify process is actually running and is fgm-viewer
    if is_process_running(pid) and is_fgm_viewer_process(pid):
        return pid

    # Stale PID - clean up
    clear_state()
    return None


def check_viewer_health(port: int, timeout: float = 1.0) -> bool:
    """Check if fgm-viewer is healthy via /api/health endpoint."""
    return check_health(f"http://127.0.0.1:{port}/api/health", timeout)


def build() -> bool:
    """Build the fgm-viewer binary. Returns True on success."""
    print("Building fgm-viewer...", end=" ", flush=True)
    start = time.time()

    result = subprocess.run(
        ["cargo", "build", "--release", "--bin", "fgm-viewer"],
        cwd=PROJECT_ROOT,
        capture_output=True,
        text=True,
    )

    elapsed = time.time() - start

    if result.returncode != 0:
        print("FAILED")
        print("\nBuild error:")
        print(result.stderr)
        return False

    print(f"done ({elapsed:.1f}s)")
    return True


def cmd_status():
    """Show server status."""
    port = calculate_local_viewer_port()
    pid = get_running_pid()
    state = read_state()

    if pid and state:
        uptime = format_uptime(state.get("start_time", time.time()))
        healthy = check_viewer_health(port)
        health_str = "ok" if healthy else "UNHEALTHY"

        print(f"Server: running (pid {pid}, uptime {uptime})")
        print(f"Health: {health_str}")
        print(f"Data:   {state.get('data_file', 'unknown')}")
        print(f"Web UI: http://127.0.0.1:{port}/")
        print(f"API:    http://127.0.0.1:{port}/api/")
        print(f"Logs:   {LOG_FILE}")
    else:
        print("Server: not running")
        if LOG_FILE.exists():
            print(f"Logs:   {LOG_FILE} (from last run)")
        return 1

    return 0


def cmd_start(data_file: str):
    """Build and start the server."""
    # Check if already running
    pid = get_running_pid()
    if pid:
        print(f"Error: Server already running (pid {pid})")
        print("Hint: Use './dev.py restart' to restart")
        return 1

    # Validate data file
    data_path = Path(data_file)
    if not data_path.is_absolute():
        print("Error: Data file must be an absolute path")
        print(f"Got: {data_file}")
        return 1

    if not data_path.exists():
        print(f"Error: Data file not found: {data_file}")
        return 1

    # Build
    if not build():
        return 1

    if not BINARY.exists():
        print(f"Error: Binary not found after build: {BINARY}")
        return 1

    # Start server
    port = calculate_local_viewer_port()
    print(f"Starting server with {data_path}...")

    ensure_dev_dir()
    log_handle = LOG_FILE.open("w")

    proc = subprocess.Popen(
        [str(BINARY), str(data_path), "--no-browser", "--port", str(port)],
        stdout=log_handle,
        stderr=subprocess.STDOUT,
        cwd=PROJECT_ROOT,
        start_new_session=True,  # Detach from terminal
    )

    # Write state
    write_state(proc.pid, port, str(data_path))

    # Wait for health check (3 minute timeout for large data files)
    file_size_mb = data_path.stat().st_size / (1024 * 1024)
    print(f"Loading {file_size_mb:.1f}MB parquet file...")
    start_wait = time.time()
    timeout_secs = 180  # 3 minutes
    check_interval = 0.5  # Check every 500ms
    last_status = 0  # Last time we printed status

    while (time.time() - start_wait) < timeout_secs:
        time.sleep(check_interval)

        # Check if process died
        if proc.poll() is not None:
            print(f"Server exited with code {proc.returncode}")
            print(f"Check logs: {LOG_FILE}")
            clear_state()
            return 1

        if check_viewer_health(port):
            elapsed = time.time() - start_wait
            print(f"Server ready ({elapsed:.1f}s)")
            break

        # Print status every 5 seconds
        elapsed = int(time.time() - start_wait)
        if elapsed > 0 and elapsed % 5 == 0 and elapsed != last_status:
            print(f"  Waiting for initial data load - {elapsed}s elapsed")
            last_status = elapsed
    else:
        elapsed = time.time() - start_wait
        print(f"Health check timed out after {elapsed:.0f}s")
        print(f"Check logs: {LOG_FILE}")
        # Don't kill it - maybe it's still loading
        print(f"\nServer may still be starting (pid {proc.pid})")
        print(f"Web UI: http://127.0.0.1:{port}/")
        return 1

    print(f"Server: running (pid {proc.pid})")
    print(f"Web UI: http://127.0.0.1:{port}/")
    print(f"API:    http://127.0.0.1:{port}/api/")
    print(f"Logs:   {LOG_FILE}")
    return 0


def cmd_stop():
    """Stop the running server."""
    pid = get_running_pid()
    if not pid:
        print("Server: not running")
        return 0

    print(f"Stopping server (pid {pid})...", end=" ", flush=True)

    try:
        os.kill(pid, signal.SIGTERM)

        # Wait for process to exit
        for _ in range(50):  # 5 seconds
            time.sleep(0.1)
            if not is_process_running(pid):
                break
        else:
            # Force kill
            print("forcing...", end=" ", flush=True)
            os.kill(pid, signal.SIGKILL)
            time.sleep(0.5)

        print("stopped")
        clear_state()
        return 0

    except ProcessLookupError:
        print("already stopped")
        clear_state()
        return 0
    except Exception as e:
        print(f"error: {e}")
        return 1


def cmd_restart(data_file: str | None):
    """Stop, rebuild, and start the server."""
    # Get current data file if not specified
    if data_file is None:
        state = read_state()
        if state and state.get("data_file"):
            data_file = state["data_file"]
        else:
            # Use default
            data_file = str(DEFAULT_DATA.resolve())

    # Stop if running
    pid = get_running_pid()
    if pid:
        ret = cmd_stop()
        if ret != 0:
            return ret

    # Start (includes build)
    return cmd_start(data_file)


# --- Local development commands (non-viewer) ---


def cmd_build():
    """Build all release binaries."""
    print("Building all release binaries...")
    start = time.time()

    result = subprocess.run(
        ["cargo", "build", "--release"],
        cwd=PROJECT_ROOT,
    )

    elapsed = time.time() - start

    if result.returncode != 0:
        print(f"\nBuild failed after {elapsed:.1f}s")
        return 1

    print(f"\nBuild complete ({elapsed:.1f}s)")

    # List built binaries
    target_dir = PROJECT_ROOT / "target" / "release"
    binaries = ["fine-grained-monitor", "fgm-viewer", "fgm-consolidator", "generate-bench-data"]
    print("\nBinaries:")
    for name in binaries:
        path = target_dir / name
        if path.exists():
            size_mb = path.stat().st_size / (1024 * 1024)
            print(f"  {name}: {size_mb:.1f}MB")

    return 0


def cmd_test():
    """Run tests."""
    print("Running tests...")

    result = subprocess.run(
        ["cargo", "test"],
        cwd=PROJECT_ROOT,
    )

    return result.returncode


def cmd_clippy():
    """Run clippy lints."""
    print("Running clippy...")

    result = subprocess.run(
        ["cargo", "clippy", "--all-targets", "--", "-D", "warnings"],
        cwd=PROJECT_ROOT,
    )

    return result.returncode


# --- Cluster deployment commands ---


def fgm_run_cmd(cmd: list[str], description: str, capture: bool = False) -> tuple[bool, str]:
    """Run a command with status output (FGM-specific: uses PROJECT_ROOT as cwd)."""
    return run_cmd(cmd, description, capture=capture, cwd=PROJECT_ROOT)


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


def merge_kubeconfig(cluster_name: str):
    """Merge cluster kubeconfig into ~/.kube/config."""
    backend = _get_backend()
    q_branch_dev.merge_kubeconfig(backend, cluster_name)


def get_kind_containers(exclude_cluster: str | None = None) -> list[str]:
    """Get names of all running Kind containers, optionally excluding a cluster."""
    backend = _get_backend()
    return q_branch_dev.get_kind_containers(backend, exclude_cluster)


def stop_containers(containers: list[str]) -> bool:
    """Stop Docker containers by name."""
    backend = _get_backend()
    return q_branch_dev.stop_containers(backend, containers)


def start_containers(containers: list[str]) -> bool:
    """Start Docker containers by name."""
    backend = _get_backend()
    return q_branch_dev.start_containers(backend, containers)


def create_cluster() -> bool:
    """Create Kind cluster for this worktree.

    Note: Kind multi-node cluster creation can fail if other Kind clusters
    are running concurrently due to resource contention during kubeadm init.
    This function temporarily stops other clusters during creation.
    """
    cluster_name = get_cluster_name()
    api_port = calculate_api_port()
    backend = _get_backend()

    # FGM-specific Kind config (single control-plane node)
    kind_config = f"""kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: {api_port}
nodes:
  - role: control-plane
  # TEMP: Disabled workers for single-node testing (kube-proxy debug)
  # - role: worker
  # - role: worker
"""

    # Use shared create_cluster with callback for container management
    success = q_branch_dev.create_cluster(
        backend,
        cluster_name,
        api_port,
        kind_config=kind_config,
        other_containers_callback=get_kind_containers,
    )

    if not success:
        return False

    # Merge kubeconfig
    print("Merging kubeconfig...", end=" ", flush=True)
    merge_kubeconfig(cluster_name)
    print("done")

    return True


def generate_manifest() -> str:
    """Generate DaemonSet manifest with worktree-specific values."""
    image_tag = get_image_tag()
    data_dir = get_data_dir()
    cluster_name = get_cluster_name()

    # Read template and substitute
    template_path = PROJECT_ROOT / "deploy" / "daemonset.yaml"
    manifest = template_path.read_text()

    # Replace placeholders
    manifest = manifest.replace("fine-grained-monitor:latest", f"fine-grained-monitor:{image_tag}")
    manifest = manifest.replace("/var/lib/fine-grained-monitor", data_dir)
    manifest = manifest.replace('value: "gadget-dev"', f'value: "{cluster_name}"')

    return manifest


def get_cluster_pods() -> list[str]:
    """Get list of fine-grained-monitor pod names."""
    kube_context = get_kube_context()
    result = subprocess.run(
        [
            "kubectl",
            "get",
            "pods",
            "-l",
            POD_LABEL,
            "-n",
            NAMESPACE,
            "--context",
            kube_context,
            "-o",
            "jsonpath={.items[*].metadata.name}",
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return []
    return result.stdout.strip().split()


def cmd_deploy():
    """Build docker image, load into Kind cluster, and restart pods."""
    cluster_name = get_cluster_name()
    kube_context = get_kube_context()
    image_tag = get_image_tag()
    image_full = f"{IMAGE_NAME}:{image_tag}"
    env = _get_env()
    backend = _get_backend()

    # Check environment is ready
    if env.mode == Mode.VM:
        print(f"Checking Lima VM ({LIMA_VM})...", end=" ", flush=True)
        if not check_lima_vm():
            print("NOT RUNNING")
            print(f"  Start it with: limactl start {LIMA_VM}")
            return 1
        print("running")
    else:
        print(f"Using direct mode (no VM needed)")
        if not env.has_docker:
            print("Error: Docker is not available")
            return 1

    # On-demand cluster creation
    if not cluster_exists():
        print(f"Cluster '{cluster_name}' not found, creating...")
        if not create_cluster():
            return 1
    else:
        print(f"Using existing cluster '{cluster_name}'")

    # Step 1: Build docker image
    success, _ = fgm_run_cmd(
        ["docker", "build", "-t", image_full, "."],
        f"Building docker image ({image_full})",
    )
    if not success:
        return 1

    # Step 2: Load image into Kind cluster (handles VM vs Direct mode)
    if not q_branch_dev.load_image(backend, cluster_name, image_full, env.mode):
        return 1

    # Step 4: Apply manifests (always, to pick up any changes)
    pods = get_cluster_pods()

    # Apply RBAC for DaemonSet (includes Namespace creation)
    rbac_path = PROJECT_ROOT / "deploy" / "rbac.yaml"
    if rbac_path.exists():
        subprocess.run(
            ["kubectl", "apply", "-f", str(rbac_path), "--context", kube_context],
            capture_output=True,
        )

    # Apply generated DaemonSet manifest via stdin
    manifest = generate_manifest()
    result = subprocess.run(
        ["kubectl", "apply", "-f", "-", "--context", kube_context],
        input=manifest,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(f"Failed to apply DaemonSet manifest: {result.stderr}")
        return 1

    # Apply MCP server manifest (REQ-MCP-006)
    mcp_path = PROJECT_ROOT / "deploy" / "mcp-server.yaml"
    if mcp_path.exists():
        mcp_manifest = mcp_path.read_text()
        # Replace image tag to match what we loaded
        mcp_manifest = mcp_manifest.replace("fine-grained-monitor:latest", f"fine-grained-monitor:{image_tag}")
        result = subprocess.run(
            ["kubectl", "apply", "-f", "-", "--context", kube_context],
            input=mcp_manifest,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            print(f"Failed to apply MCP server manifest: {result.stderr}")
            return 1

    if not pods:
        print("Manifests applied (first deploy)")
    else:
        # Delete existing pods to trigger restart with new image
        print(f"Restarting {len(pods)} DaemonSet pod(s)...", end=" ", flush=True)
        start = time.time()

        for pod in pods:
            subprocess.run(
                ["kubectl", "delete", "pod", pod, "-n", NAMESPACE, "--context", kube_context],
                capture_output=True,
            )

        # Also restart MCP server pod if it exists
        mcp_result = subprocess.run(
            [
                "kubectl",
                "get",
                "pods",
                "-l",
                "app=mcp-metrics-viewer",
                "-n",
                NAMESPACE,
                "--context",
                kube_context,
                "-o",
                "jsonpath={.items[*].metadata.name}",
            ],
            capture_output=True,
            text=True,
        )
        mcp_pods = mcp_result.stdout.strip().split()
        if mcp_pods and mcp_pods[0]:
            print(f"Also restarting {len(mcp_pods)} MCP pod(s)...")
            for pod in mcp_pods:
                subprocess.run(
                    ["kubectl", "delete", "pod", pod, "-n", NAMESPACE, "--context", kube_context],
                    capture_output=True,
                )

    # Wait for new pods to be ready
    print("Waiting for pods...", end=" ", flush=True)
    start = time.time()
    time.sleep(2)
    for _ in range(60):  # 60 second timeout
        result = subprocess.run(
            [
                "kubectl",
                "get",
                "pods",
                "-l",
                POD_LABEL,
                "-n",
                NAMESPACE,
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
        print("TIMEOUT waiting for pods")
        return 1

    print(f"done ({time.time() - start:.1f}s)")

    # Wait for MCP server pod to be ready
    print("Waiting for MCP server...", end=" ", flush=True)
    mcp_ready = False
    for _ in range(60):  # 60 second timeout
        result = subprocess.run(
            [
                "kubectl",
                "get",
                "pods",
                "-l",
                "app=mcp-metrics-viewer",
                "-n",
                NAMESPACE,
                "--context",
                kube_context,
                "-o",
                "jsonpath={.items[*].status.containerStatuses[0].ready}",
            ],
            capture_output=True,
            text=True,
        )
        if result.stdout.strip() == "true":
            mcp_ready = True
            break
        time.sleep(1)

    if mcp_ready:
        print("ready")

        # Start MCP port-forward
        print("Starting MCP port-forward...", end=" ", flush=True)
        if start_mcp_forward():
            mcp_port = calculate_mcp_port()
            print(f"done (port {mcp_port})")

            # Update .mcp.json with metrics-viewer entry
            update_mcp_json_metrics_viewer()
            print(f"Updated .mcp.json with metrics-viewer at http://127.0.0.1:{mcp_port}/mcp")
        else:
            print("FAILED (check logs)")
    else:
        print("TIMEOUT (MCP features unavailable)")

    # Show final status
    print("\nDeployment complete!")
    return cmd_cluster_status()


def cmd_cluster_status():
    """Show cluster pod status."""
    cluster_name = get_cluster_name()
    kube_context = get_kube_context()

    print(f"Cluster: {cluster_name} (context: {kube_context})")
    print(f"Worktree: {get_worktree_id()}")
    print(f"API port: {calculate_api_port()}")
    print(f"Data dir: {get_data_dir()}")

    # Show DaemonSet pods
    print(f"\nDaemonSet pods ({POD_LABEL}):")
    result = subprocess.run(
        [
            "kubectl",
            "get",
            "pods",
            "-l",
            POD_LABEL,
            "-n",
            NAMESPACE,
            "--context",
            kube_context,
            "-o",
            "wide",
        ],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0:
        print(f"Error: {result.stderr}")
        return 1

    print(result.stdout if result.stdout.strip() else "  No pods found")

    # Show MCP server pods
    print("MCP server pods (app=mcp-metrics-viewer):")
    result = subprocess.run(
        [
            "kubectl",
            "get",
            "pods",
            "-l",
            "app=mcp-metrics-viewer",
            "-n",
            NAMESPACE,
            "--context",
            kube_context,
            "-o",
            "wide",
        ],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0:
        print(f"Error: {result.stderr}")
    else:
        print(result.stdout if result.stdout.strip() else "  No pods found")

    # Show port-forward hints
    pods = get_cluster_pods()
    if pods:
        port = calculate_cluster_viewer_port()
        print(f"To access viewer: ./dev.py cluster viewer start (port {port})")

    # Show MCP forward status
    mcp_pid = get_mcp_forward_pid()
    mcp_port = calculate_mcp_port()
    if mcp_pid:
        print(f"MCP forward:      running (pid {mcp_pid}, port {mcp_port})")
    else:
        print("MCP forward:      not running (will start on next deploy)")

    return 0


def get_forward_pid() -> int | None:
    """Get PID of running port-forward, handling stale PIDs."""
    if not FORWARD_PID_FILE.exists():
        return None

    try:
        pid = int(FORWARD_PID_FILE.read_text().strip())
    except (ValueError, OSError):
        FORWARD_PID_FILE.unlink(missing_ok=True)
        return None

    # Verify process is actually running and is kubectl
    if is_process_running(pid):
        try:
            result = subprocess.run(
                ["ps", "-p", str(pid), "-o", "comm="],
                capture_output=True,
                text=True,
            )
            if "kubectl" in result.stdout:
                return pid
        except Exception:
            pass

    # Stale PID - clean up
    FORWARD_PID_FILE.unlink(missing_ok=True)
    return None


def cmd_viewer(pod_name: str | None):
    """Start port-forward to cluster viewer pod."""
    kube_context = get_kube_context()

    # Stop existing forward if running
    existing_pid = get_forward_pid()
    if existing_pid:
        print(f"Stopping existing port-forward (pid {existing_pid})...", end=" ", flush=True)
        try:
            os.kill(existing_pid, signal.SIGTERM)
            time.sleep(0.5)
            print("stopped")
        except ProcessLookupError:
            print("already stopped")
        FORWARD_PID_FILE.unlink(missing_ok=True)

    port = calculate_cluster_viewer_port()
    ensure_dev_dir()
    log_handle = FORWARD_LOG_FILE.open("w")

    # Forward to viewer pod
    if pod_name:
        target = pod_name
    else:
        pods = get_cluster_pods()
        if not pods:
            print("No pods found with label", POD_LABEL)
            print("  Deploy first with: ./dev.py cluster deploy")
            return 1
        target = pods[0]

    remote_port = 8050  # Container always listens on 8050
    print(f"Starting port-forward to {target} (local port {port})...")

    # Verify pod exists
    result = subprocess.run(
        ["kubectl", "get", "pod", target, "-n", NAMESPACE, "--context", kube_context],
        capture_output=True,
    )
    if result.returncode != 0:
        print(f"Pod not found: {target}")
        return 1

    proc = subprocess.Popen(
        [
            "kubectl",
            "port-forward",
            target,
            f"{port}:{remote_port}",
            "-n",
            NAMESPACE,
            "--context",
            kube_context,
        ],
        stdout=log_handle,
        stderr=subprocess.STDOUT,
        start_new_session=True,  # Detach from terminal
    )

    # Write PID file
    FORWARD_PID_FILE.write_text(str(proc.pid))

    # Wait for port to be ready
    for _ in range(20):  # 2 second timeout
        time.sleep(0.1)
        # Check if process died
        if proc.poll() is not None:
            print(f"Port-forward exited with code {proc.returncode}")
            print(f"Check logs: {FORWARD_LOG_FILE}")
            FORWARD_PID_FILE.unlink(missing_ok=True)
            return 1
        # Check if port is listening
        try:
            import socket

            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
                s.settimeout(0.1)
                s.connect(("127.0.0.1", port))
                break
        except (ConnectionRefusedError, TimeoutError, OSError):
            continue
    else:
        print("Timeout waiting for port-forward")
        print(f"Check logs: {FORWARD_LOG_FILE}")
        return 1

    print(f"Port-forward running (pid {proc.pid})")
    print(f"  Target: {target}")
    print(f"  URL:    http://127.0.0.1:{port}/")
    print(f"  Logs:   {FORWARD_LOG_FILE}")
    print("  Stop:   ./dev.py cluster viewer stop")
    return 0


def cmd_viewer_stop():
    """Stop viewer port-forward."""
    pid = get_forward_pid()
    if not pid:
        print("Port-forward: not running")
        return 0

    print(f"Stopping port-forward (pid {pid})...", end=" ", flush=True)

    try:
        os.kill(pid, signal.SIGTERM)

        # Wait for process to exit
        for _ in range(30):  # 3 seconds
            time.sleep(0.1)
            if not is_process_running(pid):
                break
        else:
            # Force kill
            print("forcing...", end=" ", flush=True)
            os.kill(pid, signal.SIGKILL)
            time.sleep(0.3)

        print("stopped")
        FORWARD_PID_FILE.unlink(missing_ok=True)
        return 0

    except ProcessLookupError:
        print("already stopped")
        FORWARD_PID_FILE.unlink(missing_ok=True)
        return 0
    except Exception as e:
        print(f"error: {e}")
        return 1


# --- MCP Forward Management ---


def get_mcp_forward_pid() -> int | None:
    """Get PID of running MCP port-forward, handling stale PIDs."""
    if not MCP_FORWARD_PID_FILE.exists():
        return None

    try:
        pid = int(MCP_FORWARD_PID_FILE.read_text().strip())
    except (ValueError, OSError):
        MCP_FORWARD_PID_FILE.unlink(missing_ok=True)
        return None

    # Verify process is actually running and is kubectl
    if is_process_running(pid):
        try:
            result = subprocess.run(
                ["ps", "-p", str(pid), "-o", "comm="],
                capture_output=True,
                text=True,
            )
            if "kubectl" in result.stdout:
                return pid
        except Exception:
            pass

    # Stale PID - clean up
    MCP_FORWARD_PID_FILE.unlink(missing_ok=True)
    return None


def stop_mcp_forward() -> None:
    """Stop any existing MCP port-forward."""
    pid = get_mcp_forward_pid()
    if pid:
        try:
            os.kill(pid, signal.SIGTERM)
            time.sleep(0.3)
        except ProcessLookupError:
            pass
        MCP_FORWARD_PID_FILE.unlink(missing_ok=True)


def start_mcp_forward() -> bool:
    """Start port-forward to MCP service. Returns True on success."""
    kube_context = get_kube_context()
    mcp_port = calculate_mcp_port()

    # Stop any existing MCP forward
    stop_mcp_forward()

    ensure_dev_dir()
    log_handle = MCP_FORWARD_LOG_FILE.open("w")

    # Verify service exists
    result = subprocess.run(
        ["kubectl", "get", "svc", "fgm-mcp-server", "-n", NAMESPACE, "--context", kube_context],
        capture_output=True,
    )
    if result.returncode != 0:
        return False

    proc = subprocess.Popen(
        [
            "kubectl",
            "port-forward",
            "svc/fgm-mcp-server",
            f"{mcp_port}:8080",
            "-n",
            NAMESPACE,
            "--context",
            kube_context,
        ],
        stdout=log_handle,
        stderr=subprocess.STDOUT,
        start_new_session=True,  # Detach from terminal
    )

    # Write PID file
    MCP_FORWARD_PID_FILE.write_text(str(proc.pid))

    # Wait for port to be ready
    import socket

    for _ in range(30):  # 3 second timeout
        time.sleep(0.1)
        # Check if process died
        if proc.poll() is not None:
            MCP_FORWARD_PID_FILE.unlink(missing_ok=True)
            return False
        # Check if port is listening
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
                s.settimeout(0.1)
                s.connect(("127.0.0.1", mcp_port))
                return True
        except (ConnectionRefusedError, TimeoutError, OSError):
            continue

    return False


def cmd_mcp_start() -> int:
    """Start MCP port-forward."""
    mcp_port = calculate_mcp_port()

    print(f"Starting MCP port-forward (local port {mcp_port})...")
    if start_mcp_forward():
        pid = get_mcp_forward_pid()
        print(f"MCP port-forward running (pid {pid})")
        print(f"  URL:  http://127.0.0.1:{mcp_port}/mcp")
        print(f"  Logs: {MCP_FORWARD_LOG_FILE}")
        print("  Stop: ./dev.py cluster mcp stop")
        return 0
    else:
        print("Failed to start MCP port-forward")
        print(f"Check logs: {MCP_FORWARD_LOG_FILE}")
        print("Is the MCP server deployed? Run: ./dev.py cluster deploy")
        return 1


def cmd_mcp_stop() -> int:
    """Stop MCP port-forward."""
    pid = get_mcp_forward_pid()
    if not pid:
        print("MCP port-forward: not running")
        return 0

    print(f"Stopping MCP port-forward (pid {pid})...", end=" ", flush=True)
    stop_mcp_forward()
    print("stopped")
    return 0


def get_git_root() -> Path:
    """Get the git repository root directory."""
    result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        capture_output=True,
        text=True,
        cwd=PROJECT_ROOT,
    )
    if result.returncode == 0:
        return Path(result.stdout.strip())
    # Fallback to parent.parent if git command fails
    return PROJECT_ROOT.parent.parent


def update_mcp_json_metrics_viewer() -> None:
    """Update .mcp.json at repo root with metrics-viewer entry."""
    git_root = get_git_root()
    mcp_json_path = git_root / ".mcp.json"
    mcp_port = calculate_mcp_port()

    # Read existing config or create new
    if mcp_json_path.exists():
        try:
            mcp_config = json.loads(mcp_json_path.read_text())
        except json.JSONDecodeError:
            mcp_config = {"mcpServers": {}}
    else:
        mcp_config = {"mcpServers": {}}

    if "mcpServers" not in mcp_config:
        mcp_config["mcpServers"] = {}

    # Add/update metrics-viewer entry
    # Use "http" transport (SSE is deprecated and triggers OAuth probing)
    mcp_config["mcpServers"]["metrics-viewer"] = {
        "type": "http",
        "url": f"http://127.0.0.1:{mcp_port}/mcp",
    }

    mcp_json_path.write_text(json.dumps(mcp_config, indent=2) + "\n")

    # Add .mcp.json to repo root .gitignore if not present
    gitignore = git_root / ".gitignore"
    if gitignore.exists():
        content = gitignore.read_text()
        if ".mcp.json" not in content:
            with gitignore.open("a") as f:
                f.write("\n# MCP server config (worktree-specific)\n.mcp.json\n")


def cmd_cluster_create():
    """Create Kind cluster for this worktree (explicit, though deploy does it on-demand)."""
    cluster_name = get_cluster_name()
    env = _get_env()

    # Check environment is ready
    if env.mode == Mode.VM:
        print(f"Checking Lima VM ({LIMA_VM})...", end=" ", flush=True)
        if not check_lima_vm():
            print("NOT RUNNING")
            print(f"  Start it with: limactl start {LIMA_VM}")
            return 1
        print("running")
    else:
        print("Using direct mode")
        if not env.has_docker:
            print("Error: Docker is not available")
            return 1

    if cluster_exists():
        print(f"Cluster '{cluster_name}' already exists")
        return 0

    if create_cluster():
        return 0
    return 1


def cmd_cluster_destroy():
    """Destroy this worktree's Kind cluster."""
    cluster_name = get_cluster_name()
    env = _get_env()
    backend = _get_backend()

    # Check environment is ready
    if env.mode == Mode.VM:
        print(f"Checking Lima VM ({LIMA_VM})...", end=" ", flush=True)
        if not check_lima_vm():
            print("NOT RUNNING")
            print(f"  Start it with: limactl start {LIMA_VM}")
            return 1
        print("running")
    else:
        print("Using direct mode")

    if not cluster_exists():
        print(f"Cluster '{cluster_name}' does not exist")
        return 0

    if q_branch_dev.delete_cluster(backend, cluster_name):
        return 0
    return 1


def cmd_cluster_list():
    """List all fgm-* Kind clusters."""
    env = _get_env()
    backend = _get_backend()

    # Check environment is ready
    if env.mode == Mode.VM:
        print(f"Checking Lima VM ({LIMA_VM})...", end=" ", flush=True)
        if not check_lima_vm():
            print("NOT RUNNING")
            print(f"  Start it with: limactl start {LIMA_VM}")
            return 1
        print("running\n")
    else:
        print("Using direct mode\n")

    returncode, stdout, stderr = backend.exec(["kind", "get", "clusters"], check=False)

    if returncode != 0:
        print(f"Error getting clusters: {stderr}")
        return 1

    all_clusters = stdout.strip().split("\n") if stdout.strip() else []
    fgm_clusters = [c for c in all_clusters if c.startswith("fgm-")]

    if not fgm_clusters:
        print("No fgm-* clusters found")
        print(f"\nThis worktree would create: {get_cluster_name()}")
        return 0

    current = get_cluster_name()
    print("fgm-* clusters:")
    for cluster in fgm_clusters:
        marker = " <- this worktree" if cluster == current else ""
        # Extract worktree name from cluster name
        worktree = cluster[4:]  # Remove "fgm-" prefix
        print(f"  {cluster} (worktree: {worktree}){marker}")

    if current not in fgm_clusters:
        print(f"\nThis worktree's cluster ({current}) does not exist yet")
        print("  Run: ./dev.py cluster deploy")

    return 0


def get_mcp_kubeconfig_path() -> Path:
    """Get path to MCP kubeconfig for this worktree."""
    cluster_name = get_cluster_name()
    return Path.home() / ".kube" / f"mcp-{cluster_name}.kubeconfig"


# --- Benchmark commands ---


def cleanup_old_bench_runs():
    """Remove benchmark runs older than BENCH_RETENTION_DAYS."""
    if not BENCH_DIR.exists():
        return

    cutoff = datetime.now() - timedelta(days=BENCH_RETENTION_DAYS)
    removed = 0

    for run_dir in BENCH_DIR.iterdir():
        if not run_dir.is_dir():
            continue

        # Check directory mtime
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
        print(f"Cleaned up {removed} benchmark run(s) older than {BENCH_RETENTION_DAYS} days")


def ensure_bench_data() -> bool:
    """Ensure benchmark data exists, generate if needed. Returns True if data is ready."""
    if BENCH_DATA_DIR.exists():
        # Check if there are any parquet files
        parquet_files = list(BENCH_DATA_DIR.glob("**/*.parquet"))
        if parquet_files:
            return True

    print("Benchmark data not found, generating...")
    print("  This may take a few minutes on first run.\n")

    # Build generate-bench-data binary first
    result = subprocess.run(
        ["cargo", "build", "--release", "--bin", "generate-bench-data"],
        cwd=PROJECT_ROOT,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print("Failed to build generate-bench-data:")
        print(result.stderr)
        return False

    # Run data generation
    result = subprocess.run(
        [
            str(PROJECT_ROOT / "target" / "release" / "generate-bench-data"),
            "--scenario",
            "realistic1h",
        ],
        cwd=PROJECT_ROOT,
    )

    if result.returncode != 0:
        print("Failed to generate benchmark data")
        return False

    print("\nBenchmark data generated successfully")
    return True


def get_bench_run_dir(guid: str) -> Path:
    """Get the directory for a benchmark run."""
    return BENCH_DIR / guid


def get_bench_pid(guid: str) -> int | None:
    """Get PID of a running benchmark, handling stale PIDs."""
    run_dir = get_bench_run_dir(guid)
    pid_file = run_dir / "bench.pid"

    if not pid_file.exists():
        return None

    try:
        pid = int(pid_file.read_text().strip())
    except (ValueError, OSError):
        return None

    # Verify process is actually running
    if is_process_running(pid):
        return pid

    return None


def is_bench_complete(guid: str) -> bool:
    """Check if a benchmark run has completed."""
    run_dir = get_bench_run_dir(guid)

    # If no pid file, check if results exist
    pid_file = run_dir / "bench.pid"
    if not pid_file.exists():
        # Completed runs have no pid file but have logs
        return (run_dir / "logs.stdout").exists()

    # If pid file exists, check if process is still running
    pid = get_bench_pid(guid)
    return pid is None


def cmd_bench(filter_pattern: str | None, full_suite: bool, data_dir: str | None):
    """Run benchmarks in background."""
    # Validate args - must specify --filter or --full-suite
    if filter_pattern is None and not full_suite:
        print("Error: Must specify benchmark target")
        print("  ./dev.py bench --filter <benchmark>   # Run specific benchmark")
        print("  ./dev.py bench --full-suite           # Run all benchmarks")
        print("\nAvailable benchmarks:")
        print("  scan_metadata")
        print("  get_timeseries_single_container")
        print("  get_timeseries_all_containers")
        print("\nAvailable data scenarios (use --data):")
        print("  realistic1h     # Single pod, same containers (default)")
        print("  multipod        # Multiple pods, different containers per pod")
        print("  container-churn # Single pod, containers restart over time")
        return 1

    # Resolve data directory
    bench_data = BENCH_DATA_DIR
    if data_dir:
        bench_data = PROJECT_ROOT / "testdata" / "bench" / data_dir
        if not bench_data.exists():
            print(f"Error: Benchmark data not found: {bench_data}")
            print("  Generate with: cargo run --release --bin generate-bench-data -- --scenario <name>")
            return 1

    # Clean up old runs
    cleanup_old_bench_runs()

    # Ensure benchmark data exists
    if not bench_data.exists() or not list(bench_data.glob("**/*.parquet")):
        if data_dir:
            print(f"Error: No parquet files in {bench_data}")
            print("  Generate with: cargo run --release --bin generate-bench-data -- --scenario <name>")
            return 1
        if not ensure_bench_data():
            return 1

    # Generate GUID for this run
    guid = str(uuid.uuid4())[:8]
    run_dir = get_bench_run_dir(guid)
    run_dir.mkdir(parents=True, exist_ok=True)

    # Build cargo bench command
    cargo_cmd = ["cargo", "bench"]
    if filter_pattern:
        cargo_cmd.extend(["--", filter_pattern])

    # Save run metadata
    metadata = {
        "guid": guid,
        "filter": filter_pattern,
        "full_suite": full_suite,
        "data_dir": data_dir or "realistic1h",
        "started_at": datetime.now().isoformat(),
        "command": " ".join(cargo_cmd),
    }
    (run_dir / "metadata.json").write_text(json.dumps(metadata, indent=2))

    # Start benchmark in background
    stdout_file = run_dir / "logs.stdout"
    stderr_file = run_dir / "logs.stderr"

    env = os.environ.copy()
    env["BENCH_DATA"] = str(bench_data)

    stdout_handle = stdout_file.open("w")
    stderr_handle = stderr_file.open("w")

    proc = subprocess.Popen(
        cargo_cmd,
        stdout=stdout_handle,
        stderr=stderr_handle,
        cwd=PROJECT_ROOT,
        env=env,
        start_new_session=True,  # Detach from terminal
    )

    # Write PID file
    (run_dir / "bench.pid").write_text(str(proc.pid))

    # Print status message
    print(f"Running benchmark {guid}")
    print(f"  Filter:  {filter_pattern or 'full suite'}")
    print(f"  Data:    {data_dir or 'realistic1h'}")
    print(f"  Logs:    {run_dir}/logs.{{stdout,stderr}}")
    print(f"  Wait:    ./dev.py bench wait {guid}")
    print(f"  Results available for {BENCH_RETENTION_DAYS} days in {run_dir}/")

    return 0


def cmd_bench_wait(guid: str):
    """Wait for a benchmark to complete and show results."""
    run_dir = get_bench_run_dir(guid)

    if not run_dir.exists():
        print(f"Error: Benchmark run '{guid}' not found")
        print("\nRecent benchmark runs:")
        cmd_bench_list()
        return 1

    pid_file = run_dir / "bench.pid"
    stdout_file = run_dir / "logs.stdout"
    stderr_file = run_dir / "logs.stderr"

    # Check if already complete
    if is_bench_complete(guid):
        print(f"Benchmark {guid} already complete")
    else:
        # Wait for completion
        pid = get_bench_pid(guid)
        if pid:
            print(f"Waiting for benchmark {guid} (pid {pid})...")
            while is_process_running(pid):
                time.sleep(1)

        # Clean up pid file
        if pid_file.exists():
            pid_file.unlink()

        print(f"Benchmark {guid} complete")

    # Show last 50 lines of output
    print("\n" + "=" * 60)
    print("Results (last 50 lines of stdout):")
    print("=" * 60 + "\n")

    if stdout_file.exists():
        lines = stdout_file.read_text().splitlines()
        for line in lines[-50:]:
            print(line)
    else:
        print("(no stdout)")

    # Check for errors
    if stderr_file.exists():
        stderr_content = stderr_file.read_text().strip()
        # Filter out common cargo warnings that aren't errors
        error_lines = [
            line for line in stderr_content.splitlines() if not line.strip().startswith("warning:") and line.strip()
        ]
        if error_lines:
            print("\n" + "=" * 60)
            print("Errors (stderr):")
            print("=" * 60 + "\n")
            for line in error_lines[-20:]:
                print(line)

    print(f"\nFull logs: {run_dir}/logs.{{stdout,stderr}}")
    return 0


def cmd_bench_list():
    """List recent benchmark runs."""
    if not BENCH_DIR.exists():
        print("No benchmark runs found")
        return 0

    runs = []
    for run_dir in BENCH_DIR.iterdir():
        if not run_dir.is_dir():
            continue

        metadata_file = run_dir / "metadata.json"
        if not metadata_file.exists():
            continue

        try:
            metadata = json.loads(metadata_file.read_text())
            metadata["_dir"] = run_dir
            metadata["_complete"] = is_bench_complete(metadata["guid"])
            runs.append(metadata)
        except (json.JSONDecodeError, KeyError):
            continue

    if not runs:
        print("No benchmark runs found")
        return 0

    # Sort by start time, newest first
    runs.sort(key=lambda r: r.get("started_at", ""), reverse=True)

    print("Recent benchmark runs:\n")
    for run in runs[:10]:  # Show last 10
        guid = run.get("guid", "???")
        filter_pattern = run.get("filter") or "full suite"
        data_scenario = run.get("data_dir", "realistic1h")
        started = run.get("started_at", "???")[:19]  # Trim microseconds
        status = "complete" if run["_complete"] else "running"

        print(f"  {guid}  {status:<10}  {data_scenario:<15}  {filter_pattern:<30}  {started}")

    if len(runs) > 10:
        print(f"\n  ... and {len(runs) - 10} more (oldest auto-cleaned after {BENCH_RETENTION_DAYS} days)")

    return 0


def cmd_mcp_setup():
    """Setup MCP server integration for this worktree's cluster (one-time)."""
    cluster_name = get_cluster_name()
    kube_context = get_kube_context()
    kubeconfig_path = get_mcp_kubeconfig_path()

    # Check cluster exists
    if not cluster_exists():
        print(f"Cluster '{cluster_name}' does not exist")
        print("  Run: ./dev.py cluster deploy")
        return 1

    print(f"Setting up MCP integration for cluster '{cluster_name}'...")

    # Step 1: Create namespace
    print("Creating mcp namespace...", end=" ", flush=True)
    result = subprocess.run(
        ["kubectl", "--context", kube_context, "create", "namespace", "mcp"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0 and "already exists" not in result.stderr:
        print("FAILED")
        print(f"  Error: {result.stderr}")
        return 1
    print("done")

    # Step 2: Create service account
    print("Creating service account...", end=" ", flush=True)
    result = subprocess.run(
        ["kubectl", "--context", kube_context, "create", "serviceaccount", "mcp-viewer", "-n", "mcp"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0 and "already exists" not in result.stderr:
        print("FAILED")
        print(f"  Error: {result.stderr}")
        return 1
    print("done")

    # Step 3: Create cluster role binding
    print("Creating cluster role binding...", end=" ", flush=True)
    result = subprocess.run(
        [
            "kubectl",
            "--context",
            kube_context,
            "create",
            "clusterrolebinding",
            "mcp-viewer-crb",
            "--clusterrole=cluster-admin",
            "--serviceaccount=mcp:mcp-viewer",
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0 and "already exists" not in result.stderr:
        print("FAILED")
        print(f"  Error: {result.stderr}")
        return 1
    print("done")

    # Step 4: Create token (1 year duration)
    print("Creating service account token...", end=" ", flush=True)
    result = subprocess.run(
        ["kubectl", "--context", kube_context, "create", "token", "mcp-viewer", "--duration=8760h", "-n", "mcp"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print("FAILED")
        print(f"  Error: {result.stderr}")
        return 1
    token = result.stdout.strip()
    print("done")

    # Step 5: Get API server and CA data
    print("Extracting cluster credentials...", end=" ", flush=True)
    result = subprocess.run(
        [
            "kubectl",
            "config",
            "view",
            "--context",
            kube_context,
            "--minify",
            "-o",
            "jsonpath={.clusters[0].cluster.server}",
        ],
        capture_output=True,
        text=True,
    )
    api_server = result.stdout.strip()

    result = subprocess.run(
        [
            "kubectl",
            "config",
            "view",
            "--context",
            kube_context,
            "--minify",
            "--raw",
            "-o",
            "jsonpath={.clusters[0].cluster.certificate-authority-data}",
        ],
        capture_output=True,
        text=True,
    )
    ca_data = result.stdout.strip()
    print("done")

    # Step 6: Write kubeconfig
    print(f"Writing kubeconfig to {kubeconfig_path}...", end=" ", flush=True)
    kubeconfig_content = f"""apiVersion: v1
kind: Config
clusters:
- name: mcp-{cluster_name}
  cluster:
    server: {api_server}
    certificate-authority-data: {ca_data}
users:
- name: mcp-viewer
  user:
    token: {token}
contexts:
- name: mcp-{cluster_name}
  context:
    cluster: mcp-{cluster_name}
    user: mcp-viewer
current-context: mcp-{cluster_name}
"""
    kubeconfig_path.parent.mkdir(parents=True, exist_ok=True)
    kubeconfig_path.write_text(kubeconfig_content)
    kubeconfig_path.chmod(0o600)
    print("done")

    # Step 7: Verify kubeconfig works
    print("Verifying kubeconfig...", end=" ", flush=True)
    result = subprocess.run(
        ["kubectl", f"--kubeconfig={kubeconfig_path}", "get", "pods", "-A"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print("FAILED")
        print(f"  Error: {result.stderr}")
        return 1
    print("done")

    # Step 8: Create/update .mcp.json in git repo root (not fine-grained-monitor subdir)
    print("Updating .mcp.json...", end=" ", flush=True)
    git_root = get_git_root()
    mcp_json_path = git_root / ".mcp.json"

    # Read existing config or create new
    if mcp_json_path.exists():
        try:
            mcp_config = json.loads(mcp_json_path.read_text())
        except json.JSONDecodeError:
            mcp_config = {"mcpServers": {}}
    else:
        mcp_config = {"mcpServers": {}}

    if "mcpServers" not in mcp_config:
        mcp_config["mcpServers"] = {}

    # Add/update kubernetes-mcp-server entry
    mcp_config["mcpServers"]["kubernetes-mcp-server"] = {
        "command": "npx",
        "args": ["-y", "kubernetes-mcp-server@latest"],
        "env": {"KUBECONFIG": str(kubeconfig_path)},
    }

    mcp_json_path.write_text(json.dumps(mcp_config, indent=2) + "\n")
    print("done")

    # Add .mcp.json to repo root .gitignore if not present
    gitignore = git_root / ".gitignore"
    if gitignore.exists():
        content = gitignore.read_text()
        if ".mcp.json" not in content:
            with gitignore.open("a") as f:
                f.write("\n# MCP server config (worktree-specific)\n.mcp.json\n")

    print("\nMCP integration configured!")
    print(f"  Cluster:    {cluster_name}")
    print(f"  Kubeconfig: {kubeconfig_path}")
    print(f"  MCP config: {mcp_json_path}")
    print("\nRestart Claude Code to pick up the new MCP server configuration.")
    return 0


def cmd_unified_status():
    """Show status of ALL viewers (local and cluster) to avoid confusion.

    This is the recommended way to check what's running - it clearly shows
    which viewers are active and on which ports.
    """
    print("=" * 60)
    print("VIEWER STATUS OVERVIEW")
    print("=" * 60)
    print(f"Worktree: {get_worktree_id()}")
    print()

    # --- Local viewer status ---
    local_port = calculate_local_viewer_port()
    local_pid = get_running_pid()
    local_state = read_state()

    print("LOCAL VIEWER (./dev.py local viewer)")
    print("-" * 40)
    if local_pid and local_state:
        uptime = format_uptime(local_state.get("start_time", time.time()))
        healthy = check_health(local_port)
        health_str = "healthy" if healthy else "UNHEALTHY"
        print(f"  Status:  RUNNING (pid {local_pid}, {uptime})")
        print(f"  Health:  {health_str}")
        print(f"  Port:    {local_port}")
        print(f"  URL:     http://127.0.0.1:{local_port}/")
        print(f"  Data:    {local_state.get('data_file', 'unknown')}")
    else:
        print("  Status:  not running")
        print(f"  Port:    {local_port} (would use if started)")
    print()

    # --- Cluster viewer status ---
    cluster_port = calculate_cluster_viewer_port()
    cluster_pid = get_forward_pid()

    print("CLUSTER VIEWER (./dev.py cluster viewer)")
    print("-" * 40)
    if cluster_pid:
        # Check if port-forward is actually working
        import socket

        try:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
                s.settimeout(0.5)
                s.connect(("127.0.0.1", cluster_port))
            forward_healthy = True
        except (ConnectionRefusedError, TimeoutError, OSError):
            forward_healthy = False

        health_str = "connected" if forward_healthy else "DISCONNECTED"
        print(f"  Status:  FORWARDING (pid {cluster_pid})")
        print(f"  Health:  {health_str}")
        print(f"  Port:    {cluster_port}")
        print(f"  URL:     http://127.0.0.1:{cluster_port}/")
        print(f"  Target:  pod in Kind cluster '{get_cluster_name()}'")
    else:
        print("  Status:  not running")
        print(f"  Port:    {cluster_port} (would use if started)")
    print()

    # --- MCP status ---
    mcp_port = calculate_mcp_port()
    mcp_pid = get_mcp_forward_pid()

    print("MCP SERVER (./dev.py cluster mcp)")
    print("-" * 40)
    if mcp_pid:
        print(f"  Status:  FORWARDING (pid {mcp_pid})")
        print(f"  Port:    {mcp_port}")
        print(f"  URL:     http://127.0.0.1:{mcp_port}/mcp")
    else:
        print("  Status:  not running")
        print(f"  Port:    {mcp_port} (would use if started)")
    print()

    # --- Port summary ---
    print("PORT ALLOCATION (this worktree)")
    print("-" * 40)
    print(f"  Local viewer:   {local_port}")
    print(f"  Cluster viewer: {cluster_port}")
    print(f"  MCP server:     {mcp_port}")
    print()

    return 0


def main():
    parser = argparse.ArgumentParser(
        description="Development workflow manager for fine-grained-monitor (fgm-*) binaries",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  ./dev.py status                          # Show ALL viewer status (recommended!)

  ./dev.py local build                     # Build all release binaries
  ./dev.py local test                      # Run tests
  ./dev.py local clippy                    # Run clippy lints
  ./dev.py local viewer start              # Start LOCAL fgm-viewer (port 8050+)
  ./dev.py local viewer start --data /path/to/file.parquet
  ./dev.py local viewer stop               # Stop local fgm-viewer
  ./dev.py local viewer status             # Check local fgm-viewer status

  ./dev.py cluster deploy                  # Build image, load to Kind, restart pods
  ./dev.py cluster status                  # Show cluster pod status
  ./dev.py cluster viewer start            # Port-forward to CLUSTER viewer (port 8550+)
  ./dev.py cluster viewer start --pod NAME # Port-forward to specific pod
  ./dev.py cluster viewer stop             # Stop cluster viewer port-forward
  ./dev.py cluster create                  # Create Kind cluster for this worktree
  ./dev.py cluster destroy                 # Destroy Kind cluster for this worktree
  ./dev.py cluster list                    # List all fgm-* clusters
  ./dev.py cluster mcp setup               # Setup MCP server for this worktree's cluster
  ./dev.py cluster mcp start               # Start MCP port-forward
  ./dev.py cluster mcp stop                # Stop MCP port-forward

  ./dev.py bench --filter <name>           # Run specific benchmark in background
  ./dev.py bench --full-suite              # Run all benchmarks in background
  ./dev.py bench wait <guid>               # Wait for benchmark and show results
  ./dev.py bench list                      # List recent benchmark runs

Port allocation (per-worktree):
  Local viewer:   8050 + offset  (./dev.py local viewer)
  Cluster viewer: 8550 + offset  (./dev.py cluster viewer)
  MCP server:     9050 + offset  (./dev.py cluster mcp)

  Each worktree gets unique offsets based on directory path hash.
  Use './dev.py status' to see your worktree's port assignments.
""",
    )

    subparsers = parser.add_subparsers(dest="group", required=True)

    # --- Top-level status command ---
    subparsers.add_parser("status", help="Show status of ALL viewers (local + cluster) - RECOMMENDED")

    # --- Local subcommands ---
    local_parser = subparsers.add_parser("local", help="Local development commands")
    local_subs = local_parser.add_subparsers(dest="command", required=True)

    # local build
    local_subs.add_parser("build", help="Build all release binaries")

    # local test
    local_subs.add_parser("test", help="Run tests")

    # local clippy
    local_subs.add_parser("clippy", help="Run clippy lints")

    # local viewer (nested)
    viewer_parser = local_subs.add_parser("viewer", help="fgm-viewer lifecycle commands")
    viewer_subs = viewer_parser.add_subparsers(dest="viewer_command", required=True)

    # local viewer status
    viewer_subs.add_parser("status", help="Show viewer server status")

    # local viewer start
    start_parser = viewer_subs.add_parser("start", help="Build and start viewer server")
    start_parser.add_argument(
        "--data",
        type=str,
        default=str(DEFAULT_DATA.resolve()),
        help="Absolute path to parquet data file",
    )

    # local viewer stop
    viewer_subs.add_parser("stop", help="Stop viewer server")

    # local viewer restart
    restart_parser = viewer_subs.add_parser("restart", help="Stop, rebuild, start viewer server")
    restart_parser.add_argument(
        "--data",
        type=str,
        default=None,
        help="Absolute path to parquet data file (uses current if not specified)",
    )

    # --- Cluster subcommands ---
    cluster_parser = subparsers.add_parser("cluster", help="Cluster deployment commands (Kind via Lima)")
    cluster_subs = cluster_parser.add_subparsers(dest="command", required=True)

    # cluster deploy
    cluster_subs.add_parser("deploy", help="Build image, load to Kind cluster, restart pods")

    # cluster status
    cluster_subs.add_parser("status", help="Show cluster pod status")

    # cluster viewer (subcommand group for viewer port-forward)
    viewer_parser = cluster_subs.add_parser("viewer", help="Viewer web UI port-forward")
    viewer_subs = viewer_parser.add_subparsers(dest="viewer_command", required=True)
    viewer_start_parser = viewer_subs.add_parser("start", help="Start viewer port-forward")
    viewer_start_parser.add_argument(
        "--pod",
        type=str,
        default=None,
        help="Specific pod name (default: first available pod)",
    )
    viewer_subs.add_parser("stop", help="Stop viewer port-forward")

    # cluster mcp (subcommand group for MCP operations)
    mcp_parser = cluster_subs.add_parser("mcp", help="MCP server operations")
    mcp_subs = mcp_parser.add_subparsers(dest="mcp_command", required=True)
    mcp_subs.add_parser("setup", help="One-time Claude Code integration setup")
    mcp_subs.add_parser("start", help="Start MCP port-forward")
    mcp_subs.add_parser("stop", help="Stop MCP port-forward")

    # cluster create
    cluster_subs.add_parser("create", help="Create Kind cluster for this worktree")

    # cluster destroy
    cluster_subs.add_parser("destroy", help="Destroy Kind cluster for this worktree")

    # cluster list
    cluster_subs.add_parser("list", help="List all fgm-* clusters")

    # --- Bench subcommands ---
    bench_parser = subparsers.add_parser("bench", help="Benchmark commands")
    bench_parser.add_argument(
        "--filter",
        type=str,
        default=None,
        help="Run specific benchmark (e.g., get_timeseries_single_container)",
    )
    bench_parser.add_argument(
        "--full-suite",
        action="store_true",
        help="Run all benchmarks",
    )
    bench_parser.add_argument(
        "--data",
        type=str,
        default=None,
        help="Benchmark data scenario: realistic1h (default), multipod, container-churn",
    )

    bench_subs = bench_parser.add_subparsers(dest="bench_command")

    # bench wait <guid>
    bench_wait_parser = bench_subs.add_parser("wait", help="Wait for benchmark to complete and show results")
    bench_wait_parser.add_argument("guid", type=str, help="Benchmark run GUID")

    # bench list
    bench_subs.add_parser("list", help="List recent benchmark runs")

    args = parser.parse_args()

    if args.group == "status":
        sys.exit(cmd_unified_status())
    elif args.group == "local":
        if args.command == "build":
            sys.exit(cmd_build())
        elif args.command == "test":
            sys.exit(cmd_test())
        elif args.command == "clippy":
            sys.exit(cmd_clippy())
        elif args.command == "viewer":
            if args.viewer_command == "status":
                sys.exit(cmd_status())
            elif args.viewer_command == "start":
                sys.exit(cmd_start(args.data))
            elif args.viewer_command == "stop":
                sys.exit(cmd_stop())
            elif args.viewer_command == "restart":
                sys.exit(cmd_restart(args.data))
    elif args.group == "cluster":
        if args.command == "deploy":
            sys.exit(cmd_deploy())
        elif args.command == "status":
            sys.exit(cmd_cluster_status())
        elif args.command == "viewer":
            if args.viewer_command == "start":
                sys.exit(cmd_viewer(args.pod))
            elif args.viewer_command == "stop":
                sys.exit(cmd_viewer_stop())
        elif args.command == "mcp":
            if args.mcp_command == "setup":
                sys.exit(cmd_mcp_setup())
            elif args.mcp_command == "start":
                sys.exit(cmd_mcp_start())
            elif args.mcp_command == "stop":
                sys.exit(cmd_mcp_stop())
        elif args.command == "create":
            sys.exit(cmd_cluster_create())
        elif args.command == "destroy":
            sys.exit(cmd_cluster_destroy())
        elif args.command == "list":
            sys.exit(cmd_cluster_list())
    elif args.group == "bench":
        if args.bench_command == "wait":
            sys.exit(cmd_bench_wait(args.guid))
        elif args.bench_command == "list":
            sys.exit(cmd_bench_list())
        else:
            # No subcommand means run benchmark
            sys.exit(cmd_bench(args.filter, args.full_suite, args.data))


if __name__ == "__main__":
    main()
