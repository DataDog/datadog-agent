import unittest
from unittest.mock import MagicMock

from tasks.libs.common.git import check_local_branch, check_uncommitted_changes, get_current_branch, get_staged_files


class TestGit(unittest.TestCase):
    def setUp(self) -> None:
        super().setUp()
        self.ctx_mock = MagicMock()

    def test_get_staged_files(self):
        self.ctx_mock.run.return_value.stdout = "file1\nfile2\nfile3"
        files = list(get_staged_files(self.ctx_mock, include_deleted_files=True))

        self.assertEqual(files, ["file1", "file2", "file3"])
        self.ctx_mock.run.assert_called_once_with("git diff --name-only --staged HEAD", hide=True)

    @unittest.mock.patch("os.path.isfile", side_effect=[True, False, True])
    def test_get_staged_files_without_deleted_files(self, _):
        self.ctx_mock.run.return_value.stdout = "file1\nfile2\nfile3"
        files = list(get_staged_files(self.ctx_mock))

        self.assertEqual(files, ["file1", "file3"])
        self.ctx_mock.run.assert_called_once_with("git diff --name-only --staged HEAD", hide=True)

    def test_get_current_branch(self):
        self.ctx_mock.run.return_value.stdout = "  main  \n"
        branch = get_current_branch(self.ctx_mock)

        self.assertEqual(branch, "main")
        self.ctx_mock.run.assert_called_once_with("git rev-parse --abbrev-ref HEAD", hide=True)

    def test_check_uncommitted_changes(self):
        tests = [
            {
                "stdout": "  12  \n",
                "expected": True,
            },
            {
                "stdout": "  0  \n",
                "expected": False,
            },
        ]

        for test in tests:
            with self.subTest(expected=test["expected"]):
                self.ctx_mock.run.return_value.stdout = test["stdout"]
                res = check_uncommitted_changes(self.ctx_mock)

                self.assertEqual(res, test["expected"])
                self.ctx_mock.run.assert_called_once_with("git --no-pager diff --name-only HEAD | wc -l", hide=True)
                self.ctx_mock.run.reset_mock()

    def test_check_local_branch(self):
        tests = [
            {
                "branch": "main",
                "stdout": "  1  \n",
                "expected": True,
            },
            {
                "branch": "doesnotexist",
                "stdout": "  0  \n",
                "expected": False,
            },
        ]

        for test in tests:
            with self.subTest(branch=test["branch"]):
                self.ctx_mock.run.return_value.stdout = test["stdout"]
                res = check_local_branch(self.ctx_mock, test["branch"])

                self.assertEqual(res, test["expected"])
                self.ctx_mock.run.assert_called_once_with(
                    f"git --no-pager branch --list {test['branch']} | wc -l", hide=True
                )
                self.ctx_mock.run.reset_mock()
