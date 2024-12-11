import unittest

from invoke.context import Context
from invoke.exceptions import Exit

from tasks.git import check_protected_branch


class TestGit(unittest.TestCase):
    def setUp(self) -> None:
        super().setUp()
        self.ctx = Context()

    @unittest.mock.patch('tasks.git.get_current_branch')
    def test_check_protected_branch(self, get_current_branch_mock):
        get_current_branch_mock.return_value = 'my/branch'
        check_protected_branch(self.ctx)

    @unittest.mock.patch('tasks.git.get_current_branch')
    def test_check_protected_branch_error(self, get_current_branch_mock):
        protected_branches = (
            'main',
            '7.54.x',
            '6.54.x',
        )

        for branch_name in protected_branches:
            with self.subTest(branch=branch_name):
                get_current_branch_mock.return_value = branch_name
                self.assertRaises(Exit, check_protected_branch, self.ctx)
