"""
Map filtering and utility functions.
"""

from typing import Dict, Set

from .constants import MAP_NAME_PREFIX_LENGTH


def filter_maps_by_names(
    maps: Dict[str, any],
    requested_names: Set[str]
) -> Dict[str, any]:
    """Filter a map dictionary to only include requested map names.

    Supports both exact name matches and prefix matching to handle
    truncated map names that may appear in eBPF tools.

    Args:
        maps: Dictionary of map names to map metadata
        requested_names: Set of requested map names (can be full or partial)

    Returns:
        Filtered dictionary containing only requested maps

    Example:
        >>> maps = {"conn_tcp_v4": {...}, "conn_tcp_v6": {...}, "other": {...}}
        >>> requested = {"conn_tcp_v4", "conn_tcp_v6"}
        >>> filter_maps_by_names(maps, requested)
        {"conn_tcp_v4": {...}, "conn_tcp_v6": {...}}
    """
    return {
        k: v for k, v in maps.items()
        if k in requested_names or any(k.startswith(r[:MAP_NAME_PREFIX_LENGTH]) for r in requested_names)
    }