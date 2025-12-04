"""
Download static bpftool binary from GitHub releases.
"""

import os
import platform
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
from typing import Optional

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


def _download_with_curl(url: str, dest: str, verbose: bool = False) -> bool:
    """Try downloading using curl."""
    try:
        cmd = ["curl", "-fsSL", "-o", dest, url]
        result = safe_subprocess_run(cmd, capture_output=True, timeout=60)
        return result.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def _download_with_wget(url: str, dest: str, verbose: bool = False) -> bool:
    """Try downloading using wget."""
    try:
        cmd = ["wget", "-q", "-O", dest, url]
        result = safe_subprocess_run(cmd, capture_output=True, timeout=60)
        return result.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def _download_with_urllib(url: str, dest: str, verbose: bool = False) -> bool:
    """Try downloading using Python's urllib."""
    try:
        urllib.request.urlretrieve(url, dest)
        return True
    except Exception as e:
        if verbose:
            print(f"urllib download failed: {e}", file=sys.stderr)
        return False


def _download_file(url: str, dest: str, verbose: bool = False) -> bool:
    """Download a file using available methods (curl, wget, urllib)."""
    # Try curl first (best certificate handling)
    if _download_with_curl(url, dest, verbose):
        return True

    # Try wget as fallback
    if _download_with_wget(url, dest, verbose):
        return True

    # Last resort: Python's urllib
    return _download_with_urllib(url, dest, verbose)


def download_bpftool(install_dir: str = "/tmp", verbose: bool = False) -> Optional[str]:
    """
    Download and extract static bpftool binary.

    Args:
        install_dir: Directory to install bpftool to (default: /tmp)
        verbose: Print progress messages

    Returns:
        Path to bpftool binary if successful, None otherwise
    """
    url = get_download_url()
    if not url:
        arch = platform.machine()
        if verbose:
            print(f"Unsupported architecture for bpftool download: {arch}", file=sys.stderr)
        return None

    bpftool_path = os.path.join(install_dir, "bpftool")

    # Skip download if already exists and works
    if os.path.exists(bpftool_path):
        try:
            result = safe_subprocess_run([bpftool_path, "version"], capture_output=True, timeout=5)
            if result.returncode == 0:
                if verbose:
                    print(f"Using existing bpftool at {bpftool_path}")
                return bpftool_path
        except (subprocess.TimeoutExpired, OSError):
            pass  # Re-download

    if verbose:
        print(f"Downloading bpftool from {url}...")

    try:
        # Download to a temporary file
        with tempfile.NamedTemporaryFile(suffix=".tar.gz", delete=False) as tmp:
            tmp_path = tmp.name

        if not _download_file(url, tmp_path, verbose):
            if verbose:
                print("Failed to download bpftool (tried curl, wget, urllib)", file=sys.stderr)
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
                if verbose:
                    print("Error: bpftool binary not found in archive", file=sys.stderr)
                os.unlink(tmp_path)
                return None

        os.unlink(tmp_path)  # Clean up tarball

        # Make executable
        os.chmod(bpftool_path, 0o755)

        # Verify it works
        result = safe_subprocess_run([bpftool_path, "version"], capture_output=True, timeout=5)
        if result.returncode != 0:
            if verbose:
                print("Error: Downloaded bpftool doesn't work", file=sys.stderr)
            return None

        if verbose:
            print(f"Successfully installed bpftool to {bpftool_path}")

        return bpftool_path

    except Exception as e:
        if verbose:
            print(f"Error downloading bpftool: {e}", file=sys.stderr)
        return None