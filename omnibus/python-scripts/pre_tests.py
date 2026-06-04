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
        # Simulates an OCI upgrade where the baseline was written by an older Agent
        # version to the legacy /opt/datadog-packages/tmp location and the new
        # storage location (run dir) does not yet contain it.
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
        # Simulates a deb/rpm -> OCI migration where the baseline only exists in the
        # install directory and neither the primary nor the legacy location has it.
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

    def test_pre_no_baseline_anywhere_returns_zero(self):
        # When no baseline exists in any location (e.g. the reaper removed it before the
        # persistent storage move, or this is a fresh install), pre must be a non-fatal
        # no-op: return 0 and create nothing.
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
