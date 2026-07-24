"""Writes a version-manifest style JSON file for a built package.

This approximates the omnibus `<artifact>.metadata.json` sidecar
(lib/omnibus/metadata.rb + lib/omnibus/manifest.rb in the legacy omnibus-ruby
gem): artifact facts (basename, sha256, sha512), project facts (name, version,
license), and a nested "version_manifest" listing every package whose license
we gathered.

This is a first pass at capturing the overall structure; it does not yet
match the legacy field-for-field (e.g. per-package locked_version/source_type
are not tracked here).
"""

import argparse
import hashlib
import json


def _hash_file(path, algo):
    digest = hashlib.new(algo)
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package", required=True, help="Path to the built .deb/.rpm/.tar file.")
    parser.add_argument("--licenses", required=True, help="Path to the license manifest JSON (from gather_licenses.py).")
    parser.add_argument("--out", required=True, help="Path to write the version manifest JSON to.")
    parser.add_argument("--name", required=True)
    parser.add_argument("--friendly_name", default="")
    parser.add_argument("--homepage", default="")
    parser.add_argument("--version", required=True)
    parser.add_argument("--build_git_revision", default="")
    parser.add_argument("--license", default="")
    parser.add_argument("--arch", default="")
    parser.add_argument("--license_file", default="", help="Path to the project's own top-level LICENSE text.")
    options = parser.parse_args()

    with open(options.licenses, encoding="UTF-8") as f:
        license_entries = json.load(f)

    software = {}
    for entry in license_entries:
        software[entry["name"]] = {
            "license": entry["license"],
            "origin": entry["origin"],
            "copyright": entry["copyright"],
        }

    license_content = ""
    if options.license_file:
        with open(options.license_file, encoding="UTF-8", errors="ignore") as f:
            license_content = f.read()

    data = {
        "basename": options.package.split("/")[-1],
        "sha256": _hash_file(options.package, "sha256"),
        "sha512": _hash_file(options.package, "sha512"),
        "arch": options.arch,
        "name": options.name,
        "friendly_name": options.friendly_name,
        "homepage": options.homepage,
        "version": options.version,
        "license": options.license,
        "version_manifest": {
            "manifest_format": 2,
            "software": software,
            "build_version": options.version,
            "build_git_revision": options.build_git_revision or None,
            "license": options.license,
        },
        "license_content": license_content,
    }

    with open(options.out, "w", encoding="UTF-8") as f:
        json.dump(data, f, indent=2)


if __name__ == "__main__":
    main()
