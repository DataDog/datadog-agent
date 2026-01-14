#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///
"""
q_branch Kubernetes Development Environment Manager

Per-worktree cluster management for Kind clusters via Lima VM or Direct mode.
Primarily a library for q_branch projects (fine-grained-monitor, etc.) with
a minimal CLI for status and cluster listing.

Usage:
    ./dev.py                  # Show environment status (default)
    ./dev.py status           # Show environment status
    ./dev.py list             # List all Kind clusters

Library usage:
    import sys
    sys.path.insert(0, str(Path(__file__).parent.parent))
    from dev import (
        get_worktree_id, get_cluster_name, get_kube_context,
        cluster_exists, create_cluster, merge_kubeconfig, load_image,
    )
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys
import time
from pathlib import Path

# Add lib to path for imports
sys.path.insert(0, str(Path(__file__).parent))

from lib.k8s_backend import (
    Mode,
    Environment,
    VMBackend,
    DirectBackend,
    CommandError,
    detect_environment,
    create_backend,
)

# Configuration
LIMA_VM = "gadget-k8s-host"
LIMA_YAML = Path(__file__).parent / "gadget-k8s-host.lima.yaml"
DEFAULT_KUBECONFIG = Path.home() / ".kube" / "config"


# --- Worktree Identification ---


def get_worktree_id(project_root: Path) -> str:
    """Get worktree identifier from project directory structure.

    Supports multiple project layouts:
    - Fine-grained-monitor: PROJECT_ROOT.parent.parent.name
      e.g., /path/to/beta-datadog-agent/q_branch/fine-grained-monitor -> "beta-datadog-agent"
    - Generic q_branch projects: PROJECT_ROOT.parent.name
      e.g., /path/to/beta-datadog-agent/q_branch/my-project -> "beta-datadog-agent"

    The worktree ID is used for cluster naming, port allocation, and data isolation.
    """
    # Check if we're in a subdirectory of q_branch (like fine-grained-monitor)
    if project_root.parent.name == "q_branch":
        return project_root.parent.parent.name
    # Otherwise assume we're at q_branch level
    return project_root.parent.name


def get_cluster_name(prefix: str, worktree_id: str) -> str:
    """Get Kind cluster name: {prefix}-{worktree_id}."""
    return f"{prefix}-{worktree_id}"


def get_kube_context(cluster_name: str) -> str:
    """Get kubectl context name: kind-{cluster_name}."""
    return f"kind-{cluster_name}"


def get_image_tag(worktree_id: str) -> str:
    """Get Docker image tag for a worktree."""
    return worktree_id


def get_data_dir(project_name: str, worktree_id: str) -> str:
    """Get data directory path inside containers for a project/worktree."""
    return f"/var/lib/{project_name}/{worktree_id}"


# --- Port Allocation ---


def calculate_port_offset(project_root: Path) -> int:
    """Calculate unique offset (0-499) based on checkout path for worktree support."""
    path_hash = hashlib.md5(str(project_root).encode()).hexdigest()
    return int(path_hash[:8], 16) % 500


def calculate_api_port(worktree_id: str) -> int:
    """Calculate unique API server port (6443-6447) based on worktree ID.

    Uses deterministic hash-based allocation to avoid race conditions
    when multiple worktrees create clusters simultaneously.
    """
    port_hash = hashlib.md5(worktree_id.encode()).hexdigest()
    offset = int(port_hash[:8], 16) % 5
    return 6443 + offset


# --- Cluster Lifecycle ---


def cluster_exists(backend: VMBackend | DirectBackend, cluster_name: str) -> bool:
    """Check if Kind cluster exists."""
    try:
        returncode, stdout, _ = backend.exec(["kind", "get", "clusters"], check=False)
        if returncode != 0:
            return False
        return cluster_name in stdout.strip().split("\n")
    except CommandError:
        return False


def create_cluster(
    backend: VMBackend | DirectBackend,
    cluster_name: str,
    api_port: int,
    kind_config: str | None = None,
    other_containers_callback=None,
) -> bool:
    """Create Kind cluster with given configuration.

    Args:
        backend: VMBackend or DirectBackend
        cluster_name: Name for the Kind cluster
        api_port: API server port (6443-6447)
        kind_config: Optional custom Kind config YAML
        other_containers_callback: Optional callback(exclude_cluster) -> list[str] to get
            containers to stop during creation (for multi-node stability)

    Returns True on success.
    """
    print(f"Creating Kind cluster '{cluster_name}' (API port {api_port})...")

    # Default Kind config if not provided
    if kind_config is None:
        kind_config = f"""kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: {api_port}
nodes:
  - role: control-plane
