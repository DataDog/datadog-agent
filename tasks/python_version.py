"""
Update the embedded Python version across the repository.

This task automates the process of upgrading Python patch versions, similar to
the update_go task. It fetches the latest patch version, retrieves official
SHA256 hashes from Python.org SBOM files, and updates all relevant files.
"""

from __future__ import annotations

import json
import re
import ssl
import urllib.request
from pathlib import Path

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message

# Python.org URLs
PYTHON_FTP_URL = "https://www.python.org/ftp/python/"
PYTHON_SBOM_LINUX_URL_TEMPLATE = "https://www.python.org/ftp/python/{version}/Python-{version}.tgz.spdx.json"


def _version_tuple(version: str) -> tuple[int, ...]:
    """Convert a version string like '3.13.8' to a comparable tuple (3, 13, 8)."""
    return tuple(int(x) for x in version.split('.'))


@task
def get(_):
    """Print the current embedded Python version."""
    current_version = _get_current_python_version()
    print(current_version)


@task(
    help={
        "release_note": "Whether to create a release note (default: True).",
    }
)
def update(
    ctx: Context,
    release_note: bool = True,
):
    """
    Update the embedded Python to the latest patch version.

    This task automatically finds and upgrades to the latest patch version
    of the current major.minor Python version (e.g., 3.13.7 -> 3.13.9).

    Minor version updates (e.g., 3.13.x -> 3.14.x) must be done manually.

    Example:
        dda inv python-version.update
    """
    current_version = _get_current_python_version()
    current_major_minor = _get_major_minor_version(current_version)

    # Find latest patch version for current major.minor
    print(f"Finding latest Python {current_major_minor} patch version...")
    target_version = _get_latest_python_version(current_major_minor)
    if not target_version:
        raise Exit(f"Could not find latest Python version for {current_major_minor}")

    # Check if update is needed
    if _version_tuple(target_version) <= _version_tuple(current_version):
        print(color_message(f"Already at Python {current_version} (target: {target_version})", "yellow"))
        return

    print(color_message(f"Updating Python from {current_version} to {target_version}", "blue"))

    # Fetch SHA256 hashes from official Python SBOM
    print("Fetching SHA256 hashes from Python.org SBOM files...")
    try:
        sha256_hash = _get_python_sha256_hash(target_version)
    except Exception as e:
        raise Exit(f"Failed to fetch SHA256 hash: {e}") from e

    # Update all files
    print("Updating version references...")

    # Prepare all updates first to validate patterns before writing anything
    updates = []
    try:
        updates.append(_prepare_omnibus_update(target_version, sha256_hash))
        updates.append(_prepare_bazel_update(target_version, sha256_hash))
        updates.append(_prepare_test_update(target_version))
    except Exit:
        # If any validation fails, don't write anything
        raise

    # All validations passed, now write all files
    for file_path, new_content in updates:
        file_path.write_text(new_content)
        print(f"  ✓ Updated {file_path}")

    if release_note:
        releasenote_path = _create_releasenote(ctx, current_version, target_version)
        if releasenote_path:
            print(color_message(f"Release note created at {releasenote_path}", "green"))

    print(color_message(f"\n✓ Python version upgraded from {current_version} to {target_version}", "green"))


def _get_current_python_version() -> str:
    """Get the current Python version from omnibus config."""
    omnibus_file = Path("omnibus/config/software/python3.rb")
    content = omnibus_file.read_text()

    match = re.search(r'^default_version\s+"([0-9.]+)"', content, re.MULTILINE)
    if not match:
        raise Exit("Could not find default_version in omnibus/config/software/python3.rb")

    return match.group(1)


def _get_major_minor_version(version: str) -> str:
    """Extract major.minor version from full version string."""
    parts = version.split('.')
    if len(parts) < 2:
        raise Exit(f"Invalid version format: {version}")
    return f"{parts[0]}.{parts[1]}"


def _validate_version_string(version: str) -> bool:
    """Validate that version string matches expected format."""
    return bool(re.match(r'^\d+\.\d+\.\d+$', version))


def _validate_sha256(hash_str: str) -> bool:
    """Validate SHA256 hash format (64 hex characters)."""
    return bool(re.match(r'^[0-9A-Fa-f]{64}$', hash_str))


def _url_get(url: str, timeout: int = 30) -> str:
    """Fetch a URL and return its text content."""
    ctx = ssl.create_default_context()
    req = urllib.request.Request(url)
    with urllib.request.urlopen(req, timeout=timeout, context=ctx) as response:
        return response.read().decode('utf-8')


def _get_latest_python_version(major_minor: str) -> str | None:
    """
    Get the latest Python patch version from python.org FTP directory.

    Args:
        major_minor: Python version in format "3.13"

    Returns:
        Latest version string (e.g., "3.13.8") or None if not found
    """
    try:
        html = _url_get(PYTHON_FTP_URL)
    except Exception as e:
        print(color_message(f"Error fetching Python versions: {e}", "red"))
        return None

    # Parse directory listing for version folders
    pattern = rf'<a href="({re.escape(major_minor)}\.\d+)/">'
    versions = []

    for line in html.split("\n"):
        match = re.search(pattern, line)
        if match:
            version_str = match.group(1)
            if _validate_version_string(version_str):
                versions.append(version_str)

    if not versions:
        return None

    versions.sort(key=_version_tuple)
    return versions[-1]


