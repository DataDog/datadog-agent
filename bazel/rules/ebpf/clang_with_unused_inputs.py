# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Run clang and report declared inputs absent from its dependency file."""

import argparse
import posixpath
import shlex
import subprocess
import sys
from pathlib import Path


def parse_depfile(path: Path) -> set[str]:
    content = path.read_text(encoding="utf-8").replace("\\\r\n", "").replace("\\\n", "")
    _, separator, prerequisites = content.partition(":")
    if not separator:
        raise ValueError(f"invalid dependency file: {path}")
    return {posixpath.normpath(item) for item in shlex.split(prerequisites)}


def find_unused_inputs(declared_inputs: list[str], used_inputs: set[str]) -> list[str]:
    normalized_used_inputs = {posixpath.normpath(path) for path in used_inputs}
    return sorted(path for path in declared_inputs if posixpath.normpath(path) not in normalized_used_inputs)


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(fromfile_prefix_chars="@")
    parser.add_argument("--compiler", required=True)
    parser.add_argument("--depfile", required=True, type=Path)
    parser.add_argument("--unused-inputs-list", required=True, type=Path)
    parser.add_argument("--declared-input", action="append", default=[])
    parser.add_argument("compiler_args", nargs=argparse.REMAINDER)
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    compiler_args = args.compiler_args
    if compiler_args[:1] == ["--"]:
        compiler_args = compiler_args[1:]

    result = subprocess.run([args.compiler, *compiler_args], check=False)
    if result.returncode:
        return result.returncode

    used_inputs = parse_depfile(args.depfile)
    unused_inputs = find_unused_inputs(args.declared_input, used_inputs)
    content = "".join(f"{path}\n" for path in unused_inputs)
    args.unused_inputs_list.write_text(content, encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
