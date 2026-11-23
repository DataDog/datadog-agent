import os
import tempfile
import unittest
from dataclasses import asdict
from unittest.mock import MagicMock, patch

import yaml

from tasks.files_inventory import _compare_inventories, _compare_inventory_reports, check
from tasks.static_quality_gates.experimental_gates import (
    FileChange,
    FileInfo,
)


class TestCompareInventory(unittest.TestCase):
    default_inventory = [
        FileInfo(relative_path='/a/b/c', size_bytes=1024, is_symlink=False, chmod=0o755, owner=1000, group=1000),
        FileInfo(relative_path='/a/b/d', size_bytes=1024, is_symlink=False, chmod=0o755, owner=1000, group=1000),
        FileInfo(relative_path='/a/b/e', size_bytes=1024, is_symlink=False, chmod=0o755, owner=1000, group=1000),
        FileInfo(relative_path='/a/b/f', size_bytes=1024, is_symlink=False, chmod=0o755, owner=1000, group=1000),
    ]

    def test_compare_identical(self):
        added, removed, changed = _compare_inventories(self.default_inventory, self.default_inventory)
        self.assertEqual(len(added), 0)
        self.assertEqual(len(removed), 0)
        self.assertEqual(len(changed), 0)

    def test_compare_added(self):
        new_file = FileInfo(
            relative_path='/a/b/g', size_bytes=1024, is_symlink=False, chmod=0o755, owner=1000, group=1000
        )
        added_inventory = self.default_inventory + [new_file]
        added, removed, changed = _compare_inventories(self.default_inventory, added_inventory)
        self.assertEqual(len(added), 1)
        self.assertEqual(len(removed), 0)
        self.assertEqual(len(changed), 0)
        self.assertEqual(added[0], new_file)

    def test_compare_removed(self):
        removed_inventory = self.default_inventory.copy()
        removed_file = removed_inventory.pop()
        added, removed, changed = _compare_inventories(self.default_inventory, removed_inventory)
        self.assertEqual(len(added), 0)
        self.assertEqual(len(removed), 1)
        self.assertEqual(len(changed), 0)
        self.assertEqual(removed[0], removed_file)

    def test_modified_chmod(self):
        changed_inventory = self.default_inventory.copy()
        changed_file = FileInfo(
            relative_path='/a/b/c', size_bytes=1024, is_symlink=False, chmod=0o744, owner=1000, group=1000
        )
        old_file = changed_inventory[0]
        changed_inventory[0] = changed_file
        added, removed, changed = _compare_inventories(self.default_inventory, changed_inventory)
        self.assertEqual(len(added), 0)
        self.assertEqual(len(removed), 0)
        self.assertEqual(len(changed), 1)
        self.assertTrue('/a/b/c' in changed)
        self.assertTrue(changed['/a/b/c'].flags & FileChange.Flags.Permissions)
        self.assertFalse(changed['/a/b/c'].flags & ~FileChange.Flags.Permissions)
        self.assertEqual(changed['/a/b/c'].current, changed_file)
        self.assertEqual(changed['/a/b/c'].previous, old_file)

    def test_empty_file_growing_is_a_size_change(self):
        empty = FileInfo(relative_path='/a/empty', size_bytes=0, is_symlink=False, chmod=0o755, owner=1000, group=1000)
        grown = FileInfo(relative_path='/a/empty', size_bytes=10, is_symlink=False, chmod=0o755, owner=1000, group=1000)
        _, _, changed = _compare_inventories([empty], [grown])
        self.assertTrue(changed['/a/empty'].flags & FileChange.Flags.Size)

    def test_modified_filesize(self):
        changed_inventory = self.default_inventory.copy()
        changed_file_ignored = FileInfo(
            relative_path='/a/b/c', size_bytes=1025, is_symlink=False, chmod=0o755, owner=1000, group=1000
        )
        changed_file_detected = FileInfo(
            relative_path='/a/b/d', size_bytes=2048, is_symlink=False, chmod=0o755, owner=1000, group=1000
        )
        changed_inventory[0] = changed_file_ignored
        changed_inventory[1] = changed_file_detected
        added, removed, changed = _compare_inventories(self.default_inventory, changed_inventory)
        self.assertEqual(len(added), 0)
        self.assertEqual(len(removed), 0)
        self.assertEqual(len(changed), 1)
        self.assertTrue('/a/b/d' in changed)
        self.assertTrue(changed['/a/b/d'].flags & FileChange.Flags.Size)
        self.assertFalse(changed['/a/b/d'].flags & ~FileChange.Flags.Size)
        self.assertEqual(changed['/a/b/d'].current, changed_file_detected)
        self.assertEqual(changed['/a/b/d'].size_percent, 100)

    def test_modified_filesize_decrease(self):
        changed_inventory = self.default_inventory.copy()
        changed_file_ignored = FileInfo(
            relative_path='/a/b/c', size_bytes=1000, is_symlink=False, chmod=0o755, owner=1000, group=1000
        )
        changed_file_detected = FileInfo(
            relative_path='/a/b/d', size_bytes=512, is_symlink=False, chmod=0o755, owner=1000, group=1000
        )
        changed_inventory[0] = changed_file_ignored
        changed_inventory[1] = changed_file_detected
        added, removed, changed = _compare_inventories(self.default_inventory, changed_inventory)
        self.assertEqual(len(added), 0)
        self.assertEqual(len(removed), 0)
        self.assertEqual(len(changed), 1)
        self.assertTrue(changed['/a/b/d'].flags & FileChange.Flags.Size)
        self.assertEqual(changed['/a/b/d'].size_percent, -50)

    def test_modified_owner_groups(self):
        changed_inventory = self.default_inventory.copy()
        changed_file = FileInfo(
            relative_path='/a/b/c', size_bytes=1024, is_symlink=False, chmod=0o755, owner=13, group=12
        )
        old_file = changed_inventory[0]
        changed_inventory[0] = changed_file
        added, removed, changed = _compare_inventories(self.default_inventory, changed_inventory)
        self.assertEqual(len(added), 0)
        self.assertEqual(len(removed), 0)
        self.assertEqual(len(changed), 1)
        self.assertTrue('/a/b/c' in changed)
        self.assertTrue(changed['/a/b/c'].flags & FileChange.Flags.Owner)
        self.assertTrue(changed['/a/b/c'].flags & FileChange.Flags.Group)
        self.assertFalse(changed['/a/b/c'].flags & ~(FileChange.Flags.Owner | FileChange.Flags.Group))
        self.assertEqual(changed['/a/b/c'].current, changed_file)
        self.assertEqual(changed['/a/b/c'].previous, old_file)


