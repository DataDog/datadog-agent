import unittest
from unittest.mock import MagicMock

from invoke import MockContext, Result

from tasks.libs.common.git import (
    check_local_branch,
    check_uncommitted_changes,
    get_commit_sha,
    get_current_branch,
    get_last_release_tag,
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
                'git rev-parse --abbrev-ref HEAD': Result("6.53.x"),
                'git ls-remote -t https://github.com/DataDog/woof "6.56.*"': Result(
                    """e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/6.56.0-rc.1
                       7c6777bb7add533a789c69293b59e3261711d330	refs/tags/6.56.0-rc.2
                       2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/6.56.0-rc.3"""
                ),
            }
        )
        _, name = get_last_release_tag(c, "woof", "6.56.*")
        self.assertEqual(name, "6.56.0-rc.3")

    def test_non_ordered(self):
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -t https://github.com/DataDog/woof "7.56.*"': Result(
                    """e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1
                       7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.11
                       2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"""
                ),
            }
        )
        _, name = get_last_release_tag(c, "woof", "7.56.*")
        self.assertEqual(name, "7.56.0-rc.11")

    def test_suffix_lower(self):
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -t https://github.com/DataDog/woof "7.56.*"': Result(
                    """e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1
                       7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.2^{}
                       2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"""
                ),
            }
        )
        _, name = get_last_release_tag(c, "woof", "7.56.*")
        self.assertEqual(name, "7.56.0-rc.3")

    def test_suffix_equal(self):
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -t https://github.com/DataDog/woof "7.56.*"': Result(
                    """e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1
                       7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.3^{}
                       2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"""
                ),
            }
        )
        commit, _ = get_last_release_tag(c, "woof", "7.56.*")
        self.assertEqual(commit, "7c6777bb7add533a789c69293b59e3261711d330")

    def test_suffix_greater(self):
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -t https://github.com/DataDog/woof "7.56.*"': Result(
                    """e1b8e9163203b7446c74fac0b8d4153eb24227a0	refs/tags/7.56.0-rc.1
                       7c6777bb7add533a789c69293b59e3261711d330	refs/tags/7.56.0-rc.4^{}
                       2b8b710b322feb03148f871a77ab92163a0a12de	refs/tags/7.56.0-rc.3"""
                ),
            }
        )
        _, name = get_last_release_tag(c, "woof", "7.56.*")
        self.assertEqual(name, "7.56.0-rc.4")

    def test_only_release_tags(self):
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -t https://github.com/DataDog/woof "7.57.*"': Result(
                    """"43638bd55a74fd6ec51264cc7b3b1003d0b1c7ac    refs/tags/7.57.0-dbm-mongo-1.5
                        e01bcf3d12e6d6742b1fa8296882938c6dba9922    refs/tags/7.57.0-devel
                        6a5ad7fda590c7b8ba7036bca70dc8a0872e7afe    refs/tags/7.57.0-devel^{}
                        2c2eb2293cccd33100d7d930a59c136319942915    refs/tags/7.57.0-installer-0.5.0-rc.1
                        2c2eb2293cccd33100d7d930a59c136319942915    refs/tags/7.57.0-installer-0.5.0-rc.2
                        6a91fcca0ade9f77f08cd98d923a8d9ec18d7e8f    refs/tags/7.57.0-installer-0.5.0-rc.3
                        7e8ffc3de15f0486e6cb2184fa59f02da6ecfab9    refs/tags/7.57.0-rc.1
                        fa72fd12e3483a2d5957ea71fe01a8b1af376424    refs/tags/7.57.0-rc.1^{}
                        22587b746d6a0876cb7477b9b335e8573bdc3ac5    refs/tags/7.57.0-rc.2
                        d6c151a36487c3b54145ae9bf200f6c356bb9348    refs/tags/7.57.0-rc.2^{}
                        948ed4dd8c8cdf0aae467997086bb2229d4f1916    refs/tags/7.57.0-rc.3
                        259ed086a45960006e110622332cc8a39f9c6bb9    refs/tags/7.57.0-rc.3^{}
                        a249f4607e5da894715a3e011dba8046b46678ed    refs/tags/7.57.0-rc.4
                        51a3b405a244348aec711d38e5810a6d88075b77    refs/tags/7.57.0-rc.4^{}
                        06519be707d6f24fb8265cde5a50cf0a66d5cb02    refs/tags/7.57.0-rc.5
                        7f43a5180446290f498742e68d8b28a75da04188    refs/tags/7.57.0-rc.5^{}
                        6bb640559e7626131290c63dab3959ba806c9886    refs/tags/7.57.0-rc.6
                        c5ed1f8b4734d31e94c2a83f307dbcb2b5a1faac    refs/tags/7.57.0-rc.6^{}
                        260697e624bb1d92ad306fdc301aab9b2975a627    refs/tags/7.57.0-rc.7
                        48617a0f56747e33b75d3dcf570bc2237726dc0e    refs/tags/7.57.0-rc.7^{}
                        5e11e104ff99b40b01ff2cfa702c0e4a465f98de    refs/tags/7.57.1-beta-ndm-rdns-enrichment
                        91c7c85d7c8fbb94421a90b273aea75630617eef    refs/tags/7.57.1-beta-ndm-rdns-enrichment^{}
                        3ad359da2894fa3de6e265c56dea8fabdb128454    refs/tags/7.57.1-beta-ndm-rdns-enrichment2
                        86683ad80578912014cc947dcf247ba020532403    refs/tags/7.57.1-beta-ndm-rdns-enrichment2^{}"""
                ),
            }
        )
        _, name = get_last_release_tag(c, "woof", "7.57.*")
        self.assertEqual(name, "7.57.0-rc.7")

    def test_final_and_rc_tag_on_same_commit(self):
        c = MockContext(
            run={
                'git rev-parse --abbrev-ref HEAD': Result("main"),
                'git ls-remote -t https://github.com/DataDog/baubau "7.61.*"': Result(
                    """8dd145cf716b5c047e81bb287dc58e150b8c2b94	refs/tags/7.61.0
                       45f19a6a26c01dae9fdfce944d3fceae7f4e6498	refs/tags/7.61.0^{}
                       1cfbd72c75d6fcfe920707b2d08764ee89ec8793	refs/tags/7.61.0-rc.1
                       52fd18ccf4391ed5da0647dad2c1fdeea8a8a70c	refs/tags/7.61.0-rc.1^{}
                       3b7310d32b0ad4d347fa64f60a02261caf910a99	refs/tags/7.61.0-rc.4
                       3944948c0c26ddcbc4026b98c2709c188d95b702	refs/tags/7.61.0-rc.4^{}
                       c54e5d5694879c51ae5ff8675dacc92976630587	refs/tags/7.61.0-rc.5
                       45f19a6a26c01dae9fdfce944d3fceae7f4e6498	refs/tags/7.61.0-rc.5^{}"""
                ),
            }
        )

        commit, _ = get_last_release_tag(c, "baubau", "7.61.*")
        self.assertEqual(commit, "45f19a6a26c01dae9fdfce944d3fceae7f4e6498")
