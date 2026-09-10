"""Shared manifest format for the healthcheck.

Lists every file/symlink that will end up in the install tree: "files" maps
a destination path to the real source file that will be installed there;
"directories" does the same for a whole directory artifact, walked here at
load time since its contents aren't knowable at Starlark analysis time;
"symlinks" records a symlink and its literal target, exactly as it will
exist once installed.

JSON format: {"files": {dest: src_path}, "directories": {dest: src_dir},
"symlinks": {dest: target}}. Produced by pkg_manifest/manifest.bzl's
write_manifest().
"""

import json
import os


class Manifest:
    def __init__(self, files: dict[str, str], symlinks: dict[str, str]):
        self.files = files
        self.symlinks = symlinks

    def resolve(self, dest_path: str) -> str | None:
        """Follow a chain of symlinks starting at dest_path.

        Returns the dest path of the terminal file if the chain ends at a
        real manifest entry, or None if it's broken (dangling symlink, or
        dest_path isn't in the manifest at all).
        """
        seen: set[str] = set()
        path = dest_path
        while path in self.symlinks:
            if path in seen:
                return None
            seen.add(path)
            target = self.symlinks[path]
            if target.startswith("/"):
                path = target.lstrip("/")
            else:
                path = os.path.normpath(os.path.join(os.path.dirname(path), target))
        return path if path in self.files else None


def load(manifest_path: str) -> Manifest:
    with open(manifest_path, encoding="utf-8") as f:
        data = json.load(f)

    files: dict[str, str] = {}
    for dest, src_path in data.get("files", {}).items():
        files[dest.strip("/")] = src_path

    for dest, src_dir in data.get("directories", {}).items():
        dest = dest.strip("/")
        for root, _, names in os.walk(src_dir):
            for name in names:
                full = os.path.join(root, name)
                rel = os.path.relpath(full, src_dir)
                files[os.path.join(dest, rel)] = full

    symlinks: dict[str, str] = {dest.strip("/"): target for dest, target in data.get("symlinks", {}).items()}

    return Manifest(files, symlinks)
