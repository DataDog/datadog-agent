"""
USM map discovery logic.
"""

from typing import Dict, List

from .constants import CONN_TUPLE_MAPS


def find_conn_tuple_maps(maps: List[Dict]) -> Dict[str, int]:
    """Filter maps to find USM ConnTuple-keyed maps.

    Args:
        maps: List of map dicts with 'name' and 'id' keys

    Returns:
        Dict mapping map_name to map_id for ConnTuple-keyed maps
    """
    result = {}
    for m in maps:
        name = m.get("name", "")
        map_id = m.get("id")
        if map_id is None:
            continue
        # Check if this is a target map (handle kernel name truncation)
        for target in CONN_TUPLE_MAPS:
            # Kernel truncates names to 15 chars
            if name == target or name == target[:15]:
                result[name] = map_id
                break
    return result
