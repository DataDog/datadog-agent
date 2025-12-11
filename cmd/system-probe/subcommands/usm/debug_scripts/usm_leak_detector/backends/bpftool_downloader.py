"""
Download static bpftool binary from GitHub releases.
"""

import os
import platform
import subprocess
import tarfile
import tempfile
import urllib.request
from typing import Optional

from ..constants import DEFAULT_SUBPROCESS_TIMEOUT, DOWNLOAD_TIMEOUT
from ..logging_config import logger
from ..subprocess_utils import safe_subprocess_run

BPFTOOL_VERSION = "v7.5.0"
BPFTOOL_BASE_URL = "https://github.com/libbpf/bpftool/releases/download"

# Map Python's machine() to bpftool release arch names
ARCH_MAP = {
    "x86_64": "amd64",
    "amd64": "amd64",
    "aarch64": "arm64",
    "arm64": "arm64",
}


def get_arch() -> Optional[str]:
    """Get the architecture name for bpftool download."""
    machine = platform.machine().lower()
    return ARCH_MAP.get(machine)


def get_download_url(version: str = BPFTOOL_VERSION) -> Optional[str]:
    """Get the download URL for the current architecture."""
    arch = get_arch()
    if not arch:
        return None
    return f"{BPFTOOL_BASE_URL}/{version}/bpftool-{version}-{arch}.tar.gz"


def _download_with_curl(url: str, dest: str) -> bool:
    """Try downloading using curl."""
    try:
        cmd = ["curl", "-fsSL", "-o", dest, url]
        result = safe_subprocess_run(cmd, capture_output=True, timeout=DOWNLOAD_TIMEOUT)
        return result.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def _download_with_wget(url: str, dest: str) -> bool:
    """Try downloading using wget."""
    try:
        cmd = ["wget", "-q", "-O", dest, url]
        result = safe_subprocess_run(cmd, capture_output=True, timeout=DOWNLOAD_TIMEOUT)
        return result.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def _download_with_urllib(url: str, dest: str) -> bool:
    """Try downloading using Python's urllib."""
    try:
        urllib.request.urlretrieve(url, dest)
        return True
    except Exception as e:
        logger.debug(f"urllib download failed: {e}")
        return False


def _download_file(url: str, dest: str) -> bool:
    """Download a file using available methods (curl, wget, urllib)."""
    # Try curl first (best certificate handling)
    if _download_with_curl(url, dest):
        return True

    # Try wget as fallback
    if _download_with_wget(url, dest):
        return True

    # Last resort: Python's urllib
    return _download_with_urllib(url, dest)


def download_bpftool(install_dir: str = "/tmp") -> Optional[str]:
    """
    Download and extract static bpftool binary.

    Args:
        install_dir: Directory to install bpftool to (default: /tmp)

    Returns:
        Path to bpftool binary if successful, None otherwise
    """
    url = get_download_url()
    if not url:
        arch = platform.machine()
        logger.debug(f"Unsupported architecture for bpftool download: {arch}")
        return None

    bpftool_path = os.path.join(install_dir, "bpftool")

    # Skip download if already exists and works
    if os.path.exists(bpftool_path):
        try:
            result = safe_subprocess_run([bpftool_path, "version"], capture_output=True, timeout=DEFAULT_SUBPROCESS_TIMEOUT)
            if result.returncode == 0:
                logger.debug(f"Using existing bpftool at {bpftool_path}")
                return bpftool_path
        except (subprocess.TimeoutExpired, OSError):
            pass  # Re-download

    logger.debug(f"Downloading bpftool from {url}...")

    try:
        # Download to a temporary file
        with tempfile.NamedTemporaryFile(suffix=".tar.gz", delete=False) as tmp:
            tmp_path = tmp.name

        if not _download_file(url, tmp_path):
            logger.debug("Failed to download bpftool (tried curl, wget, urllib)")
            return None

        # Extract bpftool binary from tarball
        with tarfile.open(tmp_path, "r:gz") as tar:
            # Find the bpftool binary in the archive
            for member in tar.getmembers():
                if member.name.endswith("bpftool") and member.isfile():
                    # Extract to install_dir
                    member.name = "bpftool"  # Rename to just "bpftool"
                    tar.extract(member, install_dir)
                    break
            else:
                logger.debug("Error: bpftool binary not found in archive")
                os.unlink(tmp_path)
                return None

        os.unlink(tmp_path)  # Clean up tarball

        # Make executable
        os.chmod(bpftool_path, 0o755)

        # Verify it works
        result = safe_subprocess_run([bpftool_path, "version"], capture_output=True, timeout=DEFAULT_SUBPROCESS_TIMEOUT)
        if result.returncode != 0:
            logger.debug("Error: Downloaded bpftool doesn't work")
            return None

        logger.debug(f"Successfully installed bpftool to {bpftool_path}")

        return bpftool_path

    except Exception as e:
        logger.debug(f"Error downloading bpftool: {e}")
        return None
