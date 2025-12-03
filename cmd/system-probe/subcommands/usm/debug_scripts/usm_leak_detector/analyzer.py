"""
Map analysis logic for detecting leaked entries.
"""

import time
from typing import Dict, List

from .backends import EbpfBackend
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


def analyze_map(
    map_name: str,
    backend: EbpfBackend,
    connection_index: Dict[int, ConnectionIndex],
    verbose: bool = False,
    recheck_delay: float = 0,
    proc_root: str = "/proc"
) -> MapLeakInfo:
    """Analyze a single map for leaks.

    Args:
        map_name: Name of the eBPF map to analyze
        backend: eBPF backend to use for map operations
        connection_index: Per-namespace index of active connections
        verbose: Enable verbose output
        recheck_delay: Seconds to wait before re-checking leaked entries (0 = disabled)
        proc_root: Path to /proc filesystem

    Returns:
        MapLeakInfo with analysis results
    """
    entries = backend.dump_map_by_name(map_name)
    total = len(entries)
    leaked: List[tuple] = []

    for entry in entries:
        key = entry.get("key")
        if key is None:
            continue

        # Handle both BTF-formatted (dict/ConnTuple) and hex array formats
        if entry.get("_btf"):
            # Key is already a ConnTuple from system-probe backend
            conn = key
        elif isinstance(key, list):
            # Hex array from bpftool
            key_bytes = hex_array_to_bytes(key)
            conn = parse_conn_tuple(key_bytes)
            if conn is None:
                if verbose:
                    print(f"  Warning: Could not parse key of {len(key_bytes)} bytes")
                continue
        else:
            if verbose:
                print(f"  Warning: Unknown key format: {type(key)}")
            continue

        valid, reason = validate_tuple(conn, connection_index)
        if not valid:
            leaked.append((conn, reason))

    race_condition_fps = 0

    # Re-check leaked entries to filter out race condition false positives
    if recheck_delay > 0 and leaked:
        if verbose:
            print(f"  Re-checking {len(leaked)} leaked entries after {recheck_delay}s delay...")
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

        if verbose and race_condition_fps > 0:
            print(f"  Filtered {race_condition_fps} false positives (race condition)")

        leaked = still_leaked

    return MapLeakInfo(
        name=map_name,
        total=total,
        leaked=len(leaked),
        samples=leaked[:100],  # Limit samples
        race_condition_fps=race_condition_fps
    )
