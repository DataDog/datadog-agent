#!/usr/bin/env python3
"""
Cluster management for MCP evaluation scenarios.

Creates and manages a dedicated KIND cluster in the Lima VM for testing
LLM diagnostic capabilities with Kubernetes scenarios.
"""

import argparse
import os
import subprocess
import sys
from pathlib import Path

# Configuration
LIMA_VM = "gadget-k8s-host"
CLUSTER_NAME = "mcp-eval"
KUBE_CONTEXT = f"kind-{CLUSTER_NAME}"


def run_command(cmd, capture_output=False, check=True):
    """Run a command and return result."""
    if capture_output:
        result = subprocess.run(cmd, capture_output=True, text=True, check=check)
        return result.stdout.strip()
    else:
        result = subprocess.run(cmd, check=check)
        return result.returncode == 0


def check_lima_vm():
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


def cluster_exists():
    """Check if the MCP evaluation cluster exists."""
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--", "kind", "get", "clusters"],
        capture_output=True,
        text=True,
    )
    return CLUSTER_NAME in result.stdout


def get_kind_containers(exclude_cluster=None):
    """Get names of all running KIND containers, optionally excluding a cluster."""
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "docker", "ps",
         "--filter", "label=io.x-k8s.kind.cluster",
         "--format", "{{.Names}}"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return []

    containers = [c.strip() for c in result.stdout.strip().split("\n") if c.strip()]
    if exclude_cluster:
        containers = [c for c in containers if not c.startswith(f"{exclude_cluster}-")]
    return containers


def stop_containers(containers):
    """Stop Docker containers by name."""
    if not containers:
        return True
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--", "docker", "stop"] + containers,
        capture_output=True,
        text=True,
    )
    return result.returncode == 0


def start_containers(containers):
    """Start Docker containers by name."""
    if not containers:
        return True
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--", "docker", "start"] + containers,
        capture_output=True,
        text=True,
    )
    return result.returncode == 0


def merge_kubeconfig():
    """Merge cluster kubeconfig into ~/.kube/config."""
    kubeconfig_file = Path.home() / ".kube" / f"{CLUSTER_NAME}.yaml"
    default_kubeconfig = Path.home() / ".kube" / "config"

    # Remove existing entries for this cluster
    for resource in ["context", "cluster", "user"]:
        subprocess.run(
            ["kubectl", "config", f"delete-{resource}", KUBE_CONTEXT],
            capture_output=True,
        )

    # Extract kubeconfig from KIND inside Lima VM
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kind", "get", "kubeconfig", "--name", CLUSTER_NAME],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(f"Warning: Failed to get kubeconfig: {result.stderr}")
        return False

    # Write kubeconfig to host machine
    kubeconfig_file.parent.mkdir(parents=True, exist_ok=True)
    kubeconfig_file.write_text(result.stdout)
    kubeconfig_file.chmod(0o600)

    # Merge with default config
    if default_kubeconfig.exists():
        env = os.environ.copy()
        env["KUBECONFIG"] = f"{default_kubeconfig}:{kubeconfig_file}"
        result = subprocess.run(
            ["kubectl", "config", "view", "--flatten"],
            capture_output=True,
            text=True,
            env=env,
        )
        if result.returncode == 0:
            default_kubeconfig.write_text(result.stdout)
        else:
            print(f"Warning: Failed to merge kubeconfig: {result.stderr}")
            return False
    else:
        default_kubeconfig.write_text(kubeconfig_file.read_text())

    return True


