#!/usr/bin/env python3
# /// script
# requires-python = ">=3.9"
# dependencies = []
# ///
"""
Gadget Development Environment Manager

Manages a Kubernetes development environment using either:
- VM mode: Lima VM + Kind cluster (macOS or Linux with KVM)
- Direct mode: Kind cluster on host Docker (Linux without KVM)
"""

import argparse
import json
import os
import platform
import subprocess
import sys
import time
from dataclasses import dataclass
from enum import Enum
from pathlib import Path

# Configuration
VM_NAME = "gadget-k8s-host"
CLUSTER_NAME = "gadget-dev"
LIMA_YAML = "gadget-k8s-host.lima.yaml"
KIND_VERSION = "v0.27.0"
KUBECTL_VERSION = "v1.32"

# Paths
SCRIPT_DIR = Path(__file__).parent.resolve()
HOME = Path.home()
KUBECONFIG_FILE = HOME / ".kube" / f"{VM_NAME}.yaml"
DEFAULT_KUBECONFIG = HOME / ".kube" / "config"


class Mode(Enum):
    """Environment mode"""

    VM = "vm"
    DIRECT = "direct"


@dataclass
class Environment:
    """Detected environment configuration"""

    mode: Mode
    os_type: str
    has_docker: bool
    has_kvm: bool
    has_lima: bool

    def __str__(self) -> str:
        return f"Mode: {self.mode.value}, OS: {self.os_type}, Docker: {self.has_docker}, KVM: {self.has_kvm}, Lima: {self.has_lima}"


class CommandError(Exception):
    """Command execution failed"""

    pass


def run_command(
    cmd: list[str], check: bool = True, capture: bool = True, input_data: str | None = None, timeout: int | None = None
) -> tuple[int, str, str]:
    """Execute a command and return exit code, stdout, stderr"""
    try:
        result = subprocess.run(cmd, check=check, capture_output=capture, text=True, input=input_data, timeout=timeout)
        return result.returncode, result.stdout, result.stderr
    except subprocess.CalledProcessError as e:
        if check:
            raise CommandError(f"Command failed: {' '.join(cmd)}\n{e.stderr}") from e
        return e.returncode, e.stdout, e.stderr
    except subprocess.TimeoutExpired as e:
        raise CommandError(f"Command timed out: {' '.join(cmd)}") from e
    except FileNotFoundError as e:
        raise CommandError(f"Command not found: {cmd[0]}") from e


def detect_environment() -> Environment:
    """Detect the runtime environment and determine mode"""
    os_type = platform.system().lower()

    # Check for Docker
    has_docker = False
    try:
        returncode, _, _ = run_command(["docker", "info"], check=False)
        has_docker = returncode == 0
    except CommandError:
        pass

    # Check for KVM (Linux only)
    has_kvm = os_type == "linux" and Path("/dev/kvm").exists()

    # Check for Lima
    has_lima = False
    try:
        returncode, _, _ = run_command(["limactl", "--version"], check=False)
        has_lima = returncode == 0
    except CommandError:
        pass

    # Determine mode
    if os_type == "linux" and has_docker and not has_kvm:
        mode = Mode.DIRECT
    else:
        mode = Mode.VM

    return Environment(mode=mode, os_type=os_type, has_docker=has_docker, has_kvm=has_kvm, has_lima=has_lima)


