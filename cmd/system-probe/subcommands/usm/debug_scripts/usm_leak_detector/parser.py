"""
ConnTuple parsing utilities.
"""

import struct
from typing import List, Optional

from .models import ConnTuple


def hex_array_to_bytes(hex_array: List[str]) -> bytes:
    """Convert bpftool hex array ["0x01", "0x02", ...] to bytes."""
    return bytes(int(x, 16) for x in hex_array)


def parse_conn_tuple(key_bytes: bytes) -> Optional[ConnTuple]:
    """Parse bytes into ConnTuple structure.

    ConnTuple layout (48 bytes):
        0-7:   saddr_h (uint64)
        8-15:  saddr_l (uint64)
        16-23: daddr_h (uint64)
        24-31: daddr_l (uint64)
        32-33: sport (uint16)
        34-35: dport (uint16)
        36-39: netns (uint32)
        40-43: pid (uint32)
        44-47: metadata (uint32)
    """
    if len(key_bytes) < 48:
        return None

    # Unpack using little-endian format
    saddr_h, saddr_l, daddr_h, daddr_l = struct.unpack_from("<QQQQ", key_bytes, 0)
    sport, dport = struct.unpack_from("<HH", key_bytes, 32)
    netns, pid, metadata = struct.unpack_from("<III", key_bytes, 36)

    return ConnTuple(
        saddr_h=saddr_h,
        saddr_l=saddr_l,
        daddr_h=daddr_h,
        daddr_l=daddr_l,
        sport=sport,
        dport=dport,
        netns=netns,
        pid=pid,
        metadata=metadata
    )
