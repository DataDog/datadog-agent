#!/usr/bin/env python

import argparse
import shutil
import tempfile
import unittest
import os

from readsecret import list_secret_names, read_file, is_valid_folder


class TestListSecretNames(unittest.TestCase):
    def test_invalid_output(self):
        with self.assertRaisesRegex(
            ValueError, r"Expecting value: line 1 column 1 \(char 0\)"
        ):
            list_secret_names("")

    def test_invalid_version(self):
        with self.assertRaisesRegex(ValueError, "incompatible protocol version 2.0"):
            list_secret_names('{"version": "2.0"}')

    def test_not_list(self):
        with self.assertRaisesRegex(ValueError, "should be an array"):
            list_secret_names('{"version": "1.0", "secrets": "one"}')

    def test_valid(self):
        names = list_secret_names('{"version": "1.0", "secrets": ["one", "two"]}')
        self.assertEqual(names, ["one", "two"])


class TestReadFile(unittest.TestCase):
    def setUp(self):
        self.folder = tempfile.mkdtemp(prefix="tmp-readsecret-test-")

    def tearDown(self):
        shutil.rmtree(self.folder, True)
        self.folder = None

    def test_path_escape(self):
        with self.assertRaisesRegex(ValueError, "outside of the specified folder"):
            read_file(self.folder, "a/../../outside/file")

    def test_path_escape_symlink(self):
        sensitive_path = os.path.join(self.folder, "sensitive")
        os.mkdir(sensitive_path)
        allowed_path = os.path.join(self.folder, "allowed")
        os.mkdir(allowed_path)

        # Create a sensitive file and symlink it in the allowed folder
        with open(os.path.join(sensitive_path, "target"), "w") as f:
            f.write("sensitive")
        os.symlink(
            os.path.join(sensitive_path, "target"),
            os.path.join(allowed_path, "target"),
        )

        with self.assertRaisesRegex(ValueError, "outside of the specified folder"):
            read_file(allowed_path, "target")

    def test_file_not_found(self):
        with self.assertRaisesRegex(IOError, "No such file or directory"):
            read_file(self.folder, "file/not/found")

    def test_file_ok(self):
        filename = "ok_file"
        with open(os.path.join(self.folder, filename), "w") as f:
            f.write("ok_contents")
        contents = read_file(self.folder, filename)
        self.assertEqual(contents, "ok_contents")

    def test_file_size_limit(self):
        filename = "big_file"
        with open(os.path.join(self.folder, filename), "w") as f:
            for _ in range(0, 2048):
                f.write("big")
        contents = read_file(self.folder, filename)
        self.assertEqual(len(contents), 1024)


class TestIsValidFolder(unittest.TestCase):
    def setUp(self):
        self.folder = tempfile.mkdtemp(prefix="tmp-readsecret-test-")

    def tearDown(self):
        shutil.rmtree(self.folder, True)
        self.folder = None

    def test_ok(self):
        self.assertEqual(self.folder, is_valid_folder(self.folder))

    def test_nok(self):
        foldername = os.path.join(self.folder, "not_found")
        with self.assertRaisesRegex(argparse.ArgumentTypeError, "does not exist"):
            is_valid_folder(foldername)


if __name__ == "__main__":
    unittest.main()
