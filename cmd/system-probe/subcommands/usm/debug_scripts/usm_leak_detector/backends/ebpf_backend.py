"""
Abstract base class for eBPF backends.
"""

import subprocess
from abc import ABC, abstractmethod
from typing import Dict, Generator, List, Optional

from ..constants import COMMAND_TIMEOUT
from ..logging_config import logger
from ..subprocess_utils import safe_subprocess_run


class EbpfBackend(ABC):
    """Abstract interface for eBPF map operations."""

    def _run_command(
        self,
        cmd: List[str],
        tool_name: str,
        timeout: int = COMMAND_TIMEOUT
    ) -> Optional[str]:
        """Run command and return stdout, or None on error.

        Args:
            cmd: Full command to run (including binary path)
            timeout: Timeout in seconds
            tool_name: Name of tool for error messages

        Returns:
            Command stdout on success, None on error
        """
        try:
            result = safe_subprocess_run(
                cmd,
                capture_output=True,
                text=True,
                timeout=timeout
            )
            if result.returncode != 0:
                logger.error(f"Error running {tool_name}: {result.stderr}")
                return None
            return result.stdout
        except FileNotFoundError:
            logger.error(f"{tool_name} not found: {cmd[0]}")
            return None
        except subprocess.TimeoutExpired:
            logger.error(f"{tool_name} timed out")
            return None

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