def create_cluster():
    """Create the MCP evaluation KIND cluster.

    Note: KIND cluster creation can fail if other KIND clusters are running
    concurrently due to resource contention during kubeadm init. This function
    temporarily stops other clusters during creation.
    """
    print(f"Creating KIND cluster: {CLUSTER_NAME}")

    if not check_lima_vm():
        print(f"ERROR: Lima VM '{LIMA_VM}' is not running", file=sys.stderr)
        print(f"Start it with: limactl start {LIMA_VM}", file=sys.stderr)
        return False

    if cluster_exists():
        print(f"Cluster '{CLUSTER_NAME}' already exists")
        return True

    # Temporarily stop other KIND clusters to avoid resource contention
    other_containers = get_kind_containers(exclude_cluster=CLUSTER_NAME)
    if other_containers:
        print(f"Temporarily stopping {len(other_containers)} containers from other clusters...")
        if not stop_containers(other_containers):
            print("Warning: Failed to stop some containers")

    # KIND cluster configuration
    # No port mappings needed - kubectl uses the API server which KIND exposes automatically
    kind_config = """
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
"""

    # Write config to temp file in Lima VM
    print("Writing KIND configuration...")
    subprocess.run(
        ["limactl", "shell", LIMA_VM, "--", "bash", "-c",
         f"cat > /tmp/kind-mcp-eval.yaml << 'EOF'\n{kind_config}\nEOF"],
        check=True,
    )

    # Create cluster
    print("Creating cluster (this may take 1-2 minutes)...")
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kind", "create", "cluster",
         "--name", CLUSTER_NAME,
         "--config", "/tmp/kind-mcp-eval.yaml"],
        check=False,
    )

    # Restart other clusters
    if other_containers:
        print(f"Restarting {len(other_containers)} containers from other clusters...")
        if not start_containers(other_containers):
            print("Warning: Failed to restart some containers")

    if result.returncode != 0:
        print("ERROR: Failed to create cluster", file=sys.stderr)
        return False

    # Extract and merge kubeconfig so context is available on host
    print("Merging kubeconfig to host machine...", end=" ", flush=True)
    if not merge_kubeconfig():
        print("failed")
        print("WARNING: Cluster created but kubeconfig merge failed", file=sys.stderr)
        return False
    print("done")

    print(f"\n✓ Cluster '{CLUSTER_NAME}' created successfully")
    print(f"  Context: {KUBE_CONTEXT}")
    print(f"\nUse cluster with:")
    print(f"  kubectl --context {KUBE_CONTEXT} get pods")
    print(f"  kubectx {KUBE_CONTEXT}")
    print(f"\nOr deploy scenarios with:")
    print(f"  ./cluster.py deploy <scenario-name>")

    return True


def destroy_cluster():
    """Destroy the MCP evaluation KIND cluster."""
    if not cluster_exists():
        print(f"Cluster '{CLUSTER_NAME}' does not exist")
        return True

    print(f"Destroying cluster '{CLUSTER_NAME}'...")
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kind", "delete", "cluster", "--name", CLUSTER_NAME],
        check=False,
    )

    if result.returncode != 0:
        print("ERROR: Failed to destroy cluster", file=sys.stderr)
        return False

    # Clean up kubeconfig entries
    print("Cleaning up kubeconfig...")
    for resource in ["context", "cluster", "user"]:
        subprocess.run(
            ["kubectl", "config", f"delete-{resource}", KUBE_CONTEXT],
            capture_output=True,
        )

    # Remove temporary kubeconfig file
    kubeconfig_file = Path.home() / ".kube" / f"{CLUSTER_NAME}.yaml"
    if kubeconfig_file.exists():
        kubeconfig_file.unlink()

    print(f"✓ Cluster '{CLUSTER_NAME}' destroyed")
    return True


def status():
    """Show cluster status."""
    print(f"Cluster: {CLUSTER_NAME}")
    print(f"Context: {KUBE_CONTEXT}")
    print(f"Lima VM: {LIMA_VM}")
    print()

    # Check Lima VM
    if not check_lima_vm():
        print(f"✗ Lima VM '{LIMA_VM}' is not running")
        print(f"  Start with: limactl start {LIMA_VM}")
        return

    print(f"✓ Lima VM '{LIMA_VM}' is running")

    # Check cluster
    if not cluster_exists():
        print(f"✗ Cluster '{CLUSTER_NAME}' does not exist")
        print(f"  Create with: ./cluster.py create")
        return

    print(f"✓ Cluster '{CLUSTER_NAME}' exists")
    print()

    # Show nodes
    print("Nodes:")
    subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kubectl", "get", "nodes", "--context", KUBE_CONTEXT],
        check=False,
    )
    print()

    # Show namespaces
    print("Namespaces:")
    subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kubectl", "get", "namespaces", "--context", KUBE_CONTEXT],
        check=False,
    )
    print()

    # Show all pods
    print("Pods (all namespaces):")
    subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kubectl", "get", "pods", "-A", "--context", KUBE_CONTEXT],
        check=False,
    )


