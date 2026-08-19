#!/usr/bin/env python3
"""Wraps macOS `pkgbuild` so it can be invoked as a Bazel action.

Bazel presents action inputs (including declare_directory outputs like
--root) as a symlink farm inside the sandbox, wrapping every payload entry in
a symlink pointing at its real, ephemeral sandbox/exec-root/cache location.
`pkgbuild` faithfully packages whatever it finds under --root, including
symlink-ness, so a naive copy either ships broken symlinks (plain
`cp -R`) or, if symlinks are blindly dereferenced (`cp -RL`), loses two things
pkg_install's NativeInstaller deliberately set: exact permission bits (a plain
`cp` applies umask to the new file's mode instead of copying the source mode)
and intentional payload symlinks (e.g. pkg_mklink entries), which get
flattened into duplicate regular files.

materialize_root.py resolves exactly the sandbox's own indirection layer
(dereferencing until it hits either real content or the payload's own
intended symlink) and preserves source file modes explicitly, so it is not
subject to the umask under which this action runs.
"""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile


def _install_script(src, dst):
    shutil.copyfile(src, dst)
    os.chmod(dst, 0o755)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--materialize-root-py", required=True, help="Path to materialize_root.py.")
    parser.add_argument("--root", required=True, help="pkg_install-materialized root directory (symlink farm).")
    parser.add_argument("--pkgbuild", required=True, help="Path to the pkgbuild binary.")
    parser.add_argument("--identifier", required=True, help="pkgbuild --identifier.")
    parser.add_argument("--version", required=True, help="pkgbuild --version.")
    parser.add_argument("--install-location", required=True, help="pkgbuild --install-location.")
    parser.add_argument("--output", required=True, help="Path to write the built .pkg to.")
    parser.add_argument("--preinstall", default="", help="Optional path to a preinstall script.")
    parser.add_argument("--postinstall", default="", help="Optional path to a postinstall script.")
    parser.add_argument("--signing-identity", default="", help="Optional pkgbuild --sign identity name.")

    args = parser.parse_args()

    real_root_dir = tempfile.mkdtemp()
    scripts_dir = tempfile.mkdtemp()
    try:
        subprocess.run(
            [sys.executable, args.materialize_root_py, args.root, real_root_dir],
            check=True,
        )

        # fmt: off
        pkgbuild_args = [
            args.pkgbuild,
            "--root", real_root_dir,
            "--identifier", args.identifier,
            "--version", args.version,
            "--install-location", args.install_location,
        ]

        if args.preinstall or args.postinstall:
            if args.preinstall:
                _install_script(args.preinstall, os.path.join(scripts_dir, "preinstall"))
            if args.postinstall:
                _install_script(args.postinstall, os.path.join(scripts_dir, "postinstall"))
            pkgbuild_args += ["--scripts", scripts_dir]

        if args.signing_identity:
            pkgbuild_args += ["--sign", args.signing_identity]

        os.makedirs(os.path.dirname(args.output), exist_ok=True)
        pkgbuild_args.append(args.output)
        subprocess.run(pkgbuild_args, check=True)
    finally:
        shutil.rmtree(real_root_dir, ignore_errors=True)
        shutil.rmtree(scripts_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
