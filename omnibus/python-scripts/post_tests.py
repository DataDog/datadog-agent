import unittest
import os
import tempfile
from post import post

class TestPost(unittest.TestCase):
    def test_post(self):
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        
        result = post(install_directory, storage_location)

        # assert it ran with no errors 
        self.assertEqual(result, 0)

        # confirm it made .post_python_installed_packages.txt
        post_file = os.path.join(storage_location, ".post_python_installed_packages.txt")
        self.assertTrue(os.path.exists(post_file))
        with open(post_file, 'r', encoding='utf-8') as f:
            content = f.read()
            self.assertIn("# DO NOT REMOVE/MODIFY", content)
            self.assertIn("invoke", content)
        
        # Cleanup
        os.remove(post_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)

    def test_post_with_empty_files(self):
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

        result = post(install_directory, storage_location, skip_flag=False)

        # assert it ran with no errors 
        self.assertEqual(result, 0)

        # confirm it made .post_python_installed_packages.txt
        self.assertTrue(os.path.exists(post_file))
        with open(post_file, 'r', encoding='utf-8') as f:
            content = f.read()
            self.assertIn("# DO NOT REMOVE/MODIFY", content)

        # Cleanup
        os.remove(post_file)
        os.remove(diff_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)