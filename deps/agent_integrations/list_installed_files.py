#!/usr/bin/env python3
import argparse
from pathlib import Path

HEADER = "# DO NOT REMOVE/MODIFY - used by package removal tasks"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime-dir", required=True, type=Path)
    parser.add_argument("--prefix", required=True)
    parser.add_argument("--output", required=True, type=Path)
    return parser.parse_args()


def main():
    args = parse_args()
    paths = sorted(
        f"{args.prefix}/{path.relative_to(args.runtime_dir).as_posix()}"
        for site_packages in args.runtime_dir.glob("lib/python*/site-packages")
        for path in site_packages.rglob("*")
    )
    args.output.write_text("\n".join([HEADER, *paths]) + "\n")


if __name__ == "__main__":
    main()
