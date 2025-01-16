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
        os.rmdir(install_directory)
        os.rmdir(storage_location)

if __name__ == '__main__':
    unittest.main()
