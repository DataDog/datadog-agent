#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# ///
"""
Development server lifecycle manager for fgm-viewer.

Usage:
    ./dev.py status              # Check server state
    ./dev.py start [--data PATH] # Build and start server
    ./dev.py stop                # Stop running server
    ./dev.py restart [--data]    # Stop, rebuild, start
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

# Default data file (in sibling directory q_branch/out/)
DEFAULT_DATA = PROJECT_ROOT.parent / "out" / "fixed-dec-22-23.parquet"

# Binary path
BINARY = PROJECT_ROOT / "target" / "release" / "fgm-viewer"


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


def main():
    parser = argparse.ArgumentParser(
        description="Development server lifecycle manager for fgm-viewer",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  ./dev.py status              # Check if server is running
  ./dev.py start               # Build and start with default data
  ./dev.py start --data /path/to/file.parquet
  ./dev.py stop                # Stop the server
  ./dev.py restart             # Rebuild and restart
""",
    )

    subparsers = parser.add_subparsers(dest="command", required=True)

    # status
    subparsers.add_parser("status", help="Show server status")

    # start
    start_parser = subparsers.add_parser("start", help="Build and start server")
    start_parser.add_argument(
        "--data",
        type=str,
        default=str(DEFAULT_DATA.resolve()),
        help="Absolute path to parquet data file",
    )

    # stop
    subparsers.add_parser("stop", help="Stop running server")

    # restart
    restart_parser = subparsers.add_parser("restart", help="Stop, rebuild, start")
    restart_parser.add_argument(
        "--data",
        type=str,
        default=None,
        help="Absolute path to parquet data file (uses current if not specified)",
    )

    args = parser.parse_args()

    if args.command == "status":
        sys.exit(cmd_status())
    elif args.command == "start":
        sys.exit(cmd_start(args.data))
    elif args.command == "stop":
        sys.exit(cmd_stop())
    elif args.command == "restart":
        sys.exit(cmd_restart(args.data))


if __name__ == "__main__":
    main()
