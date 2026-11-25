"""Deduplicate auditwheel-bundled Linux shared libraries against Agent embedded/lib.

Auditwheel renames bundled shared libraries with content hashes (for example,
``libcrypto-51ce5003.so.3``) inside ``site-packages/*.libs/``, which prevents
exact-file deduplication against the Agent's ``embedded/lib/lib*.so*`` copies
and leaves multiple copies of OpenSSL, Kerberos, curl, zlib, zstd, and unixODBC
on disk.

This module discovers hash-mangled libraries in ``site-packages/*.libs/``,
matches them by library name and SONAME major version to libraries present in
``embedded/lib/``, verifies ELF symbol and symbol-version compatibility for
every consumer binary in ``site-packages/`` (including transitive dependencies
inside ``*.libs/``), rewrites ``DT_NEEDED`` and ``DT_RPATH`` with ``patchelf``,
and removes the redundant wheel-bundled ``.so`` files.

This module is intentionally invoke-free so it can be called both from
``dda inv omnibus.deduplicate-wheel-libs`` and as a standalone script / Bazel
``py_binary``.
"""

from __future__ import annotations

import argparse
import os
import re
import stat
import subprocess
import sys
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

ELF_MAGIC = b"\x7fELF"

# Matches auditwheel hash-mangled shared library names, e.g.:
#   libcrypto-51ce5003.so.3        -> base="libcrypto", major="3"
#   libz-4e3ca1c7.so.1.2.7         -> base="libz", major="1"
#   libkrb5support-c059b95f.so.0.1 -> base="libkrb5support", major="0"
#   liblmdb-05a8c963.so            -> base="liblmdb", major=None
MANGLED_LIB_RE = re.compile(r"^(lib[a-zA-Z0-9_+.]+)-[0-9a-f]{8}\.so(?:\.(\d+)(?:\.\d+)*)?$")

_NEEDED_RE = re.compile(r"\(NEEDED\)\s+Shared library:\s+\[(.+)\]")
_RPATH_RE = re.compile(r"\((?:RPATH|RUNPATH)\)\s+Library (?:rpath|runpath):\s+\[(.+)\]")
_SONAME_RE = re.compile(r"\(SONAME\)\s+Library soname:\s+\[(.+)\]")
_VERNEED_FILE_RE = re.compile(r"\bFile:\s+(\S+)")
_VERNEED_NAME_RE = re.compile(r"\bName:\s+(\S+)")


@dataclass(frozen=True)
class ElfDynamicInfo:
    needed: tuple[str, ...]
    rpaths: tuple[str, ...]
    soname: str | None = None


@dataclass(frozen=True)
class ElfSymbolInfo:
    # (name, version) pairs exported by this binary; default versions also include (name, None).
    defined: frozenset[tuple[str, str | None]]
    defined_names: frozenset[str]
    defined_versions: frozenset[str]
    # (name, version, bind) tuples for undefined dynamic symbols.
    undefined: tuple[tuple[str, str | None, str], ...]
    # Mapping from DT_NEEDED library name -> required symbol version strings.
    verneed: dict[str, frozenset[str]]


@dataclass(frozen=True)
class CandidateLib:
    mangled_name: str
    base_name: str
    target_soname: str
    target_path: Path
    wheel_paths: tuple[Path, ...]


@dataclass
class DeduplicationResult:
    replaced: dict[str, str] = field(default_factory=dict)  # mangled_name -> target_soname
    skipped: dict[str, str] = field(default_factory=dict)  # mangled_name -> reason
    patched_files: list[Path] = field(default_factory=list)
    removed_files: list[Path] = field(default_factory=list)
    removed_dirs: list[Path] = field(default_factory=list)
    bytes_reclaimed: int = 0


def is_elf_file(path: Path) -> bool:
    """Return True if path is a regular (non-symlink) ELF binary."""
    if path.is_symlink() or not path.is_file():
        return False
    try:
        with path.open("rb") as fh:
            return fh.read(4) == ELF_MAGIC
    except OSError:
        return False


def parse_mangled_lib_name(filename: str) -> tuple[str, str] | None:
    """Parse an auditwheel hash-mangled library filename into (base_name, target_soname).

    Returns None if the filename does not match the auditwheel ``<base>-<8hex>.so[.<ver>]`` pattern.
    """
    match = MANGLED_LIB_RE.match(filename)
    if not match:
        return None
    base_name, major = match.group(1), match.group(2)
    target_soname = f"{base_name}.so.{major}" if major is not None else f"{base_name}.so"
    return base_name, target_soname


