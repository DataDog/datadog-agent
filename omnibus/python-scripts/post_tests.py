import unittest
import os
import tempfile
from unittest.mock import patch, MagicMock
from post import post
import packages

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

        # running rmdir verifies that the directory is empty
        # asserts no extra files are created
        os.rmdir(install_directory)
        os.rmdir(storage_location)

    def test_post_with_empty_files(self):
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()
        post_file = os.path.join(storage_location, '.post_python_installed_packages.txt')
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        skip_flag_file = os.path.join(storage_location, '.skip_install_python_third_party_deps')

        # Create empty post file
        with open(post_file, 'w', encoding='utf-8') as f:
            pass

        # Create empty diff file
        with open(diff_file, 'w', encoding='utf-8') as f:
            pass

        # Create empty skip flag file
        with open(skip_flag_file, 'w', encoding='utf-8') as f:
            pass

        result = post(install_directory, storage_location)

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
        os.remove(skip_flag_file)

        # running rmdir verifies that the directory is empty
        os.rmdir(install_directory)
        os.rmdir(storage_location)

    @patch('packages.install_datadog_package')
    @patch('packages.install_dependency_package')
    def test_datadog_integration_vs_python_package_installation(self, mock_instalL_dependency, mock_install_datadog):
        """Test that packages are installed with correct methods based on datadog prefix and exclusion list"""
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()

        # Create necessary files
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        with open(diff_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY - used internally by installation process\n")
            f.write("datadog-nvml==1.0.0\n")
            f.write("datadog-api-client==2.40.0\n")

        req_file = os.path.join(install_directory, 'requirements-agent-release.txt')
        with open(req_file, 'w', encoding='utf-8') as f:
            f.write('')

        result = post(install_directory, storage_location)

        # Verify the result
        self.assertEqual(result, 0)

        mock_install_datadog.assert_called_once_with('datadog-nvml==1.0.0', install_directory)
        pip = [os.path.join(install_directory, "embedded", "bin", "pip")]
        mock_instalL_dependency.assert_called_once_with(pip, 'datadog-api-client==2.40.0')

        # Cleanup
        os.remove(diff_file)
        os.remove(req_file)
        post_file = os.path.join(storage_location, ".post_python_installed_packages.txt")
        if os.path.exists(post_file):
            os.remove(post_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)

    @patch('packages.install_datadog_package')
    @patch('packages.install_dependency_package')
    @patch('packages.load_requirements')
    def test_excluded_packages_are_skipped(self, mock_load_requirements, mock_instalL_dependency, mock_install_datadog):
        """Test that packages in exclude file are skipped"""
        install_directory = tempfile.mkdtemp()
        storage_location = tempfile.mkdtemp()

        # Mock diff file with packages to install
        diff_requirements = {
            'requests': ('requests==2.25.1', '2.25.1'),
            'datadog-custom': ('datadog-custom==1.0.0', '1.0.0')
        }

        # Mock exclude file with one package to exclude
        exclude_requirements = {
            'requests': ('requests==2.25.0', '2.25.0')
        }

        # Setup mock return values
        mock_load_requirements.side_effect = [diff_requirements, exclude_requirements]

        # Create necessary files
        diff_file = os.path.join(storage_location, '.diff_python_installed_packages.txt')
        with open(diff_file, 'w', encoding='utf-8') as f:
            f.write('')

        req_file = os.path.join(install_directory, 'requirements-agent-release.txt')
        with open(req_file, 'w', encoding='utf-8') as f:
            f.write('')

        result = post(install_directory, storage_location)

        # Verify the result
        self.assertEqual(result, 0)

        # Verify only datadog-custom was installed (requests was excluded)
        mock_install_datadog.assert_called_once_with('datadog-custom==1.0.0', install_directory)
        mock_instalL_dependency.assert_not_called()

        # Cleanup
        os.remove(diff_file)
        os.remove(req_file)
        post_file = os.path.join(storage_location, ".post_python_installed_packages.txt")
        if os.path.exists(post_file):
            os.remove(post_file)
        os.rmdir(install_directory)
        os.rmdir(storage_location)
