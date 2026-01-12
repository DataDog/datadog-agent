"""
K8s Backend Abstraction for q_branch projects.

Provides VM vs Direct mode abstraction for running commands:
- VM mode: Commands run inside Lima VM via `limactl shell <vm> -- <cmd>`
- Direct mode: Commands run directly on host (for Linux without KVM, e.g., Workspaces)

Usage:
    from q_branch.lib.k8s_backend import detect_environment, create_backend

    env = detect_environment()
    backend = create_backend(env, vm_name="gadget-k8s-host")

    # All commands go through backend.exec()
    returncode, stdout, stderr = backend.exec(["kind", "get", "clusters"])
"""

import json
import os
import platform
import subprocess
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Protocol


class Mode(Enum):
    """Environment mode for K8s operations."""

    VM = "vm"  # Lima VM + Kind (macOS or Linux with KVM)
    DIRECT = "direct"  # Host Docker directly (Linux without KVM)


@dataclass
class Environment:
    """Detected environment configuration."""

    mode: Mode
    os_type: str  # "linux" | "darwin"
    has_docker: bool
    has_kvm: bool  # /dev/kvm exists (Linux only)
    has_lima: bool  # limactl available

    def __str__(self) -> str:
        return (
            f"Mode: {self.mode.value}, OS: {self.os_type}, "
            f"Docker: {self.has_docker}, KVM: {self.has_kvm}, Lima: {self.has_lima}"
        )


class CommandError(Exception):
    """Command execution failed."""

    pass


class Backend(Protocol):
    """Protocol defining the backend interface."""

    def exec(
        self,
        cmd: list[str],
        check: bool = True,
        capture: bool = True,
        input_data: str | None = None,
        timeout: int | None = None,
    ) -> tuple[int, str, str]:
        """Execute command.

        Args:
            cmd: Command and arguments
            check: Raise CommandError on non-zero exit
            capture: Capture stdout/stderr (if False, output goes to terminal)
            input_data: stdin input
            timeout: Timeout in seconds

        Returns:
            (returncode, stdout, stderr)
        """
        ...


def _run_command(
    cmd: list[str],
    check: bool = True,
    capture: bool = True,
    input_data: str | None = None,
    timeout: int | None = None,
    cwd: Path | None = None,
) -> tuple[int, str, str]:
    """Execute a command and return exit code, stdout, stderr."""
    try:
        result = subprocess.run(
            cmd,
            check=check,
            capture_output=capture,
            text=True,
            input=input_data,
            timeout=timeout,
            cwd=cwd,
        )
        return result.returncode, result.stdout or "", result.stderr or ""
    except subprocess.CalledProcessError as e:
        if check:
            raise CommandError(f"Command failed: {' '.join(cmd)}\n{e.stderr}") from e
        return e.returncode, e.stdout or "", e.stderr or ""
    except subprocess.TimeoutExpired as e:
        raise CommandError(f"Command timed out: {' '.join(cmd)}") from e
    except FileNotFoundError as e:
        raise CommandError(f"Command not found: {cmd[0]}") from e


