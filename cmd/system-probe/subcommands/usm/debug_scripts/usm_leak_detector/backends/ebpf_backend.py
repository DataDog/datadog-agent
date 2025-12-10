"""
Abstract base class for eBPF backends.
"""

from abc import ABC, abstractmethod
from typing import Dict, Generator, List


class EbpfBackend(ABC):
    """Abstract interface for eBPF map operations."""

    @abstractmethod
    def list_maps(self) -> List[Dict]:
        """List all eBPF maps. Returns list of dicts with 'name' and 'id' keys."""
        pass

    @abstractmethod
    def iter_map_by_name(self, name: str) -> Generator[Dict, None, None]:
        """Stream map entries by name, yielding one entry at a time.

        Memory-efficient O(1) streaming - processes entries without loading
        the entire map into memory.

        Args:
            name: eBPF map name

        Yields:
            Dict entries with 'key' and 'value' fields
        """
        pass

    @staticmethod
    @abstractmethod
    def is_available() -> bool:
        """Check if this backend is available."""
        pass

    @staticmethod
    @abstractmethod
    def name() -> str:
        """Return the backend name."""
        pass
