#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# ///
"""
Development server lifecycle manager for fgm-viewer.

Local development:
    ./dev.py status              # Check server state
    ./dev.py start [--data PATH] # Build and start server
    ./dev.py stop                # Stop running server
    ./dev.py restart [--data]    # Stop, rebuild, start

Cluster deployment (Kind via Lima):
    ./dev.py deploy              # Build image, load to Kind, restart pods
    ./dev.py cluster-status      # Show cluster pod status
    ./dev.py forward             # Port-forward to viewer (managed, survives restarts)
    ./dev.py forward-stop        # Stop port-forward
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
from pathlib import Path

# Project root is where this script lives
PROJECT_ROOT = Path(__file__).parent.resolve()
DEV_DIR = PROJECT_ROOT / ".dev"
PID_FILE = DEV_DIR / "server.pid"
LOG_FILE = DEV_DIR / "server.log"
STATE_FILE = DEV_DIR / "state.json"
FORWARD_PID_FILE = DEV_DIR / "forward.pid"
FORWARD_LOG_FILE = DEV_DIR / "forward.log"

# Default data file (test data in testdata/)
DEFAULT_DATA = PROJECT_ROOT / "testdata" / "1hr.parquet"

# Binary path
BINARY = PROJECT_ROOT / "target" / "release" / "fgm-viewer"

# Cluster deployment config
IMAGE_NAME = "fine-grained-monitor"
IMAGE_TAG = "latest"
LIMA_VM = "gadget-k8s-host"
KIND_CLUSTER = "gadget-dev"
KUBE_CONTEXT = f"kind-{KIND_CLUSTER}"
POD_LABEL = "app=fine-grained-monitor"
NAMESPACE = "default"


def calculate_port() -> int:
    """Calculate unique port based on checkout path for worktree support."""
    path_hash = hashlib.md5(str(PROJECT_ROOT).encode()).hexdigest()
    offset = int(path_hash[:8], 16) % 1000
    return 8050 + offset


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


def is_process_running(pid: int) -> bool:
    """Check if a process with given PID is running."""
    try:
        os.kill(pid, 0)
        return True
    except (OSError, ProcessLookupError):
        return False


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


def check_health(port: int, timeout: float = 1.0) -> bool:
    """Check if server is healthy via /api/health endpoint."""
    url = f"http://127.0.0.1:{port}/api/health"
    try:
        req = urllib.request.Request(url, method="GET")
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            data = json.loads(resp.read().decode())
            return data.get("status") == "ok"
    except (urllib.error.URLError, json.JSONDecodeError, TimeoutError, OSError):
        return False


def format_uptime(start_time: float) -> str:
    """Format uptime as human-readable string."""
    elapsed = int(time.time() - start_time)
    if elapsed < 60:
        return f"{elapsed}s"
    elif elapsed < 3600:
        mins = elapsed // 60
        secs = elapsed % 60
        return f"{mins}m {secs}s"
    else:
        hours = elapsed // 3600
        mins = (elapsed % 3600) // 60
        return f"{hours}h {mins}m"


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
    port = calculate_port()
    pid = get_running_pid()
    state = read_state()

    if pid and state:
        uptime = format_uptime(state.get("start_time", time.time()))
        healthy = check_health(port)
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
    port = calculate_port()
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

        if check_health(port):
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


# --- Cluster deployment commands ---


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


def get_cluster_pods() -> list[str]:
    """Get list of fine-grained-monitor pod names."""
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
            KUBE_CONTEXT,
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
    image_full = f"{IMAGE_NAME}:{IMAGE_TAG}"

    # Check Lima VM is running
    print(f"Checking Lima VM ({LIMA_VM})...", end=" ", flush=True)
    if not check_lima_vm():
        print("NOT RUNNING")
        print(f"  Start it with: limactl start {LIMA_VM}")
        return 1
    print("running")

    # Step 1: Build docker image
    success, _ = run_cmd(
        ["docker", "build", "-t", image_full, "."],
        f"Building docker image ({image_full})",
    )
    if not success:
        return 1

    # Step 2: Save and load into Lima VM
    print("Loading image into Lima VM...", end=" ", flush=True)
    start = time.time()

    # Use a pipe: docker save | limactl shell ... docker load
    save_proc = subprocess.Popen(
        ["docker", "save", image_full],
        stdout=subprocess.PIPE,
    )
    load_proc = subprocess.Popen(
        ["limactl", "shell", LIMA_VM, "--", "docker", "load"],
        stdin=save_proc.stdout,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    save_proc.stdout.close()  # Allow save_proc to receive SIGPIPE
    stdout, stderr = load_proc.communicate()

    if load_proc.returncode != 0:
        print("FAILED")
        print(f"  Error: {stderr.decode()}")
        return 1
    print(f"done ({time.time() - start:.1f}s)")

    # Step 3: Load into Kind cluster
    success, _ = run_cmd(
        ["limactl", "shell", LIMA_VM, "--", "kind", "load", "docker-image", image_full, "--name", KIND_CLUSTER],
        "Loading image into Kind cluster",
    )
    if not success:
        return 1

    # Step 4: Get current pods and delete them
    pods = get_cluster_pods()
    if not pods:
        print("No pods found with label", POD_LABEL)
        print("  Deploy the DaemonSet first with: kubectl apply -f deploy/")
        return 1

    print(f"Restarting {len(pods)} pod(s)...", end=" ", flush=True)
    start = time.time()

    for pod in pods:
        subprocess.run(
            ["kubectl", "delete", "pod", pod, "-n", NAMESPACE, "--context", KUBE_CONTEXT],
            capture_output=True,
        )

    # Wait for new pods to be ready
    time.sleep(2)
    for _ in range(30):  # 30 second timeout
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
                KUBE_CONTEXT,
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

    # Show final status
    print("\nDeployment complete!")
    return cmd_cluster_status()


def cmd_cluster_status():
    """Show cluster pod status."""
    print(f"Cluster: {KIND_CLUSTER} (context: {KUBE_CONTEXT})")
    print(f"Pods with label: {POD_LABEL}\n")

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
            KUBE_CONTEXT,
            "-o",
            "wide",
        ],
        capture_output=True,
        text=True,
    )

    if result.returncode != 0:
        print(f"Error: {result.stderr}")
        return 1

    print(result.stdout)

    # Show port-forward hint
    pods = get_cluster_pods()
    if pods:
        print("To access viewer: ./dev.py forward")

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


def cmd_forward(pod_name: str | None):
    """Start port-forward to cluster viewer pod."""
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

    # Get target pod
    if pod_name:
        target_pod = pod_name
    else:
        pods = get_cluster_pods()
        if not pods:
            print("No pods found with label", POD_LABEL)
            print("  Deploy first with: ./dev.py deploy")
            return 1
        target_pod = pods[0]

    # Verify pod exists
    result = subprocess.run(
        ["kubectl", "get", "pod", target_pod, "-n", NAMESPACE, "--context", KUBE_CONTEXT],
        capture_output=True,
    )
    if result.returncode != 0:
        print(f"Pod not found: {target_pod}")
        return 1

    port = calculate_port()
    print(f"Starting port-forward to {target_pod} (local port {port})...")

    ensure_dev_dir()
    log_handle = FORWARD_LOG_FILE.open("w")

    remote_port = 8050  # Container always listens on 8050
    proc = subprocess.Popen(
        [
            "kubectl",
            "port-forward",
            target_pod,
            f"{port}:{remote_port}",
            "-n",
            NAMESPACE,
            "--context",
            KUBE_CONTEXT,
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
    print(f"  Pod:    {target_pod}")
    print(f"  URL:    http://127.0.0.1:{port}/")
    print(f"  Logs:   {FORWARD_LOG_FILE}")
    print("  Stop:   ./dev.py forward-stop")
    return 0


def cmd_forward_stop():
    """Stop port-forward."""
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


def main():
    parser = argparse.ArgumentParser(
        description="Development server lifecycle manager for fgm-viewer",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Local development examples:
  ./dev.py status              # Check if server is running
  ./dev.py start               # Build and start with default data
  ./dev.py start --data /path/to/file.parquet
  ./dev.py stop                # Stop the server
  ./dev.py restart             # Rebuild and restart

Cluster deployment examples:
  ./dev.py deploy              # Build image, load to Kind, restart pods
  ./dev.py cluster-status      # Show cluster pod status
  ./dev.py forward             # Port-forward to first pod
  ./dev.py forward --pod NAME  # Port-forward to specific pod
  ./dev.py forward-stop        # Stop port-forward
""",
    )

    subparsers = parser.add_subparsers(dest="command", required=True)

    # status
    subparsers.add_parser("status", help="Show local server status")

    # start
    start_parser = subparsers.add_parser("start", help="Build and start local server")
    start_parser.add_argument(
        "--data",
        type=str,
        default=str(DEFAULT_DATA.resolve()),
        help="Absolute path to parquet data file",
    )

    # stop
    subparsers.add_parser("stop", help="Stop local server")

    # restart
    restart_parser = subparsers.add_parser("restart", help="Stop, rebuild, start local server")
    restart_parser.add_argument(
        "--data",
        type=str,
        default=None,
        help="Absolute path to parquet data file (uses current if not specified)",
    )

    # deploy
    subparsers.add_parser("deploy", help="Build image, load to Kind cluster, restart pods")

    # cluster-status
    subparsers.add_parser("cluster-status", help="Show cluster pod status")

    # forward
    forward_parser = subparsers.add_parser("forward", help="Port-forward to cluster viewer pod")
    forward_parser.add_argument(
        "--pod",
        type=str,
        default=None,
        help="Specific pod name (default: first available pod)",
    )

    # forward-stop
    subparsers.add_parser("forward-stop", help="Stop port-forward")

    args = parser.parse_args()

    if args.command == "status":
        sys.exit(cmd_status())
    elif args.command == "start":
        sys.exit(cmd_start(args.data))
    elif args.command == "stop":
        sys.exit(cmd_stop())
    elif args.command == "restart":
        sys.exit(cmd_restart(args.data))
    elif args.command == "deploy":
        sys.exit(cmd_deploy())
    elif args.command == "cluster-status":
        sys.exit(cmd_cluster_status())
    elif args.command == "forward":
        sys.exit(cmd_forward(args.pod))
    elif args.command == "forward-stop":
        sys.exit(cmd_forward_stop())


if __name__ == "__main__":
    main()
