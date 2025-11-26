#!/usr/bin/env python3
"""
USM eBPF Map Leak Detector

Detects leaked entries in USM (Universal Service Monitoring) eBPF maps by
validating ConnTuple-keyed map entries against active TCP connections.

Usage:
    python3 usm_leak_detector.py [--maps MAP1,MAP2,...] [-v] [--proc-root PATH]
                                 [--system-probe PATH]

Requirements:
    - Python 3.6+ (stdlib only, no pip dependencies)
    - bpftool binary in PATH OR system-probe binary
    - Root privileges
    - Linux kernel 4.4+

The script will try bpftool first, and fall back to system-probe's eBPF map
commands if bpftool is not available.
"""

import argparse
import json
import os
import struct
import subprocess
import sys
from dataclasses import dataclass
from typing import Dict, List, Optional, Set, Tuple

# ConnTuple-keyed maps to validate
TARGET_MAPS = [
    "connection_states",
    "pid_fd_by_tuple",
    "ssl_ctx_by_tuple",
    "http_in_flight",
    "redis_in_flight",
    "redis_key_in_fli",  # Truncated to 15 chars by kernel
    "postgres_in_flig",  # Truncated to 15 chars by kernel
    "http2_in_flight",
    "connection_proto",  # Truncated to 15 chars by kernel
    "tls_enhanced_tag",  # Truncated to 15 chars by kernel
]

# TCP states from /proc/net/tcp (hex values)
TCP_LISTEN = 0x0A


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

    @property
    def leak_rate(self) -> float:
        return self.leaked / self.total if self.total > 0 else 0.0

    @property
    def valid_rate(self) -> float:
        return 1.0 - self.leak_rate


# =============================================================================
# eBPF Backend Interface
# =============================================================================

class EbpfBackend:
    """Abstract interface for eBPF map operations."""

    def list_maps(self) -> List[Dict]:
        """List all eBPF maps. Returns list of dicts with 'name' and 'id' keys."""
        raise NotImplementedError

    def dump_map_by_name(self, name: str) -> List[Dict]:
        """Dump map contents by name. Returns list of entries with 'key' field."""
        raise NotImplementedError

    def dump_map_by_id(self, map_id: int) -> List[Dict]:
        """Dump map contents by ID. Returns list of entries with 'key' field."""
        raise NotImplementedError

    @staticmethod
    def is_available() -> bool:
        """Check if this backend is available."""
        raise NotImplementedError

    @staticmethod
    def name() -> str:
        """Return the backend name."""
        raise NotImplementedError


# =============================================================================
# bpftool Backend
# =============================================================================

class BpftoolBackend(EbpfBackend):
    """eBPF backend using bpftool."""

    @staticmethod
    def name() -> str:
        return "bpftool"

    @staticmethod
    def is_available() -> bool:
        """Check if bpftool is available."""
        try:
            result = subprocess.run(
                ["bpftool", "version"],
                capture_output=True,
                timeout=5
            )
            return result.returncode == 0
        except (FileNotFoundError, subprocess.TimeoutExpired):
            return False

    def _run(self, args: List[str]) -> Optional[str]:
        """Run bpftool command and return stdout, or None on error."""
        cmd = ["bpftool"] + args
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=30
            )
            if result.returncode != 0:
                print(f"Error running bpftool: {result.stderr}", file=sys.stderr)
                return None
            return result.stdout
        except FileNotFoundError:
            print("Error: bpftool not found in PATH", file=sys.stderr)
            return None
        except subprocess.TimeoutExpired:
            print("Error: bpftool timed out", file=sys.stderr)
            return None

    def list_maps(self) -> List[Dict]:
        """List all eBPF maps using bpftool."""
        output = self._run(["map", "list", "--json"])
        if output is None:
            return []
        try:
            return json.loads(output)
        except json.JSONDecodeError as e:
            print(f"Error parsing bpftool output: {e}", file=sys.stderr)
            return []

    def dump_map_by_id(self, map_id: int) -> List[Dict]:
        """Dump map contents by ID."""
        output = self._run(["map", "dump", "id", str(map_id), "--json"])
        if output is None:
            return []
        try:
            return json.loads(output)
        except json.JSONDecodeError as e:
            print(f"Error parsing map dump: {e}", file=sys.stderr)
            return []

    def dump_map_by_name(self, name: str) -> List[Dict]:
        """Dump map by name - bpftool doesn't support this directly, so we find by ID first."""
        maps = self.list_maps()
        for m in maps:
            if m.get("name") == name:
                return self.dump_map_by_id(m.get("id"))
        return []


