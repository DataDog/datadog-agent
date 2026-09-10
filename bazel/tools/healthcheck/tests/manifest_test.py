"""Unit tests for Manifest.resolve()'s symlink-chain following."""

import unittest

from manifest import Manifest


class ManifestResolveTest(unittest.TestCase):
    def test_direct_file_hit(self):
        m = Manifest(files={"embedded/lib/libfoo.so": "/src/libfoo.so"}, symlinks={})
        self.assertEqual(m.resolve("embedded/lib/libfoo.so"), "embedded/lib/libfoo.so")

    def test_missing_entry(self):
        m = Manifest(files={}, symlinks={})
        self.assertIsNone(m.resolve("embedded/lib/libfoo.so"))

    def test_single_hop_symlink(self):
        m = Manifest(
            files={"embedded/lib/libfoo.so.1.2.3": "/src/libfoo.so.1.2.3"},
            symlinks={"embedded/lib/libfoo.so": "libfoo.so.1.2.3"},
        )
        self.assertEqual(m.resolve("embedded/lib/libfoo.so"), "embedded/lib/libfoo.so.1.2.3")

    def test_multi_hop_symlink_chain(self):
        m = Manifest(
            files={"embedded/lib/libfoo.so.1.2.3": "/src/libfoo.so.1.2.3"},
            symlinks={
                "embedded/lib/libfoo.so": "libfoo.so.1",
                "embedded/lib/libfoo.so.1": "libfoo.so.1.2.3",
            },
        )
        self.assertEqual(m.resolve("embedded/lib/libfoo.so"), "embedded/lib/libfoo.so.1.2.3")

    def test_dangling_symlink(self):
        m = Manifest(files={}, symlinks={"embedded/lib/libfoo.so": "libfoo.so.1"})
        self.assertIsNone(m.resolve("embedded/lib/libfoo.so"))

    def test_symlink_loop(self):
        m = Manifest(
            files={},
            symlinks={
                "embedded/lib/a.so": "b.so",
                "embedded/lib/b.so": "a.so",
            },
        )
        self.assertIsNone(m.resolve("embedded/lib/a.so"))

    def test_absolute_symlink_target(self):
        m = Manifest(
            files={"opt/other/libfoo.so": "/src/libfoo.so"},
            symlinks={"embedded/lib/libfoo.so": "/opt/other/libfoo.so"},
        )
        self.assertEqual(m.resolve("embedded/lib/libfoo.so"), "opt/other/libfoo.so")


if __name__ == "__main__":
    unittest.main()
