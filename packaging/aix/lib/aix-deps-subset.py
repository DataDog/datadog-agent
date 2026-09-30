#!/usr/bin/env python3
"""Build the AIX dependency subset for the bundled Python checks.

Usage: aix-deps-subset.py <integrations-core> <agent_requirements.in> <constraints.txt> <output-file>

Dependency versions come from integrations-core's agent_requirements.in — the
same pinned input file the Linux/macOS/Windows lockfile flow (resolve-build-deps)
compiles into the per-platform .deps/resolved/*.txt wheel sets. AIX has no such
lockfile (no prebuilt AIX wheels exist), so instead of compiling one we install
the AIX-relevant subset of agent_requirements.in directly: the union of every
AIX-tagged check's [deps] extra, looked up in agent_requirements.in for the
canonical pin. Native deps Stage 06 already built and installed (pymqi, lxml,
psutil, cryptography) are seen as satisfied and not rebuilt; only missing
pure-Python deps (e.g. http_check's pysocks/requests-ntlm) are fetched from PyPI.

Native C-extension deps that Stage 06 did NOT build (e.g. pyodbc when unixODBC
headers are absent) are filtered out so the dep install does not fail on a
source build that cannot succeed. The check still installs; it surfaces a clear
ImportError at runtime if the missing extension is needed, matching the
graceful-degradation behavior for the IBM checks.

Output: the matching pinned lines from agent_requirements.in, written to
<output-file>. Skipped native deps and deps missing from agent_requirements.in
are reported on stderr.
"""

import json
import os
import re
import sys
import tomllib

# Native C extensions Stage 06 builds conditionally on host prerequisites.
# If absent from the frozen constraints (i.e. not built), skip them rather
# than fail the install with a source build that cannot succeed.
NATIVE_OPTIONAL = {"pyodbc"}


def pkg_name(spec):
    """Extract the package name from a requirement spec line."""
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

    # 1. Collect the dep package names declared by every AIX-tagged check's
    #    [deps] extra. These are the only deps the AIX package needs.
    needed = aix_check_deps(integrations_core)

    # 2. Drop native deps Stage 06 did not build (absent from the freeze).
    installed = frozen_names(constraints_file)
    for n in sorted(needed):
        if n in NATIVE_OPTIONAL and n not in installed:
            print(f"# skipped (native, not built): {n}", file=sys.stderr)
            needed.discard(n)

    # 3. Emit the matching lines from agent_requirements.in (canonical pins).
    #    pip evaluates each line's environment markers, so win32/darwin-marked
    #    lines that slipped into `needed` are skipped on AIX automatically.
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