# =============================================================================
# system-probe Backend
# =============================================================================

class SystemProbeBackend(EbpfBackend):
    """eBPF backend using system-probe's ebpf map commands."""

    # Common paths to look for system-probe binary
    SEARCH_PATHS = [
        "/opt/datadog-agent/embedded/bin/system-probe",
        "/usr/bin/system-probe",
        "/git/datadog-agent/bin/system-probe/system-probe",
        "./system-probe",
    ]

    def __init__(self, binary_path: Optional[str] = None):
        self.binary_path = binary_path or self._find_binary()

    @staticmethod
    def name() -> str:
        return "system-probe"

    def _find_binary(self) -> Optional[str]:
        """Find system-probe binary."""
        for path in self.SEARCH_PATHS:
            if os.path.isfile(path) and os.access(path, os.X_OK):
                return path
        # Try PATH
        try:
            result = subprocess.run(
                ["which", "system-probe"],
                capture_output=True,
                text=True,
                timeout=5
            )
            if result.returncode == 0:
                return result.stdout.strip()
        except (FileNotFoundError, subprocess.TimeoutExpired):
            pass
        return None

    @staticmethod
    def is_available() -> bool:
        """Check if system-probe is available."""
        backend = SystemProbeBackend()
        return backend.binary_path is not None

    def _run(self, args: List[str]) -> Optional[str]:
        """Run system-probe command and return stdout, or None on error."""
        if not self.binary_path:
            print("Error: system-probe binary not found", file=sys.stderr)
            return None

        cmd = [self.binary_path] + args
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=30
            )
            if result.returncode != 0:
                print(f"Error running system-probe: {result.stderr}", file=sys.stderr)
                return None
            return result.stdout
        except FileNotFoundError:
            print(f"Error: system-probe not found at {self.binary_path}", file=sys.stderr)
            return None
        except subprocess.TimeoutExpired:
            print("Error: system-probe timed out", file=sys.stderr)
            return None

    def list_maps(self) -> List[Dict]:
        """List all eBPF maps using system-probe.

        system-probe output format:
            32487: Hash  name ssl_certs_i2d_X  flags 0x0
                key 8B  value 16B  max_entries 1
        """
        output = self._run(["ebpf", "map", "list"])
        if output is None:
            return []

        maps = []
        lines = output.strip().split("\n")
        i = 0
        while i < len(lines):
            line = lines[i].strip()
            # Parse lines like: "32487: Hash  name ssl_certs_i2d_X  flags 0x0"
            if ":" in line and "name" in line:
                parts = line.split()
                try:
                    map_id = int(parts[0].rstrip(":"))
                    # Find "name" in parts and get the next element
                    name_idx = parts.index("name") + 1
                    name = parts[name_idx] if name_idx < len(parts) else ""
                    maps.append({"id": map_id, "name": name})
                except (ValueError, IndexError):
                    pass
            i += 1

        return maps

    def dump_map_by_name(self, name: str) -> List[Dict]:
        """Dump map contents by name using system-probe.

        system-probe returns BTF-formatted JSON with named fields:
        [{"key": {"saddr_h": 0, "saddr_l": 123, ...}, "value": {...}}, ...]

        We convert the BTF-formatted key to the same format as bpftool hex arrays
        would parse to, so the rest of the code works unchanged.
        """
        output = self._run(["ebpf", "map", "dump", "name", name])
        if output is None:
            return []

        try:
            entries = json.loads(output)
        except json.JSONDecodeError as e:
            print(f"Error parsing system-probe output: {e}", file=sys.stderr)
            return []

        # Convert BTF-formatted keys to our internal format
        result = []
        for entry in entries:
            key = entry.get("key", {})
            if isinstance(key, dict):
                # BTF-formatted key with named fields
                conn = self._parse_btf_key(key)
                if conn:
                    # Store the parsed ConnTuple directly
                    result.append({"key": conn, "_btf": True})
            elif isinstance(key, list):
                # Already hex array format (shouldn't happen with system-probe)
                result.append(entry)

        return result

    def dump_map_by_id(self, map_id: int) -> List[Dict]:
        """Dump map by ID - system-probe uses name, so we find name first."""
        maps = self.list_maps()
        for m in maps:
            if m.get("id") == map_id:
                return self.dump_map_by_name(m.get("name", ""))
        return []

    def _parse_btf_key(self, key: Dict) -> Optional[ConnTuple]:
        """Parse BTF-formatted key dict into ConnTuple."""
        try:
            return ConnTuple(
                saddr_h=key.get("saddr_h", 0),
                saddr_l=key.get("saddr_l", 0),
                daddr_h=key.get("daddr_h", 0),
                daddr_l=key.get("daddr_l", 0),
                sport=key.get("sport", 0),
                dport=key.get("dport", 0),
                netns=key.get("netns", 0),
                pid=key.get("pid", 0),
                metadata=key.get("metadata", 0),
            )
        except (TypeError, ValueError):
            return None


