"""Unit tests for the macOS @rpath/@loader_path substitution and allowlist logic.

check_entry()'s own logic (reading LC_LOAD_DYLIB/LC_RPATH via otool,
resolving against a manifest) needs a real macOS environment with otool
available to exercise meaningfully; this sandbox has no macOS toolchain, so
that part is not covered by an automated test here.
"""

import unittest

from macos import _is_allowed, _resolve_paths


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
