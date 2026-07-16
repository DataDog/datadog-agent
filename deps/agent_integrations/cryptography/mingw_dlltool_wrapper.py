#!/usr/bin/env python3
"""Runs MinGW dlltool with short temporary paths.

python3-dll-a invokes dlltool with Bazel output paths as --output-lib. GNU
binutils dlltool derives temporary filenames from that output path; on Windows
those flattened temporary names can exceed path-component limits. This wrapper
runs the real dlltool in a short temporary directory and copies the resulting
import library back to the requested output path.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def _arg_value(args: list[str], name: str) -> tuple[int, str] | None:
    try:
        index = args.index(name)
    except ValueError:
        return None
    if index + 1 >= len(args):
        raise SystemExit(f"missing value for {name}")
    return index + 1, args[index + 1]


def main() -> int:
    real_dlltool = os.environ.get("PYO3_REAL_MINGW_DLLTOOL")
    if not real_dlltool:
        raise SystemExit("PYO3_REAL_MINGW_DLLTOOL is not set")

    args = sys.argv[1:]
    input_def = _arg_value(args, "--input-def")
    output_lib = _arg_value(args, "--output-lib")
    if input_def is None or output_lib is None:
        return subprocess.call([real_dlltool, *args])

    _, input_def_path = input_def
    output_lib_index, output_lib_path = output_lib

    output_path = Path(output_lib_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    with tempfile.TemporaryDirectory(prefix="pyo3-dlltool-") as tmp:
        tmp_path = Path(tmp)
        short_def = tmp_path / "python3.def"
        short_lib = tmp_path / "python3.dll.a"
        shutil.copy2(input_def_path, short_def)

        short_args = list(args)
        short_args[input_def[0]] = str(short_def)
        short_args[output_lib_index] = str(short_lib)

        completed = subprocess.run([real_dlltool, *short_args], cwd=tmp)
        if completed.returncode != 0:
            return completed.returncode

        shutil.copy2(short_lib, output_path)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
