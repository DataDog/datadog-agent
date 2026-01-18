"""
bpftool backend for eBPF map operations.
"""

import json
import subprocess
from typing import Dict, Generator, List, Optional

from .ebpf_backend import EbpfBackend
from ..constants import DEFAULT_SUBPROCESS_TIMEOUT
from ..logging_config import logger
from .bpftool_downloader import download_bpftool
from .streaming import iter_json_objects
from ..subprocess_utils import safe_subprocess_run


class BpftoolBackend(EbpfBackend):
    """eBPF backend using bpftool."""

    def __init__(self, bpftool_path: str = "bpftool"):
        """Initialize with optional custom bpftool path."""
        self._bpftool_path = bpftool_path

    @staticmethod
    def name() -> str:
        return "bpftool"

    @staticmethod
    def is_available(try_download: bool = True) -> bool:
        """Check if bpftool is available, optionally downloading it."""
        # First try system bpftool
        try:
            result = safe_subprocess_run(
                ["bpftool", "version"],
                capture_output=True,
                timeout=DEFAULT_SUBPROCESS_TIMEOUT
            )
            if result.returncode == 0:
                return True
        except (FileNotFoundError, subprocess.TimeoutExpired):
            pass

        # Try downloading if allowed
        if try_download:
            path = download_bpftool()
            if path:
                return True

        return False

    @staticmethod
    def get_backend(try_download: bool = True) -> Optional["BpftoolBackend"]:
        """Get a BpftoolBackend instance, downloading bpftool if needed."""
        # Try system bpftool first
        try:
            result = safe_subprocess_run(
                ["bpftool", "version"],
                capture_output=True,
                timeout=DEFAULT_SUBPROCESS_TIMEOUT
            )
            if result.returncode == 0:
                return BpftoolBackend("bpftool")
        except (FileNotFoundError, subprocess.TimeoutExpired):
            pass

        # Try downloading
        if try_download:
            path = download_bpftool()
            if path:
                return BpftoolBackend(path)

        return None

    def _run(self, args: List[str]) -> Optional[str]:
        """Run bpftool command and return stdout, or None on error."""
        cmd = [self._bpftool_path] + args
        return self._run_command(cmd, tool_name="bpftool")

    def list_maps(self) -> List[Dict]:
        """List all eBPF maps using bpftool."""
        output = self._run(["map", "list", "--json"])
        if output is None:
            return []
        try:
            return json.loads(output)
        except json.JSONDecodeError as e:
            logger.error(f"Error parsing bpftool output: {e}")
            return []

    def iter_map_by_id(self, map_id: int) -> Generator[Dict, None, None]:
        """Stream map entries by ID, yielding one entry at a time.

        Uses subprocess.Popen to stream bpftool output line by line,
        parsing JSON objects incrementally to minimize memory usage.

        Args:
            map_id: eBPF map ID

        Yields:
            Dict entries with 'key' and 'value' fields
        """
        cmd = [self._bpftool_path, "map", "dump", "id", str(map_id), "--json"]
        try:
            proc = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1  # Line buffered
            )
        except FileNotFoundError:
            logger.error("bpftool not found in PATH")
            return

        try:
            # Stream JSON objects one at a time
            for entry in iter_json_objects(proc.stdout):
                yield entry
        finally:
            # Ensure process is cleaned up
            proc.stdout.close()
            proc.stderr.close()
            proc.wait()

    def iter_map_by_name(self, name: str) -> Generator[Dict, None, None]:
        """Stream map entries by name, yielding one entry at a time.

        Args:
            name: eBPF map name

        Yields:
            Dict entries with 'key' and 'value' fields
        """
        maps = self.list_maps()
        for m in maps:
            if m.get("name") == name:
                yield from self.iter_map_by_id(m.get("id"))
                return
