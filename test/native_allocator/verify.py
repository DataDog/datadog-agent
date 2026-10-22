"""Validate final Linux product executables, including their stripped form.

Run directly against installed deb/rpm/container artifacts as well as in Bazel.
The preloaded test driver is not an allocator and has no jemalloc dependency.
"""

import argparse
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

SYMBOLS = (
    "malloc calloc realloc reallocarray free posix_memalign aligned_alloc memalign valloc pvalloc "
    "malloc_usable_size mallocx rallocx xallocx sallocx dallocx sdallocx nallocx "
    "mallctl mallctlnametomib mallctlbymib malloc_stats_print malloc_conf malloc_message"
).split()


def inspect(binary):
    symbols = subprocess.check_output(["readelf", "--dyn-syms", "--wide", binary], text=True)
    for symbol in SYMBOLS:
        matches = [line.split() for line in symbols.splitlines() if line.split()[-1:] == [symbol]]
        assert matches, f"{binary}: missing unversioned dynamic export {symbol}"
        assert any(row[4] in {"GLOBAL", "WEAK"} and row[5] == "DEFAULT" and row[6] != "UND" for row in matches), symbol
    dynamic = subprocess.check_output(["readelf", "--dynamic", binary], text=True)
    assert not re.search(r"NEEDED.*jemalloc", dynamic), dynamic


def run(binary, driver, library, jemalloc=None, nosys=None, version_arg="version", library_path=None):
    inspect(binary)
    env = dict(os.environ)
    env.pop("MALLOC_CONF", None)
    env.pop("LD_PRELOAD", None)
    env["DD_ALLOCATOR_TEST_LIBRARY"] = library
    if library_path:
        env["LD_LIBRARY_PATH"] = library_path + ":" + env.get("LD_LIBRARY_PATH", "")
    preloads = [[driver]]
    if jemalloc:
        preloads.extend([[driver, jemalloc], [jemalloc, driver]])
    if nosys:
        preloads.append([nosys, driver])
        if jemalloc:
            preloads.append([nosys, jemalloc, driver])
    for preload in preloads:
        env["LD_PRELOAD"] = ":".join(preload)
        result = subprocess.run([binary, version_arg], env=env, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, result.stdout + result.stderr
        assert "native allocator bindings OK (LOCAL, GLOBAL)" in result.stderr, result.stderr
        print(f"PASS {binary}: LD_PRELOAD={env['LD_PRELOAD']}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", action="append", required=True)
    parser.add_argument("--driver", required=True)
    parser.add_argument("--library", required=True)
    parser.add_argument("--jemalloc")
    parser.add_argument("--nosys")
    parser.add_argument("--version-arg", default="version")
    args = parser.parse_args()
    driver, library = str(Path(args.driver).resolve()), str(Path(args.library).resolve())
    jemalloc = str(Path(args.jemalloc).resolve()) if args.jemalloc else None
    nosys = str(Path(args.nosys).resolve()) if args.nosys else None
    # Ensure the DSO really carries GLIBC-versioned allocation references.
    relocations = subprocess.check_output(["readelf", "--dyn-syms", "--wide", library], text=True)
    for symbol in ["malloc", "calloc", "realloc", "free", "malloc_usable_size"]:
        assert re.search(rf"UND\s+{symbol}@GLIBC_", relocations), relocations
    for path in args.binary:
        binary = str(Path(path).resolve())
        run(binary, driver, library, jemalloc, nosys, args.version_arg)
        with tempfile.TemporaryDirectory() as directory:
            stripped = str(Path(directory, Path(binary).name))
            shutil.copy2(binary, stripped)
            subprocess.run(["strip", "--strip-unneeded", stripped], check=True)
            # Moving a copy changes $ORIGIN. Preserve the original dependency
            # search paths so this tests stripping, not runtime relocation.
            dynamic = subprocess.check_output(["readelf", "--dynamic", binary], text=True)
            paths = re.findall(r"\((?:RUNPATH|RPATH)\).*\[(.*?)\]", dynamic)
            library_path = (
                ":".join(paths)
                .replace("${ORIGIN}", str(Path(binary).parent))
                .replace("$ORIGIN", str(Path(binary).parent))
            )
            run(stripped, driver, library, jemalloc, nosys, args.version_arg, library_path)


if __name__ == "__main__":
    main()
