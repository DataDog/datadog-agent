"""Smoke test to make sure python works."""

import platform
import sys
import unittest

from invoke import Exit


class TestCase(unittest.TestCase):
    def test_do_we_have_expected_python(self):
        print("Python version:", sys.version)
        print("os:", sys.platform, ", arch:", platform.processor)
        self.assertTrue(sys.version.startswith("3.12"))

    def test_basic_python_import(self):
        e = Exit()
        self.assertEqual(0, e.code)


if __name__ == "__main__":
    unittest.main()
