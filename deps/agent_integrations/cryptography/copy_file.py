#!/usr/bin/env python3
"""Copy one file to another path."""

from __future__ import annotations

import argparse
import shutil
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("src", type=Path)
    parser.add_argument("dst", type=Path)
    args = parser.parse_args()

    args.dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(args.src, args.dst)

    # Ensure the copied file is writable by subsequent actions such as
    # install_name_tool/patchelf-based rpath rewriting.
    args.dst.chmod(args.dst.stat().st_mode | 0o200)


if __name__ == "__main__":
    main()
