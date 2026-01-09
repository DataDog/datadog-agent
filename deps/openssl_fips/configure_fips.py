"""Cross-platform FIPS configuration script.

Replaces placeholder values in openssl.cnf.tmp and fipsinstall.sh with actual paths.
Works on both Windows and Linux.
"""

import argparse
import os
import sys
from pathlib import Path


def replace_in_file(filepath: Path, placeholder: str, replacement: str):
    """Replace placeholder with replacement in file."""
    if not filepath.exists():
        raise RuntimeError(f"Warning: {filepath} not found")

    content = filepath.read_text()
    new_content = content.replace(placeholder, replacement)
    filepath.write_text(new_content)
    print(f"Updated: {filepath}")
    return True


def main():
    parser = argparse.ArgumentParser(description="Configure FIPS installation paths")
    parser.add_argument("--destdir", required=True, help="Destination directory")
    parser.add_argument("--embedded_ssl_dir", required=False, help="Embedded SSL directory (defaults to destdir/ssl)")
    args = parser.parse_args()

    destdir = Path(args.destdir)
    embedded_ssl_dir = args.embedded_ssl_dir if args.embedded_ssl_dir else str(destdir / "ssl")

    openssl_cnf_tmp = destdir / "ssl" / "openssl.cnf.tmp"
    fipsinstall_sh = destdir / "bin" / "fipsinstall.sh"

    # Replace {{embedded_ssl_dir}} in openssl.cnf.tmp
    replace_in_file(openssl_cnf_tmp, "{{embedded_ssl_dir}}", embedded_ssl_dir)

    # Replace {{install_dir}} in fipsinstall.sh (Linux only)
    if os.name != "nt" and fipsinstall_sh.exists():
        replace_in_file(fipsinstall_sh, "{{install_dir}}", str(destdir))

    print("Configuration complete.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
