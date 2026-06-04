from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit

from tasks.renovate import REPO_ROOT, refresh_archive_hashes_impl


def main() -> None:
    parser = argparse.ArgumentParser(description="Refresh sha256 values for changed Bazel native deps")
    parser.add_argument(
        "--base-ref",
        default="origin/main",
        help="Git ref to compare against to detect changed http_archive/http_file calls",
    )
    args = parser.parse_args()

    workspace = os.environ.get("BUILD_WORKSPACE_DIRECTORY")
    root = Path(workspace) if workspace else REPO_ROOT
    try:
        refresh_archive_hashes_impl(Context(), root, args.base_ref)
    except Exit as exc:
        if exc.message:
            print(exc.message, file=sys.stderr)
        sys.exit(exc.code)


if __name__ == "__main__":
    main()
