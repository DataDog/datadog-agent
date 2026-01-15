"""Core event data structures for Local Detective POC."""

from dataclasses import dataclass, asdict
from typing import Literal, Optional
from datetime import datetime
import json


@dataclass
class SystemEvent:
    """Canonical event schema for all system events."""

    timestamp: datetime
    event_type: Literal["process", "file_op", "network", "resource", "system"]

    # Process events
    pid: Optional[int] = None
    ppid: Optional[int] = None
    cmd: Optional[str] = None
    cpu_pct: Optional[float] = None
    state: Optional[str] = None  # R, S, D, Z
    memory_rss_mb: Optional[float] = None  # Resident Set Size in MB

    # File events
    file_path: Optional[str] = None
    operation: Optional[str] = None  # open, read, write, lock
    blocked_duration_ms: Optional[float] = None

    # Network events
    src_ip: Optional[str] = None
    dst_ip: Optional[str] = None
    port: Optional[int] = None
    latency_ms: Optional[float] = None
    bytes_sent: Optional[int] = None
    bytes_received: Optional[int] = None

    # System-level PSI (Pressure Stall Information)
    psi_cpu_some_avg10: Optional[float] = None
    psi_memory_some_avg10: Optional[float] = None
    psi_memory_full_avg10: Optional[float] = None
    psi_io_some_avg10: Optional[float] = None
    psi_io_full_avg10: Optional[float] = None

    # Kubernetes metadata (optional)
    pod_name: Optional[str] = None
    container_name: Optional[str] = None
    namespace: Optional[str] = None

    # NO LABELS NEEDED - Zero-shot unsupervised approach
    # We removed is_anomaly and anomaly_label
    # The model learns what "normal" looks like automatically

    def to_dict(self) -> dict:
        """Convert to dictionary for JSON serialization."""
        d = asdict(self)
        d['timestamp'] = self.timestamp.isoformat()
        return d

    @classmethod
    def from_dict(cls, d: dict) -> 'SystemEvent':
        """Create from dictionary (for JSON deserialization)."""
        d = d.copy()
        d['timestamp'] = datetime.fromisoformat(d['timestamp'])
        return cls(**d)
