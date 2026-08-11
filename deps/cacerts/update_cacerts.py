#!/usr/bin/env python3
"""Check/update deps/cacerts/cacerts.MODULE.bazel against curl.se's CA extract page.

See AGENTS.md in this directory for the full procedure.

Usage:
    python3 update_cacerts.py            # dry run, prints what would change
    python3 update_cacerts.py --write    # rewrite cacerts.MODULE.bazel in place
"""

import os
import re
import sys
import urllib.request
from pathlib import Path

CAEXTRACT_URL = "https://curl.se/docs/caextract.html"
SHA256_URL = "https://curl.se/ca/cacert-{version}.pem.sha256"

# Under `bazel run`, __file__ resolves inside the sandboxed runfiles tree, not
# the real source tree, so writes there wouldn't reach the repo. Bazel sets
# BUILD_WORKSPACE_DIRECTORY to the actual workspace root for `bazel run`.
_workspace_dir = os.environ.get("BUILD_WORKSPACE_DIRECTORY")
_base_dir = Path(_workspace_dir) / "deps" / "cacerts" if _workspace_dir else Path(__file__).parent
MODULE_FILE = _base_dir / "cacerts.MODULE.bazel"


def _fetch(url):
    with urllib.request.urlopen(url) as resp:
        return resp.read().decode("utf-8")


def _latest_version():
    html = _fetch(CAEXTRACT_URL)
    dates = re.findall(r"cacert-(\d{4}-\d{2}-\d{2})\.pem\b", html)
    if not dates:
        sys.exit("could not find any cacert-YYYY-MM-DD.pem references on caextract page")
    return max(dates)


def _sha256_for(version):
    line = _fetch(SHA256_URL.format(version=version))
    return line.split()[0]


def _current_version():
    text = MODULE_FILE.read_text()
    match = re.search(r'version\s*=\s*"([^"]+)"', text)
    if not match:
        sys.exit(f"could not find version= in {MODULE_FILE}")
    return match.group(1)


def main():
    write = "--write" in sys.argv

    current = _current_version()
    latest = _latest_version()
    print(f"current version: {current}")
    print(f"latest version:  {latest}")

    if latest <= current:
        print("already up to date")
        return

    sha = _sha256_for(latest)
    print(f"new sha256: {sha}")

    if not write:
        print("dry run, pass --write to update cacerts.MODULE.bazel")
        return

    text = MODULE_FILE.read_text()
    text = re.sub(r'version\s*=\s*"[^"]+"', f'version = "{latest}"', text, count=1)
    text = re.sub(r'sha\s*=\s*"[^"]+"', f'sha = "{sha}"', text, count=1)
    MODULE_FILE.write_text(text)
    print(f"updated {MODULE_FILE}")


if __name__ == "__main__":
    main()
