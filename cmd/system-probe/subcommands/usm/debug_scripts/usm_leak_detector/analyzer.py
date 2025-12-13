"""
Map analysis logic for detecting leaked entries.
"""

import time
from typing import Dict, List

from .backends import EbpfBackend
from .constants import CONN_TUPLE_OFFSET, DEFAULT_PROC_ROOT, MAX_SAMPLES_STORED
from .logging_config import logger
from .models import ConnTuple, ConnectionIndex, MapLeakInfo
from .network import discover_namespaces, build_connection_index
from .parser import hex_array_to_bytes, parse_conn_tuple
from .validator import validate_tuple


def seccomp_safe_sleep(delay: float) -> None:
    """Sleep that works in seccomp-restricted environments.

    Falls back to busy-wait if time.sleep() is blocked by seccomp.
    """
    try:
        time.sleep(delay)
    except (PermissionError, OSError):
        # Busy-wait fallback for seccomp-restricted environments
        end = time.monotonic() + delay
        while time.monotonic() < end:
            pass


def _parse_entry_key(entry: Dict, conn_tuple_offset: int):
    """Parse a map entry's key into a ConnTuple.

    Handles multiple formats:
    - BTF-formatted dict keys (from bpftool with BTF)
    - Hex array keys (from bpftool without BTF)
    - Pre-parsed ConnTuple (from system-probe backend)

    Args:
        entry: Map entry dict with 'key' field
        conn_tuple_offset: Byte offset of ConnTuple within key

    Returns:
        ConnTuple or None if parsing failed
    """
    key = entry.get("key")
    if key is None:
        return None

    # Handle pre-parsed ConnTuple from system-probe backend
    if entry.get("_btf"):
        return key

    # Handle BTF-formatted dict keys (bpftool with BTF)
    if isinstance(key, dict):
        # BTF output has named fields like saddr_h, saddr_l, etc.
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
            logger.debug(f"  Warning: Could not parse BTF key: {key}")
            return None

    # Handle hex array keys (bpftool without BTF)
    if isinstance(key, list):
        key_bytes = hex_array_to_bytes(key)
        conn = parse_conn_tuple(key_bytes, offset=conn_tuple_offset)
        if conn is None:
            logger.debug(f"  Warning: Could not parse key of {len(key_bytes)} bytes")
        return conn

    logger.debug(f"  Warning: Unknown key format: {type(key)}")
    return None


def analyze_map(
    map_name: str,
    backend: EbpfBackend,
    connection_index: Dict[int, ConnectionIndex],
    recheck_delay: float = 0,
    proc_root: str = DEFAULT_PROC_ROOT
) -> MapLeakInfo:
    """Analyze a single map for leaks using streaming to minimize memory usage.

    Args:
        map_name: Name of the eBPF map to analyze
        backend: eBPF backend to use for map operations
        connection_index: Per-namespace index of active connections
        recheck_delay: Seconds to wait before re-checking leaked entries (0 = disabled)
        proc_root: Path to /proc filesystem

    Returns:
        MapLeakInfo with analysis results
    """
    total = 0
    leaked: List[tuple] = []

    # Look up ConnTuple offset for this map (default 0)
    conn_tuple_offset = CONN_TUPLE_OFFSET.get(map_name, 0)

    # Stream entries one at a time to minimize memory usage
    for entry in backend.iter_map_by_name(map_name):
        total += 1

        conn = _parse_entry_key(entry, conn_tuple_offset)
        if conn is None:
            continue

        valid, reason = validate_tuple(conn, connection_index)
        if not valid:
            leaked.append((conn, reason))

    race_condition_fps = 0

    # Re-check leaked entries to filter out race condition false positives
    if recheck_delay > 0 and leaked:
        logger.debug(f"  Re-checking {len(leaked)} leaked entries after {recheck_delay}s delay...")
        seccomp_safe_sleep(recheck_delay)

        # Rebuild fresh connection index
        namespaces = discover_namespaces(proc_root)
        fresh_index = build_connection_index(namespaces, proc_root)

        # Re-validate all leaked entries
        still_leaked = []
        for conn, reason in leaked:
            valid, new_reason = validate_tuple(conn, fresh_index)
            if not valid:
                still_leaked.append((conn, reason))
            else:
                race_condition_fps += 1

        if race_condition_fps > 0:
            logger.debug(f"  Filtered {race_condition_fps} false positives (race condition)")

        leaked = still_leaked

    return MapLeakInfo(
        name=map_name,
        total=total,
        leaked=len(leaked),
        samples=leaked[:MAX_SAMPLES_STORED],
        race_condition_fps=race_condition_fps
    )
