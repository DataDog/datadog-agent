"""Windows DLL base-address relocation overlap check.

Mirrors lib/omnibus/health_check.rb#relocation_check. Reads every DLL under
``embedded/bin`` in the packaging manifest directly from its build-time
file, reads the PE optional header's ImageBase and SizeOfImage, and reports
DLLs whose image ranges overlap.

This check is warning-only in omnibus ("Don't raise an error yet") because
the overlap only matters in FIPS mode. We keep that behaviour: this module
returns the conflict map, the caller decides how to report it without
failing the run.

Note: the upstream Ruby additionally skips DLLs whose architecture differs
from the agent build's; that filter is not ported (agent Windows builds are
x86_64 only today).
"""

import os

from manifest import Manifest

try:
    import pefile  # type: ignore[import-not-found]
except ImportError:  # pragma: no cover - pefile gated to Windows targets
    pefile = None


def check(manifest: Manifest) -> dict[str, dict[str, object]]:
    """Return a map of DLL name -> {base, size, conflicts: [dll, ...]}.

    Empty dict if pefile is unavailable or no overlaps are found.
    """
    if pefile is None:
        return {}

    conflict_map: dict[str, dict[str, object]] = {}
    dll_entries = sorted(
        (dest, src)
        for dest, src in manifest.files.items()
        if dest.lower().startswith("embedded/bin/") and dest.lower().endswith(".dll")
    )

    for dest, src in dll_entries:
        try:
            pe = pefile.PE(src, fast_load=True)
        except pefile.PEFormatError:
            continue

        lib_name = os.path.basename(dest)
        base = pe.OPTIONAL_HEADER.ImageBase
        size = pe.OPTIONAL_HEADER.SizeOfImage
        conflicts: list[str] = []

        for cand_name, details in conflict_map.items():
            cand_base = details["base"]  # type: ignore[index]
            cand_size = details["size"]  # type: ignore[index]
            overlap = not (cand_base >= base + size or cand_base + cand_size <= base)  # type: ignore[operator]
            if overlap:
                details["conflicts"].append(lib_name)  # type: ignore[index]
                conflicts.append(cand_name)

        conflict_map[lib_name] = {"base": base, "size": size, "conflicts": conflicts}

    return {k: v for k, v in conflict_map.items() if v["conflicts"]}
