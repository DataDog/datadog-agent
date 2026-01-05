"""
system-probe backend for eBPF map operations.
"""

import os
import subprocess
from typing import Dict, Generator, List, Optional

from .ebpf_backend import EbpfBackend
from ..constants import DEFAULT_SUBPROCESS_TIMEOUT
from ..logging_config import logger
from .streaming import iter_json_objects
from ..models import ConnTuple
from ..subprocess_utils import safe_subprocess_run


class SystemProbeBackend(EbpfBackend):
    """eBPF backend using system-probe's ebpf map commands."""

    # Common paths to look for system-probe binary
    SEARCH_PATHS = [
        "/opt/datadog-agent/embedded/bin/system-probe",
        "/usr/bin/system-probe",
        "/git/datadog-agent/bin/system-probe/system-probe",
        "./system-probe",
    ]

    def __init__(self, binary_path: Optional[str] = None):
        self.binary_path = binary_path or self._find_binary()

    @staticmethod
    def name() -> str:
        return "system-probe"

    def _find_binary(self) -> Optional[str]:
        """Find system-probe binary."""
        for path in self.SEARCH_PATHS:
            if os.path.isfile(path) and os.access(path, os.X_OK):
                return path
        # Try PATH
        try:
            result = safe_subprocess_run(
                ["which", "system-probe"],
                capture_output=True,
                text=True,
                timeout=DEFAULT_SUBPROCESS_TIMEOUT
            )
            if result.returncode == 0:
                return result.stdout.strip()
        except (FileNotFoundError, subprocess.TimeoutExpired):
            pass
        return None

    @staticmethod
    def is_available() -> bool:
        """Check if system-probe is available."""
        backend = SystemProbeBackend()
        return backend.binary_path is not None

    def _run(self, args: List[str]) -> Optional[str]:
        """Run system-probe command and return stdout, or None on error."""
        if not self.binary_path:
            logger.error("system-probe binary not found")
            return None

        cmd = [self.binary_path] + args
        return self._run_command(cmd, tool_name="system-probe")

    def list_maps(self) -> List[Dict]:
        """List all eBPF maps using system-probe.

        system-probe output format:
            32487: Hash  name ssl_certs_i2d_X  flags 0x0
                key 8B  value 16B  max_entries 1
        """
        output = self._run(["ebpf", "map", "list"])
        if output is None:
            return []

        maps = []
        lines = output.strip().split("\n")
        i = 0
        while i < len(lines):
            line = lines[i].strip()
            # Parse lines like: "32487: Hash  name ssl_certs_i2d_X  flags 0x0"
            if ":" in line and "name" in line:
                parts = line.split()
                try:
                    map_id = int(parts[0].rstrip(":"))
                    # Find "name" in parts and get the next element
                    name_idx = parts.index("name") + 1
                    name = parts[name_idx] if name_idx < len(parts) else ""
                    maps.append({"id": map_id, "name": name})
                except (ValueError, IndexError):
                    pass
            i += 1

        return maps

    def _parse_btf_key(self, key: Dict) -> Optional[ConnTuple]:
        """Parse BTF-formatted key dict into ConnTuple."""
        try:
            return ConnTuple(
                saddr_h=key.get("saddr_h", 0),
                saddr_l=key.get("saddr_l", 0),
                daddr_h=key.get("daddr_h", 0),
                daddr_l=key.get("daddr_l", 0),
                sport=key.get("sport", 0),
                dport=key.get("dport", 0),
                netns=key.get("netns", 0),
                pid=key.get("pid", 0),
                metadata=key.get("metadata", 0),
            )
        except (TypeError, ValueError):
            return None

    def iter_map_by_name(self, name: str) -> Generator[Dict, None, None]:
        """Stream map entries by name, yielding one entry at a time.

        Uses subprocess.Popen to stream system-probe output and parse
        JSON objects incrementally to minimize memory usage.

        Args:
            name: eBPF map name

        Yields:
            Dict entries with 'key' (ConnTuple) and '_btf' fields
        """
        if not self.binary_path:
            logger.error("system-probe binary not found")
            return

        cmd = [self.binary_path, "ebpf", "map", "dump", "name", name]
        try:
            proc = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1
            )
        except FileNotFoundError:
            logger.error(f"system-probe not found at {self.binary_path}")
            return

        try:
            for entry in iter_json_objects(proc.stdout):
                key = entry.get("key", {})
                if isinstance(key, dict):
                    # BTF-formatted key with named fields
                    conn = self._parse_btf_key(key)
                    if conn:
                        yield {"key": conn, "_btf": True}
                elif isinstance(key, list):
                    # Hex array format (rare with system-probe)
                    yield entry
        finally:
            proc.stdout.close()
            proc.stderr.close()
            proc.wait()