# =============================================================================
# Backend Selection
# =============================================================================

def get_backend(system_probe_path: Optional[str] = None, verbose: bool = False) -> Optional[EbpfBackend]:
    """Get the best available eBPF backend.

    If system_probe_path is explicitly provided, uses system-probe.
    Otherwise, tries bpftool first, then falls back to system-probe.
    """
    # If system-probe path is explicitly provided, use it
    if system_probe_path:
        backend = SystemProbeBackend(binary_path=system_probe_path)
        if backend.binary_path:
            if verbose:
                print(f"Using system-probe backend: {backend.binary_path}")
            return backend
        print(f"Warning: Specified system-probe not found at {system_probe_path}", file=sys.stderr)

    # Try bpftool first
    if BpftoolBackend.is_available():
        if verbose:
            print("Using bpftool backend")
        return BpftoolBackend()

    # Fall back to system-probe
    backend = SystemProbeBackend()
    if backend.binary_path:
        if verbose:
            print(f"Using system-probe backend: {backend.binary_path}")
        return backend

    return None


def find_usm_maps(maps: List[Dict]) -> Dict[str, int]:
    """Filter maps to find USM ConnTuple-keyed maps.

    Returns dict of {map_name: map_id}.
    """
    usm_maps = {}
    for m in maps:
        name = m.get("name", "")
        map_id = m.get("id")
        if map_id is None:
            continue
        # Check if this is a target map (handle kernel name truncation)
        for target in TARGET_MAPS:
            # Kernel truncates names to 15 chars
            if name == target or name == target[:15]:
                usm_maps[name] = map_id
                break
    return usm_maps


# =============================================================================
# ConnTuple Parser
# =============================================================================

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


# =============================================================================
# Namespace-Aware TCP Index
# =============================================================================

@dataclass
class ConnectionIndex:
    """Index of active TCP connections for a namespace."""
    # established: set of (local_addr_h, local_addr_l, local_port, remote_addr_h, remote_addr_l, remote_port)
    established: Set[Tuple[int, int, int, int, int, int]]
    # listening: set of (local_addr_h, local_addr_l, local_port)
    listening: Set[Tuple[int, int, int]]


def discover_namespaces(proc_root: str = "/proc") -> Dict[int, int]:
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
    if len(hex_addr) == 8:
        # IPv4: stored as little-endian 32-bit value in /proc
        addr_l = int(hex_addr, 16)
        return (0, addr_l)
    elif len(hex_addr) == 32:
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

        parts = [hex_addr[i:i+8] for i in range(0, 32, 8)]
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


def parse_proc_net_tcp(pid: int, ipv6: bool = False, proc_root: str = "/proc") -> List[Dict]:
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


def build_connection_index(namespaces: Dict[int, int], proc_root: str = "/proc") -> Dict[int, ConnectionIndex]:
    """Build per-namespace connection indexes.

    Returns dict of {netns_inode: ConnectionIndex}.
    """
    indexes = {}

    for netns_id, pid in namespaces.items():
        established = set()
        listening = set()

        # Parse both IPv4 and IPv6 connections
        for ipv6 in [False, True]:
            conns = parse_proc_net_tcp(pid, ipv6=ipv6, proc_root=proc_root)
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


# =============================================================================
# Validation Logic
# =============================================================================

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


# =============================================================================
# Analysis and Reporting
# =============================================================================

def analyze_map(map_name: str, backend: EbpfBackend, connection_index: Dict[int, ConnectionIndex], verbose: bool = False) -> MapLeakInfo:
    """Analyze a single map for leaks."""
    entries = backend.dump_map_by_name(map_name)
    total = len(entries)
    leaked = []

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


