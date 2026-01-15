"""Host /proc filesystem collector for Local Detective.

Collects process, network, and file data from the host /proc filesystem.
Designed to run as a privileged pod with hostPID: true and access to /host/proc.
"""

import os
import glob
import re
import subprocess
from typing import List, Dict, Optional, Tuple
from datetime import datetime
from collections import defaultdict

try:
    from .events import SystemEvent
except ImportError:
    # For standalone execution
    from events import SystemEvent


class HostProcCollector:
    """Collect system events from host /proc filesystem."""

    def __init__(self, proc_root: str = "/proc"):
        """
        Initialize collector.

        Args:
            proc_root: Path to proc filesystem (use /host/proc in privileged pod)
        """
        self.proc_root = proc_root
        self.last_cpu_times = {}  # For CPU% calculation
        self.last_network_bytes = {}  # For network bytes delta calculation
        self.container_cache = {}  # Cache PID -> container_id mappings

    def collect(self) -> List[SystemEvent]:
        """
        Collect all events from the system.

        Returns:
            List of SystemEvent objects
        """
        timestamp = datetime.now()
        events = []

        # Collect PSI (Pressure Stall Information)
        print(f"Collecting PSI data...")
        psi_event = self._collect_psi(timestamp)
        if psi_event:
            events.append(psi_event)

        # Collect network bytes for delta calculation
        current_net_bytes = self._collect_network_bytes()

        # Collect processes
        print(f"Collecting processes from {self.proc_root}...")
        events.extend(self._collect_processes(timestamp))

        # Collect network connections
        print(f"Collecting network connections...")
        events.extend(self._collect_network(timestamp))

        # Collect file operations
        print(f"Collecting file operations...")
        events.extend(self._collect_files(timestamp))

        # Store network bytes for next collection
        self.last_network_bytes = current_net_bytes

        return events

    def _collect_processes(self, timestamp: datetime) -> List[SystemEvent]:
        """Collect all process information."""
        events = []

        # Find all process directories
        pid_dirs = glob.glob(os.path.join(self.proc_root, '[0-9]*'))

        for pid_dir in pid_dirs:
            try:
                pid = int(os.path.basename(pid_dir))

                # Read /proc/<pid>/stat
                stat_path = os.path.join(pid_dir, 'stat')
                if not os.path.exists(stat_path):
                    continue

                with open(stat_path, 'r') as f:
                    stat_line = f.read().strip()

                # Parse stat line
                # Format: pid (comm) state ppid pgrp session tty_nr tpgid flags ...
                # Example: 1234 (bash) S 1 1234 1234 34816 1234 4194304 ...
                match = re.match(r'(\d+) \(([^)]+)\) (\S) (\d+)', stat_line)
                if not match:
                    continue

                pid_check = int(match.group(1))
                comm = match.group(2)
                state = match.group(3)
                ppid = int(match.group(4))

                # Parse rest of stat for CPU times
                parts = stat_line.split()
                if len(parts) >= 15:
                    utime = int(parts[13])  # CPU time in user mode
                    stime = int(parts[14])  # CPU time in kernel mode
                    cpu_time = utime + stime

                    # Calculate CPU percentage
                    cpu_pct = self._calculate_cpu_percent(pid, cpu_time)
                else:
                    cpu_pct = 0.0

                # Read memory (RSS) from /proc/<pid>/status
                memory_rss_mb = None
                status_path = os.path.join(pid_dir, 'status')
                if os.path.exists(status_path):
                    try:
                        with open(status_path, 'r') as f:
                            for line in f:
                                if line.startswith('VmRSS:'):
                                    # Format: VmRSS:    12345 kB
                                    parts_mem = line.split()
                                    if len(parts_mem) >= 2:
                                        rss_kb = int(parts_mem[1])
                                        memory_rss_mb = rss_kb / 1024.0
                                    break
                    except (OSError, ValueError):
                        pass

                # Get container ID
                container_id = self._get_container_id(pid)

                # Create event
                event = SystemEvent(
                    timestamp=timestamp,
                    event_type="process",
                    pid=pid,
                    ppid=ppid if ppid > 0 else None,
                    cmd=comm,
                    cpu_pct=cpu_pct,
                    state=state,
                    memory_rss_mb=memory_rss_mb
                )

                # Add container metadata if available
                if container_id:
                    event.container_name = container_id[:12]  # Short ID

                events.append(event)

            except (OSError, ValueError, IndexError) as e:
                # Process may have exited during collection
                continue

        return events

    def _collect_network(self, timestamp: datetime) -> List[SystemEvent]:
        """Collect network connection information with latency metrics."""
        events = []

        # Build inode -> PID mapping
        inode_to_pid = self._build_socket_inode_map()

        # Get RTT data from ss command (if available)
        ss_rtt_map = self._get_ss_rtt_data()

        # Parse /proc/net/tcp and /proc/net/tcp6
        for tcp_file in ['net/tcp', 'net/tcp6']:
            tcp_path = os.path.join(self.proc_root, tcp_file)
            if not os.path.exists(tcp_path):
                continue

            try:
                with open(tcp_path, 'r') as f:
                    lines = f.readlines()

                # Skip header
                for line in lines[1:]:
                    parts = line.split()
                    if len(parts) < 13:
                        continue

                    # Parse addresses
                    # Format: sl local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode
                    local_addr = parts[1]
                    remote_addr = parts[2]
                    state = int(parts[3], 16)

                    # Extended fields from /proc/net/tcp
                    tx_queue = int(parts[4].split(':')[0], 16) if ':' in parts[4] else 0
                    rx_queue = int(parts[4].split(':')[1], 16) if ':' in parts[4] else 0
                    retrnsmt = int(parts[6], 16) if len(parts) > 6 else 0
                    inode = int(parts[9])

                    # Only interested in established connections
                    # State 01 = ESTABLISHED (TCP_ESTABLISHED)
                    if state != 0x01:
                        continue

                    # Parse addresses
                    local_ip, local_port = self._parse_address(local_addr)
                    dst_ip, dst_port = self._parse_address(remote_addr)

                    # Skip connections to localhost or 0.0.0.0
                    if dst_ip in ['127.0.0.1', '0.0.0.0', '::1', '::']:
                        continue

                    # Find owning PID
                    pid = inode_to_pid.get(inode)
                    if not pid:
                        continue

                    # Calculate latency from multiple sources
                    latency_ms = None

                    # Method 1: Get RTT from ss command output
                    conn_key = (local_ip, local_port, dst_ip, dst_port)
                    if conn_key in ss_rtt_map:
                        latency_ms = ss_rtt_map[conn_key]

                    # Method 2: Estimate from queue sizes and retransmits
                    # High tx_queue or rx_queue indicates backpressure
                    # Retransmits indicate packet loss -> high latency
                    if latency_ms is None and (tx_queue > 1000 or rx_queue > 1000 or retrnsmt > 0):
                        # Heuristic: estimate latency from queue size and retransmits
                        queue_latency = (tx_queue + rx_queue) / 100.0  # Rough estimate
                        retrans_latency = retrnsmt * 50.0  # Each retransmit ~50ms penalty
                        latency_ms = queue_latency + retrans_latency

                    # Get container ID
                    container_id = self._get_container_id(pid)

                    # Create event
                    event = SystemEvent(
                        timestamp=timestamp,
                        event_type="network",
                        pid=pid,
                        dst_ip=dst_ip,
                        port=dst_port,
                        latency_ms=latency_ms
                    )

                    if container_id:
                        event.container_name = container_id[:12]

                    events.append(event)

            except (OSError, ValueError) as e:
                continue

        return events

    def _collect_files(self, timestamp: datetime) -> List[SystemEvent]:
        """Collect file operation information."""
        events = []

        # Collect from /proc/locks (file locks)
        locks_path = os.path.join(self.proc_root, 'locks')
        if os.path.exists(locks_path):
            try:
                with open(locks_path, 'r') as f:
                    lines = f.readlines()

                for line in lines:
                    # Format:
                    # 1: FLOCK  ADVISORY  WRITE 1234 08:01:98765 0 EOF
                    parts = line.split()
                    if len(parts) < 6:
                        continue

                    try:
                        pid = int(parts[4])
                        lock_type = parts[1]  # FLOCK, POSIX, etc.

                        # Get container ID
                        container_id = self._get_container_id(pid)

                        # Create event for file lock
                        event = SystemEvent(
                            timestamp=timestamp,
                            event_type="file_op",
                            pid=pid,
                            file_path=f"locked_inode_{parts[5]}",
                            operation="lock",
                            blocked_duration_ms=None  # Can't measure from /proc/locks
                        )

                        if container_id:
                            event.container_name = container_id[:12]

                        events.append(event)

                    except (ValueError, IndexError):
                        continue

            except OSError:
                pass

        # Collect open file descriptors (sample from processes)
        # This can be expensive, so we sample a subset
        pid_dirs = glob.glob(os.path.join(self.proc_root, '[0-9]*'))

        # Sample up to 50 processes for open FDs
        import random
        sampled_pids = random.sample(pid_dirs, min(50, len(pid_dirs)))

        for pid_dir in sampled_pids:
            try:
                pid = int(os.path.basename(pid_dir))
                fd_dir = os.path.join(pid_dir, 'fd')

                if not os.path.isdir(fd_dir):
                    continue

                # List file descriptors
                fds = os.listdir(fd_dir)

                for fd in fds:
                    try:
                        fd_path = os.path.join(fd_dir, fd)
                        target = os.readlink(fd_path)

                        # Skip non-file targets (pipes, sockets, etc.)
                        if target.startswith(('pipe:', 'socket:', 'anon_inode:')):
                            continue

                        # Skip /dev entries
                        if target.startswith('/dev/'):
                            continue

                        # Get container ID
                        container_id = self._get_container_id(pid)

                        # Create event for open file
                        event = SystemEvent(
                            timestamp=timestamp,
                            event_type="file_op",
                            pid=pid,
                            file_path=target,
                            operation="open",
                            blocked_duration_ms=None
                        )

                        if container_id:
                            event.container_name = container_id[:12]

                        events.append(event)

                    except (OSError, FileNotFoundError):
                        continue

            except (OSError, ValueError):
                continue

        return events

    def _build_socket_inode_map(self) -> Dict[int, int]:
        """Build mapping from socket inode to PID."""
        inode_to_pid = {}

        pid_dirs = glob.glob(os.path.join(self.proc_root, '[0-9]*'))

        for pid_dir in pid_dirs:
            try:
                pid = int(os.path.basename(pid_dir))
                fd_dir = os.path.join(pid_dir, 'fd')

                if not os.path.isdir(fd_dir):
                    continue

                # List file descriptors
                fds = os.listdir(fd_dir)

                for fd in fds:
                    try:
                        fd_path = os.path.join(fd_dir, fd)
                        target = os.readlink(fd_path)

                        # Check if it's a socket
                        # Format: socket:[12345]
                        if target.startswith('socket:'):
                            inode = int(target[8:-1])
                            inode_to_pid[inode] = pid

                    except (OSError, ValueError):
                        continue

            except (OSError, ValueError):
                continue

        return inode_to_pid

    def _get_container_id(self, pid: int) -> Optional[str]:
        """Get container ID for a process via cgroup."""
        # Check cache first
        if pid in self.container_cache:
            return self.container_cache[pid]

        cgroup_path = os.path.join(self.proc_root, str(pid), 'cgroup')

        try:
            with open(cgroup_path, 'r') as f:
                cgroup = f.read()

            # Parse cgroup path
            # Example: 0::/kubepods/burstable/pod<uuid>/<container-id>
            # Extract container ID (64 hex chars)
            match = re.search(r'pod[a-f0-9-]+/([a-f0-9]{64})', cgroup)
            if match:
                container_id = match.group(1)
                self.container_cache[pid] = container_id
                return container_id

            # Also try docker-style cgroup
            # Example: 0::/docker/<container-id>
            match = re.search(r'/docker/([a-f0-9]{64})', cgroup)
            if match:
                container_id = match.group(1)
                self.container_cache[pid] = container_id
                return container_id

            # Not in a container
            self.container_cache[pid] = None
            return None

        except (OSError, FileNotFoundError):
            return None

    def _get_ss_rtt_data(self) -> Dict[Tuple[str, int, str, int], float]:
        """
        Get RTT (Round Trip Time) data from ss command.

        Returns:
            Dict mapping (local_ip, local_port, remote_ip, remote_port) -> rtt_ms
        """
        rtt_map = {}

        try:
            # Run ss command with options:
            # -t: TCP only
            # -i: Show internal TCP information (includes RTT)
            # -n: Numeric addresses (don't resolve)
            # -o: Show timer information
            result = subprocess.run(
                ['ss', '-tino'],
                capture_output=True,
                text=True,
                timeout=5
            )

            if result.returncode != 0:
                # ss command not available or failed
                return rtt_map

            lines = result.stdout.strip().split('\n')
            i = 0

            while i < len(lines):
                line = lines[i]

                # Look for connection lines
                # Format: ESTAB  0  0  10.0.1.5:42134  10.0.2.100:5432
                if line.startswith('ESTAB'):
                    parts = line.split()
                    if len(parts) >= 5:
                        # Parse local and remote addresses
                        local_addr = parts[3]
                        remote_addr = parts[4]

                        # Parse address:port
                        if ':' in local_addr and ':' in remote_addr:
                            local_ip, local_port_str = local_addr.rsplit(':', 1)
                            remote_ip, remote_port_str = remote_addr.rsplit(':', 1)

                            try:
                                local_port = int(local_port_str)
                                remote_port = int(remote_port_str)

                                # Look at the next line for RTT info
                                # Format: cubic wscale:7,7 rto:204 rtt:3.5/2.1 ato:40
                                if i + 1 < len(lines):
                                    info_line = lines[i + 1]

                                    # Extract RTT value
                                    # Format: rtt:3.5/2.1 or rtt:3.5
                                    rtt_match = re.search(r'rtt:([\d.]+)', info_line)
                                    if rtt_match:
                                        rtt_ms = float(rtt_match.group(1))

                                        # Store in map
                                        conn_key = (local_ip, local_port, remote_ip, remote_port)
                                        rtt_map[conn_key] = rtt_ms

                            except (ValueError, IndexError):
                                pass

                i += 1

        except (FileNotFoundError, subprocess.TimeoutExpired, Exception) as e:
            # ss command not available or failed - not critical
            pass

        return rtt_map

    def _calculate_cpu_percent(self, pid: int, cpu_time: int) -> float:
        """
        Calculate CPU percentage for a process.

        This requires two samples to calculate the delta.
        First sample will return 0.0.
        """
        if pid not in self.last_cpu_times:
            self.last_cpu_times[pid] = cpu_time
            return 0.0

        last_time = self.last_cpu_times[pid]
        delta = cpu_time - last_time

        # Update cache
        self.last_cpu_times[pid] = cpu_time

        # Convert to percentage
        # Assuming 100 HZ (jiffies per second), 1 second elapsed
        # This is simplified - proper implementation would track wall time
        cpu_pct = (delta / 100.0) * 100.0

        return min(cpu_pct, 100.0)

    def _parse_address(self, addr_hex: str) -> Tuple[str, int]:
        """
        Parse hex address from /proc/net/tcp.

        Format: "0100007F:1F40" = 127.0.0.1:8000
        """
        ip_hex, port_hex = addr_hex.split(':')

        # Parse IP (little-endian hex)
        ip_int = int(ip_hex, 16)

        # Convert to dotted notation (handle endianness)
        if len(ip_hex) == 8:  # IPv4
            ip = '.'.join([
                str((ip_int >> 0) & 0xFF),
                str((ip_int >> 8) & 0xFF),
                str((ip_int >> 16) & 0xFF),
                str((ip_int >> 24) & 0xFF)
            ])
        else:  # IPv6
            # For now, just return hex representation
            ip = ip_hex

        # Parse port
        port = int(port_hex, 16)

        return ip, port

    def _collect_psi(self, timestamp: datetime) -> Optional[SystemEvent]:
        """
        Collect Pressure Stall Information (PSI) from /proc/pressure/.

        PSI provides system-wide resource pressure metrics.
        Returns a single 'system' event with PSI metrics.
        """
        psi_data = {}

        # Collect CPU pressure
        cpu_path = os.path.join(self.proc_root, 'pressure', 'cpu')
        if os.path.exists(cpu_path):
            try:
                with open(cpu_path, 'r') as f:
                    for line in f:
                        # Format: "some avg10=0.20 avg60=0.15 avg300=0.10 total=125000"
                        if line.startswith('some'):
                            match = re.search(r'avg10=([\d.]+)', line)
                            if match:
                                psi_data['cpu_some_avg10'] = float(match.group(1))
            except (OSError, ValueError):
                pass

        # Collect memory pressure
        mem_path = os.path.join(self.proc_root, 'pressure', 'memory')
        if os.path.exists(mem_path):
            try:
                with open(mem_path, 'r') as f:
                    for line in f:
                        if line.startswith('some'):
                            match = re.search(r'avg10=([\d.]+)', line)
                            if match:
                                psi_data['memory_some_avg10'] = float(match.group(1))
                        elif line.startswith('full'):
                            match = re.search(r'avg10=([\d.]+)', line)
                            if match:
                                psi_data['memory_full_avg10'] = float(match.group(1))
            except (OSError, ValueError):
                pass

        # Collect I/O pressure
        io_path = os.path.join(self.proc_root, 'pressure', 'io')
        if os.path.exists(io_path):
            try:
                with open(io_path, 'r') as f:
                    for line in f:
                        if line.startswith('some'):
                            match = re.search(r'avg10=([\d.]+)', line)
                            if match:
                                psi_data['io_some_avg10'] = float(match.group(1))
                        elif line.startswith('full'):
                            match = re.search(r'avg10=([\d.]+)', line)
                            if match:
                                psi_data['io_full_avg10'] = float(match.group(1))
            except (OSError, ValueError):
                pass

        # Create system event if we collected any PSI data
        if psi_data:
            return SystemEvent(
                timestamp=timestamp,
                event_type="system",
                psi_cpu_some_avg10=psi_data.get('cpu_some_avg10'),
                psi_memory_some_avg10=psi_data.get('memory_some_avg10'),
                psi_memory_full_avg10=psi_data.get('memory_full_avg10'),
                psi_io_some_avg10=psi_data.get('io_some_avg10'),
                psi_io_full_avg10=psi_data.get('io_full_avg10')
            )

        return None

    def _collect_network_bytes(self) -> Dict[str, Tuple[int, int]]:
        """
        Collect network interface byte counters from /proc/net/dev.

        Returns dict mapping interface name -> (bytes_received, bytes_transmitted)
        """
        net_bytes = {}

        dev_path = os.path.join(self.proc_root, 'net', 'dev')
        if not os.path.exists(dev_path):
            return net_bytes

        try:
            with open(dev_path, 'r') as f:
                lines = f.readlines()

            # Skip first two header lines
            for line in lines[2:]:
                # Format: "  eth0: 1234567 8910 ..."
                parts = line.split()
                if len(parts) < 10:
                    continue

                # Interface name (remove trailing colon)
                interface = parts[0].rstrip(':')

                # Skip loopback
                if interface == 'lo':
                    continue

                try:
                    bytes_recv = int(parts[1])   # Receive bytes
                    bytes_sent = int(parts[9])   # Transmit bytes
                    net_bytes[interface] = (bytes_recv, bytes_sent)
                except (ValueError, IndexError):
                    continue

        except OSError:
            pass

        return net_bytes