def parse_dynamic_section(readelf_d_output: str) -> ElfDynamicInfo:
    """Parse ``readelf -dW`` output into NEEDED, RPATH/RUNPATH, and SONAME."""
    needed: list[str] = []
    rpaths: list[str] = []
    soname: str | None = None
    for line in readelf_d_output.splitlines():
        m = _NEEDED_RE.search(line)
        if m:
            needed.append(m.group(1))
            continue
        m = _RPATH_RE.search(line)
        if m:
            for entry in m.group(1).split(":"):
                if entry and entry not in rpaths:
                    rpaths.append(entry)
            continue
        m = _SONAME_RE.search(line)
        if m:
            soname = m.group(1)
    return ElfDynamicInfo(needed=tuple(needed), rpaths=tuple(rpaths), soname=soname)


def parse_dynamic_symbols(
    readelf_syms_output: str,
) -> tuple[frozenset[tuple[str, str | None]], frozenset[str], frozenset[str], tuple[tuple[str, str | None, str], ...]]:
    """Parse ``readelf --dyn-syms -W`` output."""
    defined: set[tuple[str, str | None]] = set()
    defined_names: set[str] = set()
    defined_versions: set[str] = set()
    undefined: list[tuple[str, str | None, str]] = []

    for line in readelf_syms_output.splitlines():
        parts = line.split()
        if len(parts) < 8 or not parts[0].endswith(":"):
            continue
        bind = parts[4]
        ndx = parts[6]
        raw_name = parts[7]
        if bind not in ("GLOBAL", "WEAK"):
            continue
        if ndx == "UND":
            if "@@" in raw_name:
                name, ver = raw_name.split("@@", 1)
            elif "@" in raw_name:
                name, ver = raw_name.split("@", 1)
            else:
                name, ver = raw_name, None
            undefined.append((name, ver, bind))
        else:
            if "@@" in raw_name:
                name, ver = raw_name.split("@@", 1)
                defined.add((name, ver))
                defined.add((name, None))
                defined_versions.add(ver)
            elif "@" in raw_name:
                name, ver = raw_name.split("@", 1)
                defined.add((name, ver))
                defined_versions.add(ver)
            else:
                name = raw_name
                defined.add((name, None))
            defined_names.add(name)

    return (
        frozenset(defined),
        frozenset(defined_names),
        frozenset(defined_versions),
        tuple(undefined),
    )


def parse_version_info(readelf_v_output: str) -> dict[str, frozenset[str]]:
    """Parse the VERNEED table from ``readelf -V -W`` output."""
    verneed: dict[str, set[str]] = {}
    in_verneed = False
    current_file: str | None = None

    for line in readelf_v_output.splitlines():
        if line.startswith("Version needs section"):
            in_verneed = True
            current_file = None
            continue
        if line.startswith("Version "):
            in_verneed = False
            current_file = None
            continue
        if not in_verneed:
            continue
        file_match = _VERNEED_FILE_RE.search(line)
        if file_match:
            current_file = file_match.group(1)
            verneed.setdefault(current_file, set())
            continue
        name_match = _VERNEED_NAME_RE.search(line)
        if name_match and current_file is not None:
            verneed[current_file].add(name_match.group(1))

    return {lib: frozenset(vers) for lib, vers in verneed.items()}


def read_dynamic_info(path: Path, readelf: str = "readelf") -> ElfDynamicInfo:
    proc = subprocess.run([readelf, "-dW", str(path)], capture_output=True, text=True, check=True)
    return parse_dynamic_section(proc.stdout)


def read_symbol_info(path: Path, readelf: str = "readelf") -> ElfSymbolInfo:
    syms_proc = subprocess.run([readelf, "--dyn-syms", "-W", str(path)], capture_output=True, text=True, check=True)
    ver_proc = subprocess.run([readelf, "-V", "-W", str(path)], capture_output=True, text=True, check=True)
    defined, defined_names, defined_versions, undefined = parse_dynamic_symbols(syms_proc.stdout)
    verneed = parse_version_info(ver_proc.stdout)
    return ElfSymbolInfo(
        defined=defined,
        defined_names=defined_names,
        defined_versions=defined_versions,
        undefined=undefined,
        verneed=verneed,
    )


