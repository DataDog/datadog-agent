"""Unit tests for Manifest.resolve()'s symlink-chain following."""

import json
import os
import tempfile
import unittest

from manifest import Manifest, load


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


class ManifestLoadTest(unittest.TestCase):
    def test_directory_symlink_is_recorded_as_symlink(self):
        with tempfile.TemporaryDirectory() as src_dir, tempfile.TemporaryDirectory() as manifest_dir:
            real_path = os.path.join(src_dir, "libfoo.so.1")
            open(real_path, "w").close()
            os.symlink("libfoo.so.1", os.path.join(src_dir, "libfoo.so"))

            manifest_path = os.path.join(manifest_dir, "manifest.json")
            with open(manifest_path, "w", encoding="utf-8") as f:
                json.dump({"directories": {"embedded/lib": src_dir}}, f)

            m = load(manifest_path)

            self.assertEqual(m.symlinks.get("embedded/lib/libfoo.so"), "libfoo.so.1")
            self.assertNotIn("embedded/lib/libfoo.so", m.files)
            self.assertIn("embedded/lib/libfoo.so.1", m.files)
            self.assertEqual(m.resolve("embedded/lib/libfoo.so"), "embedded/lib/libfoo.so.1")

    def test_directory_dangling_symlink_fails_to_resolve(self):
        with tempfile.TemporaryDirectory() as src_dir, tempfile.TemporaryDirectory() as manifest_dir:
            os.symlink("libfoo.so.1", os.path.join(src_dir, "libfoo.so"))

            manifest_path = os.path.join(manifest_dir, "manifest.json")
            with open(manifest_path, "w", encoding="utf-8") as f:
                json.dump({"directories": {"embedded/lib": src_dir}}, f)

            m = load(manifest_path)

            self.assertIsNone(m.resolve("embedded/lib/libfoo.so"))


if __name__ == "__main__":
    unittest.main()
