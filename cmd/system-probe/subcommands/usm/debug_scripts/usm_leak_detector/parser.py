"""
ConnTuple parsing utilities.
"""

import struct
from typing import List, Optional

from .models import ConnTuple

# ConnTuple structure layout (48 bytes total)
CONN_TUPLE_SIZE = 48  # Total size in bytes
CONN_TUPLE_PORT_OFFSET = 32  # Offset of sport/dport fields
CONN_TUPLE_META_OFFSET = 36  # Offset of netns/pid/metadata fields


def hex_array_to_bytes(hex_array: List[str]) -> bytes:
    """Convert bpftool hex array ["0x01", "0x02", ...] to bytes."""
    return bytes(int(x, 16) for x in hex_array)


def parse_conn_tuple(key_bytes: bytes, offset: int = 0) -> Optional[ConnTuple]:
    """Parse bytes into ConnTuple structure.

    Args:
        key_bytes: Raw key bytes from BPF map
        offset: Byte offset where ConnTuple starts (default 0)

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
    if len(key_bytes) < offset + CONN_TUPLE_SIZE:
        return None

    # Unpack using little-endian format from specified offset
    saddr_h, saddr_l, daddr_h, daddr_l = struct.unpack_from("<QQQQ", key_bytes, offset)
    sport, dport = struct.unpack_from("<HH", key_bytes, offset + CONN_TUPLE_PORT_OFFSET)
    netns, pid, metadata = struct.unpack_from("<III", key_bytes, offset + CONN_TUPLE_META_OFFSET)

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
