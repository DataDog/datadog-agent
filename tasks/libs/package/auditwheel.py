#!/usr/bin/env python3

from __future__ import annotations

import argparse
import re
import subprocess
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from pathlib import Path

PatchelfRunner = Callable[[Sequence[str | Path]], str]

_LIBRARY_ABIS = (
    ('libssl', '3'),
    ('libcrypto', '3'),
    ('libz', '1'),
    ('libcurl', '4'),
    ('libkrb5', '3'),
    ('libk5crypto', '3'),
    ('libkrb5support', '0'),
    ('libcom_err', '3'),
    ('libgssapi_krb5', '2'),
    ('libodbc', '2'),
)
_AUDITWHEEL_NAME_PATTERNS = tuple(
    (
        re.compile(rf'^{re.escape(library)}-[0-9a-f]{{8}}\.so\.{abi}(?:\.\d+)*$'),
        f'{library}.so.{abi}',
    )
    for library, abi in _LIBRARY_ABIS
)


@dataclass(frozen=True)
class NormalizationResult:
    removed_library_count: int
    removed_logical_bytes: int
    patched_consumer_count: int


def canonical_library_name(filename: str) -> str | None:
    """Return the embedded ABI SONAME for a supported auditwheel-renamed library."""
    for pattern, canonical_name in _AUDITWHEEL_NAME_PATTERNS:
        if pattern.fullmatch(filename):
            return canonical_name
    return None


def discover_duplicate_libraries(site_packages: Path) -> tuple[Path, ...]:
    """Find supported auditwheel library copies without matching unrelated bundled libraries."""
    return tuple(
        path
        for path in sorted(site_packages.rglob('*'))
        if (path.is_file() or path.is_symlink()) and canonical_library_name(path.name) is not None
    )


def _is_elf(path: Path) -> bool:
    if not path.is_file() or path.is_symlink():
        return False
    with path.open('rb') as file:
        return file.read(4) == b'\x7fELF'


def _discover_elf_files(site_packages: Path) -> tuple[Path, ...]:
    return tuple(path for path in sorted(site_packages.rglob('*')) if _is_elf(path))


def _run_patchelf(arguments: Sequence[str | Path]) -> str:
    command = ['patchelf', *(str(argument) for argument in arguments)]
    completed = subprocess.run(command, check=True, capture_output=True, text=True)
    return completed.stdout.rstrip('\n')


def _read_needed(elf_files: Sequence[Path], runner: PatchelfRunner) -> dict[Path, tuple[str, ...]]:
    return {
        elf_file: tuple(needed for needed in runner(('--print-needed', elf_file)).splitlines() if needed)
        for elf_file in elf_files
    }


def _format_needed_references(references: Sequence[tuple[Path, str]], site_packages: Path) -> str:
    return ', '.join(f'{consumer.relative_to(site_packages)} -> {needed}' for consumer, needed in references)


def normalize_auditwheel_libraries(
    site_packages: Path,
    embedded_lib: Path,
    *,
    runner: PatchelfRunner = _run_patchelf,
) -> NormalizationResult:
    """Retarget supported wheel libraries to embedded SONAMEs, then remove the duplicate copies."""
    site_packages = Path(site_packages)
    embedded_lib = Path(embedded_lib)
    duplicate_libraries = discover_duplicate_libraries(site_packages)
    duplicate_names = {library.name for library in duplicate_libraries}
    removed_logical_bytes = sum(library.stat().st_size for library in duplicate_libraries if not library.is_symlink())

    missing_canonical_libraries = sorted(
        {
            embedded_lib / canonical_name
            for library in duplicate_libraries
            if (canonical_name := canonical_library_name(library.name)) is not None
            and not (embedded_lib / canonical_name).exists()
        }
    )
    if missing_canonical_libraries:
        missing = ', '.join(str(path) for path in missing_canonical_libraries)
        raise RuntimeError(f'canonical library is missing: {missing}')

    elf_files = _discover_elf_files(site_packages)
    needed_by_file = _read_needed(elf_files, runner)
    unmatched_references = [
        (consumer, needed)
        for consumer, needed_libraries in needed_by_file.items()
        for needed in needed_libraries
        if canonical_library_name(needed) is not None and needed not in duplicate_names
    ]
    if unmatched_references:
        references = _format_needed_references(unmatched_references, site_packages)
        raise RuntimeError(f'auditwheel DT_NEEDED has no matching bundled library: {references}')

    patched_consumers: set[Path] = set()
    for consumer, needed_libraries in needed_by_file.items():
        for needed in needed_libraries:
            canonical_name = canonical_library_name(needed)
            if canonical_name is None:
                continue
            runner(('--replace-needed', needed, canonical_name, consumer))
            patched_consumers.add(consumer)

    embedded_lib_rpath = str(embedded_lib)
    for consumer in sorted(patched_consumers):
        existing_rpath = runner(('--print-rpath', consumer))
        if embedded_lib_rpath not in existing_rpath.split(':'):
            runner(('--add-rpath', embedded_lib_rpath, consumer))

    remaining_needed = _read_needed(elf_files, runner)
    stale_references = [
        (consumer, needed)
        for consumer, needed_libraries in remaining_needed.items()
        for needed in needed_libraries
        if canonical_library_name(needed) is not None
    ]
    if stale_references:
        references = _format_needed_references(stale_references, site_packages)
        raise RuntimeError(f'stale auditwheel DT_NEEDED after patching: {references}')

    for library in duplicate_libraries:
        library.unlink()

    return NormalizationResult(
        removed_library_count=len(duplicate_libraries),
        removed_logical_bytes=removed_logical_bytes,
        patched_consumer_count=len(patched_consumers),
    )


def main() -> None:
    parser = argparse.ArgumentParser(
        description='Retarget supported auditwheel libraries to canonical embedded ABI SONAMEs.'
    )
    parser.add_argument('site_packages', type=Path)
    parser.add_argument('embedded_lib', type=Path)
    arguments = parser.parse_args()

    result = normalize_auditwheel_libraries(arguments.site_packages, arguments.embedded_lib)
    print(
        f'Removed {result.removed_library_count} duplicate auditwheel libraries '
        f'({result.removed_logical_bytes} logical bytes); patched {result.patched_consumer_count} ELF consumers.'
    )


if __name__ == '__main__':
    main()
