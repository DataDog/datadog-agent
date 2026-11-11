"""Unit tests for the macOS @rpath/@loader_path substitution and allowlist logic."""

import unittest
from unittest import mock

from macos import _get_install_name, _is_allowed, _resolve_paths


class GetInstallNameTest(unittest.TestCase):
    @mock.patch("macos._otool", return_value="_bisect.so:\n@rpath/_bisect.so\n")
    def test_so_suffix(self, _):
        self.assertEqual(_get_install_name("_bisect.so"), "@rpath/_bisect.so")

    @mock.patch("macos._otool", return_value="agent:\n")
    def test_no_install_name(self, _):
        self.assertIsNone(_get_install_name("agent"))


class ResolvePathsTest(unittest.TestCase):
    def test_plain_absolute_path_is_unchanged(self):
        self.assertEqual(
            _resolve_paths("/usr/lib/libSystem.B.dylib", "embedded/lib", []), ["/usr/lib/libSystem.B.dylib"]
        )

    def test_loader_path_substitution(self):
        self.assertEqual(
            _resolve_paths("@loader_path/libbar.dylib", "embedded/lib", []),
            ["embedded/lib/libbar.dylib"],
        )

    def test_rpath_substitution_across_all_candidates(self):
        rpaths = ["embedded/lib", "embedded/lib2"]
        self.assertEqual(
            _resolve_paths("@rpath/libbar.dylib", "embedded/lib", rpaths),
            ["embedded/lib/libbar.dylib", "embedded/lib2/libbar.dylib"],
        )

    def test_rpath_with_no_candidates_falls_back_to_literal(self):
        self.assertEqual(_resolve_paths("@rpath/libbar.dylib", "embedded/lib", []), ["@rpath/libbar.dylib"])

    def test_rpath_entry_itself_using_loader_path(self):
        self.assertEqual(
            _resolve_paths("@rpath/libbar.dylib", "embedded/lib", ["@loader_path/../lib"]),
            ["embedded/lib/../lib/libbar.dylib"],
        )


class IsAllowedTest(unittest.TestCase):
    def test_allowlisted_system_dylib(self):
        self.assertTrue(_is_allowed("libSystem.B.dylib", "embedded/lib/foo.dylib"))

    def test_non_allowlisted_dylib(self):
        self.assertFalse(_is_allowed("libxyz.dylib", "embedded/lib/foo.dylib"))


if __name__ == "__main__":
    unittest.main()
