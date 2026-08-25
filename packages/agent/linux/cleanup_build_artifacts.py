# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

import argparse
import shutil
from pathlib import Path

_BUILD_SOURCE_SUFFIXES = frozenset({".c", ".h", ".hpp", ".pxd", ".pxi", ".pyx"})
_CMAKE_FILENAMES = frozenset({"CMakeLists.txt"})
_CMAKE_SUFFIXES = (".cmake", ".cmake.in")


def _is_build_only_site_packages_file(path: Path) -> bool:
    return path.suffix in _BUILD_SOURCE_SUFFIXES or path.name in _CMAKE_FILENAMES or path.name.endswith(_CMAKE_SUFFIXES)


def cleanup_build_artifacts(install_dir: Path) -> None:
    embedded_dir = install_dir / "embedded"
    if not embedded_dir.is_dir():
        raise FileNotFoundError(f"embedded Agent directory does not exist: {embedded_dir}")

    include_dir = embedded_dir / "include"
    if include_dir.exists():
        shutil.rmtree(include_dir)
    (embedded_dir / "lib/libpcap.a").unlink(missing_ok=True)

    for site_packages in (embedded_dir / "lib").glob("python*/site-packages"):
        for path in site_packages.rglob("*"):
            if path.is_file() and _is_build_only_site_packages_file(path):
                path.unlink()


def main() -> None:
    parser = argparse.ArgumentParser(description="Remove build-only files from a staged Linux Agent installation")
    parser.add_argument("--install-dir", required=True, type=Path)
    args = parser.parse_args()
    cleanup_build_artifacts(args.install_dir)


if __name__ == "__main__":
    main()
