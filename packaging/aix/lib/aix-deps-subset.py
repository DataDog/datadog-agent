#!/usr/bin/env python3
"""Compute the AIX dependency subset for the bundled Python checks.

Usage: aix-deps-subset.py <integrations-core> <agent_requirements.in> <constraints.txt> <output-file>

AIX has no lockfile (no prebuilt wheels exist), so the subset is installed
directly from agent_requirements.in: the union of every AIX-tagged check's
[deps] extra, pinned to the same canonical pins the other platforms compile
into their lockfiles.

Deps whose native extension Stage 06 did not build (absent from the freeze)
are skipped so the install doesn't fail on a source build that cannot
succeed; the check surfaces an ImportError at runtime instead.
"""

import json
import os
import re
import sys
import tomllib

# Native extensions Stage 06 builds conditionally on host prerequisites.
NATIVE_OPTIONAL = {"pyodbc"}


def pkg_name(spec):
    """Extract the lowercased package name from a requirement spec line.

    Input examples — everything from the first version operator, extras
    bracket, or environment marker onward is dropped:
      'requests==2.34.2'                          -> 'requests'
      'psycopg[c,pool]==3.3.4'                    -> 'psycopg'
      'pywin32==312; sys_platform == "win32"'     -> 'pywin32'
    """
    return re.split(r"[<>=!;\[]", spec.strip(), maxsplit=1)[0].strip().lower()


def aix_check_deps(integrations_core):
    """Names of every package declared in an AIX-tagged check's [deps] extra."""
    needed = set()
    for name in sorted(os.listdir(integrations_core)):
        pyproject = os.path.join(integrations_core, name, "pyproject.toml")
        manifest = os.path.join(integrations_core, name, "manifest.json")
        if not (os.path.isfile(pyproject) and os.path.isfile(manifest)):
            continue
        with open(manifest) as f:
            if "Supported OS::AIX" not in json.load(f).get("tile", {}).get("classifier_tags", []):
                continue
        with open(pyproject, "rb") as f:
            t = tomllib.load(f)
        for d in t.get("project", {}).get("optional-dependencies", {}).get("deps", []):
            needed.add(pkg_name(d))
    return needed


def frozen_names(constraints_file):
    """Package names recorded in Stage 08's freeze of the installed state."""
    installed = set()
    with open(constraints_file) as f:
        for line in f:
            n = pkg_name(line)
            if n:
                installed.add(n)
    return installed


def main():
    if len(sys.argv) != 5:
        print(
            f"usage: {sys.argv[0]} <integrations-core> <agent_requirements.in> <constraints.txt> <output-file>",
            file=sys.stderr,
        )
        return 2
    integrations_core, req_in, constraints_file, out_file = sys.argv[1:5]

    # 1. Deps declared by every AIX-tagged check's [deps] extra.
    needed = aix_check_deps(integrations_core)

    # 2. Drop native deps Stage 06 did not build (absent from the freeze).
    installed = frozen_names(constraints_file)
    for n in sorted(needed):
        if n in NATIVE_OPTIONAL and n not in installed:
            print(f"# skipped (native, not built): {n}", file=sys.stderr)
            needed.discard(n)

    # 3. Emit the matching pinned lines. pip evaluates environment markers,
    #    so platform-specific lines are skipped automatically.
    missing = set(needed)
    with open(req_in) as f, open(out_file, "w") as w:
        for line in f:
            if pkg_name(line) in needed:
                w.write(line)
                missing.discard(pkg_name(line))
    for n in sorted(missing):
        print(f"# WARNING: {n} needed by an AIX check but not in agent_requirements.in", file=sys.stderr)


if __name__ == "__main__":
    sys.exit(main())
