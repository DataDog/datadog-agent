import unittest
import os
import tempfile
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

    def test_pre_with_orphaned_integration(self):
        # Simulate a partial pip install: datadog_checks/myplugin/ exists on disk but
        # no dist-info was written, so importlib.metadata cannot see the package.
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()

        # Create a fake site-packages tree with an orphaned integration dir
        site_packages = tempfile.mkdtemp()
        checks_dir = os.path.join(site_packages, 'datadog_checks')
        os.makedirs(os.path.join(checks_dir, 'myplugin'))

        post_file = os.path.join(storage_location, '.post_python_installed_packages.txt')
        with open(post_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            # myplugin is absent from the post-file (was never successfully installed)

        result = pre(install_directory, storage_location,
                     _orphan_site_packages_dir=site_packages)

        self.assertEqual(result, 0)

        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        self.assertTrue(os.path.exists(diff_file))
        with open(diff_file, 'r', encoding='utf-8') as f:
            content = f.read()
        self.assertIn('datadog-myplugin', content)

        # Cleanup
        os.remove(diff_file)
        import shutil
        shutil.rmtree(site_packages)
        os.rmdir(install_directory)
        os.rmdir(storage_location)

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
