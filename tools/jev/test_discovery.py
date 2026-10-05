"""E2E test discovery for the Jev e2e tooling: suite entry points, their
file source and the suite provisioning definitions."""

from __future__ import annotations  # python 3.9 compat

import os
import re

from pr_context import truncate

MAX_TEST_CODE_BYTES = 32_000
MAX_SUITE_DEFINITION_BYTES = 8_000

# E2E suites live under this directory in the datadog-agent repo
E2E_TESTS_DIR = "test/new-e2e/tests"


def extract_function(code: str, func_header: str) -> str:
    """Extract a Go function body from `code`, from the header line to balanced braces."""
    lines = code.splitlines(keepends=True)
    start = None
    for i, line in enumerate(lines):
        if line.strip().startswith(func_header):
            start = i
            break
    if start is None:
        return func_header
    out, depth, opened = [], 0, False
    for line in lines[start:]:
        out.append(line)
        depth += line.count("{") - line.count("}")
        if "{" in line:
            opened = True
        if opened and depth <= 0:
            break
    return "".join(out)


def strip_license_header(code: str) -> str:
    """Drop the Apache license header comment from a Go file (noise for the model)."""
    m = re.search(r"^package ", code, re.MULTILINE)
    return code[m.start() :] if m else code


def suite_definition(suite_dir: str) -> tuple[str, str]:
    """Base suite definition of the e2e suite (the `suite/` subpackage).

    E2e test suites embed a base suite (e.g. `suite.FleetSuite`) defined in
    `test/new-e2e/tests/<suite>/suite/`: it declares what the suite provisions
    (platforms, VMs, agent components, install method, backend), which is the
    main signal for whether a PR can affect the suite. Returns (path, code).
    """
    suite_pkg = os.path.join(suite_dir, "suite")
    if not os.path.isdir(suite_pkg):
        return "", ""
    chunks = []
    for f in sorted(os.listdir(suite_pkg)):
        if not f.endswith(".go"):
            continue
        try:
            code = open(os.path.join(suite_pkg, f), encoding="utf-8", errors="ignore").read()
        except OSError:
            continue
        code = strip_license_header(code)
        chunks.append((f, code))
    if not chunks:
        return "", ""
    full = "\n\n".join(f"// --- {f} ---\n{code}" for f, code in chunks)
    return suite_pkg, truncate(full, MAX_SUITE_DEFINITION_BYTES, "suite definition")


def find_test_code(suite_dir: str, test_name: str) -> tuple[str, str]:
    """Return (file_path, source) for a test entry point or suite method."""
    for root, _, go_files in os.walk(suite_dir):
        for f in sorted(go_files):
            if not f.endswith(".go") or f.endswith("_test_helpers.go"):
                continue
            path = os.path.join(root, f)
            try:
                code = open(path, encoding="utf-8", errors="ignore").read()
            except OSError:
                continue
            for pattern in (rf"func {test_name}\(", rf"func \(s \*\w+\) {test_name}\("):
                m = re.search(pattern, code)
                if m:
                    header = m.group(0)
                    return path, extract_function(code, header)
    return "", test_name  # not found: fall back to the bare name


def list_suites(suite_dir: str) -> list:
    """All test entry points in the suite dir, with the full source of their file.

    Each e2e suite file defines one `func TestXxx(t *testing.T)` entry point that
    runs a suite of `func (s *...) TestYyy()` methods, so evaluating per entry
    point with the whole file gives Jev the complete test logic.
    Returns a list of (entry_point, file_path, file_code).
    """
    suites = []
    for root, _, go_files in os.walk(suite_dir):
        for f in sorted(go_files):
            if not f.endswith(".go"):
                continue
            path = os.path.join(root, f)
            code = open(path, encoding="utf-8", errors="ignore").read()
            code = strip_license_header(code)
            entries = re.findall(r"^func (Test\w+)\(", code, re.MULTILINE)
            for entry in entries:
                suites.append((entry, path, code))
    return sorted(suites)


# ---------------------------------------------------------------- Jev client
