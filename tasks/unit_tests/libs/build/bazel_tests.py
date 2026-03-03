import os
import unittest
from unittest.mock import patch

from tasks.libs.build.bazel import bazel
from tasks.libs.common.utils import get_repo_root


class TestBazel(unittest.TestCase):
    def test_bazel_call(self):
        self.assertEqual(bazel("info", "release"), 0)

    def test_bazel_output(self):
        expected_version = (get_repo_root() / ".bazelversion").read_text().strip()
        actual_output = bazel("info", "release", capture_output=True).strip()
        self.assertEqual(actual_output, f"release {expected_version}")

    @patch.dict(os.environ, {"PATH": os.devnull})
    def test_bazel_not_found(self):
        with self.assertRaises(SystemExit) as cm:
            bazel("info")
        self.assertIn("Please run `inv install-tools` for `bazel` support!", cm.exception.code)


if __name__ == "__main__":
    unittest.main()
