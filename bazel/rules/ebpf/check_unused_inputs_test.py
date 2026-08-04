# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

import tempfile
import unittest
from pathlib import Path

from bazel.rules.ebpf.check_unused_inputs import main


class CheckUnusedInputsTest(unittest.TestCase):
    def run_check(self, unused: str, allowed: list[str]) -> tuple[int, Path]:
        directory = Path(tempfile.mkdtemp())
        listing = directory / "program.unused_inputs"
        listing.write_text(unused, encoding="utf-8")
        marker = directory / "program.unused_check"
        argv = [
            "--unused-inputs-list",
            str(listing),
            "--marker",
            str(marker),
            "--label",
            "//pkg/example:program",
        ]
        for entry in allowed:
            argv += ["--allowed", entry]
        return main(argv), marker

    def test_passes_when_only_kernel_headers_are_unused(self) -> None:
        code, marker = self.run_check("external/linux_headers/linux/bpf.h\n", [])
        self.assertEqual(code, 0)
        self.assertTrue(marker.exists())

    def test_fails_on_unused_repo_header(self) -> None:
        code, marker = self.run_check("pkg/ebpf/c/unused.h\n", [])
        self.assertEqual(code, 1)
        self.assertFalse(marker.exists())

    def test_allows_listed_header(self) -> None:
        code, _ = self.run_check("pkg/ebpf/c/conditional.h\n", ["pkg/ebpf/c/conditional.h"])
        self.assertEqual(code, 0)

    def test_fails_on_stale_allowance(self) -> None:
        code, _ = self.run_check("", ["pkg/ebpf/c/now_used.h"])
        self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()
