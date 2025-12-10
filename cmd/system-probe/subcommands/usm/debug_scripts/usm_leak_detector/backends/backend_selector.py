"""
Backend selection logic.
"""

from typing import Optional

from ..logging_config import logger
from .ebpf_backend import EbpfBackend
from .bpftool import BpftoolBackend
from .system_probe import SystemProbeBackend


def get_backend(system_probe_path: Optional[str] = None) -> Optional[EbpfBackend]:
    """Get the best available eBPF backend.

    If system_probe_path is explicitly provided, uses system-probe.
    Otherwise, tries bpftool first, then falls back to system-probe.

    Args:
        system_probe_path: Explicit path to system-probe binary

    Returns:
        An EbpfBackend instance, or None if no backend is available
    """
    # If system-probe path is explicitly provided, use it
    if system_probe_path:
        backend = SystemProbeBackend(binary_path=system_probe_path)
        if backend.binary_path:
            logger.debug(f"Using system-probe backend: {backend.binary_path}")
            return backend
        logger.warning(f"Specified system-probe not found at {system_probe_path}")

    # Try bpftool first (with auto-download if not available)
    backend = BpftoolBackend.get_backend(try_download=True)
    if backend:
        logger.debug("Using bpftool backend")
        return backend

    # Fall back to system-probe
    backend = SystemProbeBackend()
    if backend.binary_path:
        logger.debug(f"Using system-probe backend: {backend.binary_path}")
        return backend

    return None
