"""
Map analysis logic for detecting leaked entries.
"""

from typing import Dict, List

from .backends import EbpfBackend
from .models import ConnTuple, ConnectionIndex, MapLeakInfo
from .parser import hex_array_to_bytes, parse_conn_tuple
from .validator import validate_tuple


def analyze_map(
    map_name: str,
    backend: EbpfBackend,
    connection_index: Dict[int, ConnectionIndex],
    verbose: bool = False
) -> MapLeakInfo:
    """Analyze a single map for leaks.

    Args:
        map_name: Name of the eBPF map to analyze
        backend: eBPF backend to use for map operations
        connection_index: Per-namespace index of active connections
        verbose: Enable verbose output

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

    return MapLeakInfo(
        name=map_name,
        total=total,
        leaked=len(leaked),
        samples=leaked[:100]  # Limit samples
    )
