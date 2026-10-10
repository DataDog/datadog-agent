"""Go build info check: every Go executable must record the module owning its main package."""

import subprocess

import file_type
from manifest import Manifest


def check(manifest: Manifest, gobuildinfo_path: str) -> dict[str, str]:
    """Return the build info problems of the manifest's Go executables, keyed by install path."""
    executables = {
        src: dest
        for dest, src in manifest.files.items()
        if file_type.is_elf(src) or file_type.is_macho(src) or file_type.is_pe(src)
    }
    # check=True: an unreadable executable must fail the run, not pass unchecked.
    proc = subprocess.run(
        [gobuildinfo_path],
        input="\n".join(executables),
        capture_output=True,
        text=True,
        check=True,
    )
    return {
        executables[src]: problem for src, problem in (line.split("\t", 1) for line in proc.stdout.splitlines())
    }
