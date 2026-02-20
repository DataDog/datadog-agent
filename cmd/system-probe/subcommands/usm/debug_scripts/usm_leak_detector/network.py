"""
Network namespace discovery and TCP connection parsing.
"""

import os
import struct
from typing import Dict, List, Tuple

from .models import ConnectionIndex
from .constants import TCP_LISTEN

# /proc/net/tcp hex address format
IPV4_HEX_CHARS = 8  # Hex chars for IPv4 address (e.g., "0100007F")
IPV6_HEX_CHARS = 32  # Total hex chars for IPv6 address
IPV6_HEX_CHARS_PER_WORD = 8  # Hex chars per 32-bit word in /proc/net/tcp6


def discover_namespaces(proc_root: str) -> Dict[int, int]:
    """Scan /proc/*/ns/net to build netns -> pid mapping.

    Returns dict of {netns_inode: representative_pid}.
    """
    namespaces = {}

    try:
        proc_entries = os.listdir(proc_root)
    except OSError:
        return namespaces

    for entry in proc_entries:
        if not entry.isdigit():
            continue

        pid = int(entry)
        netns_path = os.path.join(proc_root, entry, "ns", "net")

        try:
            stat = os.stat(netns_path)
            netns_id = stat.st_ino
            if netns_id not in namespaces:
                namespaces[netns_id] = pid
        except (FileNotFoundError, OSError, PermissionError):
            continue

    return namespaces


def parse_hex_addr(hex_addr: str) -> Tuple[int, int]:
    """Parse hex address from /proc/net/tcp into (addr_h, addr_l).

    Returns address in the same format as eBPF ConnTuple (little-endian uint64s).

    IPv4: 8 hex chars (e.g., "0100007F" for 127.0.0.1)
    IPv6: 32 hex chars (four 32-bit little-endian values in network order)
    """
    if len(hex_addr) == IPV4_HEX_CHARS:
        # IPv4: stored as little-endian 32-bit value in /proc
        addr_l = int(hex_addr, 16)
        return (0, addr_l)
    elif len(hex_addr) == IPV6_HEX_CHARS:
        # IPv6 in /proc/net/tcp6: four 32-bit words, each printed in big-endian hex
        # but representing little-endian values. We need to match eBPF ConnTuple format.
        #
        # Example: "0000000000000000FFFF0000030012AC" represents ::ffff:172.18.0.3
        # Parts: ['00000000', '00000000', 'FFFF0000', '030012AC']
        #
        # eBPF ConnTuple stores this as two little-endian uint64s:
        #   addr_h = 0x0000000000000000
        #   addr_l = 0x030012ACFFFF0000 (IPv4_le << 32 | 0xFFFF0000)
        #
        # The /proc format is: word0, word1, word2, word3 where each word is
        # printed as big-endian hex but stored as little-endian in memory.
        # To get the eBPF format: interpret as little-endian uint64s.

        parts = [hex_addr[i:i+IPV6_HEX_CHARS_PER_WORD] for i in range(0, IPV6_HEX_CHARS, IPV6_HEX_CHARS_PER_WORD)]
        # /proc/net/tcp6 prints each 32-bit word as big-endian hex, but the
        # actual bytes in memory are little-endian. Pack as little-endian.
        reconstructed = b""
        for part in parts:
            val = int(part, 16)
            reconstructed += struct.pack("<I", val)
        # Interpret as two little-endian uint64s to match eBPF format
        addr_h, addr_l = struct.unpack("<QQ", reconstructed)
        return (addr_h, addr_l)
    else:
        return (0, 0)


def parse_proc_net_tcp(pid: int, proc_root: str, ipv6: bool = False) -> List[Dict]:
    """Parse /proc/<pid>/net/tcp{,6}.

    Returns list of dicts with local/remote addr info and state.
    """
    filename = "tcp6" if ipv6 else "tcp"
    filepath = os.path.join(proc_root, str(pid), "net", filename)

    connections = []
    try:
        with open(filepath, "r") as f:
            # Skip header line
            next(f)
            for line in f:
                parts = line.split()
                if len(parts) < 4:
                    continue

                # Parse local address:port
                local_addr_port = parts[1].split(":")
                local_hex_addr = local_addr_port[0]
                local_port = int(local_addr_port[1], 16)

                # Parse remote address:port
                remote_addr_port = parts[2].split(":")
                remote_hex_addr = remote_addr_port[0]
                remote_port = int(remote_addr_port[1], 16)

                # Parse TCP state
                state = int(parts[3], 16)

                local_addr_h, local_addr_l = parse_hex_addr(local_hex_addr)
                remote_addr_h, remote_addr_l = parse_hex_addr(remote_hex_addr)

                connections.append({
                    "local_addr_h": local_addr_h,
                    "local_addr_l": local_addr_l,
                    "local_port": local_port,
                    "remote_addr_h": remote_addr_h,
                    "remote_addr_l": remote_addr_l,
                    "remote_port": remote_port,
                    "state": state,
                })
    except (FileNotFoundError, OSError, PermissionError):
        pass

    return connections


def build_connection_index(namespaces: Dict[int, int], proc_root: str) -> Dict[int, ConnectionIndex]:
    """Build per-namespace connection indexes.

    Returns dict of {netns_inode: ConnectionIndex}.
    """
    indexes = {}

    for netns_id, pid in namespaces.items():
        established = set()
        listening = set()

        # Parse both IPv4 and IPv6 connections
        for ipv6 in [False, True]:
            conns = parse_proc_net_tcp(pid, proc_root, ipv6=ipv6)
            for conn in conns:
                local_key = (
                    conn["local_addr_h"],
                    conn["local_addr_l"],
                    conn["local_port"]
                )

                if conn["state"] == TCP_LISTEN:
                    listening.add(local_key)
                else:
                    # For established connections, include remote info
                    full_key = (
                        conn["local_addr_h"],
                        conn["local_addr_l"],
                        conn["local_port"],
                        conn["remote_addr_h"],
                        conn["remote_addr_l"],
                        conn["remote_port"]
                    )
                    established.add(full_key)

        indexes[netns_id] = ConnectionIndex(established=established, listening=listening)

    return indexes