class TestCompareInventoryReports(unittest.TestCase):
    inventory = TestCompareInventory.default_inventory

    def _write_report(self, tmpdir, name, inventory):
        path = os.path.join(tmpdir, name)
        with open(path, 'w') as f:
            yaml.safe_dump({'file_inventory': [asdict(info) for info in inventory]}, f)
        return path

    def test_no_change(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            parent = self._write_report(tmpdir, 'parent.yml', self.inventory)
            current = self._write_report(tmpdir, 'current.yml', self.inventory)
            has_changes, body = _compare_inventory_reports(parent, current)
        self.assertFalse(has_changes)
        self.assertEqual(body, "No change detected\n\n")

    def test_change(self):
        new_file = FileInfo(
            relative_path='/a/b/g', size_bytes=1024, is_symlink=False, chmod=0o755, owner=1000, group=1000
        )
        with tempfile.TemporaryDirectory() as tmpdir:
            parent = self._write_report(tmpdir, 'parent.yml', self.inventory)
            current = self._write_report(tmpdir, 'current.yml', self.inventory + [new_file])
            has_changes, body = _compare_inventory_reports(parent, current)
        self.assertTrue(has_changes)
        self.assertIn('1 Added files', body)
        self.assertIn('/a/b/g', body)


@patch.dict('os.environ', {'CI_COMMIT_SHORT_SHA': 'abcdef12', 'CI_JOB_NAME': 'files_inventory_check'})
@patch('tasks.files_inventory._get_parent_report', new=MagicMock())
@patch('tasks.files_inventory._measure_package_local', new=MagicMock())
@patch('tasks.files_inventory.get_ancestor', new=MagicMock(return_value='0123456789abcdef'))
class TestCheck(unittest.TestCase):
    def setUp(self):
        self.reports_folder = tempfile.TemporaryDirectory()
        self.addCleanup(self.reports_folder.cleanup)
        for package in ['datadog-agent_7.0.0-1_amd64.deb', 'datadog-iot-agent_7.0.0-1_amd64.deb']:
            open(os.path.join(self.reports_folder.name, package), 'w').close()
        self.pr = MagicMock()
        github = MagicMock()
        github.get_pr_for_branch.return_value = [self.pr]
        patcher = patch('tasks.files_inventory.GithubAPI', return_value=github)
        patcher.start()
        self.addCleanup(patcher.stop)

    def _run_check(self, comparison_results):
        with (
            patch('tasks.files_inventory._compare_inventory_reports', side_effect=comparison_results),
            patch('tasks.files_inventory.pr_commenter') as mock_pr_commenter,
        ):
            check.body(MagicMock(), 'my-branch', self.reports_folder.name)
        return mock_pr_commenter

    def test_no_change_deletes_comment(self):
        mock_pr_commenter = self._run_check([(False, "No change detected\n\n"), (False, "No change detected\n\n")])
        mock_pr_commenter.assert_called_once()
        kwargs = mock_pr_commenter.call_args.kwargs
        self.assertEqual(kwargs['title'], 'Files inventory check summary')
        self.assertIs(kwargs['pr'], self.pr)
        self.assertTrue(kwargs['delete'])
        self.assertTrue(kwargs['force_delete'])
        self.assertNotIn('body', kwargs)

    def test_change_posts_comment(self):
        mock_pr_commenter = self._run_check([(False, "No change detected\n\n"), (True, "## Detected file changes:\n")])
        mock_pr_commenter.assert_called_once()
        kwargs = mock_pr_commenter.call_args.kwargs
        self.assertEqual(kwargs['title'], 'Files inventory check summary')
        self.assertIs(kwargs['pr'], self.pr)
        self.assertNotIn('delete', kwargs)
        self.assertIn('No change detected', kwargs['body'])
        self.assertIn('## Detected file changes:', kwargs['body'])


if __name__ == '__main__':
    unittest.main()
