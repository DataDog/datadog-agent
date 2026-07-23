import os
import tempfile
import unittest
import unittest.mock
from pathlib import Path

from invoke import Context, MockContext

from tasks.macos import remove_inactive_versions


class TestRemoveInactiveVersions(unittest.TestCase):
    @unittest.mock.patch('builtins.print')
    @unittest.mock.patch('tasks.macos.list_runner_active_versions')
    @unittest.mock.patch('tasks.macos.list_ci_active_versions')
    def test_remove_specific(self, ci_mock, runner_mock, print_mock):
        ci_mock.return_value = {'3.12', '3.11'}
        runner_mock.return_value = {'3.12.1', '3.13.42', '3.11.3', '3.11.6', '3.9.7', '2.7.18'}

        # 3.13 should not be removed
        remove_inactive_versions(MockContext(), 'python', '3.13.42', dry_run=True)

        # The order is non deterministic
        print_mock.assert_any_call('Removing python version 3.9.7')
        print_mock.assert_any_call('Removing python version 2.7.18')
        self.assertEqual(print_mock.call_count, 2)

    @unittest.mock.patch('builtins.print')
    @unittest.mock.patch('tasks.macos.list_runner_active_versions')
    @unittest.mock.patch('tasks.macos.list_ci_active_versions')
    def test_remove_global(self, ci_mock, runner_mock, print_mock):
        ci_mock.return_value = {'3.12', '3.11'}
        runner_mock.return_value = {'3.12.1', '3.13.42', '3.11.3', '3.11.6', '3.9.7', '2.7.18'}

        # 3.13 should not be removed
        remove_inactive_versions(MockContext(), 'python', '3.13', dry_run=True)

        # The order is non deterministic
        print_mock.assert_any_call('Removing python version 3.9.7')
        print_mock.assert_any_call('Removing python version 2.7.18')
        self.assertEqual(print_mock.call_count, 2)

    @unittest.mock.patch('builtins.print')
    @unittest.mock.patch('tasks.macos.list_runner_active_versions')
    @unittest.mock.patch('tasks.macos.list_ci_active_versions')
    def test_remove_no_target(self, ci_mock, runner_mock, print_mock):
        ci_mock.return_value = {'3.12', '3.11'}
        runner_mock.return_value = {'3.12.1', '3.13.42', '3.11.3', '3.11.6', '3.9.7', '2.7.18'}

        # 3.13 should be removed
        remove_inactive_versions(MockContext(), 'python', dry_run=True)

        # The order is non deterministic
        print_mock.assert_any_call('Removing python version 3.9.7')
        print_mock.assert_any_call('Removing python version 2.7.18')
        print_mock.assert_any_call('Removing python version 3.13.42')
        self.assertEqual(print_mock.call_count, 3)

    @unittest.mock.patch("tasks.macos.list_runner_active_versions")
    @unittest.mock.patch("tasks.macos.list_ci_active_versions")
    def test_remove_go_deletes_only_inactive_version(self, ci_active_versions, runner_active_versions):
        ci_active_versions.return_value = {"1.26"}  # 1.25 is inactive
        runner_active_versions.return_value = {"1.25.9", "1.26.0", "1.26.5"}

        with tempfile.TemporaryDirectory() as home:
            versions_dir = Path(home, ".gimme", "versions")
            for arch_dir in ("go1.25.9.darwin.amd64", "go1.26.0.darwin.amd64", "go1.26.5.darwin.amd64"):
                (versions_dir / arch_dir).mkdir(parents=True)

            with unittest.mock.patch.dict(os.environ, {"HOME": home}):
                remove_inactive_versions(Context(), "go")

            remaining = {p.name for p in versions_dir.iterdir()}

        self.assertEqual(remaining, {"go1.26.0.darwin.amd64", "go1.26.5.darwin.amd64"})