def inject_scripts(content, scenario_dir):
    """Inject Python scripts into deploy.yaml template.

    Replaces __INJECT_SCRIPT__ markers with actual script content from .py files.
    Supports multiple scripts by matching filename patterns in the YAML.
    """
    import re

    # Find all Python files in scenario directory
    python_files = {f.name: f for f in scenario_dir.glob("*.py")}

    if not python_files:
        return content

    # Find all patterns like "filename.py: __INJECT_SCRIPT__"
    # and replace with the actual script content
    for filename, filepath in python_files.items():
        # Pattern: "  filename.py: __INJECT_SCRIPT__"
        # We need to preserve indentation
        pattern = rf'^(\s*)({re.escape(filename)}:\s*)__INJECT_SCRIPT__'

        def replace_with_script(match):
            indent = match.group(1)
            key_part = match.group(2)

            # Read script content
            with open(filepath) as f:
                script_content = f.read()

            # Format script for YAML: literal style with proper indentation
            # The content needs to be indented relative to the key
            lines = script_content.split("\n")
            formatted_lines = ["|"]
            for line in lines:
                if line:  # Non-empty lines get 4 extra spaces
                    formatted_lines.append(f"{indent}    {line}")
                else:  # Empty lines stay empty
                    formatted_lines.append("")

            return f"{indent}{key_part}{formatted_lines[0]}\n" + "\n".join(formatted_lines[1:])

        content = re.sub(pattern, replace_with_script, content, flags=re.MULTILINE)

    return content


def deploy_scenario(scenario_name):
    """Deploy a scenario to the cluster."""
    scenarios_dir = Path(__file__).parent / "scenarios"
    scenario_dir = scenarios_dir / scenario_name

    if not scenario_dir.exists():
        print(f"ERROR: Scenario '{scenario_name}' not found", file=sys.stderr)
        print(f"\nAvailable scenarios:", file=sys.stderr)
        for s in sorted(scenarios_dir.iterdir()):
            if s.is_dir():
                print(f"  - {s.name}", file=sys.stderr)
        return False

    deploy_file = scenario_dir / "deploy.yaml"
    if not deploy_file.exists():
        print(f"ERROR: {deploy_file} not found", file=sys.stderr)
        return False

    if not cluster_exists():
        print(f"ERROR: Cluster '{CLUSTER_NAME}' does not exist", file=sys.stderr)
        print(f"Create it with: ./cluster.py create", file=sys.stderr)
        return False

    print(f"Deploying scenario: {scenario_name}")

    # Read deploy file and inject scripts
    with open(deploy_file) as f:
        content = f.read()

    content = inject_scripts(content, scenario_dir)

    # Copy processed deploy file to Lima VM
    temp_file = f"/tmp/scenario-{scenario_name}.yaml"
    subprocess.run(
        ["limactl", "shell", LIMA_VM, "--", "bash", "-c",
         f"cat > {temp_file} << 'EOF'\n{content}\nEOF"],
        check=True,
    )

    # Apply the manifest
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kubectl", "apply", "-f", temp_file, "--context", KUBE_CONTEXT],
        check=False,
    )

    if result.returncode != 0:
        print("ERROR: Failed to deploy scenario", file=sys.stderr)
        return False

    print(f"\n✓ Scenario '{scenario_name}' deployed")
    print(f"\nView logs with:")
    print(f"  kubectl logs <pod-name> --context {KUBE_CONTEXT}")
    print(f"\nOr use: ./cluster.py logs {scenario_name}")

    return True


def list_scenarios():
    """List available scenarios."""
    scenarios_dir = Path(__file__).parent / "scenarios"

    print("Available scenarios:")
    print()

    for scenario_dir in sorted(scenarios_dir.iterdir()):
        if scenario_dir.is_dir():
            deploy_file = scenario_dir / "deploy.yaml"
            if deploy_file.exists():
                print(f"  {scenario_dir.name}")


