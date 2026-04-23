"""Substitute placeholders in a FIPS config file.

Usage (Bazel genrule):
    configure_fips --input <src> --output <dst> KEY=VALUE ...

Usage (manual, legacy):
    configure_fips --destdir <dir> [--embedded_ssl_dir <path>]
"""

import argparse
import os
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Substitute placeholders in a FIPS config file")
    # Bazel genrule interface
    parser.add_argument("--input", help="Input template file")
    parser.add_argument("--output", help="Output file")
    parser.add_argument("substitutions", nargs="*", metavar="KEY=VALUE")
    # Legacy interface kept for manual use
    parser.add_argument("--destdir", help="(legacy) Destination directory")
    parser.add_argument("--embedded_ssl_dir", help="(legacy) Embedded SSL directory")
    args = parser.parse_args()

    if args.input and args.output:
        content = Path(args.input).read_text()
        for kv in args.substitutions:
            key, _, value = kv.partition("=")
            content = content.replace(key, value)
        Path(args.output).write_text(content)
        return 0

    if args.destdir:
        destdir = Path(args.destdir)
        embedded_ssl_dir = args.embedded_ssl_dir or str(destdir / "ssl")
        _replace_in_file(destdir / "ssl" / "openssl.cnf.tmp", "{{embedded_ssl_dir}}", embedded_ssl_dir)
        if os.name != "nt":
            _replace_in_file(destdir / "bin" / "fipsinstall.sh", "{{install_dir}}", str(destdir))
        return 0

    parser.error("either --input/--output or --destdir is required")


def _replace_in_file(path: Path, placeholder: str, replacement: str):
    if not path.exists():
        raise RuntimeError(f"{path} not found")
    path.write_text(path.read_text().replace(placeholder, replacement))


if __name__ == "__main__":
    sys.exit(main())