class VMBackend:
    """Lima VM operations"""

    def __init__(self, vm_name: str, lima_yaml: Path):
        self.vm_name = vm_name
        self.lima_yaml = lima_yaml

    def exists(self) -> bool:
        """Check if VM exists"""
        try:
            returncode, stdout, _ = run_command(["limactl", "list", "--format", "{{.Name}}"], check=False)
            if returncode != 0:
                return False
            return self.vm_name in stdout.splitlines()
        except CommandError:
            return False

    def status(self) -> str | None:
        """Get VM status (Running, Stopped, etc.)"""
        if not self.exists():
            return None
        try:
            returncode, stdout, _ = run_command(["limactl", "list", "--format", "{{.Name}} {{.Status}}"], check=False)
            if returncode != 0:
                return None
            for line in stdout.splitlines():
                parts = line.split()
                if parts and parts[0] == self.vm_name:
                    return parts[1] if len(parts) > 1 else "Unknown"
        except CommandError:
            pass
        return None

    def create(self) -> None:
        """Create the VM"""
        if not self.lima_yaml.exists():
            raise CommandError(f"Lima config not found: {self.lima_yaml}")

        print("Creating VM (this takes ~5 minutes)...")
        run_command(["limactl", "create", "--name", self.vm_name, str(self.lima_yaml), "--tty=false"], capture=False)

    def start(self) -> None:
        """Start the VM"""
        print(f"Starting VM '{self.vm_name}'...")
        run_command(["limactl", "start", self.vm_name], capture=False)

    def stop(self) -> None:
        """Stop the VM"""
        print(f"Stopping VM '{self.vm_name}'...")
        run_command(["limactl", "stop", self.vm_name], capture=False)

    def delete(self) -> None:
        """Delete the VM"""
        print(f"Deleting VM '{self.vm_name}'...")
        run_command(["limactl", "delete", self.vm_name], capture=False)

    def exec(
        self, cmd: list[str], check: bool = True, capture: bool = True, input_data: str | None = None
    ) -> tuple[int, str, str]:
        """Execute command inside VM"""
        lima_cmd = ["limactl", "shell", self.vm_name, "--"] + cmd
        return run_command(lima_cmd, check=check, capture=capture, input_data=input_data)

    def wait_for_docker(self, timeout: int = 120) -> None:
        """Wait for Docker to be ready inside VM"""
        print("Waiting for Docker in VM...")
        start_time = time.time()
        while time.time() - start_time < timeout:
            try:
                returncode, _, _ = self.exec(["docker", "info"], check=False)
                if returncode == 0:
                    return
            except CommandError:
                pass
            time.sleep(2)
        raise CommandError("Timeout waiting for Docker in VM")


class DirectBackend:
    """Direct host Docker operations"""

    def exec(
        self, cmd: list[str], check: bool = True, capture: bool = True, input_data: str | None = None
    ) -> tuple[int, str, str]:
        """Execute command on host"""
        return run_command(cmd, check=check, capture=capture, input_data=input_data)


class KindCluster:
    """Kind cluster operations"""

    KIND_CONFIG = """kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: 6443
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30000
        hostPort: 30000
      - containerPort: 30080
        hostPort: 30080
      - containerPort: 30443
        hostPort: 30443
  - role: worker
  - role: worker
"""

    def __init__(self, cluster_name: str, backend):
        self.cluster_name = cluster_name
        self.backend = backend

    def exists(self) -> bool:
        """Check if cluster exists"""
        try:
            returncode, stdout, _ = self.backend.exec(["kind", "get", "clusters"], check=False)
            if returncode != 0:
                return False
            return self.cluster_name in stdout.splitlines()
        except CommandError:
            return False

    def create(self) -> None:
        """Create Kind cluster"""
        print(f"Creating Kind cluster '{self.cluster_name}'...")
        self.backend.exec(
            ["kind", "create", "cluster", "--name", self.cluster_name, "--config", "/dev/stdin"],
            input_data=self.KIND_CONFIG,
            capture=False,
        )

    def delete(self) -> None:
        """Delete Kind cluster"""
        print(f"Deleting cluster '{self.cluster_name}'...")
        self.backend.exec(["kind", "delete", "cluster", "--name", self.cluster_name], capture=False)

    def get_kubeconfig(self) -> str:
        """Get kubeconfig for cluster"""
        returncode, stdout, stderr = self.backend.exec(["kind", "get", "kubeconfig", "--name", self.cluster_name])
        if returncode != 0:
            raise CommandError(f"Failed to get kubeconfig: {stderr}")
        return stdout

    def get_nodes(self) -> list[str]:
        """Get cluster nodes"""
        try:
            returncode, stdout, _ = run_command(
                ["kubectl", "--context", f"kind-{self.cluster_name}", "get", "nodes", "-o", "json"], check=False
            )
            if returncode != 0:
                return []
            data = json.loads(stdout)
            return [node["metadata"]["name"] for node in data.get("items", [])]
        except (CommandError, json.JSONDecodeError, KeyError):
            return []


class KubeConfig:
    """Kubeconfig management"""

    @staticmethod
    def merge(cluster_name: str, new_config_content: str) -> None:
        """Merge kubeconfig into default config"""
        context_name = f"kind-{cluster_name}"

        # Delete existing entries for this cluster
        for resource in ["context", "cluster", "user"]:
            try:
                run_command(["kubectl", "config", "delete-" + resource, context_name], check=False, capture=True)
            except CommandError:
                pass

        # Write new config to temp file
        KUBECONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
        KUBECONFIG_FILE.write_text(new_config_content)
        KUBECONFIG_FILE.chmod(0o600)

        if DEFAULT_KUBECONFIG.exists():
            # Merge with existing config
            env = os.environ.copy()
            env["KUBECONFIG"] = f"{DEFAULT_KUBECONFIG}:{KUBECONFIG_FILE}"

            result = subprocess.run(
                ["kubectl", "config", "view", "--flatten"], capture_output=True, text=True, env=env, check=True
            )

            temp_file = DEFAULT_KUBECONFIG.with_suffix(".tmp")
            temp_file.write_text(result.stdout)
            temp_file.replace(DEFAULT_KUBECONFIG)
        else:
            # No existing config, just copy
            KUBECONFIG_FILE.replace(DEFAULT_KUBECONFIG)

        DEFAULT_KUBECONFIG.chmod(0o600)

        # Clean up temp file
        if KUBECONFIG_FILE.exists():
            KUBECONFIG_FILE.unlink()


