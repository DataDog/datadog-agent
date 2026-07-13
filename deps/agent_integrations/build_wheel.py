#!/usr/bin/env python3
"""Build a wheel from source using `hatchling` directly.

`hatchling` is the PEP-517-compliant build backend used by all Python packages in integrations-core
The main reason to invoke it directly, instead of going through a build frontend, is that
we need control over its behavior that can only achieved by direct calls.
"""

import argparse
from pathlib import Path

from hatchling.builders.wheel import WheelBuilder
from hatchling.metadata.core import load_toml

HATCHLING_BUILD_BACKEND = "hatchling.build"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--src", required=True, type=Path)
    parser.add_argument("--output-dir", required=True)
    parser.add_argument(
        "--exclude",
        action="append",
        default=[],
        help="Additional hatchling wheel-target exclusion pattern. May be passed multiple times.",
    )
    args = parser.parse_args()

    config = load_toml(args.src / "pyproject.toml")

    # Validate that the project defines hatchling as its build backend
    build_backend = config.get("build-system", {}).get("build-backend")
    if build_backend != HATCHLING_BUILD_BACKEND:
        raise ValueError(
            f"Unsupported build backend {build_backend!r} in {args.src / 'pyproject.toml'}; "
            f"expected {HATCHLING_BUILD_BACKEND!r}"
        )

    hatch_build_config = config.setdefault("tool", {}).setdefault("hatch", {}).setdefault("build", {})

    # Disable the default non-hermetic behaviour which walks up the filesystem in search of
    # .gitignore / .hgignore files as a way to potentially exclude files based on them.
    # https://hatch.pypa.io/1.16/config/build/#vcs
    hatch_build_config["ignore-vcs"] = True

    # Modify the hatch build config to explicitly exclude provided files or patterns, as per
    # https://hatch.pypa.io/1.16/config/build/#patterns
    if args.exclude:
        wheel_config = hatch_build_config.get("targets", {}).get("wheel", {})
        # If there's globally set excludes but no wheel-level excludes, avoid overriding globals
        # by modifying the global setting instead.
        if "exclude" not in wheel_config and "exclude" in hatch_build_config:
            hatch_exclude = hatch_build_config["exclude"]
        else:
            hatch_exclude = (
                hatch_build_config.setdefault("targets", {}).setdefault("wheel", {}).setdefault("exclude", [])
            )

        hatch_exclude.extend(args.exclude)

    # This is essentially equivalent to to hatchling's `build_wheel` PEP-517 hook
    next(WheelBuilder(str(args.src), config=config).build(directory=args.output_dir, versions=["standard"]))


if __name__ == "__main__":
    main()
