"""Runs the healthcheck against a packaging manifest, for use as a Bazel
validation action: touches a stamp file on success, propagates the
healthcheck's exit code otherwise.
"""

import argparse
import sys

import healthcheck


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("manifest_path")
    parser.add_argument("target_os", choices=["linux", "macos"])
    parser.add_argument("stamp_out")
    parser.add_argument("install_prefix", nargs="?")
    parser.add_argument("readelf_path", nargs="?")
    args = parser.parse_args(argv)

    healthcheck_args = ["--manifest-path", args.manifest_path, "--target-os", args.target_os]
    if args.install_prefix:
        healthcheck_args += ["--install-prefix", args.install_prefix]
    if args.readelf_path:
        healthcheck_args += ["--readelf-path", args.readelf_path]

    rc = healthcheck.main(healthcheck_args)
    if rc != 0:
        return rc

    with open(args.stamp_out, "w", encoding="utf-8"):
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
