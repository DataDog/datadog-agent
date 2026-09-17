"""Unit tests for the Linux RPATH-to-manifest-directory conversion.

The rest of linux.py's logic (reading DT_NEEDED/RPATH via readelf, resolving
against a manifest, rejecting $ORIGIN) needs real ELF files to exercise
meaningfully; that's covered by the integration test in
broken_install_test.py against real compiled fixtures.
"""

import unittest

from linux import _strip_install_prefix


class StripInstallPrefixTest(unittest.TestCase):
    def test_exact_match(self):
        self.assertEqual(_strip_install_prefix("/opt/datadog-agent", "/opt/datadog-agent"), "")

    def test_subdirectory(self):
        self.assertEqual(
            _strip_install_prefix("/opt/datadog-agent/embedded/lib", "/opt/datadog-agent"),
            "embedded/lib",
        )

    def test_unrelated_directory(self):
        self.assertIsNone(_strip_install_prefix("/usr/lib/x86_64-linux-gnu", "/opt/datadog-agent"))

    def test_sibling_dir_with_shared_prefix_is_not_matched(self):
        # /opt/datadog-agent-other shares a string prefix with the install
        # prefix but is a different directory; it must not be treated as "inside".
        self.assertIsNone(_strip_install_prefix("/opt/datadog-agent-other/lib", "/opt/datadog-agent"))

    def test_trailing_slash_on_prefix_is_tolerated(self):
        self.assertEqual(
            _strip_install_prefix("/opt/datadog-agent/embedded/lib", "/opt/datadog-agent/"),
            "embedded/lib",
        )


if __name__ == "__main__":
    unittest.main()
