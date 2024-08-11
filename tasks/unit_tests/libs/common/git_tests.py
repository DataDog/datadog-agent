import unittest
from unittest.mock import MagicMock

from invoke import MockContext, Result

from tasks.libs.common.git import (
    check_local_branch,
    check_uncommitted_changes,
    get_commit_sha,
    get_current_branch,
    get_last_tag,
    get_staged_files,
)


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

    def test_get_commit_sha(self):
        tests = [
            {
                "short": False,
                "stdout": "  1cb775ac4873a09738b31521815c6c3a6f59f451  \n",
                "expected": "1cb775ac4873a09738b31521815c6c3a6f59f451",
            },
            {
                "short": True,
                "stdout": "  0b87e9a50f  \n",
                "expected": "0b87e9a50f",
            },
        ]

        for test in tests:
            with self.subTest(short=test["short"]):
                self.ctx_mock.run.return_value.stdout = test["stdout"]
                sha = get_commit_sha(self.ctx_mock, short=test["short"])

                self.assertEqual(sha, test["expected"])
                self.ctx_mock.run.assert_called_once_with(
                    f"git rev-parse {'--short ' if test['short'] else ''}HEAD", hide=True
                )
                self.ctx_mock.run.reset_mock()


class TestGetLastTag(unittest.TestCase):
    def test_ordered(self):
        c = MockContext(
            run={
                'git ls-remote --sort=creatordate -t https://github.com/DataDog/woof "7.56.*"': Result(
                    "e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1\n7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.2\n2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"
                )
            }
        )
        _, name = get_last_tag(c, "woof", "7.56.*")
        self.assertEqual(name, "7.56.0-rc.3")

    def test_non_ordered(self):
        c = MockContext(
            run={
                'git ls-remote --sort=creatordate -t https://github.com/DataDog/woof "7.56.*"': Result(
                    "e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1\n7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.11\n2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"
                )
            }
        )
        _, name = get_last_tag(c, "woof", "7.56.*")
        self.assertEqual(name, "7.56.0-rc.11")

    def test_suffix_lower(self):
        c = MockContext(
            run={
                'git ls-remote --sort=creatordate -t https://github.com/DataDog/woof "7.56.*"': Result(
                    "e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1\n7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.2^{}\n2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"
                )
            }
        )
        _, name = get_last_tag(c, "woof", "7.56.*")
        self.assertEqual(name, "7.56.0-rc.3")

    def test_suffix_equal(self):
        c = MockContext(
            run={
                'git ls-remote --sort=creatordate -t https://github.com/DataDog/woof "7.56.*"': Result(
                    "e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1\n7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.3^{}\n2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"
                )
            }
        )
        commit, _ = get_last_tag(c, "woof", "7.56.*")
        self.assertEqual(commit, "7c6777bb7add533a789c69293b59e3261711d330")

    def test_suffix_greater(self):
        c = MockContext(
            run={
                'git ls-remote --sort=creatordate -t https://github.com/DataDog/woof "7.56.*"': Result(
                    "e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1\n7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.4^{}\n2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"
                )
            }
        )
        _, name = get_last_tag(c, "woof", "7.56.*")
        self.assertEqual(name, "7.56.0-rc.4")