def check_consumer_compatibility(
    consumer_syms: ElfSymbolInfo,
    mangled_name: str,
    wheel_lib_syms: ElfSymbolInfo,
    agent_lib_syms: ElfSymbolInfo,
) -> list[str]:
    """Verify that ``agent_lib_syms`` satisfies every symbol and version ``consumer_syms`` needs from ``mangled_name``.

    Returns a list of human-readable incompatibility descriptions (empty if compatible).
    """
    issues: list[str] = []

    required_versions = consumer_syms.verneed.get(mangled_name, frozenset())
    missing_versions = sorted(required_versions - agent_lib_syms.defined_versions)
    if missing_versions:
        issues.append(f"missing ELF symbol versions: {', '.join(missing_versions)}")

    missing_symbols: list[str] = []
    version_mismatches: list[str] = []
    for sym_name, sym_ver, _bind in consumer_syms.undefined:
        if sym_ver is not None:
            provided_by_wheel = (sym_name, sym_ver) in wheel_lib_syms.defined
        else:
            provided_by_wheel = sym_name in wheel_lib_syms.defined_names
        if not provided_by_wheel:
            continue

        if (sym_name, sym_ver) not in agent_lib_syms.defined:
            label = f"{sym_name}@{sym_ver}" if sym_ver else sym_name
            if sym_name in agent_lib_syms.defined_names:
                version_mismatches.append(label)
            else:
                missing_symbols.append(label)

    if version_mismatches:
        sample = ", ".join(sorted(set(version_mismatches))[:10])
        issues.append(f"symbol version mismatch ({len(version_mismatches)}): {sample}")
    if missing_symbols:
        sample = ", ".join(sorted(set(missing_symbols))[:10])
        issues.append(f"missing symbols ({len(missing_symbols)}): {sample}")

    return issues


def build_updated_rpath(existing_rpaths: tuple[str, ...], rpath_dir: str) -> str:
    """Append ``rpath_dir`` to existing RPATH entries if not already present."""
    entries = list(existing_rpaths)
    if rpath_dir not in entries:
        entries.append(rpath_dir)
    return ":".join(entries)


def patch_elf(
    path: Path,
    replacements: dict[str, str],
    existing_rpaths: tuple[str, ...],
    rpath_dir: str,
    patchelf: str = "patchelf",
) -> None:
    """Patch an ELF binary's DT_NEEDED entries and DT_RPATH via patchelf."""
    cmd = [patchelf]
    for old_needed, new_needed in sorted(replacements.items()):
        cmd.extend(["--replace-needed", old_needed, new_needed])
    new_rpath = build_updated_rpath(existing_rpaths, rpath_dir)
    cmd.extend(["--force-rpath", "--set-rpath", new_rpath, str(path)])

    orig_mode = path.stat().st_mode
    made_writable = False
    if not (orig_mode & stat.S_IWUSR):
        os.chmod(path, orig_mode | stat.S_IWUSR)
        made_writable = True
    try:
        subprocess.run(cmd, check=True, capture_output=True, text=True)
    finally:
        if made_writable:
            os.chmod(path, orig_mode)


def find_site_packages_dirs(install_dir: Path) -> list[Path]:
    """Locate ``embedded/lib/python*/site-packages`` directories under ``install_dir``."""
    embedded_lib = install_dir / "embedded" / "lib"
    if not embedded_lib.is_dir():
        return []
    return sorted(p for p in embedded_lib.glob("python*/site-packages") if p.is_dir())


def _discover_wheel_libs(
    site_packages_dir: Path,
    embedded_lib_dir: Path,
) -> tuple[dict[str, CandidateLib], dict[str, tuple[str, list[Path]]]]:
    """Find hash-mangled libraries inside ``site_packages_dir/*.libs/``.

    Returns:
        candidates: mangled_name -> CandidateLib (matching library exists in embedded_lib_dir)
        unmatched:  mangled_name -> (base_name, list[Path]) (no matching library in embedded_lib_dir)
    """
    mangled_paths: dict[str, list[Path]] = defaultdict(list)
    for child in sorted(site_packages_dir.iterdir()):
        if not child.is_dir() or not (child.name.endswith(".libs") or child.name.startswith(".libs")):
            continue
        for entry in sorted(child.rglob("*")):
            if not is_elf_file(entry):
                continue
            if parse_mangled_lib_name(entry.name) is not None:
                mangled_paths[entry.name].append(entry)

    candidates: dict[str, CandidateLib] = {}
    unmatched: dict[str, tuple[str, list[Path]]] = {}
    for mangled_name, paths in sorted(mangled_paths.items()):
        parsed = parse_mangled_lib_name(mangled_name)
        assert parsed is not None
        base_name, target_soname = parsed
        target_path = embedded_lib_dir / target_soname
        if target_path.exists() and is_elf_file(target_path.resolve()):
            candidates[mangled_name] = CandidateLib(
                mangled_name=mangled_name,
                base_name=base_name,
                target_soname=target_soname,
                target_path=target_path,
                wheel_paths=tuple(paths),
            )
        else:
            unmatched[mangled_name] = (base_name, paths)

    return candidates, unmatched


