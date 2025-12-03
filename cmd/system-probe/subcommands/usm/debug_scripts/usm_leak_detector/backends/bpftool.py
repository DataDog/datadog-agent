"""
bpftool backend for eBPF map operations.
"""

import json
import subprocess
import sys
from typing import Dict, List, Optional

from .ebpf_backend import EbpfBackend
from .bpftool_downloader import download_bpftool


class BpftoolBackend(EbpfBackend):
    """eBPF backend using bpftool."""

    def __init__(self, bpftool_path: str = "bpftool"):
        """Initialize with optional custom bpftool path."""
        self._bpftool_path = bpftool_path

    @staticmethod
    def name() -> str:
        return "bpftool"

    @staticmethod
    def is_available(try_download: bool = True, verbose: bool = False) -> bool:
        """Check if bpftool is available, optionally downloading it."""
        # First try system bpftool
        try:
            result = subprocess.run(
                ["bpftool", "version"],
                capture_output=True,
                timeout=5
            )
            if result.returncode == 0:
                return True
        except (FileNotFoundError, subprocess.TimeoutExpired):
            pass

        # Try downloading if allowed
        if try_download:
            path = download_bpftool(verbose=verbose)
            if path:
                return True

        return False

    @staticmethod
    def get_backend(try_download: bool = True, verbose: bool = False) -> Optional["BpftoolBackend"]:
        """Get a BpftoolBackend instance, downloading bpftool if needed."""
        # Try system bpftool first
        try:
            result = subprocess.run(
                ["bpftool", "version"],
                capture_output=True,
                timeout=5
            )
            if result.returncode == 0:
                return BpftoolBackend("bpftool")
        except (FileNotFoundError, subprocess.TimeoutExpired):
            pass

        # Try downloading
        if try_download:
            path = download_bpftool(verbose=verbose)
            if path:
                return BpftoolBackend(path)

        return None

    def _run(self, args: List[str]) -> Optional[str]:
        """Run bpftool command and return stdout, or None on error."""
        cmd = [self._bpftool_path] + args
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=30
            )
            if result.returncode != 0:
                print(f"Error running bpftool: {result.stderr}", file=sys.stderr)
                return None
            return result.stdout
        except FileNotFoundError:
            print("Error: bpftool not found in PATH", file=sys.stderr)
            return None
        except subprocess.TimeoutExpired:
            print("Error: bpftool timed out", file=sys.stderr)
            return None

    def list_maps(self) -> List[Dict]:
        """List all eBPF maps using bpftool."""
        output = self._run(["map", "list", "--json"])
        if output is None:
            return []
        try:
            return json.loads(output)
        except json.JSONDecodeError as e:
            print(f"Error parsing bpftool output: {e}", file=sys.stderr)
            return []

    def dump_map_by_id(self, map_id: int) -> List[Dict]:
        """Dump map contents by ID."""
        output = self._run(["map", "dump", "id", str(map_id), "--json"])
        if output is None:
            return []
        try:
            return json.loads(output)
        except json.JSONDecodeError as e:
            print(f"Error parsing map dump: {e}", file=sys.stderr)
            return []

    def dump_map_by_name(self, name: str) -> List[Dict]:
        """Dump map by name - bpftool doesn't support this directly, so we find by ID first."""
        maps = self.list_maps()
        for m in maps:
            if m.get("name") == name:
                return self.dump_map_by_id(m.get("id"))
        return []
