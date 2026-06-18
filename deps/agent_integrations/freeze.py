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


def generate_constraints(wheel_inputs: list[Path]) -> list[str]:
    packages = {}
    for wheel in expand_wheel_inputs(wheel_inputs):
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

    return [f"{packages[canonical_name][0]}=={packages[canonical_name][1]}" for canonical_name in sorted(packages)]


def write_constraints(wheel_inputs: list[Path], output_path: Path):
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text("\n".join(generate_constraints(wheel_inputs)) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("wheels", nargs="+", type=Path)
    args = parser.parse_args()

    write_constraints(args.wheels, args.output)


if __name__ == "__main__":
    main()
