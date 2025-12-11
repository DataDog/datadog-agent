"""
Map filtering and utility functions.
"""

from typing import Dict, Set

from .constants import MAP_NAME_PREFIX_LENGTH
from .logging_config import logger


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
    filtered = {
        k: v for k, v in maps.items()
        if k in requested_names or any(k.startswith(r[:MAP_NAME_PREFIX_LENGTH]) for r in requested_names)
    }

    # Log warning if requested maps were not found
    if not filtered and requested_names:
        logger.warning(f"No maps found matching requested names: {', '.join(sorted(requested_names))}")

    return filtered
