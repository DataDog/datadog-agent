"""Describe how ELF binaries were built, without comparing their contents."""

import re
import subprocess
from pathlib import Path

ELF_MAGIC = b'\x7fELF'


class UnreadableBinary(Exception):
    """A tool ran but could not read the file, e.g. a truncated or malformed ELF."""


def _run(command: list[str]) -> str:
    # A missing tool raises FileNotFoundError, which is a wiring failure and must not be hidden.
    result = subprocess.run(command, capture_output=True, text=True, encoding='utf-8')
    if result.returncode:
        raise UnreadableBinary(result.stderr.strip() or f'exit status {result.returncode}')
    return result.stdout


# Build settings that identify a configuration. Linker flags (and stamps) are left out: the go command
# records them but Bazel's rules_go does not, and shipped binaries are stripped anyway.
GO_SETTINGS = ('CGO_ENABLED', '-tags', '-buildmode', '-compiler', '-trimpath', 'GOARCH', 'GOOS')


def is_elf(path: Path) -> bool:
    if path.is_symlink() or not path.is_file():
        return False
    with path.open('rb') as f:
        return f.read(4) == ELF_MAGIC


def parse_go_build_info(output: str) -> dict[str, str]:
    """Parse `go version -m` output into the Go version and the identifying build settings."""
    info = {}
    for line in output.splitlines():
        if not line.startswith('\t') and ': go' in line:
            info['go'] = line.rsplit(': ', 1)[1]
        parts = line.strip().split('\t', 1)
        if parts[0] == 'build' and len(parts) == 2:
            key, _, value = parts[1].partition('=')
            if key in GO_SETTINGS:
                info[key] = value
    return info


def elf_signature(path: Path, readelf: str) -> dict:
    """Return the properties of a binary that describe its build configuration."""
    # A single readelf run prints four sections, each parsed below:
    #   -S section headers: section names, to tell whether symbols/debug info were stripped, or Go build info exists
    #   -h ELF header: the file type (EXEC for a fixed address binary, DYN for a PIE or shared library)
    #   -d dynamic section: the NEEDED libraries and the RPATH/RUNPATH search paths
    #   -l program headers: the requested dynamic loader, absent for statically linked binaries
    # -W prints wide lines, so names and paths are never truncated.
    dump = _run([readelf, '-W', '-S', '-h', '-d', '-l', str(path)])
    sections = set(re.findall(r'\]\s+(\.\S+)', dump))
    interpreter = re.search(r'Requesting program interpreter: (.*)\]', dump)
    signature = {
        'stripped': '.symtab' not in sections and not any(s.startswith('.debug_') for s in sections),
        'type': re.search(r'Type:\s+(\w+)', dump).group(1),
        'interpreter': interpreter.group(1) if interpreter else None,
        'needed': sorted(re.findall(r'\(NEEDED\)\s+Shared library: \[(.*)\]', dump)),
        'rpath': sorted(re.findall(r'\(R(?:UN)?PATH\)\s+Library r(?:un)?path: \[(.*)\]', dump)),
        'go': None,
    }
    if '.go.buildinfo' in sections:
        signature['go'] = parse_go_build_info(_run(['go', 'version', '-m', str(path)]))
    return signature


def binary_signatures(directory: str, paths: list[str], readelf: str) -> dict[str, dict]:
    """Signatures of the ELF files (executables and shared libraries) among paths, relative to directory."""
    root = Path(directory)
    signatures = {}
    for path in paths:
        if is_elf(root / path):
            try:
                signatures[path] = elf_signature(root / path, readelf)
            except UnreadableBinary as error:
                signatures[path] = {'unreadable': str(error)}
    return signatures


def signature_differences(previous: dict, current: dict) -> list[str]:
    """Names of the signature properties that differ between two binaries."""
    if 'unreadable' in previous or 'unreadable' in current:
        return [] if 'unreadable' in previous and 'unreadable' in current else ['unreadable']
    return [key for key in previous if previous[key] != current.get(key)]
