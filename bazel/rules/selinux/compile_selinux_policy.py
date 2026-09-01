#!/usr/bin/env python3
"""Wraps `checkmodule`/`semodule_package` so they can be invoked as a Bazel action.

Mirrors `dda inv selinux.compile-system-probe-policy-file` (tasks/selinux.py):
compile the .te source into a .mod module, then package it into a .pp module.
"""

import argparse
import os
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--checkmodule", required=True, help="Path to the checkmodule binary.")
    parser.add_argument("--semodule-package", required=True, help="Path to the semodule_package binary.")
    parser.add_argument("--policy-version", required=True, help="checkmodule -c policy version.")
    parser.add_argument("--te-file", required=True, help="Path to the .te policy source.")
    parser.add_argument("--output", required=True, help="Path to write the packaged .pp module to.")

    args = parser.parse_args()

    with tempfile.TemporaryDirectory() as tmp_dir:
        mod_file = os.path.join(tmp_dir, "policy.mod")
        subprocess.run(
            [args.checkmodule, "-M", "-m", "-c", args.policy_version, "-o", mod_file, args.te_file],
            check=True,
        )
        subprocess.run(
            [args.semodule_package, "-o", args.output, "-m", mod_file],
            check=True,
        )


if __name__ == "__main__":
    main()
