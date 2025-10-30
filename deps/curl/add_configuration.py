"""Add a new configuration to the overlay for a module."""

import argparse
import contextlib
import os
import platform
import subprocess
import sys
from pathlib import Path


def do_configure(
    pristine_path: Path = None, configured_path: Path = None, config_name: Path = None, configure_options: Path = None
) -> None:
    configure_path = pristine_path.relative_to(configured_path, walk_up=True) / "configure"
    cmd = [str(configure_path)]
    cmd.extend(configure_options)
    with contextlib.chdir(config_name):
        subprocess.run(cmd, capture_output=True, text=True, check=True)


def main():
    parser = argparse.ArgumentParser(description="Run configure for a project for the current host")
    parser.add_argument("--pristine_dir", help="Path to pristine source tree", required=True)
    parser.add_argument(
        "--configure_options",
        help="Path to configure options file",
        required=True,
    )
    parser.add_argument("--configured_name", help="Name of the configuration")

    # parser.add_argument("--configured_dir", help="Path to configured source tree")
    # parser.add_argument("--overlay_dir", help="Path to overlay directory")
    args = parser.parse_args()

    pristine_path = Path(args.pristine_dir)
    if not pristine_path.exists():
        raise FileNotFoundError(f"Pristine directory not found: {args.pristine_dir}")

    configure_options_path = Path(args.configure_options)
    with open(configure_options_path) as inp:
        configure_options = inp.read().split("\n")

    if args.configured_name:
        config_name = args.configured_name
    else:
        config_name = f"{sys.platform}_{platform.machine()}"
    print(f"Configuring for {config_name}")

    configured_path = pristine_path.parent / config_name
    if not configured_path.exists():
        os.mkdir(configured_path)

    do_configure(pristine_path, configured_path, config_name, configure_options)

    # TODO: Call analyzer.
    """
    overlay_path = Path(args.overlay_dir)
    print(f"Analyzing pristine directory: {pristine_path}")
    source_files, generated_files = analyze_pristine(pristine_path)
    print(
        f"Found {len(source_files)} source files and {len(generated_files)} generated files"
    )
    """


if __name__ == "__main__":
    main()
