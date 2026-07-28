#!/usr/bin/env python3
"""Copies a pkg_install-materialized tree out of Bazel's sandbox symlink farm.

Usage: materialize_root.py <src_dir> <dst_dir>

Bazel presents a declare_directory() action input as a symlink farm: every
entry is (transitively) a symlink to its real, ephemeral location under the
sandbox/exec-root/cache. This walks that farm and, for each entry, resolves
symlinks one hop at a time until it finds either real content (a regular
file/directory) or another symlink -- the latter case means the payload
itself (via pkg_install's NativeInstaller) intentionally created that
symlink, e.g. for a pkg_mklink() entry, so its target text is preserved
verbatim rather than being flattened into a duplicate file.

File/directory modes are copied explicitly via shutil.copymode()/copystat()
so the result isn't affected by this process's umask.
"""

import os
import shutil
import sys


def _resolve_one_hop(path):
    """If path is a symlink, returns (resolved_path, target_is_symlink)."""
    if not os.path.islink(path):
        return path, False
    target = os.readlink(path)
    resolved = (
        target
        if os.path.isabs(target)
        else os.path.normpath(
            os.path.join(os.path.dirname(path), target),
        )
    return resolved, os.path.islink(resolved)


def materialize(src, dst):
    real_src, is_intentional_symlink = _resolve_one_hop(src)
    if is_intentional_symlink:
        os.symlink(os.readlink(real_src), dst)
        return

    if os.path.isdir(real_src):
        os.makedirs(dst, exist_ok=True)
        for entry in os.listdir(real_src):
            materialize(os.path.join(real_src, entry), os.path.join(dst, entry))
        shutil.copystat(real_src, dst)
    else:
        shutil.copyfile(real_src, dst)
        shutil.copymode(real_src, dst)


def main():
    src_dir, dst_dir = sys.argv[1], sys.argv[2]
    for entry in os.listdir(src_dir):
        materialize(os.path.join(src_dir, entry), os.path.join(dst_dir, entry))


if __name__ == "__main__":
    main()
