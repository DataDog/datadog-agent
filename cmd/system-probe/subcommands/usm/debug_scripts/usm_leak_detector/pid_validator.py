"""
PID-based leak detection for TLS/SSL argument maps.

These maps use uint64 pid_tgid keys where the PID is in the upper 32 bits.
Entries are considered leaked if the process no longer exists.
"""

import os
import struct
from typing import Dict, List, Optional, Set

from .backends import EbpfBackend
from .constants import MAP_NAME_PREFIX_LENGTH, PID_KEYED_MAPS
from .logging_config import logger
from .models import PIDLeakInfo

# PID extraction from uint64 pid_tgid key
PID_SHIFT_BITS = 32  # Upper 32 bits contain PID
PID_TGID_SIZE = 8  # Size of uint64 in bytes
UINT32_MASK = 0xFFFFFFFF  # 32-bit mask


def extract_pid(pid_tgid: int) -> int:
    """Extract PID from a uint64 pid_tgid key.

    In Linux, pid_tgid is stored as (pid << 32) | tgid.
    The PID is in the upper 32 bits.
    """
    return (pid_tgid >> PID_SHIFT_BITS) & UINT32_MASK


def pid_exists(pid: int, proc_root: str) -> bool:
    """Check if a process with the given PID exists."""
    return os.path.exists(os.path.join(proc_root, str(pid)))


def parse_pid_tgid_key(key) -> Optional[int]:
    """Parse a pid_tgid key from various formats.

    Args:
        key: The key data - can be hex array, bytes, or dict with pid_tgid field

    Returns:
        The pid_tgid as an integer, or None if parsing fails
    """
    if isinstance(key, int):
        return key

    if isinstance(key, dict):
        # BTF-formatted key from system-probe
        if "pid_tgid" in key:
            return key["pid_tgid"]
        return None

    if isinstance(key, list):
        # Hex array from bpftool: ["0x01", "0x02", ...]
        try:
            key_bytes = bytes(int(x, 16) for x in key)
        except (ValueError, TypeError):
            return None
    elif isinstance(key, bytes):
        key_bytes = key
    else:
        return None

    if len(key_bytes) != PID_TGID_SIZE:
        return None

    # Unpack as little-endian uint64
    return struct.unpack("<Q", key_bytes)[0]


def find_pid_keyed_maps(maps: List[Dict]) -> Dict[str, int]:
    """Filter maps to find PID-keyed TLS maps.

    Args:
        maps: List of map dicts with 'name' and 'id' keys

    Returns:
        Dict mapping map_name to map_id for PID-keyed maps
    """
    pid_maps = {}
    for m in maps:
        name = m.get("name", "")
        map_id = m.get("id")
        if map_id is None:
            continue
        # Check if this is a target map (handle kernel name truncation)
        for target in PID_KEYED_MAPS:
            # Kernel truncates names to MAP_NAME_PREFIX_LENGTH chars
            if name == target or name == target[:MAP_NAME_PREFIX_LENGTH]:
                pid_maps[name] = map_id
                break
    return pid_maps


def analyze_pid_map(
    map_name: str,
    backend: EbpfBackend,
    proc_root: str
) -> PIDLeakInfo:
    """Analyze a PID-keyed map for leaked entries using streaming.

    Args:
        map_name: Name of the eBPF map to analyze
        backend: eBPF backend to use for map operations
        proc_root: Path to /proc filesystem

    Returns:
        PIDLeakInfo with analysis results
    """
    total = 0

    # Track seen PIDs to avoid redundant /proc lookups
    seen_pids: Dict[int, bool] = {}
    dead_pids: Set[int] = set()
    leaked_count = 0

    # Stream entries one at a time to minimize memory usage
    for entry in backend.iter_map_by_name(map_name):
        total += 1
        key = entry.get("key")
        if key is None:
            continue

        # Handle BTF-formatted keys from system-probe
        if entry.get("_btf") and isinstance(key, dict):
            pid_tgid = key.get("pid_tgid")
            if pid_tgid is None:
                logger.debug("  Warning: BTF key missing pid_tgid field")
                continue
        else:
            pid_tgid = parse_pid_tgid_key(key)
            if pid_tgid is None:
                logger.debug("  Warning: Could not parse pid_tgid key")
                continue

        pid = extract_pid(pid_tgid)

        # Check if we've already validated this PID
        if pid in seen_pids:
            if not seen_pids[pid]:
                leaked_count += 1
            continue

        # Validate PID exists
        exists = pid_exists(pid, proc_root)
        seen_pids[pid] = exists

        if not exists:
            leaked_count += 1
            dead_pids.add(pid)

    return PIDLeakInfo(
        name=map_name,
        total=total,
        leaked=leaked_count,
        dead_pids=sorted(dead_pids)
    )
