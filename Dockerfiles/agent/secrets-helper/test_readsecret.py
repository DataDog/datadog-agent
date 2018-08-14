#!/usr/bin/env python

import argparse
import shutil
import tempfile
import unittest

from readsecret import *


class TestListSecretNames(unittest.TestCase):
    def test_invalid_output(self):
        with self.assertRaisesRegexp(ValueError, "No JSON object could be decoded"):
            list_secret_names("")

    def test_invalid_version(self):
        with self.assertRaisesRegexp(ValueError, "unknown protocol version 2.0"):
            list_secret_names('{"version": "2.0"}')

    def test_not_list(self):
        with self.assertRaisesRegexp(ValueError, "should be an array"):
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
        with self.assertRaisesRegexp(ValueError, "outside of the specified folder"):
            read_file(self.folder, "a/../../outside/file")

    def test_file_not_found(self):
        with self.assertRaisesRegexp(IOError, "No such file or directory"):
            read_file(self.folder, "file/not/found")

    def test_file_ok(self):
        filename = "ok_file"
        with open(os.path.join(self.folder, filename), "w") as f:
            f.write("ok_contents")
        contents = read_file(self.folder, filename)
        self.assertEqual(contents, "ok_contents")


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
        with self.assertRaisesRegexp(argparse.ArgumentTypeError, "does not exist"):
            is_valid_folder(foldername)


if __name__ == '__main__':
    unittest.main()