class VMBackend:
    """Lima VM operations - wraps commands with limactl shell."""

    def __init__(self, vm_name: str):
        self.vm_name = vm_name

    def exists(self) -> bool:
        """Check if VM exists."""
        try:
            returncode, stdout, _ = _run_command(["limactl", "list", "--format", "{{.Name}}"], check=False)
            if returncode != 0:
                return False
            return self.vm_name in stdout.splitlines()
        except CommandError:
            return False

    def status(self) -> str | None:
        """Get VM status (Running, Stopped, etc.)."""
        if not self.exists():
            return None
        try:
            returncode, stdout, _ = _run_command(["limactl", "list", "--format", "{{.Name}} {{.Status}}"], check=False)
            if returncode != 0:
                return None
            for line in stdout.splitlines():
                parts = line.split()
                if parts and parts[0] == self.vm_name:
                    return parts[1] if len(parts) > 1 else "Unknown"
        except CommandError:
            pass
        return None

    def start(self) -> None:
        """Start the VM."""
        print(f"Starting VM '{self.vm_name}'...")
        _run_command(["limactl", "start", self.vm_name], capture=False)

    def stop(self) -> None:
        """Stop the VM."""
        print(f"Stopping VM '{self.vm_name}'...")
        _run_command(["limactl", "stop", self.vm_name], capture=False)

    def exec(
        self,
        cmd: list[str],
        check: bool = True,
        capture: bool = True,
        input_data: str | None = None,
        timeout: int | None = None,
    ) -> tuple[int, str, str]:
        """Execute command inside VM via limactl shell."""
        lima_cmd = ["limactl", "shell", self.vm_name, "--"] + cmd
        return _run_command(lima_cmd, check=check, capture=capture, input_data=input_data, timeout=timeout)

    def wait_for_docker(self, timeout: int = 120) -> None:
        """Wait for Docker to be ready inside VM."""
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
    """Direct host operations - runs commands directly."""

    def exec(
        self,
        cmd: list[str],
        check: bool = True,
        capture: bool = True,
        input_data: str | None = None,
        timeout: int | None = None,
    ) -> tuple[int, str, str]:
        """Execute command directly on host."""
        return _run_command(cmd, check=check, capture=capture, input_data=input_data, timeout=timeout)


def detect_environment() -> Environment:
    """Detect the runtime environment and determine mode.

    Returns VM mode on macOS or Linux with KVM.
    Returns DIRECT mode on Linux without KVM (e.g., Workspaces).
    """
    os_type = platform.system().lower()

    # Check for Docker
    has_docker = False
    try:
        returncode, _, _ = _run_command(["docker", "info"], check=False, capture=True)
        has_docker = returncode == 0
    except CommandError:
        pass

    # Check for KVM (Linux only)
    has_kvm = os_type == "linux" and Path("/dev/kvm").exists()

    # Check for Lima
    has_lima = False
    try:
        returncode, _, _ = _run_command(["limactl", "--version"], check=False, capture=True)
        has_lima = returncode == 0
    except CommandError:
        pass

    # Determine mode:
    # - macOS: always VM mode (needs Lima for Docker/Kind)
    # - Linux with KVM: VM mode (can use Lima for isolation)
    # - Linux without KVM: Direct mode (Workspaces, containers)
    if os_type == "darwin":
        mode = Mode.VM
    elif os_type == "linux" and has_docker and not has_kvm:
        mode = Mode.DIRECT
    else:
        mode = Mode.VM

    return Environment(
        mode=mode,
        os_type=os_type,
        has_docker=has_docker,
        has_kvm=has_kvm,
        has_lima=has_lima,
    )


def create_backend(env: Environment, vm_name: str = "gadget-k8s-host") -> VMBackend | DirectBackend:
    """Create appropriate backend based on environment."""
    if env.mode == Mode.VM:
        return VMBackend(vm_name)
    return DirectBackend()


# --- Generic Utilities ---


def run_cmd(
    cmd: list[str],
    description: str,
    capture: bool = False,
    cwd: Path | None = None,
) -> tuple[bool, str]:
    """Run a command with status output. Returns (success, output)."""
    print(f"{description}...", end=" ", flush=True)
    start = time.time()

    result = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        cwd=cwd,
    )

    elapsed = time.time() - start

    if result.returncode != 0:
        print("FAILED")
        if result.stderr:
            print(f"  Error: {result.stderr.strip()}")
        return False, result.stderr

    print(f"done ({elapsed:.1f}s)")
    return True, result.stdout


def is_process_running(pid: int) -> bool:
    """Check if a process with given PID is running."""
    try:
        os.kill(pid, 0)
        return True
    except (OSError, ProcessLookupError):
        return False


def check_health(url: str, timeout: float = 1.0) -> bool:
    """Check if HTTP endpoint returns healthy status."""
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