def interactive_confirm(prompt: str, force: bool = False) -> bool:
    """Ask user for confirmation (returns True if force=True)"""
    if force:
        return True

    response = input(f"{prompt} [y/N] ").strip().lower()
    return response in ("y", "yes")


def cmd_status(env: Environment, backend, cluster: KindCluster) -> int:
    """Show status of environment"""
    print("=" * 60)
    print("Gadget Development Environment Status")
    print("=" * 60)
    print(f"Mode: {env.mode.value}")
    print(f"OS: {env.os_type}")
    print(f"Docker available: {env.has_docker}")

    if env.mode == Mode.VM:
        print(f"KVM available: {env.has_kvm}")
        print(f"Lima available: {env.has_lima}")

        if isinstance(backend, VMBackend):
            vm_status = backend.status()
            print(f"\nVM '{VM_NAME}': {vm_status if vm_status else 'Not created'}")

    cluster_exists = cluster.exists()
    print(f"\nCluster '{CLUSTER_NAME}': {'Running' if cluster_exists else 'Not created'}")

    if cluster_exists:
        nodes = cluster.get_nodes()
        if nodes:
            print(f"Nodes ({len(nodes)}):")
            for node in nodes:
                print(f"  - {node}")

        print(f"\nKubectl context: kind-{CLUSTER_NAME}")

    print("=" * 60)
    return 0


def cmd_start(env: Environment, backend, cluster: KindCluster, force: bool = False, recreate: bool = False) -> int:
    """Start/create the development environment"""
    print(f"Setting up development environment in {env.mode.value} mode...")

    if env.mode == Mode.VM:
        if not isinstance(backend, VMBackend):
            print("Error: VM backend not available", file=sys.stderr)
            return 1

        if not env.has_lima:
            print("Error: Lima is not installed. Please install Lima first.", file=sys.stderr)
            print("  https://lima-vm.io/", file=sys.stderr)
            return 1

        # Handle VM
        if backend.exists():
            vm_status = backend.status()
            if vm_status == "Running":
                print(f"VM '{VM_NAME}' is already running.")
            else:
                print(f"VM exists but is {vm_status}. Starting...")
                backend.start()
        else:
            backend.create()
            backend.start()
            # Restart to ensure docker group membership is active
            print("Restarting VM to activate docker group...")
            backend.stop()
            backend.start()

        # Wait for Docker
        backend.wait_for_docker()

    elif env.mode == Mode.DIRECT:
        if not env.has_docker:
            print("Error: Docker is not available", file=sys.stderr)
            return 1

        # Check if kind is installed
        try:
            run_command(["kind", "--version"], check=True, capture=True)
        except CommandError:
            print("Error: kind is not installed", file=sys.stderr)
            print("", file=sys.stderr)
            print("Install kind with:", file=sys.stderr)
            if env.os_type == "linux":
                arch = platform.machine()
                kind_arch = "amd64" if arch == "x86_64" else "arm64"
                print(
                    f"  curl -Lo /tmp/kind https://kind.sigs.k8s.io/dl/{KIND_VERSION}/kind-linux-{kind_arch}",
                    file=sys.stderr,
                )
                print("  chmod +x /tmp/kind", file=sys.stderr)
                print("  sudo mv /tmp/kind /usr/local/bin/kind", file=sys.stderr)
            else:
                print("  See: https://kind.sigs.k8s.io/docs/user/quick-start/#installation", file=sys.stderr)
            return 1

        # Check if helm is installed and set up Datadog repo
        try:
            run_command(["helm", "version"], check=True, capture=True)
            print("Setting up Datadog Helm repository...")
            # helm repo add is idempotent - safe to run multiple times
            run_command(["helm", "repo", "add", "datadog", "https://helm.datadoghq.com"], check=False, capture=True)
            run_command(["helm", "repo", "update"], check=False, capture=True)
        except CommandError:
            print("Note: helm is not installed (optional)", file=sys.stderr)
            print("", file=sys.stderr)
            print("To install Helm (for Datadog Operator installation):", file=sys.stderr)
            print("  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash", file=sys.stderr)

        print("Using host Docker directly (no VM needed)")

    # Handle cluster
    if cluster.exists():
        if recreate or interactive_confirm("Cluster exists. Recreate?", force):
            cluster.delete()
        else:
            print("Keeping existing cluster. Updating kubeconfig...")
            kubeconfig = cluster.get_kubeconfig()
            KubeConfig.merge(CLUSTER_NAME, kubeconfig)
            print(f"Done. Context 'kind-{CLUSTER_NAME}' is available.")
            return 0

    # Create cluster
    cluster.create()

    # Merge kubeconfig
    print("Merging kubeconfig into ~/.kube/config...")
    kubeconfig = cluster.get_kubeconfig()
    KubeConfig.merge(CLUSTER_NAME, kubeconfig)

    # Verify
    print("\nVerifying...")
    run_command(["kubectl", "--context", f"kind-{CLUSTER_NAME}", "get", "nodes"], capture=False)

    print("\n" + "=" * 60)
    print("Ready!")
    print("")
    print(f"  kubectl --context kind-{CLUSTER_NAME} get nodes")
    print(f"  kubectx kind-{CLUSTER_NAME}")
    if env.mode == Mode.VM:
        print(f"\nSSH: limactl shell {VM_NAME}")
    print("=" * 60)

    return 0