def deduplicate_wheel_libs(
    site_packages_dir: Path,
    embedded_lib_dir: Path,
    *,
    rpath_dir: str | None = None,
    require_dedup: set[str] | None = None,
    readelf: str = "readelf",
    patchelf: str = "patchelf",
    dry_run: bool = False,
) -> DeduplicationResult:
    """Deduplicate wheel-bundled ELF libraries in ``site_packages_dir`` against ``embedded_lib_dir``."""
    result = DeduplicationResult()
    if not site_packages_dir.is_dir() or not embedded_lib_dir.is_dir():
        return result

    effective_rpath = rpath_dir or str(embedded_lib_dir)
    required = set(require_dedup or ())

    candidates, unmatched = _discover_wheel_libs(site_packages_dir, embedded_lib_dir)
    for mangled_name, (base_name, _) in unmatched.items():
        parsed = parse_mangled_lib_name(mangled_name)
        target_soname = parsed[1] if parsed else base_name
        result.skipped[mangled_name] = f"{target_soname} not present in {embedded_lib_dir}"

    if not candidates and not required:
        return result

    # Scan all ELF files under site_packages_dir once for DT_NEEDED / DT_RPATH.
    all_elfs: dict[Path, ElfDynamicInfo] = {}
    for root, _, filenames in os.walk(site_packages_dir):
        root_path = Path(root)
        for filename in sorted(filenames):
            elf_path = root_path / filename
            if is_elf_file(elf_path):
                all_elfs[elf_path] = read_dynamic_info(elf_path, readelf=readelf)

    # Caches for symbol tables read on demand.
    sym_cache: dict[Path, ElfSymbolInfo] = {}

    def get_syms(p: Path) -> ElfSymbolInfo:
        resolved = p.resolve()
        if resolved not in sym_cache:
            sym_cache[resolved] = read_symbol_info(resolved, readelf=readelf)
        return sym_cache[resolved]

    # Fixed-point ABI verification: if any candidate fails compatibility for a remaining
    # consumer, drop that candidate so it becomes a remaining ELF itself and re-verify
    # any candidates it transitively depends on.
    active: dict[str, CandidateLib] = dict(candidates)
    while True:
        changed = False
        active_wheel_paths = {p for cand in active.values() for p in cand.wheel_paths}

        for mangled_name in list(active):
            cand = active[mangled_name]
            wheel_syms = get_syms(cand.wheel_paths[0])
            agent_syms = get_syms(cand.target_path)

            failure_reasons: list[str] = []
            for elf_path, dyn_info in all_elfs.items():
                if elf_path in active_wheel_paths:
                    continue
                if mangled_name not in dyn_info.needed:
                    continue
                consumer_syms = get_syms(elf_path)
                issues = check_consumer_compatibility(consumer_syms, mangled_name, wheel_syms, agent_syms)
                if issues:
                    rel = elf_path.relative_to(site_packages_dir)
                    failure_reasons.append(f"{rel}: {'; '.join(issues)}")

            if failure_reasons:
                result.skipped[mangled_name] = "ABI incompatibility: " + " | ".join(failure_reasons)
                del active[mangled_name]
                changed = True

        if not changed:
            break

    # Enforce required deduplications (e.g. libcrypto/libssl in FIPS builds).
    if required:
        missing_required: list[str] = []
        all_discovered_bases: dict[str, list[str]] = defaultdict(list)
        for mangled_name, cand in candidates.items():
            all_discovered_bases[cand.base_name].append(mangled_name)
        for mangled_name, (base_name, _) in unmatched.items():
            all_discovered_bases[base_name].append(mangled_name)

        for req_base in sorted(required):
            for mangled_name in all_discovered_bases.get(req_base, ()):
                if mangled_name not in active:
                    reason = result.skipped.get(mangled_name, "unknown reason")
                    missing_required.append(f"{mangled_name} ({reason})")
        if missing_required:
            raise RuntimeError(
                "Required wheel libraries could not be deduplicated against "
                f"{embedded_lib_dir}: {', '.join(missing_required)}"
            )

    if not active:
        return result

    active_wheel_paths = {p for cand in active.values() for p in cand.wheel_paths}

    # Patch every remaining ELF that references any replaced library.
    for elf_path, dyn_info in sorted(all_elfs.items()):
        if elf_path in active_wheel_paths:
            continue
        replacements = {needed: active[needed].target_soname for needed in dyn_info.needed if needed in active}
        if not replacements:
            continue
        if not dry_run:
            patch_elf(
                elf_path,
                replacements,
                dyn_info.rpaths,
                effective_rpath,
                patchelf=patchelf,
            )
        result.patched_files.append(elf_path)

    # Remove redundant wheel-bundled shared libraries and any now-empty *.libs dirs.
    candidate_dirs: set[Path] = set()
    for mangled_name, cand in sorted(active.items()):
        result.replaced[mangled_name] = cand.target_soname
        for wheel_path in cand.wheel_paths:
            candidate_dirs.add(wheel_path.parent)
            if wheel_path.exists():
                result.bytes_reclaimed += wheel_path.stat().st_size
                if not dry_run:
                    wheel_path.unlink()
                result.removed_files.append(wheel_path)

    for libs_dir in sorted(candidate_dirs):
        if libs_dir.is_dir() and not any(libs_dir.iterdir()):
            if not dry_run:
                libs_dir.rmdir()
            result.removed_dirs.append(libs_dir)

    # Post-patch sanity check: ensure no remaining ELF still references a deleted library.
    if not dry_run:
        deleted_names = set(active.keys())
        dangling: list[str] = []
        for elf_path in sorted(all_elfs):
            if elf_path in active_wheel_paths or not elf_path.exists():
                continue
            post_dyn = read_dynamic_info(elf_path, readelf=readelf)
            bad = [n for n in post_dyn.needed if n in deleted_names]
            if bad:
                dangling.append(f"{elf_path.relative_to(site_packages_dir)} -> {bad}")
        if dangling:
            raise RuntimeError(
                f"Dangling DT_NEEDED references remain after wheel library deduplication: {', '.join(dangling)}"
            )

    return result