def _get_python_sha256_hash(version: str) -> str:
    """
    Fetch SHA256 hash for Python source tarball using SBOM file.

    Args:
        version: Python version string (e.g., "3.13.8")

    Returns:
        SHA256 hash string

    Raises:
        ValueError: If version format is invalid
        RuntimeError: If SBOM file cannot be fetched or parsed
    """
    if not _validate_version_string(version):
        raise ValueError(f"Invalid version format: {version}")

    sbom_url = PYTHON_SBOM_LINUX_URL_TEMPLATE.format(version=version)

    try:
        text = _url_get(sbom_url)
        data = json.loads(text)

        if not isinstance(data, dict) or 'packages' not in data:
            raise ValueError("Invalid SBOM format")

        packages = data.get('packages', [])
    except Exception as e:
        raise RuntimeError(f'Error fetching SBOM from {sbom_url}: {e}') from e

    # Find CPython package and extract SHA256
    cpython_package = next((pkg for pkg in packages if pkg.get('name') == "CPython"), None)
    if not cpython_package:
        raise ValueError("Could not find CPython package in SBOM")

    checksums = cpython_package.get('checksums', [])
    checksum = next((cs for cs in checksums if cs.get('algorithm') == "SHA256"), None)
    if not checksum:
        raise ValueError("Could not find SHA256 checksum in SBOM")

    hash_value = checksum.get('checksumValue', '')
    if not _validate_sha256(hash_value):
        raise ValueError(f"Invalid SHA256 hash format from SBOM: {hash_value}")

    return hash_value.lower()


def _prepare_omnibus_update(version: str, sha256: str) -> tuple[Path, str]:
    """Prepare Python version and SHA256 update for omnibus config.

    Returns:
        Tuple of (file_path, new_content) ready to write
    """
    file_path = Path("omnibus/config/software/python3.rb")
    content = file_path.read_text()

    # Update version
    version_pattern = r'^(default_version\s+")([0-9.]+)(")$'
    new_content, count = re.subn(version_pattern, rf'\g<1>{version}\g<3>', content, flags=re.MULTILINE)

    if count != 1:
        raise Exit(f"Expected 1 version match in {file_path}, found {count}")

    # Update SHA256
    sha_pattern = r'(:sha256\s+=>\s+")([0-9a-fA-F]{64})(")'
    new_content, count = re.subn(sha_pattern, rf'\g<1>{sha256}\g<3>', new_content)

    if count != 1:
        raise Exit(f"Expected 1 SHA256 match in {file_path}, found {count}")

    return (file_path, new_content)


def _prepare_bazel_update(version: str, sha256: str) -> tuple[Path, str]:
    """Prepare Python version and SHA256 update for Bazel module file.

    Returns:
        Tuple of (file_path, new_content) ready to write
    """
    file_path = Path("deps/cpython/cpython.MODULE.bazel")
    content = file_path.read_text()

    # Update PYTHON_VERSION constant
    version_pattern = r'^(PYTHON_VERSION\s+=\s+")([0-9.]+)(")$'
    new_content, count = re.subn(version_pattern, rf'\g<1>{version}\g<3>', content, flags=re.MULTILINE)

    if count != 1:
        raise Exit(f"Expected 1 PYTHON_VERSION match in {file_path}, found {count}")

    # Update sha256
    sha_pattern = r'(sha256\s+=\s+")([0-9a-fA-F]{64})(")'
    new_content, count = re.subn(sha_pattern, rf'\g<1>{sha256}\g<3>', new_content)

    if count != 1:
        raise Exit(f"Expected 1 sha256 match in {file_path}, found {count}")

    return (file_path, new_content)


def _prepare_test_update(version: str) -> tuple[Path, str]:
    """Prepare expected Python version update for E2E tests.

    Returns:
        Tuple of (file_path, new_content) ready to write
    """
    file_path = Path("test/new-e2e/tests/agent-platform/common/agent_behaviour.go")
    content = file_path.read_text()

    pattern = r'(ExpectedPythonVersion3\s+=\s+")([0-9.]+)(")'
    new_content, count = re.subn(pattern, rf'\g<1>{version}\g<3>', content)

    if count != 1:
        raise Exit(f"Expected 1 version match in {file_path}, found {count}")

    return (file_path, new_content)


def _create_releasenote(ctx: Context, old_version: str, new_version: str) -> str | None:
    """Create a release note for the Python patch version update."""
    template = f"""---
enhancements:
- |
    The Agent's embedded Python has been upgraded from {old_version} to {new_version}
"""

    # Create release note using reno
    res = ctx.run(f'reno new "Bump embedded Python to {new_version}"', hide='both')
    if not res:
        print(color_message("WARNING: Could not create release note. Please create manually.", "orange"))
        return None

    match = re.match(r"^Created new notes file in (.*)$", res.stdout)
    if not match:
        print(color_message("WARNING: Could not get created release note path. Please create manually.", "orange"))
        return None

    path = match.group(1)
    Path(path).write_text(template)

    return path
