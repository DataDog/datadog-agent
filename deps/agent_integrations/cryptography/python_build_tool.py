#!/usr/bin/env python3
"""Small Python interpreter shim for cryptography Rust build scripts.

cryptography-cffi's build.rs expects PYO3_PYTHON to point at something that can
run both `python -c ...` snippets and Python script files. This py_binary gives
that build script a Bazel-declared Python environment containing build-time
Python dependencies such as cffi and setuptools.
"""

from __future__ import annotations

import runpy
import sys


def main() -> None:
    args = sys.argv[1:]
    if not args:
        return

    if args[0] == "-c":
        if len(args) < 2:
            raise SystemExit("argument expected for the -c option")
        sys.argv = ["-c", *args[2:]]
        exec(compile(args[1], "<string>", "exec"), {"__name__": "__main__"})
        return

    script = args[0]
    sys.argv = [script, *args[1:]]
    runpy.run_path(script, run_name="__main__")


if __name__ == "__main__":
    main()