def cmd_stop(env: Environment, backend, cluster: KindCluster) -> int:
    """Stop the environment"""
    if env.mode == Mode.VM:
        if not isinstance(backend, VMBackend):
            print("Error: VM backend not available", file=sys.stderr)
            return 1

        if not backend.exists():
            print(f"VM '{VM_NAME}' does not exist")
            return 1

        vm_status = backend.status()
        if vm_status == "Stopped":
            print(f"VM '{VM_NAME}' is already stopped")
            return 0

        backend.stop()
        print(f"VM '{VM_NAME}' stopped")
    else:
        print("Direct mode: nothing to stop (cluster remains active)")
        print("Use 'delete' to remove the cluster")

    return 0


def cmd_delete(env: Environment, backend, cluster: KindCluster, force: bool = False) -> int:
    """Delete the environment"""
    if not force and not interactive_confirm("Delete environment?", force):
        print("Cancelled")
        return 0

    if env.mode == Mode.VM:
        if not isinstance(backend, VMBackend):
            print("Error: VM backend not available", file=sys.stderr)
            return 1

        if backend.exists():
            backend.delete()
            print(f"VM '{VM_NAME}' deleted (cluster deleted with it)")
        else:
            print(f"VM '{VM_NAME}' does not exist")
    else:
        if cluster.exists():
            cluster.delete()
            print(f"Cluster '{CLUSTER_NAME}' deleted")
        else:
            print(f"Cluster '{CLUSTER_NAME}' does not exist")

    return 0


