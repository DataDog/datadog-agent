"""
Abstract base class for eBPF backends.
"""

from typing import Dict, Generator, List


class EbpfBackend:
    """Abstract interface for eBPF map operations."""

    def list_maps(self) -> List[Dict]:
        """List all eBPF maps. Returns list of dicts with 'name' and 'id' keys."""
        raise NotImplementedError

    def iter_map_by_name(self, name: str) -> Generator[Dict, None, None]:
        """Stream map entries by name, yielding one entry at a time.

        Memory-efficient O(1) streaming - processes entries without loading
        the entire map into memory.

        Args:
            name: eBPF map name

        Yields:
            Dict entries with 'key' and 'value' fields
        """
        raise NotImplementedError

    @staticmethod
    def is_available() -> bool:
        """Check if this backend is available."""
        raise NotImplementedError

    @staticmethod
    def name() -> str:
        """Return the backend name."""
        raise NotImplementedError
