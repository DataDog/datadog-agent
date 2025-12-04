"""
system-probe backend for eBPF map operations.
"""

import json
import os
import subprocess
import sys
from typing import Dict, List, Optional

from .ebpf_backend import EbpfBackend
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
                timeout=5
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
            print("Error: system-probe binary not found", file=sys.stderr)
            return None

        cmd = [self.binary_path] + args
        try:
            result = safe_subprocess_run(
                cmd,
                capture_output=True,
                text=True,
                timeout=30
            )
            if result.returncode != 0:
                print(f"Error running system-probe: {result.stderr}", file=sys.stderr)
                return None
            return result.stdout
        except FileNotFoundError:
            print(f"Error: system-probe not found at {self.binary_path}", file=sys.stderr)
            return None
        except subprocess.TimeoutExpired:
            print("Error: system-probe timed out", file=sys.stderr)
            return None

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

    def dump_map_by_name(self, name: str) -> List[Dict]:
        """Dump map contents by name using system-probe.

        system-probe returns BTF-formatted JSON with named fields:
        [{"key": {"saddr_h": 0, "saddr_l": 123, ...}, "value": {...}}, ...]

        We convert the BTF-formatted key to the same format as bpftool hex arrays
        would parse to, so the rest of the code works unchanged.
        """
        output = self._run(["ebpf", "map", "dump", "name", name])
        if output is None:
            return []

        try:
            entries = json.loads(output)
        except json.JSONDecodeError as e:
            print(f"Error parsing system-probe output: {e}", file=sys.stderr)
            return []

        # Convert BTF-formatted keys to our internal format
        result = []
        for entry in entries:
            key = entry.get("key", {})
            if isinstance(key, dict):
                # BTF-formatted key with named fields
                conn = self._parse_btf_key(key)
                if conn:
                    # Store the parsed ConnTuple directly
                    result.append({"key": conn, "_btf": True})
            elif isinstance(key, list):
                # Already hex array format (shouldn't happen with system-probe)
                result.append(entry)

        return result

    def dump_map_by_id(self, map_id: int) -> List[Dict]:
        """Dump map by ID - system-probe uses name, so we find name first."""
        maps = self.list_maps()
        for m in maps:
            if m.get("id") == map_id:
                return self.dump_map_by_name(m.get("name", ""))
        return []

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
