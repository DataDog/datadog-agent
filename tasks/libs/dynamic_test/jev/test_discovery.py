"""E2E test discovery for the Jev e2e tooling: suite entry points, their
file source and the suite provisioning definitions."""

from __future__ import annotations

import os
import re
from pathlib import Path

from tasks.libs.dynamic_test.jev.pr_context import truncate

MAX_TEST_CODE_BYTES = 32_000
MAX_SUITE_DEFINITION_BYTES = 8_000

# E2E suites live under this directory in the datadog-agent repo
E2E_TESTS_DIR = "test/new-e2e/tests"


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
            code = Path(suite_pkg, f).read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        code = strip_license_header(code)
        chunks.append((f, code))
    if not chunks:
        return "", ""
    full = "\n\n".join(f"// --- {f} ---\n{code}" for f, code in chunks)
    return suite_pkg, truncate(full, MAX_SUITE_DEFINITION_BYTES, "suite definition")


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
            if not f.endswith("_test.go"):
                continue
            path = os.path.join(root, f)
            code = Path(path).read_text(encoding="utf-8", errors="ignore")
            code = strip_license_header(code)
            entries = re.findall(r"^func (Test\w+)\(\w+ \*testing\.T\)", code, re.MULTILINE)
            for entry in entries:
                suites.append((entry, path, code))
    return sorted(suites)