# Example usage / test
if __name__ == "__main__":
    import sys

    # Use /proc by default, or /host/proc if running in privileged pod
    proc_root = "/host/proc" if os.path.exists("/host/proc") else "/proc"

    print("=" * 70)
    print(f"Host Proc Collector Test")
    print(f"Using proc root: {proc_root}")
    print("=" * 70)
    print()

    collector = HostProcCollector(proc_root=proc_root)

    print("Collecting events...")
    events = collector.collect()

    print(f"\nCollected {len(events)} events")
    print()

    # Count by type
    from collections import Counter
    type_counts = Counter(e.event_type for e in events)

    print("Events by type:")
    for event_type, count in type_counts.items():
        print(f"  {event_type:15s}: {count:4d}")
    print()

    # Show PSI data
    psi_events = [e for e in events if e.event_type == "system"]
    if psi_events:
        print("System PSI (Pressure Stall Information):")
        psi = psi_events[0]
        if psi.psi_cpu_some_avg10 is not None:
            print(f"  CPU pressure:    some={psi.psi_cpu_some_avg10:.2f}%")
        if psi.psi_memory_some_avg10 is not None:
            print(f"  Memory pressure: some={psi.psi_memory_some_avg10:.2f}% full={psi.psi_memory_full_avg10:.2f}%")
        if psi.psi_io_some_avg10 is not None:
            print(f"  I/O pressure:    some={psi.psi_io_some_avg10:.2f}% full={psi.psi_io_full_avg10:.2f}%")
        print()

    # Show sample events
    print("Sample process events:")
    for event in [e for e in events if e.event_type == "process"][:5]:
        container = f" (container: {event.container_name})" if event.container_name else ""
        mem = f" mem={event.memory_rss_mb:.1f}MB" if event.memory_rss_mb else ""
        print(f"  PID {event.pid:5d}: {event.cmd:20s} state={event.state} cpu={event.cpu_pct:.1f}%{mem}{container}")
    print()

    print("Sample network events:")
    for event in [e for e in events if e.event_type == "network"][:5]:
        container = f" (container: {event.container_name})" if event.container_name else ""
        latency = f" latency={event.latency_ms:.1f}ms" if event.latency_ms else ""
        print(f"  PID {event.pid:5d} -> {event.dst_ip}:{event.port}{latency}{container}")
    print()

    print("Sample file events:")
    for event in [e for e in events if e.event_type == "file_op"][:5]:
        container = f" (container: {event.container_name})" if event.container_name else ""
        print(f"  PID {event.pid:5d}: {event.operation} {event.file_path[:50]}{container}")
    print()
