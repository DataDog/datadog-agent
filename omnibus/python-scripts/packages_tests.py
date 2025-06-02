import unittest
from packages import extract_version, create_python_installed_packages_file, create_diff_installed_packages_file, check_file_owner_system_windows
import packaging.requirements
import os
import tempfile

class TestPackages(unittest.TestCase):

    def test_extract_version(self):
        req = packaging.requirements.Requirement("package==1.0.0")
        expected_version = "1.0.0"
        
        result = extract_version(req)
        
        self.assertEqual(result, expected_version)

    def test_create_python_installed_packages_file(self):
        # create temp directory
        test_directory = tempfile.mkdtemp()
        test_filename = os.path.join(test_directory, "test_installed_packages.txt")
        os.makedirs(test_directory, exist_ok=True)
        
        create_python_installed_packages_file(test_filename)
        
        self.assertTrue(os.path.exists(test_filename))
        
        with open(test_filename, 'r', encoding='utf-8') as f:
            content = f.read()
            self.assertIn("# DO NOT REMOVE/MODIFY", content)
            self.assertIn("invoke", content)

        
        # Cleanup
        os.remove(test_filename)
        
        # running rmdir verifies that the directory is empty
        os.rmdir(test_directory)

    def test_create_diff_installed_packages_file(self):
        test_directory = tempfile.mkdtemp()
        old_file = os.path.join(test_directory, "old_installed_packages.txt")
        new_file = os.path.join(test_directory, "new_installed_packages.txt")
        diff_file = os.path.join(test_directory, ".diff_python_installed_packages.txt")

        with open(old_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("package==1.0.0\n")

        with open(new_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("package==1.0.0\n")
            f.write("newpackage==2.0.0\n")

        create_diff_installed_packages_file(test_directory, old_file, new_file)

        self.assertTrue(os.path.exists(diff_file))
        
        with open(diff_file, 'r', encoding='utf-8') as f:
            content = f.read()
            self.assertIn("# DO NOT REMOVE/MODIFY", content)
            self.assertIn("newpackage==2.0.0", content)

        # Cleanup
        os.remove(old_file)
        os.remove(new_file)
        os.remove(diff_file)
        
        # running rmdir verifies that the directory is empty
        # asserts no extra files are created
        os.rmdir(test_directory)