def format_summary(result: DeduplicationResult) -> str:
    lines: list[str] = []
    reclaimed_mib = result.bytes_reclaimed / (1024 * 1024)
    lines.append(
        f"Wheel library deduplication: replaced {len(result.replaced)} unique libraries "
        f"({len(result.removed_files)} files removed, {len(result.patched_files)} ELF binaries patched, "
        f"{reclaimed_mib:.2f} MiB raw reclaimed)"
    )
    for mangled, target in sorted(result.replaced.items()):
        lines.append(f"  [REPLACED] {mangled} -> {target}")
    for mangled, reason in sorted(result.skipped.items()):
        lines.append(f"  [SKIPPED]  {mangled}: {reason}")
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--install-dir", type=Path, help="Agent install directory (e.g. /opt/datadog-agent)")
    parser.add_argument("--site-packages", type=Path, help="Explicit path to Python site-packages directory")
    parser.add_argument("--embedded-lib", type=Path, help="Explicit path to embedded/lib directory")
    parser.add_argument("--rpath-dir", help="RPATH directory to add (defaults to --embedded-lib)")
    parser.add_argument(
        "--require-dedup",
        default="",
        help="Comma-separated library base names that must be deduplicated if present (e.g. libcrypto,libssl)",
    )
    parser.add_argument("--patchelf", default="patchelf", help="Path to patchelf binary")
    parser.add_argument("--readelf", default="readelf", help="Path to readelf binary")
    parser.add_argument("--dry-run", action="store_true", help="Analyze and verify without modifying files")
    args = parser.parse_args(argv)

    if args.site_packages and args.embedded_lib:
        site_packages_dirs = [args.site_packages]
        embedded_lib_dir = args.embedded_lib
    elif args.install_dir:
        embedded_lib_dir = args.embedded_lib or (args.install_dir / "embedded" / "lib")
        site_packages_dirs = [args.site_packages] if args.site_packages else find_site_packages_dirs(args.install_dir)
    else:
        parser.error("Provide either --install-dir or both --site-packages and --embedded-lib")

    required = {item.strip() for item in args.require_dedup.split(",") if item.strip()}
    for sp_dir in site_packages_dirs:
        res = deduplicate_wheel_libs(
            sp_dir,
            embedded_lib_dir,
            rpath_dir=args.rpath_dir,
            require_dedup=required,
            readelf=args.readelf,
            patchelf=args.patchelf,
            dry_run=args.dry_run,
        )
        print(format_summary(res))
    return 0


if __name__ == "__main__":
    sys.exit(main())
