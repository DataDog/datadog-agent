"""Go build info check: every Go executable must record a versioned main module and pinned deps."""

import subprocess

import file_type
from manifest import Manifest


def check(manifest: Manifest, gobuildinfo_path: str, first_party_metadata_path: str) -> dict[str, str]:
    """Return the build info problems of the manifest's Go executables, keyed by install path."""
    executables = {
        src: dest
        for dest, src in manifest.files.items()
        if file_type.is_elf(src) or file_type.is_macho(src) or file_type.is_pe(src)
    }
    output = subprocess.check_output(
        [gobuildinfo_path, "-first-party-metadata", first_party_metadata_path], input="\n".join(executables), text=True
    )
    return {executables[src]: problem for src, problem in (line.split("\t", 1) for line in output.splitlines())}
