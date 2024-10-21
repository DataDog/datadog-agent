import os
import shutil
import tempfile
import unittest

from codeowners import CodeOwners

from tasks.libs.owners.linter import codeowner_has_orphans, directory_has_packages_without_owner


class TestCodeownerLinter(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()
        self.fake_pkgs = ["fake_a", "fake_b", "fake_c"]
        self.pkg_dir = os.path.join(self.test_dir, "pkg")
        self.backup_cwd = os.getcwd()

        # Create pkgs dir
        os.makedirs(self.pkg_dir)
        for pkg in self.fake_pkgs:
            os.makedirs(os.path.join(self.pkg_dir, pkg))

        os.chdir(self.test_dir)

    def tearDown(self):
        shutil.rmtree(self.test_dir)
        os.chdir(self.backup_cwd)

    def test_all_pkg_have_codeowner(self):
        codeowner = CodeOwners("\n".join("/pkg/" + pkg for pkg in self.fake_pkgs))
        self.assertFalse(directory_has_packages_without_owner(codeowner))
        self.assertFalse(codeowner_has_orphans(codeowner))

    def test_pkg_is_missing_codeowner(self):
        codeowner = CodeOwners("\n".join(os.path.join("/pkg/", pkg) for pkg in self.fake_pkgs[:-1]))
        self.assertTrue(directory_has_packages_without_owner(codeowner))
        self.assertFalse(codeowner_has_orphans(codeowner))

    def test_codeowner_rule_is_outdated(self):
        codeowner = CodeOwners("\n".join(os.path.join("/pkg/", pkg) for pkg in [*self.fake_pkgs, "old_deleted_pkg"]))
        self.assertFalse(directory_has_packages_without_owner(codeowner))
        self.assertTrue(codeowner_has_orphans(codeowner))
