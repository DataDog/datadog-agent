#!/usr/bin/env python3
"""Install wheel files into the Agent embedded Python layout."""

import argparse
from pathlib import Path

from installer import install
from installer.destinations import SchemeDictionaryDestination
from installer.sources import WheelFile


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--python-version", required=True)
    parser.add_argument(
        "--interpreter",
        required=True,
        help="Interpreter path to write into generated console script shebangs.",
    )
    parser.add_argument(
        "--script-kind",
        default="posix",
        choices=("posix", "win-amd64"),
    )
    parser.add_argument("wheels", nargs="+", type=Path)
    return parser.parse_args()


def main():
    args = parse_args()
    site_packages = args.output / "lib" / f"python{args.python_version}" / "site-packages"

    scheme = {
        "purelib": str(site_packages),
        "platlib": str(site_packages),
        "headers": str(args.output / "include" / f"python{args.python_version}"),
        "scripts": str(args.output / "bin"),
        "data": str(args.output),
    }

    destination = SchemeDictionaryDestination(
        scheme_dict=scheme,
        interpreter=args.interpreter,
        script_kind=args.script_kind,
        bytecode_optimization_levels=[],
    )

    for wheel in sorted(args.wheels):
        with WheelFile.open(wheel) as source:
            install(
                source=source,
                destination=destination,
                additional_metadata={},
            )


if __name__ == "__main__":
    main()
