# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Port-forward management for lab environments.

Handles automatic port-forwarding for Kubernetes services with:
- Background process management
- Automatic restart on failure
- PID tracking for cleanup
- Health checking
"""

from __future__ import annotations

import os
import signal
import subprocess
import time
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from dda.cli.application import Application


class PortForwardManager:
    """Manages kubectl port-forward processes for lab environments."""

    # Timeout configuration
    PORT_READY_TIMEOUT_SEC = 15.0
    PORT_READY_CHECK_INTERVAL = 0.5
    SIGTERM_WAIT_SEC = 1.0
    SIGTERM_CHECK_INTERVAL = 0.1
    SOCKET_TIMEOUT_SEC = 1.0

    def __init__(self, state_dir: Path | None = None):
        """
        Initialize port-forward manager.

        Args:
            state_dir: Directory to store PID files (default: ~/.dda/lab/port-forwards)
        """
        if state_dir is None:
            state_dir = Path.home() / ".dda" / "lab" / "port-forwards"
        self.state_dir = state_dir
        self.state_dir.mkdir(parents=True, exist_ok=True)

    def _get_pid_file(self, env_name: str, service: str) -> Path:
        """Get PID file path for a port-forward."""
        return self.state_dir / f"{env_name}-{service}.pid"

    def _is_running(self, pid: int) -> bool:
        """Check if a process is running."""
        try:
            os.kill(pid, 0)
            return True
        except (OSError, ProcessLookupError):
            return False

    def _is_port_available(self, local_port: int) -> bool:
        """Check if a port is available and responding."""
        import socket

        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.settimeout(self.SOCKET_TIMEOUT_SEC)
            result = sock.connect_ex(("localhost", local_port))
            return result == 0

    def get_port_forward(
        self,
        app: Application,
        env_name: str,
        service: str,
        namespace: str,
        local_port: int,
        remote_port: int,
        context: str | None = None,
    ) -> int:
        """
        Get or create a port-forward, returning the local port.

        Args:
            app: Application instance
            env_name: Environment name
            service: Service name
            namespace: Kubernetes namespace
            local_port: Local port to forward to
            remote_port: Remote port on the service
            context: Kubernetes context (optional)

        Returns:
            The local port number
        """
        pid_file = self._get_pid_file(env_name, service)

        # Check if port-forward already exists and is healthy
        if pid_file.exists():
            try:
                pid = int(pid_file.read_text().strip())
                if self._is_running(pid) and self._is_port_available(local_port):
                    return local_port
                else:
                    # Stale PID file, clean it up
                    self.stop_port_forward(env_name, service)
            except (ValueError, OSError):
                # Corrupted PID file
                pid_file.unlink(missing_ok=True)

        # Start new port-forward
        cmd = ["kubectl", "port-forward", "-n", namespace, f"svc/{service}", f"{local_port}:{remote_port}"]
        if context:
            cmd.extend(["--context", context])

        app.display_info(f"Starting port-forward: {service}:{remote_port} -> localhost:{local_port}")

        # Start in background
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            stdin=subprocess.DEVNULL,  # Prevent hanging on stdin
            start_new_session=True,  # Detach from parent
        )

        # Save PID
        pid_file.write_text(str(process.pid))

        # Wait for port to be available
        max_attempts = int(self.PORT_READY_TIMEOUT_SEC / self.PORT_READY_CHECK_INTERVAL)
        for _ in range(max_attempts):
            if self._is_port_available(local_port):
                app.display_success(f"Port-forward ready: localhost:{local_port}")
                return local_port
            time.sleep(self.PORT_READY_CHECK_INTERVAL)

        # Port-forward failed
        self.stop_port_forward(env_name, service)
        raise RuntimeError(f"Port-forward failed to start after {self.PORT_READY_TIMEOUT_SEC:.0f} seconds")

    def stop_port_forward(self, env_name: str, service: str) -> None:
        """Stop a port-forward."""
        pid_file = self._get_pid_file(env_name, service)

        if not pid_file.exists():
            return

        try:
            pid = int(pid_file.read_text().strip())
            if self._is_running(pid):
                os.kill(pid, signal.SIGTERM)
                # Give it time to terminate gracefully
                max_attempts = int(self.SIGTERM_WAIT_SEC / self.SIGTERM_CHECK_INTERVAL)
                for _ in range(max_attempts):
                    if not self._is_running(pid):
                        break
                    time.sleep(self.SIGTERM_CHECK_INTERVAL)
                # Force kill if still running
                if self._is_running(pid):
                    os.kill(pid, signal.SIGKILL)
        except (ValueError, OSError, ProcessLookupError):
            pass
        finally:
            pid_file.unlink(missing_ok=True)

    def stop_all(self, env_name: str) -> None:
        """Stop all port-forwards for an environment."""
        for pid_file in self.state_dir.glob(f"{env_name}-*.pid"):
            service = pid_file.stem.split("-", 1)[1]
            self.stop_port_forward(env_name, service)

    def cleanup_stale(self) -> None:
        """Clean up stale PID files."""
        for pid_file in self.state_dir.glob("*.pid"):
            try:
                pid = int(pid_file.read_text().strip())
                if not self._is_running(pid):
                    pid_file.unlink()
            except (ValueError, OSError):
                pid_file.unlink(missing_ok=True)


# Global instance
_port_forward_manager: PortForwardManager | None = None


def get_port_forward_manager() -> PortForwardManager:
    """Get the global port-forward manager instance."""
    global _port_forward_manager
    if _port_forward_manager is None:
        _port_forward_manager = PortForwardManager()
    return _port_forward_manager
