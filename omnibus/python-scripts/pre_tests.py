import unittest
import os
import tempfile
from unittest import mock
import pre as pre_module
from pre import pre

class TestPre(unittest.TestCase):
    def test_pre(self):
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        post_file = os.path.join(storage_location, '.post_python_installed_packages.txt')
        
        with open(post_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("package==1.0.0\n")
        
        result = pre(install_directory, storage_location)

        # assert it ran with no errors 
        self.assertEqual(result, 0)
        self.assertFalse(os.path.exists(post_file))

        # assert that the diff file was created
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        self.assertTrue(os.path.exists(diff_file))
        with open(diff_file, 'r', encoding='utf-8') as f:
            content = f.read()
            self.assertIn("# DO NOT REMOVE/MODIFY", content)
            self.assertIn("invoke", content)

        # Cleanup
        os.remove(diff_file)
        
        # running rmdir verifies that the directory is empty
        # asserts no extra files are created
        os.rmdir(install_directory)
        os.rmdir(storage_location)

    def test_pre_with_empty_files(self):
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        post_file = os.path.join(storage_location, '.post_python_installed_packages.txt')
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')

        # Create empty post file
        with open(post_file, 'w', encoding='utf-8') as f:
            pass

        # Create empty diff file
        with open(diff_file, 'w', encoding='utf-8') as f:
            pass

        result = pre(install_directory, storage_location)

        # assert it ran with no errors 
        self.assertEqual(result, 0)
        self.assertFalse(os.path.exists(post_file))

        # assert that the diff file was created
        self.assertTrue(os.path.exists(diff_file))
        with open(diff_file, 'r', encoding='utf-8') as f:
            content = f.read()
            self.assertIn("# DO NOT REMOVE/MODIFY", content)

        # Cleanup
        os.remove(diff_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)

    @unittest.skipIf(os.name == 'nt', "legacy OCI tmp fallback is Linux-only")
    def test_pre_falls_back_to_legacy_tmp(self):
        # OCI upgrade: baseline only in the legacy tmp dir, not yet in the run dir.
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        legacy_location = tempfile.mkdtemp()
        legacy_post_file = os.path.join(legacy_location, '.post_python_installed_packages.txt')

        with open(legacy_post_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("package==1.0.0\n")

        with mock.patch.object(pre_module, 'LEGACY_OCI_STORAGE_LOCATION', legacy_location):
            result = pre(install_directory, storage_location)

        # assert it ran with no errors and consumed the legacy baseline
        self.assertEqual(result, 0)
        self.assertFalse(os.path.exists(legacy_post_file))

        # assert that the diff file was created in the (new) storage location
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        self.assertTrue(os.path.exists(diff_file))

        # Cleanup
        os.remove(diff_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)
        os.rmdir(legacy_location)

    def test_pre_falls_back_to_install_directory(self):
        # deb/rpm -> OCI migration: baseline only in the install dir.
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        legacy_location = tempfile.mkdtemp()
        install_post_file = os.path.join(install_directory, '.post_python_installed_packages.txt')

        with open(install_post_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("package==1.0.0\n")

        with mock.patch.object(pre_module, 'LEGACY_OCI_STORAGE_LOCATION', legacy_location):
            result = pre(install_directory, storage_location)

        self.assertEqual(result, 0)
        self.assertFalse(os.path.exists(install_post_file))

        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        self.assertTrue(os.path.exists(diff_file))

        # Cleanup
        os.remove(diff_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)
        os.rmdir(legacy_location)

    def test_pre_no_baseline_first_install_returns_zero(self):
        # First install: no baseline and no .installed_by_pkg.txt marker -> non-fatal no-op.
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        legacy_location = tempfile.mkdtemp()

        with mock.patch.object(pre_module, 'LEGACY_OCI_STORAGE_LOCATION', legacy_location):
            result = pre(install_directory, storage_location)

        # graceful no-op
        self.assertEqual(result, 0)

        # nothing should have been written
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        pre_file = os.path.join(storage_location, '.pre_python_installed_packages.txt')
        self.assertFalse(os.path.exists(diff_file))
        self.assertFalse(os.path.exists(pre_file))

        # Cleanup (rmdir verifies the directories are empty)
        os.rmdir(install_directory)
        os.rmdir(storage_location)
        os.rmdir(legacy_location)

    def test_pre_no_baseline_on_upgrade_returns_error(self):
        # Upgrade with a lost baseline: no baseline but .installed_by_pkg.txt marker present
        # -> return 1 to surface the reaping bug (caller treats it as non-fatal).
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        legacy_location = tempfile.mkdtemp()

        embedded_dir = os.path.join(install_directory, 'embedded')
        os.makedirs(embedded_dir)
        installed_by_pkg_file = os.path.join(embedded_dir, '.installed_by_pkg.txt')
        with open(installed_by_pkg_file, 'w', encoding='utf-8') as f:
            f.write("embedded/lib/python3.12/site-packages/datadog_checks_base\n")

        with mock.patch.object(pre_module, 'LEGACY_OCI_STORAGE_LOCATION', legacy_location):
            result = pre(install_directory, storage_location)

        # error surfaced (non-fatal at the caller)
        self.assertEqual(result, 1)

        # nothing should have been written
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        pre_file = os.path.join(storage_location, '.pre_python_installed_packages.txt')
        self.assertFalse(os.path.exists(diff_file))
        self.assertFalse(os.path.exists(pre_file))

        # Cleanup
        os.remove(installed_by_pkg_file)
        os.rmdir(embedded_dir)
        os.rmdir(install_directory)
        os.rmdir(storage_location)
        os.rmdir(legacy_location)

    def test_pre_with_populated_pre_file(self):
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        pre_file = os.path.join(storage_location, '.pre_python_installed_packages.txt')
        post_file = os.path.join(storage_location, '.post_python_installed_packages.txt')
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')

        # Create empty post file
        with open(post_file, 'w', encoding='utf-8') as f:
            pass

        # Create populated pre file
        with open(pre_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("package==1.0.0\n")

        result = pre(install_directory, storage_location)

        # assert it ran with no errors 
        self.assertEqual(result, 0)
        self.assertFalse(os.path.exists(post_file))
        self.assertTrue(os.path.exists(diff_file))

        # Cleanup
        os.remove(diff_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)
