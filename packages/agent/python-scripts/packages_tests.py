import unittest
from packages import extract_version, create_python_installed_packages_file, create_diff_installed_packages_file, check_file_owner_system_windows
from packages import run_command, install_datadog_package, install_dependency_package, install_diff_packages_file
from packages import IntegrationInstallError, IntegrationsRestoreError
import packages
import packaging.requirements
import os
import tempfile
from unittest.mock import patch, call, MagicMock

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

    # ------------------------------------------------------------------ #
    # run_command
    # ------------------------------------------------------------------ #

    def test_run_command_success_returns_zero_rc(self):
        with patch('subprocess.run') as mock_run:
            mock_run.return_value = MagicMock(stdout='ok\n', stderr='', returncode=0)
            stdout, stderr, rc = run_command(['echo', 'ok'])
        self.assertEqual(rc, 0)
        self.assertEqual(stdout, 'ok\n')

    def test_run_command_failure_returns_nonzero_rc(self):
        import subprocess
        with patch('subprocess.run') as mock_run:
            exc = subprocess.CalledProcessError(1, ['false'], output='', stderr='something went wrong')
            mock_run.side_effect = exc
            stdout, stderr, rc = run_command(['false'])
        self.assertEqual(rc, 1)
        self.assertEqual(stderr, 'something went wrong')

    # ------------------------------------------------------------------ #
    # install_datadog_package — retry logic
    # ------------------------------------------------------------------ #

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_install_datadog_package_succeeds_on_first_attempt(self):
        with patch('packages.run_command', return_value=('', '', 0)) as mock_cmd:
            install_datadog_package('datadog-ping==1.0.2', '/tmp/agent')
        mock_cmd.assert_called_once()

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_install_datadog_package_succeeds_on_retry(self):
        """First attempt fails, second succeeds — no exception raised."""
        with patch('packages.run_command', side_effect=[('', 'err', 1), ('', '', 0)]) as mock_cmd:
            install_datadog_package('datadog-ping==1.0.2', '/tmp/agent')
        self.assertEqual(mock_cmd.call_count, 2)

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_install_datadog_package_raises_after_two_failures(self):
        """Both attempts fail — IntegrationInstallError is raised."""
        with patch('packages.run_command', return_value=('', 'some error', 2)) as mock_cmd:
            with self.assertRaises(IntegrationInstallError) as ctx:
                install_datadog_package('datadog-ping==1.0.2', '/tmp/agent')
        self.assertEqual(mock_cmd.call_count, 2)
        self.assertEqual(ctx.exception.package, 'datadog-ping==1.0.2')
        self.assertEqual(ctx.exception.returncode, 2)

    # ------------------------------------------------------------------ #
    # install_diff_packages_file — error collection
    # ------------------------------------------------------------------ #

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_install_diff_packages_file_collects_failures(self):
        """All packages are attempted even when some fail; IntegrationsRestoreError lists failures."""
        install_dir = tempfile.mkdtemp()
        storage_dir = tempfile.mkdtemp()
        diff_file = os.path.join(storage_dir, '.diff_python_installed_packages.txt')
        req_file = os.path.join(install_dir, 'requirements-agent-release.txt')

        with open(diff_file, 'w') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("datadog-ping==1.0.2\n")
            f.write("datadog-snmp==1.0.0\n")

        with open(req_file, 'w') as f:
            f.write('')

        # install_datadog_package always raises
        with patch('packages.install_datadog_package', side_effect=IntegrationInstallError('pkg', 1, 'err')) as mock_install:
            with self.assertRaises(IntegrationsRestoreError) as ctx:
                install_diff_packages_file(install_dir, diff_file, req_file)

        # Both packages were attempted
        self.assertEqual(mock_install.call_count, 2)
        self.assertEqual(len(ctx.exception.failures), 2)

        # Cleanup
        os.remove(diff_file)
        os.remove(req_file)
        os.rmdir(install_dir)
        os.rmdir(storage_dir)

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_install_diff_packages_file_succeeds_silently(self):
        """When all installs succeed no exception is raised."""
        install_dir = tempfile.mkdtemp()
        storage_dir = tempfile.mkdtemp()
        diff_file = os.path.join(storage_dir, '.diff_python_installed_packages.txt')
        req_file = os.path.join(install_dir, 'requirements-agent-release.txt')

        with open(diff_file, 'w') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("datadog-ping==1.0.2\n")

        with open(req_file, 'w') as f:
            f.write('')

        with patch('packages.install_datadog_package') as mock_install:
            install_diff_packages_file(install_dir, diff_file, req_file)  # must not raise

        mock_install.assert_called_once_with('datadog-ping==1.0.2', install_dir)

        # Cleanup
        os.remove(diff_file)
        os.remove(req_file)
        os.rmdir(install_dir)
        os.rmdir(storage_dir)
