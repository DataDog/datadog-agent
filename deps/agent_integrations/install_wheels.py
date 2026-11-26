#!/usr/bin/env python3
"""Install wheel files into the Agent embedded Python layout."""

import argparse
import shutil
import zipfile
from pathlib import Path, PurePosixPath

from installer import install
from installer.destinations import SchemeDictionaryDestination
from installer.sources import WheelFile


def is_site_packages_test_file(path: str, data_dir: str, scheme: dict[str, str], site_packages: Path) -> bool:
    parts = PurePosixPath(path).parts
    if parts[0] == data_dir:
        destination = Path(scheme[parts[1]], *parts[2:])
    else:
        destination = site_packages.joinpath(*parts)
    if not destination.is_relative_to(site_packages):
        return False
    return "tests" in destination.relative_to(site_packages).parts[:-1]


class SkipTestsWheelFile(WheelFile):
    def __init__(self, f: zipfile.ZipFile, scheme: dict[str, str], site_packages: Path) -> None:
        super().__init__(f)
        self._scheme = scheme
        self._site_packages = site_packages

    def get_contents(self):
        for record, stream, is_executable in super().get_contents():
            if not is_site_packages_test_file(record[0], self.data_dir, self._scheme, self._site_packages):
                yield record, stream, is_executable


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
        with zipfile.ZipFile(wheel) as f:
            source = SkipTestsWheelFile(f, scheme, site_packages)
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
