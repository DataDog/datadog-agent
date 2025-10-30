"""Smoke test to make sure python works."""

import platform
import sys
import unittest

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
            self.assertTrue(sys.version.startswith(expected))

    def test_basic_python_import(self):
        e = Exit()
        self.assertEqual(0, e.code)


if __name__ == "__main__":
    unittest.main()
