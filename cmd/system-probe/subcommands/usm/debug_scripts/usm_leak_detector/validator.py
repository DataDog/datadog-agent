"""
Validation logic for ConnTuple entries against active TCP connections.
"""

from typing import Dict, Tuple

from .models import ConnTuple, ConnectionIndex


def validate_tuple(conn: ConnTuple, connection_index: Dict[int, ConnectionIndex]) -> Tuple[bool, str]:
    """Check if tuple corresponds to active connection.

    Returns (is_valid, reason).
    """
    netns = conn.netns

    if netns not in connection_index:
        return False, "unknown_namespace"

    ns_index = connection_index[netns]

    if conn.is_listening:
        # Check listening index by local addr:port only
        key = (conn.saddr_h, conn.saddr_l, conn.sport)
        if key in ns_index.listening:
            return True, "listening"
        # Also check for wildcard listeners (0.0.0.0 or ::)
        wildcard_key = (0, 0, conn.sport)
        if wildcard_key in ns_index.listening:
            return True, "listening_wildcard"
        return False, "no_listener"
    else:
        # Check established in both directions
        fwd = (conn.saddr_h, conn.saddr_l, conn.sport,
               conn.daddr_h, conn.daddr_l, conn.dport)
        rev = (conn.daddr_h, conn.daddr_l, conn.dport,
               conn.saddr_h, conn.saddr_l, conn.sport)

        if fwd in ns_index.established or rev in ns_index.established:
            return True, "established"
        return False, "no_connection"
