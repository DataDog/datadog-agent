"""macOS library-dependency healthcheck.

Mirrors lib/omnibus/health_check.rb#health_check_otool: reads each Mach-O's
LC_LOAD_DYLIB / LC_RPATH entries directly from its build-time file and
resolves candidates against the packaging manifest (dest -> src mapping):
  - Run ``otool -L`` to enumerate each file's linked libraries.
  - Resolve ``@rpath`` against ``otool -l`` LC_RPATH entries and
    ``@loader_path`` against the file's own destination directory (not its
    build-time source directory).
  - Anything that does not resolve into the manifest must match an
    allowlist regex.

A linked entry that exactly matches the library's own install name (from
``otool -D``) is not really a dependency and is skipped, same as in Ruby.
"""

import os
import re
import subprocess

import allowlists
import file_type
from manifest import Manifest
from result import Failure, Failures


def _otool(args: list[str]) -> str:
    # check=True: a real otool failure (corrupt file, tool missing) must
    # surface as an error, not be silently read as "no dependencies found".
    # Not verified against real otool exit codes in this sandbox (no macOS
    # toolchain available); confirm on a real macOS run before relying on it.
    proc = subprocess.run(["otool", *args], capture_output=True, text=True, check=True)
    return proc.stdout


def _get_install_name(src_path: str) -> str | None:
    # otool -D output: header line ending with ":", then (optionally) the
    # install name on its own line. Pick the first line ending in ".dylib".
    for line in _otool(["-D", src_path]).splitlines():
        stripped = line.strip()
        if stripped.endswith(".dylib"):
            return stripped
    return None


def _get_rpaths(src_path: str) -> list[str]:
    out = _otool(["-l", src_path])
    rpaths: list[str] = []
    lines = out.splitlines()
    i = 0
    while i < len(lines):
        if "LC_RPATH" in lines[i]:
            # next few lines hold ``         path <value> (offset N)``
            for j in range(i + 1, min(i + 4, len(lines))):
                stripped = lines[j].strip()
                if stripped.startswith("path "):
                    parts = stripped.split()
                    if len(parts) >= 2:
                        rpaths.append(parts[1])
                    break
        i += 1
    return rpaths


def _get_linked_dylibs(src_path: str) -> list[str]:
    lines = _otool(["-L", src_path]).splitlines()
    linked: list[str] = []
    for line in lines[1:]:  # first line just echoes the queried path
        m = re.match(r"^\s+(.+?) \(.+\)\s*$", line)
        if m:
            linked.append(m.group(1))
    return linked


def _resolve_paths(linked: str, loader_dir: str, rpaths: list[str]) -> list[str]:
    if "@rpath" in linked:
        return [linked.replace("@rpath", r).replace("@loader_path", loader_dir) for r in rpaths] or [linked]
    if "@loader_path" in linked:
        return [linked.replace("@loader_path", loader_dir)]
    return [linked]


def _is_allowed(dep_name: str, current_library: str) -> bool:
    for pat in allowlists.MACOS_ALLOWLIST:
        if pat.search(dep_name):
            return True
    for pat in allowlists.FILE_ALLOWLIST:
        if pat.search(current_library):
            return True
    return False


def check_entry(dest_path: str, src_path: str, manifest: Manifest) -> Failures:
    """Check a single Mach-O manifest entry's linked dylibs against the manifest."""
    failures: Failures = set()
    install_name = _get_install_name(src_path) or ""
    rpaths = _get_rpaths(src_path)
    loader_dir = os.path.dirname(dest_path)

    for linked in _get_linked_dylibs(src_path):
        # Skip self-references via install name (Ruby health_check.rb line 502).
        if install_name and install_name == linked:
            continue
        dep_name = os.path.basename(linked)
        if _is_allowed(dep_name, dest_path):
            continue
        possibilities = _resolve_paths(linked, loader_dir, rpaths)
        resolved = any(manifest.resolve(os.path.normpath(p).lstrip("/")) is not None for p in possibilities)
        if not resolved:
            failures.add(Failure(dest_path, dep_name, linked))

    return failures


def check(manifest: Manifest) -> Failures:
    failures: Failures = set()
    for dest_path, src_path in manifest.files.items():
        if file_type.is_macho(src_path):
            failures |= check_entry(dest_path, src_path, manifest)
    return failures