"""

    # Kind multi-node creation can fail with other clusters running - stop them temporarily
    other_containers = []
    if other_containers_callback:
        other_containers = other_containers_callback(exclude_cluster=cluster_name)
        if other_containers:
            print(f"Temporarily stopping {len(other_containers)} containers from other clusters...")
            stop_containers(backend, other_containers)

    # Create cluster
    returncode, stdout, stderr = backend.exec(
        ["kind", "create", "cluster", "--name", cluster_name, "--config", "/dev/stdin"],
        input_data=kind_config,
        check=False,
        capture=False,
    )

    # Restart other clusters regardless of success/failure
    if other_containers:
        print(f"Restarting {len(other_containers)} containers from other clusters...")
        start_containers(backend, other_containers)

    if returncode != 0:
        print(f"Failed to create cluster: {stderr}")
        return False

    print(f"Cluster '{cluster_name}' created successfully")
    return True


def delete_cluster(backend: VMBackend | DirectBackend, cluster_name: str) -> bool:
    """Delete Kind cluster."""
    print(f"Deleting cluster '{cluster_name}'...", end=" ", flush=True)

    returncode, _, stderr = backend.exec(
        ["kind", "delete", "cluster", "--name", cluster_name],
        check=False,
        capture=True,
    )

    if returncode != 0:
        print("FAILED")
        print(f"  Error: {stderr}")
        return False

    # Clean up kubeconfig entries
    context_name = f"kind-{cluster_name}"
    for resource in ["context", "cluster", "user"]:
        subprocess.run(
            ["kubectl", "config", f"delete-{resource}", context_name],
            capture_output=True,
        )

    print("done")
    return True


def merge_kubeconfig(backend: VMBackend | DirectBackend, cluster_name: str) -> None:
    """Merge cluster kubeconfig into ~/.kube/config."""
    context_name = f"kind-{cluster_name}"
    kubeconfig_file = Path.home() / ".kube" / f"{cluster_name}.yaml"

    # Remove existing entries for this cluster
    for resource in ["context", "cluster", "user"]:
        subprocess.run(
            ["kubectl", "config", f"delete-{resource}", context_name],
            capture_output=True,
        )

    # Extract kubeconfig from Kind
    returncode, stdout, stderr = backend.exec(
        ["kind", "get", "kubeconfig", "--name", cluster_name],
        check=False,
    )
    if returncode != 0:
        print(f"Warning: Failed to get kubeconfig: {stderr}")
        return

    kubeconfig_file.parent.mkdir(parents=True, exist_ok=True)
    kubeconfig_file.write_text(stdout)
    kubeconfig_file.chmod(0o600)

    # Merge with default config
    if DEFAULT_KUBECONFIG.exists():
        env = os.environ.copy()
        env["KUBECONFIG"] = f"{DEFAULT_KUBECONFIG}:{kubeconfig_file}"
        result = subprocess.run(
            ["kubectl", "config", "view", "--flatten"],
            capture_output=True,
            text=True,
            env=env,
        )
        DEFAULT_KUBECONFIG.write_text(result.stdout)
    else:
        DEFAULT_KUBECONFIG.write_text(kubeconfig_file.read_text())

    DEFAULT_KUBECONFIG.chmod(0o600)
    kubeconfig_file.unlink()


# --- Container Management ---


def get_kind_containers(backend: VMBackend | DirectBackend, exclude_cluster: str | None = None) -> list[str]:
    """Get names of all running Kind containers, optionally excluding a cluster."""
    returncode, stdout, _ = backend.exec(
        ["docker", "ps", "--filter", "label=io.x-k8s.kind.cluster", "--format", "{{.Names}}"],
        check=False,
    )
    if returncode != 0:
        return []
    containers = [c.strip() for c in stdout.strip().split("\n") if c.strip()]
    if exclude_cluster:
        containers = [c for c in containers if not c.startswith(f"{exclude_cluster}-")]
    return containers


def stop_containers(backend: VMBackend | DirectBackend, containers: list[str]) -> bool:
    """Stop Docker containers by name."""
    if not containers:
        return True
    returncode, _, _ = backend.exec(
        ["docker", "stop"] + containers,
        check=False,
    )
    return returncode == 0


def start_containers(backend: VMBackend | DirectBackend, containers: list[str]) -> bool:
    """Start Docker containers by name."""
    if not containers:
        return True
    returncode, _, _ = backend.exec(
        ["docker", "start"] + containers,
        check=False,
    )
    return returncode == 0


# --- Image Loading ---


def load_image(
    backend: VMBackend | DirectBackend,
    cluster_name: str,
    image_name: str,
    mode: Mode,
) -> bool:
    """Load Docker image into Kind cluster.

    Handles the difference between VM and Direct modes:
    - VM mode: docker save | limactl shell ... docker load, then kind load inside VM
    - Direct mode: kind load directly
    """
    print(f"Loading image '{image_name}' into cluster '{cluster_name}'...")

    if mode == Mode.VM:
        # VM mode: need to transfer image to VM first
        print("  Transferring to VM...", end=" ", flush=True)
        start = time.time()

        # docker save | limactl shell ... docker load
        save_proc = subprocess.Popen(
            ["docker", "save", image_name],
            stdout=subprocess.PIPE,
        )

        # Get VM name from backend
        vm_name = backend.vm_name if isinstance(backend, VMBackend) else LIMA_VM

        load_proc = subprocess.Popen(
            ["limactl", "shell", vm_name, "--", "docker", "load"],
            stdin=save_proc.stdout,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        save_proc.stdout.close()
        stdout, stderr = load_proc.communicate()

        if load_proc.returncode != 0:
            print("FAILED")
            print(f"    Error: {stderr.decode()}")
            return False
        print(f"done ({time.time() - start:.1f}s)")

        # Now load into Kind cluster (inside VM)
        print("  Loading into Kind...", end=" ", flush=True)
        start = time.time()
        returncode, _, stderr = backend.exec(
            ["kind", "load", "docker-image", image_name, "--name", cluster_name],
            check=False,
        )
        if returncode != 0:
            print("FAILED")
            print(f"    Error: {stderr}")
            return False
        print(f"done ({time.time() - start:.1f}s)")

    else:
        # Direct mode: kind load directly
        print("  Loading into Kind...", end=" ", flush=True)
        start = time.time()
        returncode, _, stderr = backend.exec(
            ["kind", "load", "docker-image", image_name, "--name", cluster_name],
            check=False,
        )
        if returncode != 0:
            print("FAILED")
            print(f"    Error: {stderr}")
            return False
        print(f"done ({time.time() - start:.1f}s)")

    print(f"Successfully loaded '{image_name}'")
    return True


# --- CLI Commands ---


def cmd_status(env: Environment, backend: VMBackend | DirectBackend) -> int:
    """Show environment status."""
    print("=" * 60)
    print("q_branch Development Environment")
    print("=" * 60)
    print(f"Mode:   {env.mode.value}")
    print(f"OS:     {env.os_type}")
    print(f"Docker: {'available' if env.has_docker else 'NOT AVAILABLE'}")

    if env.mode == Mode.VM:
        print(f"KVM:    {'available' if env.has_kvm else 'not available'}")
        print(f"Lima:   {'available' if env.has_lima else 'NOT AVAILABLE'}")

        if isinstance(backend, VMBackend):
            vm_status = backend.status()
            print(f"\nVM '{LIMA_VM}': {vm_status if vm_status else 'Not created'}")

            if vm_status == "Running":
                # Check Docker inside VM
                returncode, _, _ = backend.exec(["docker", "info"], check=False, capture=True)
                docker_in_vm = "ready" if returncode == 0 else "NOT READY"
                print(f"Docker in VM: {docker_in_vm}")
    else:
        print("\nDirect mode: Using host Docker (no VM needed)")

    # List Kind clusters
    returncode, stdout, _ = backend.exec(["kind", "get", "clusters"], check=False)
    if returncode == 0 and stdout.strip():
        clusters = stdout.strip().split("\n")
        print(f"\nKind clusters ({len(clusters)}):")
        for cluster in clusters:
            print(f"  - {cluster}")
    else:
        print("\nNo Kind clusters found")

    print("=" * 60)
    return 0


def cmd_list(env: Environment, backend: VMBackend | DirectBackend) -> int:
    """List all Kind clusters."""
    returncode, stdout, stderr = backend.exec(["kind", "get", "clusters"], check=False)

    if returncode != 0:
        print(f"Error listing clusters: {stderr}")
        return 1

    clusters = stdout.strip().split("\n") if stdout.strip() else []

    if not clusters:
        print("No Kind clusters found")
        return 0

    print(f"Kind clusters ({len(clusters)}):\n")
    for cluster in clusters:
        context = f"kind-{cluster}"
        # Try to get node count
        result = subprocess.run(
            ["kubectl", "--context", context, "get", "nodes", "-o", "json"],
            capture_output=True,
            text=True,
        )
        if result.returncode == 0:
            try:
                data = json.loads(result.stdout)
                node_count = len(data.get("items", []))
                print(f"  {cluster} ({node_count} nodes)")
            except json.JSONDecodeError:
                print(f"  {cluster}")
        else:
            print(f"  {cluster}")

    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="q_branch Kubernetes Development Environment Manager",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  ./dev.py               # Show environment status
  ./dev.py status        # Show environment status
  ./dev.py list          # List all Kind clusters

This script is primarily a library for q_branch projects.
For project-specific operations, use the project's dev.py:
  ./fine-grained-monitor/dev.py cluster deploy
""",
    )

    subparsers = parser.add_subparsers(dest="command")

    # status (default)
    subparsers.add_parser("status", help="Show environment status")

    # list
    subparsers.add_parser("list", help="List all Kind clusters")

    args = parser.parse_args()

    # Default to status if no command
    if not args.command:
        args.command = "status"

    try:
        # Detect environment and create backend
        env = detect_environment()
        backend = create_backend(env, LIMA_VM)

        if args.command == "status":
            return cmd_status(env, backend)
        elif args.command == "list":
            return cmd_list(env, backend)
        else:
            parser.print_help()
            return 1

    except CommandError as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nCancelled", file=sys.stderr)
        return 130


if __name__ == "__main__":
    sys.exit(main())
