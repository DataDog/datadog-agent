"""Linux library-dependency healthcheck.

Mirrors lib/omnibus/health_check.rb#health_check_ldd: reads each ELF's
DT_NEEDED / DT_RPATH / DT_RUNPATH directly from its build-time file and
resolves candidates against the packaging manifest (dest -> src mapping).

$ORIGIN in RPATH/RUNPATH is rejected outright rather than resolved: the
dynamic linker ignores $ORIGIN in secure-execution mode (setuid binaries,
files with capabilities), so a packaged binary relying on it would silently
break at runtime for exactly the binaries that need that behavior most.
Agent binaries must use an absolute RPATH.
"""

import os
import re
import subprocess

import allowlists
import file_type
from manifest import Manifest
from result import Failure, Failures

_NEEDED_RE = re.compile(r"\(NEEDED\)\s+Shared library: \[(.+)\]")
_RPATH_RE = re.compile(r"\((?:RPATH|RUNPATH)\)\s+Library (?:rpath|runpath): \[(.+)\]")


def _readelf_dynamic(path: str, readelf_path: str) -> str:
    # check=True: a real readelf failure (corrupt file, tool missing) must
    # surface as an error, not be silently read as "no dependencies found".
    # A binary with no dynamic section at all (e.g. static) still exits 0.
    proc = subprocess.run([readelf_path, "-d", path], capture_output=True, text=True, check=True)
    return proc.stdout


def _get_needed_and_rpaths(path: str, readelf_path: str) -> tuple[list[str], list[str]]:
    needed: list[str] = []
    rpaths: list[str] = []
    for line in _readelf_dynamic(path, readelf_path).splitlines():
        m = _NEEDED_RE.search(line)
        if m:
            needed.append(m.group(1))
            continue
        m = _RPATH_RE.search(line)
        if m:
            rpaths.extend(m.group(1).split(":"))
    return needed, rpaths


def _is_allowed(dep_name: str, current_library: str) -> bool:
    for pat in allowlists.LINUX_ALLOWLIST:
        if pat.search(dep_name):
            return True
    for pat in allowlists.FILE_ALLOWLIST:
        if pat.search(current_library):
            return True
    return False


def _strip_install_prefix(rpath_dir: str, install_prefix: str) -> str | None:
    """Convert an absolute RPATH directory into a manifest-relative dest
    directory. Returns None if the entry doesn't fall under install_prefix.
    """
    install_prefix = install_prefix.rstrip("/")
    if rpath_dir == install_prefix or rpath_dir.startswith(install_prefix + "/"):
        return rpath_dir[len(install_prefix) :].strip("/")
    return None


def check_entry(dest_path: str, src_path: str, manifest: Manifest, install_prefix: str, readelf_path: str) -> Failures:
    """Check a single ELF manifest entry's NEEDED/RPATH against the manifest."""
    failures: Failures = set()
    needed, rpaths = _get_needed_and_rpaths(src_path, readelf_path)

    candidate_dirs = []
    for r in rpaths:
        if "$ORIGIN" in r:
            failures.add(Failure(dest_path, "$ORIGIN in rpath", r))
            continue
        rel = _strip_install_prefix(r, install_prefix)
        if rel is not None:
            candidate_dirs.append(rel)

    for name in needed:
        if _is_allowed(name, dest_path):
            continue
        found = any(manifest.resolve(os.path.join(d, name) if d else name) is not None for d in candidate_dirs)
        if not found:
            failures.add(Failure(dest_path, name, "not found"))

    return failures


def check(manifest: Manifest, install_prefix: str, readelf_path: str = "readelf") -> Failures:
    failures: Failures = set()
    for dest_path, src_path in manifest.files.items():
        if file_type.is_elf(src_path):
            failures |= check_entry(dest_path, src_path, manifest, install_prefix, readelf_path)
    return failures
