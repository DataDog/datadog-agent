#!/usr/bin/env python3
"""Install wheel files into the Agent embedded Python layout."""

import argparse
import shutil
from pathlib import Path

from installer import install
from installer.destinations import SchemeDictionaryDestination
from installer.sources import WheelFile


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(fromfile_prefix_chars="@")
    parser.add_argument("--runtime-output", required=True, type=Path)
    parser.add_argument("--bin-output", required=True, type=Path)
    parser.add_argument("--python-version", required=True)
    parser.add_argument(
        "--interpreter",
        required=True,
        help="Interpreter path to write into generated console script shebangs.",
    )
    parser.add_argument(
        "--entrypoints-dirname",
        required=True,
        help="Expected location of entrypoint scripts, relevant for filling RECORD correctly.",
    )
    parser.add_argument(
        "--platform",
        choices=("posix", "windows"),
    )
    parser.add_argument("wheels", nargs="+", type=Path)
    return parser.parse_args()


def expand_wheels(paths: list[Path]) -> list[Path]:
    # paths might be direct references to wheels or to folders containing wheels.
    # This handles both cases and returns a uniformized and sorted list of paths to wheels.
    wheels = []
    for path in paths:
        if path.is_dir():
            wheels.extend(path.glob("*.whl"))
        else:
            wheels.append(path)
    return sorted(wheels)


def main():
    args = parse_args()
    args.runtime_output.mkdir(parents=True, exist_ok=True)

    if args.platform == "posix":
        site_packages = args.runtime_output / "lib" / f"python{args.python_version}" / "site-packages"
        headers_path = args.runtime_output / "include" / f"python{args.python_version}"
        script_kind = "posix"
    else:
        site_packages = args.runtime_output / "Lib" / "site-packages"
        headers_path = args.runtime_output / "include"
        script_kind = "win-amd64"

    # During the installation, set the bin path to the relative location where it would normally be.
    # This is to ensure that the RECORD entries have the expected relative path - we'll move them afterwards.
    bin_path = args.runtime_output / args.entrypoints_dirname

    scheme = {
        "purelib": str(site_packages),
        "platlib": str(site_packages),
        "headers": str(headers_path),
        "scripts": str(bin_path),
        "data": str(args.runtime_output),
    }

    destination = SchemeDictionaryDestination(
        scheme_dict=scheme,
        interpreter=args.interpreter,
        script_kind=script_kind,
        bytecode_optimization_levels=[],
    )

    for wheel in expand_wheels(args.wheels):
        with WheelFile.open(wheel) as source:
            install(
                source=source,
                destination=destination,
                additional_metadata={},
            )

    # Move the scripts directory to the requested location
    if bin_path.exists():
        shutil.copytree(bin_path, args.bin_output, dirs_exist_ok=True)
        shutil.rmtree(bin_path)
    else:
        args.bin_output.mkdir(parents=True)


if __name__ == "__main__":
    main()
