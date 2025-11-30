"""
Data models for the USM leak detector.
"""

import struct
from dataclasses import dataclass
from typing import List, Set, Tuple


@dataclass
class ConnTuple:
    """Represents a connection tuple from eBPF maps."""
    saddr_h: int  # Source address high (IPv6) or 0
    saddr_l: int  # Source address low
    daddr_h: int  # Dest address high (IPv6) or 0
    daddr_l: int  # Dest address low
    sport: int    # Source port
    dport: int    # Destination port
    netns: int    # Network namespace inode
    pid: int      # Process ID (may be 0)
    metadata: int # Flags (bit 1 = IPv6)

    @property
    def is_ipv6(self) -> bool:
        return (self.metadata & 0x2) != 0

    @property
    def is_listening(self) -> bool:
        """Check if this is a listening socket (remote addr/port = 0)."""
        return self.daddr_h == 0 and self.daddr_l == 0 and self.dport == 0

    def format_addr(self, addr_h: int, addr_l: int) -> str:
        """Format address fields as IP string."""
        if self.is_ipv6:
            # Check for IPv4-mapped IPv6 address (::ffff:x.x.x.x)
            # Format in memory: addr_h=0, addr_l = (IPv4_le << 32) | 0xFFFF0000
            # Where IPv4_le is the IPv4 address in little-endian byte order
            if addr_h == 0 and (addr_l & 0xFFFFFFFF) == 0xFFFF0000:
                # Extract IPv4 from upper 32 bits (stored as little-endian)
                ipv4_le = (addr_l >> 32) & 0xFFFFFFFF
                ipv4_bytes = struct.pack("<I", ipv4_le)
                return ".".join(str(b) for b in ipv4_bytes)

            # Check for all-zeros (::)
            if addr_h == 0 and addr_l == 0:
                return "::"

            # Regular IPv6: combine both uint64s, big-endian bytes
            ipv6_bytes = struct.pack(">QQ", addr_h, addr_l)
            parts = []
            for i in range(0, 16, 2):
                val = (ipv6_bytes[i] << 8) | ipv6_bytes[i + 1]
                parts.append(f"{val:x}")
            addr = ":".join(parts)
            return addr
        else:
            # IPv4: use low 32 bits of addr_l, little-endian
            ipv4_bytes = struct.pack("<I", addr_l & 0xFFFFFFFF)
            return ".".join(str(b) for b in ipv4_bytes)

    @property
    def saddr_str(self) -> str:
        return self.format_addr(self.saddr_h, self.saddr_l)

    @property
    def daddr_str(self) -> str:
        return self.format_addr(self.daddr_h, self.daddr_l)

    def __str__(self) -> str:
        if self.is_listening:
            return f"{self.saddr_str}:{self.sport} -> *:* (netns={self.netns}, pid={self.pid})"
        return f"{self.saddr_str}:{self.sport} -> {self.daddr_str}:{self.dport} (netns={self.netns}, pid={self.pid})"


@dataclass
class MapLeakInfo:
    """Results of leak analysis for a single map."""
    name: str
    total: int
    leaked: int
    samples: List[Tuple[ConnTuple, str]]  # (tuple, reason)
    race_condition_fps: int = 0  # False positives filtered by re-check

    @property
    def leak_rate(self) -> float:
        return self.leaked / self.total if self.total > 0 else 0.0

    @property
    def valid_rate(self) -> float:
        return 1.0 - self.leak_rate


@dataclass
class ConnectionIndex:
    """Index of active TCP connections for a namespace."""
    # established: set of (local_addr_h, local_addr_l, local_port, remote_addr_h, remote_addr_l, remote_port)
    established: Set[Tuple[int, int, int, int, int, int]]
    # listening: set of (local_addr_h, local_addr_l, local_port)
    listening: Set[Tuple[int, int, int]]


@dataclass
class PIDLeakInfo:
    """Results of PID-based leak analysis for a single map."""
    name: str
    total: int
    leaked: int
    dead_pids: List[int]  # List of PIDs that no longer exist

    @property
    def leak_rate(self) -> float:
        return self.leaked / self.total if self.total > 0 else 0.0

    @property
    def valid_rate(self) -> float:
        return 1.0 - self.leak_rate
