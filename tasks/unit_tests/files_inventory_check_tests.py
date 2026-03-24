import unittest

from tasks.files_inventory import _compare_inventories
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


if __name__ == '__main__':
    unittest.main()
