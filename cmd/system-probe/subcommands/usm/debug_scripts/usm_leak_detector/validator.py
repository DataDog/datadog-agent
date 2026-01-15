"""
Validation logic for ConnTuple entries against active TCP connections.
"""

from typing import Dict, Tuple

from .models import ConnTuple, ConnectionIndex


def _validate_in_namespace(conn: ConnTuple, ns_index: ConnectionIndex) -> Tuple[bool, str]:
    """Validate connection against a specific namespace index."""
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


def _validate_across_all_namespaces(conn: ConnTuple, connection_index: Dict[int, ConnectionIndex]) -> Tuple[bool, str]:
    """Search all namespaces for a matching connection when netns=0."""
    for netns_id, ns_index in connection_index.items():
        valid, reason = _validate_in_namespace(conn, ns_index)
        if valid:
            # Found in some namespace - mark as valid with special reason
            return True, f"{reason}_netns0_found_in_{netns_id}"

    # Not found in any namespace - this is a real leak
    return False, "no_connection_any_namespace"


def validate_tuple(conn: ConnTuple, connection_index: Dict[int, ConnectionIndex]) -> Tuple[bool, str]:
    """Check if tuple corresponds to active connection.

    Returns (is_valid, reason).
    """
    netns = conn.netns

    # Handle netns=0 (metadata not populated) - search all namespaces
    if netns == 0:
        return _validate_across_all_namespaces(conn, connection_index)

    # Normal case: validate against specific namespace
    if netns not in connection_index:
        return False, "unknown_namespace"

    return _validate_in_namespace(conn, connection_index[netns])