def print_report(results: List[MapLeakInfo], namespaces: Dict[int, int], verbose: bool = False):
    """Print human-readable leak report."""
    print("USM eBPF Map Leak Detection")
    print("=" * 60)
    print()

    print(f"Network Namespaces Discovered: {len(namespaces)}")
    if verbose:
        for netns_id, pid in sorted(namespaces.items()):
            print(f"  - {netns_id} (pid={pid})")
    print()

    print("## Connection Tuple-Keyed Maps")
    print("-" * 60)

    total_maps = 0
    maps_with_leaks = 0
    total_leaked = 0

    for info in results:
        total_maps += 1
        total_leaked += info.leaked
        if info.leaked > 0:
            maps_with_leaks += 1

        valid_pct = info.valid_rate * 100
        print(f"{info.name}: {info.total - info.leaked}/{info.total} entries ({valid_pct:.1f}% valid)")

        if info.leaked > 0:
            print(f"  Leaked entries: {info.leaked}")
            for conn, reason in info.samples[:10]:  # Show max 10 samples
                print(f"    {conn} [{reason}]")
            if len(info.samples) > 10:
                print(f"    ... and {len(info.samples) - 10} more")
        else:
            print("  No leaks detected")
        print()

    print("## Summary")
    print("-" * 60)
    print(f"Total maps checked: {total_maps}")
    print(f"Maps with leaks: {maps_with_leaks}")
    print(f"Total leaked entries: {total_leaked}")


# =============================================================================
# Main
# =============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="Detect leaked entries in USM eBPF maps"
    )
    parser.add_argument(
        "--maps",
        type=str,
        help="Comma-separated list of specific map names to check"
    )
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="Enable verbose output"
    )
    parser.add_argument(
        "--proc-root",
        type=str,
        default="/proc",
        help="Path to /proc filesystem (default: /proc)"
    )
    parser.add_argument(
        "--system-probe",
        type=str,
        metavar="PATH",
        help="Path to system-probe binary (auto-detected if not specified)"
    )

    args = parser.parse_args()

    # Check for root privileges
    if os.geteuid() != 0:
        print("Warning: Not running as root. May not have access to all maps/namespaces.",
              file=sys.stderr)

    # Step 1: Get eBPF backend
    backend = get_backend(system_probe_path=args.system_probe, verbose=args.verbose)
    if backend is None:
        print("Error: No eBPF backend available. Install bpftool or ensure system-probe is accessible.",
              file=sys.stderr)
        sys.exit(1)

    # Step 2: List eBPF maps
    if args.verbose:
        print("Listing eBPF maps...")
    all_maps = backend.list_maps()
    if not all_maps:
        print("Error: Could not list eBPF maps.", file=sys.stderr)
        sys.exit(1)

    # Step 3: Filter to USM maps
    usm_maps = find_usm_maps(all_maps)
    if not usm_maps:
        print("No USM maps found. Is system-probe running with USM enabled?", file=sys.stderr)
        sys.exit(1)

    if args.verbose:
        print(f"Found {len(usm_maps)} USM maps: {list(usm_maps.keys())}")

    # Filter to specific maps if requested
    if args.maps:
        requested = set(args.maps.split(","))
        usm_maps = {k: v for k, v in usm_maps.items()
                    if k in requested or any(k.startswith(r[:15]) for r in requested)}

    # Step 4: Discover network namespaces
    if args.verbose:
        print("Discovering network namespaces...")
    namespaces = discover_namespaces(args.proc_root)
    if not namespaces:
        print("Warning: No network namespaces discovered.", file=sys.stderr)

    if args.verbose:
        print(f"Found {len(namespaces)} namespaces")

    # Step 5: Build connection indexes
    if args.verbose:
        print("Building connection indexes...")
    connection_index = build_connection_index(namespaces, args.proc_root)

    # Step 6: Analyze each map
    results = []
    for map_name in sorted(usm_maps.keys()):
        if args.verbose:
            print(f"Analyzing {map_name}...")
        info = analyze_map(map_name, backend, connection_index, args.verbose)
        results.append(info)

    # Step 7: Print report
    print()
    print_report(results, namespaces, args.verbose)


if __name__ == "__main__":
    main()
