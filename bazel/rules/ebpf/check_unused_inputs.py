# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Fail a build whose eBPF program declares headers clang never opened.

Kernel headers are excluded: they are declared in bulk by the linux_headers
repositories and can never be pruned per program.
"""

import argparse
import sys
from pathlib import Path

EXTERNAL_PREFIX = "external/"


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(fromfile_prefix_chars="@")
    parser.add_argument("--unused-inputs-list", required=True, type=Path)
    parser.add_argument("--marker", required=True, type=Path)
    parser.add_argument("--label", required=True)
    parser.add_argument("--allowed", action="append", default=[])
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)

    unused = {
        line
        for line in args.unused_inputs_list.read_text(encoding="utf-8").splitlines()
        if line and not line.startswith(EXTERNAL_PREFIX)
    }
    allowed = set(args.allowed)

    problems = []
    if undeclared := sorted(unused - allowed):
        problems.append(
            f"{args.label} declares headers that clang never opened:\n"
            + "".join(f"    {path}\n" for path in undeclared)
            + "Drop them from deps, or if the include sits behind an #ifdef,\n"
            "list them in allowed_unused with a comment saying which one."
        )
    if stale := sorted(allowed - unused):
        problems.append(
            f"{args.label} lists allowed_unused entries that are now used:\n"
            + "".join(f"    {path}\n" for path in stale)
            + "Remove them from allowed_unused."
        )

    if problems:
        print("\n".join(problems), file=sys.stderr)
        return 1

    args.marker.write_text("", encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
