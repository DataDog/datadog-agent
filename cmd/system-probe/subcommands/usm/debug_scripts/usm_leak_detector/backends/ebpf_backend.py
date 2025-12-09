"""
Abstract base class for eBPF backends.
"""

from typing import Dict, Generator, List


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

    def iter_map_by_name(self, name: str) -> Generator[Dict, None, None]:
        """Stream map entries by name, yielding one entry at a time.

        This is the memory-efficient alternative to dump_map_by_name().
        Default implementation falls back to dump_map_by_name().

        Args:
            name: eBPF map name

        Yields:
            Dict entries with 'key' and 'value' fields
        """
        # Default: fall back to non-streaming version
        for entry in self.dump_map_by_name(name):
            yield entry

    @staticmethod
    def is_available() -> bool:
        """Check if this backend is available."""
        raise NotImplementedError

    @staticmethod
    def name() -> str:
        """Return the backend name."""
        raise NotImplementedError
