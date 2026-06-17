#!/usr/bin/env python3
"""Generate a pip constraints file from wheel metadata."""

import argparse
import re
from pathlib import Path

from installer.sources import WheelFile
from installer.utils import parse_metadata_file

from deps.agent_integrations.wheels import expand_wheel_inputs

_NORMALIZE_RE = re.compile(r"[-_.]+")


def _normalize_name(name: str) -> str:
    # https://packaging.python.org/en/latest/specifications/name-normalization
    return _NORMALIZE_RE.sub("-", name).lower()


def read_name_and_version(wheel: Path) -> tuple[str, str]:
    with WheelFile.open(wheel) as source:
        metadata_contents = source.read_dist_info("METADATA")

    metadata = parse_metadata_file(metadata_contents)
    name = metadata["Name"]
    version = metadata["Version"]
    if not name or not version:
        raise ValueError(f"missing Name or Version in METADATA from {wheel}")

    return name, version


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("wheels", nargs="+", type=Path)
    args = parser.parse_args()

    packages = {}
    for wheel in expand_wheel_inputs(args.wheels):
        name, version = read_name_and_version(wheel)
        canonical_name = _normalize_name(name)
        existing = packages.get(canonical_name)
        if existing:
            if existing[1] != version:
                raise ValueError(
                    f"conflicting versions for {canonical_name}: " f"{existing[0]}=={existing[1]} and {name}=={version}"
                )
            continue
        packages[canonical_name] = (name, version)

    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w", encoding="utf-8") as output:
        for canonical_name in sorted(packages):
            name, version = packages[canonical_name]
            output.write(f"{name}=={version}\n")


if __name__ == "__main__":
    main()
