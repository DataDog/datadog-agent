import os
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from tasks.libs.common.utils import (
    link_or_copy,
    running_in_ci,
    running_in_github_actions,
    running_in_gitlab_ci,
    running_in_pre_commit,
    running_in_pyapp,
)


class TestRunningIn(unittest.TestCase):
    def test_running_in(self):
        parameters = [
            ("PRE_COMMIT", "1", True, running_in_pre_commit),
            ("PRE_COMMIT", "", False, running_in_pre_commit),
            ("PYAPP", "1", True, running_in_pyapp),
            ("PYAPP", "", False, running_in_pyapp),
            ("GITLAB_CI", "true", True, running_in_gitlab_ci),
            ("GITLAB_CI", "", False, running_in_gitlab_ci),
            ("GITHUB_ACTIONS", "true", True, running_in_github_actions),
            ("GITHUB_ACTIONS", "", False, running_in_github_actions),
            ("GITHUB_ACTIONS", "true", True, running_in_ci),
            ("GITLAB_CI", "true", True, running_in_ci),
            ("GITHUB_ACTIONS", "false", False, running_in_ci),
            ("GITLAB_CI", "false", False, running_in_ci),
        ]

        for env_var, value, expected, func in parameters:
            with self.subTest(env_var=env_var, value=value, expected_value=expected):
                with mock.patch.dict(os.environ, {env_var: value}, clear=True):
                    self.assertEqual(expected, func())


class TestLinkOrCopy(unittest.TestCase):
    def setUp(self):
        self._tmpdir = Path(tempfile.mkdtemp(prefix="test-link-or-copy"))
        self.src = self._tmpdir / "src.txt"
        self.dst = self._tmpdir / "dst.txt"
        self.src.write_text("new")

    def tearDown(self):
        shutil.rmtree(self._tmpdir)

    def test_links_or_copies(self):
        link_or_copy(self.src, self.dst)
        self.assertEqual(self.dst.read_text(), "new")

    def test_removes_old_dst(self):
        self.dst.write_text("old")
        link_or_copy(self.src, self.dst)
        self.assertEqual(self.dst.read_text(), "new")

    @mock.patch("tasks.libs.common.utils.shutil.copy2")
    @mock.patch.object(Path, "hardlink_to", side_effect=OSError)
    @mock.patch.object(Path, "symlink_to", side_effect=OSError)
    def test_tries_methods_in_order(self, symlink_to, hardlink_to, copy2):
        in_order = mock.Mock()
        in_order.attach_mock(symlink_to, "symlink_to")
        in_order.attach_mock(hardlink_to, "hardlink_to")
        in_order.attach_mock(copy2, "copy2")

        link_or_copy(self.src, self.dst)

        in_order.assert_has_calls(
            (
                mock.call.symlink_to(Path("src.txt")),
                mock.call.hardlink_to(self.src),
                mock.call.symlink_to(self.src),
                mock.call.copy2(self.src, self.dst),
            )
        )

    @mock.patch("tasks.libs.common.utils.shutil.copy2", side_effect=OSError)
    @mock.patch.object(Path, "hardlink_to", side_effect=NotImplementedError)
    @mock.patch.object(Path, "symlink_to", side_effect=NotImplementedError)
    def test_propagates_last_error(self, symlink_to, hardlink_to, copy2):
        with self.assertRaises(OSError):
            link_or_copy(self.src, self.dst)