def cmd_load_image(env: Environment, backend, cluster: KindCluster, image_name: str) -> int:
    """Load a Docker image into the Kind cluster"""
    # Verify cluster exists
    if not cluster.exists():
        print(f"Error: Cluster '{CLUSTER_NAME}' does not exist", file=sys.stderr)
        print("Run './gadget-dev.py start' first", file=sys.stderr)
        return 1

    # Verify image exists locally
    try:
        returncode, _, _ = run_command(["docker", "images", "-q", image_name], check=False)
        if returncode != 0:
            print(f"Error: Image '{image_name}' not found locally", file=sys.stderr)
            print("Build the image first (e.g., dda inv omnibus.docker-build)", file=sys.stderr)
            return 1
    except CommandError as e:
        print(f"Error checking for image: {e}", file=sys.stderr)
        return 1

    print(f"Loading image '{image_name}' into cluster '{CLUSTER_NAME}'...")

    if env.mode == Mode.VM:
        if not isinstance(backend, VMBackend):
            print("Error: VM backend not available", file=sys.stderr)
            return 1

        print("Saving image from host Docker...")
        # docker save and pipe to VM docker load, then kind load
        save_cmd = ["docker", "save", image_name]
        try:
            # Save image
            save_proc = subprocess.Popen(save_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

            # Load into VM docker
            load_result = subprocess.run(
                ["limactl", "shell", VM_NAME, "--", "docker", "load"],
                stdin=save_proc.stdout,
                capture_output=True,
                text=True,
            )
            save_proc.stdout.close()
            save_proc.wait()

            if load_result.returncode != 0:
                print(f"Error loading image into VM: {load_result.stderr}", file=sys.stderr)
                return 1

            print("Loading image into Kind cluster...")
            # Load into Kind
            backend.exec(["kind", "load", "docker-image", image_name, "--name", CLUSTER_NAME], capture=False)
        except Exception as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1

    else:  # Direct mode
        try:
            run_command(["kind", "load", "docker-image", image_name, "--name", CLUSTER_NAME], capture=False)
        except CommandError as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1

    print(f"Successfully loaded '{image_name}' into cluster")
    return 0


def cmd_deploy(env: Environment, backend, cluster: KindCluster, yaml_file: str) -> int:
    """Deploy manifest and restart agent pods"""
    # Verify cluster exists
    if not cluster.exists():
        print(f"Error: Cluster '{CLUSTER_NAME}' does not exist", file=sys.stderr)
        print("Run './gadget-dev.py start' first", file=sys.stderr)
        return 1

    # Verify yaml file exists
    yaml_path = Path(yaml_file)
    if not yaml_path.exists():
        print(f"Error: File '{yaml_file}' not found", file=sys.stderr)
        return 1

    context = f"kind-{CLUSTER_NAME}"

    # Apply manifest
    print(f"Applying {yaml_file}...")
    try:
        run_command(["kubectl", "apply", "-f", str(yaml_path), "--context", context], capture=False)
    except CommandError as e:
        print(f"Error applying manifest: {e}", file=sys.stderr)
        return 1

    # Wait a moment for deployment to register
    print("Waiting for deployment to register...")
    time.sleep(2)

    # Restart agent pods
    print("Restarting agent pods...")
    try:
        returncode, stdout, stderr = run_command(
            [
                "kubectl",
                "delete",
                "pods",
                "-l",
                "app.kubernetes.io/name=datadog-agent-deployment",
                "-n",
                "default",
                "--context",
                context,
            ],
            check=False,
        )
        if returncode != 0:
            if "No resources found" in stderr:
                print("Note: No existing agent pods found (first deployment?)")
            else:
                print(f"Warning: Error restarting pods: {stderr}", file=sys.stderr)
        else:
            print(stdout.strip())
    except CommandError as e:
        print(f"Warning: Could not restart agent pods: {e}", file=sys.stderr)

    print("\nDeployment complete!")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Gadget Development Environment Manager", formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("--force", "-f", action="store_true", help="Skip interactive prompts (answer yes to all)")

    subparsers = parser.add_subparsers(dest="command", help="Command to execute")

    # status command (also default)
    subparsers.add_parser("status", help="Show environment status")

    # start command
    start_parser = subparsers.add_parser("start", help="Start/create environment")
    start_parser.add_argument("--recreate", action="store_true", help="Force recreate cluster if it exists")

    # stop command
    subparsers.add_parser("stop", help="Stop environment (VM mode only)")

    # delete command
    subparsers.add_parser("delete", help="Delete environment")

    # load-image command
    load_image_parser = subparsers.add_parser("load-image", help="Load Docker image into Kind cluster")
    load_image_parser.add_argument("image_name", help="Docker image name (e.g., localhost/datadog-agent:local)")

    # deploy command
    deploy_parser = subparsers.add_parser("deploy", help="Deploy manifest and restart agent pods")
    deploy_parser.add_argument(
        "yaml_file", nargs="?", default="test-cluster.yaml", help="Path to manifest file (default: test-cluster.yaml)"
    )

    args = parser.parse_args()

    # Default to status if no command specified
    if not args.command:
        args.command = "status"

    try:
        # Detect environment
        env = detect_environment()

        # Create backend
        if env.mode == Mode.VM:
            lima_yaml = SCRIPT_DIR / LIMA_YAML
            backend = VMBackend(VM_NAME, lima_yaml)
        else:
            backend = DirectBackend()

        # Create cluster manager
        cluster = KindCluster(CLUSTER_NAME, backend)

        # Execute command
        if args.command == "status":
            return cmd_status(env, backend, cluster)
        elif args.command == "start":
            return cmd_start(env, backend, cluster, args.force, args.recreate)
        elif args.command == "stop":
            return cmd_stop(env, backend, cluster)
        elif args.command == "delete":
            return cmd_delete(env, backend, cluster, args.force)
        elif args.command == "load-image":
            return cmd_load_image(env, backend, cluster, args.image_name)
        elif args.command == "deploy":
            return cmd_deploy(env, backend, cluster, args.yaml_file)
        else:
            parser.print_help()
            return 1

    except CommandError as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nCancelled", file=sys.stderr)
        return 130
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        import traceback

        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())
