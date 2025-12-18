"""Smoke test to make sure python works."""

import platform
import sys
import unittest
from pathlib import Path

from invoke import Exit
from python.runfiles import Runfiles


class TestCase(unittest.TestCase):
    def test_do_we_have_expected_python(self):
        """Are we using .python-version?"""
        print("Python version:", sys.version)
        print("os:", sys.platform, ", arch:", platform.processor())
        r = Runfiles.Create()
        data_path = r.Rlocation("_main/.python-version")
        with open(data_path) as f:
            expected = f.read().strip() + "."
            self.assertEqual(sys.version[: len(expected)], expected, sys.version)

    def test_python_comes_from_hermetic_toolchain(self):
        normalized_arch = {"AMD64": "x86_64", "arm64": "aarch64"}.get(platform.machine(), platform.machine())
        self.assertRegex(
            str(Path(sys.executable).resolve()),
            f"rules_python.+{normalized_arch}.+{platform.system().lower()}",
            "python must come from hermetic toolchain instead of host!",
        )

    def test_do_we_have_expected_python(self):
        """Are we using .python-version?"""
        expected_ver = Path(".python-version").read_text().strip().split(".")
        self.assertGreaterEqual(len(expected_ver), 2, ".python-version must at least define major.minor!")
        actual_ver = sys.version.split(".")[: len(expected_ver)]
        self.assertSequenceEqual(actual_ver, expected_ver, "toolchain must be in-sync with .python-version!")

    def test_basic_python_import(self):
        e = Exit()
        self.assertEqual(0, e.code)


if __name__ == "__main__":
    unittest.main()