def logs_scenario(scenario_name):
    """Show logs for a deployed scenario."""
    if not cluster_exists():
        print(f"ERROR: Cluster '{CLUSTER_NAME}' does not exist", file=sys.stderr)
        return False

    # Try to find pods with the scenario label or name
    print(f"Showing logs for scenario: {scenario_name}")

    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kubectl", "get", "pods", "-A",
         "-o", "jsonpath={.items[*].metadata.name}",
         "--context", KUBE_CONTEXT],
        capture_output=True,
        text=True,
    )

    pods = result.stdout.strip().split()
    matching_pods = [p for p in pods if scenario_name in p]

    if not matching_pods:
        print(f"No pods found for scenario '{scenario_name}'", file=sys.stderr)
        return False

    pod_name = matching_pods[0]
    print(f"Following logs for pod: {pod_name}")
    print()

    subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kubectl", "logs", "-f", pod_name, "--context", KUBE_CONTEXT],
        check=False,
    )

    return True


def delete_scenario(scenario_name):
    """Delete a scenario and its resources from the cluster."""
    scenarios_dir = Path(__file__).parent / "scenarios"
    scenario_dir = scenarios_dir / scenario_name

    if not scenario_dir.exists():
        print(f"ERROR: Scenario '{scenario_name}' not found", file=sys.stderr)
        print(f"\nAvailable scenarios:", file=sys.stderr)
        for s in sorted(scenarios_dir.iterdir()):
            if s.is_dir():
                print(f"  - {s.name}", file=sys.stderr)
        return False

    deploy_file = scenario_dir / "deploy.yaml"
    if not deploy_file.exists():
        print(f"ERROR: {deploy_file} not found", file=sys.stderr)
        return False

    if not cluster_exists():
        print(f"ERROR: Cluster '{CLUSTER_NAME}' does not exist", file=sys.stderr)
        return False

    print(f"Deleting scenario: {scenario_name}")

    # Read deploy file and inject scripts
    with open(deploy_file) as f:
        content = f.read()

    content = inject_scripts(content, scenario_dir)

    # Copy processed deploy file to Lima VM
    temp_file = f"/tmp/scenario-{scenario_name}.yaml"
    subprocess.run(
        ["limactl", "shell", LIMA_VM, "--", "bash", "-c",
         f"cat > {temp_file} << 'EOF'\n{content}\nEOF"],
        check=True,
    )

    # Delete the resources using kubectl delete -f
    result = subprocess.run(
        ["limactl", "shell", LIMA_VM, "--",
         "kubectl", "delete", "-f", temp_file, "--context", KUBE_CONTEXT],
        check=False,
    )

    if result.returncode != 0:
        print("ERROR: Failed to delete scenario", file=sys.stderr)
        return False

    print(f"\n✓ Scenario '{scenario_name}' deleted")

    return True


def main():
    parser = argparse.ArgumentParser(
        description="Manage KIND cluster for MCP evaluation scenarios"
    )
    subparsers = parser.add_subparsers(dest="command", help="Command to run")

    # Create command
    subparsers.add_parser("create", help="Create the KIND cluster")

    # Destroy command
    subparsers.add_parser("destroy", help="Destroy the KIND cluster")

    # Status command
    subparsers.add_parser("status", help="Show cluster status")

    # Deploy command
    deploy_parser = subparsers.add_parser("deploy", help="Deploy a scenario")
    deploy_parser.add_argument("scenario", help="Scenario name to deploy")

    # List command
    subparsers.add_parser("list", help="List available scenarios")

    # Logs command
    logs_parser = subparsers.add_parser("logs", help="Show scenario logs")
    logs_parser.add_argument("scenario", help="Scenario name")

    # Delete command
    delete_parser = subparsers.add_parser("delete", help="Delete a scenario")
    delete_parser.add_argument("scenario", help="Scenario name to delete")

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        sys.exit(1)

    if args.command == "create":
        success = create_cluster()
        sys.exit(0 if success else 1)

    elif args.command == "destroy":
        success = destroy_cluster()
        sys.exit(0 if success else 1)

    elif args.command == "status":
        status()
        sys.exit(0)

    elif args.command == "deploy":
        success = deploy_scenario(args.scenario)
        sys.exit(0 if success else 1)

    elif args.command == "list":
        list_scenarios()
        sys.exit(0)

    elif args.command == "logs":
        success = logs_scenario(args.scenario)
        sys.exit(0 if success else 1)

    elif args.command == "delete":
        success = delete_scenario(args.scenario)
        sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
