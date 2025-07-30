import unittest
from unittest.mock import MagicMock, patch

from invoke import MockContext
from invoke.exceptions import Exit

from tasks.coverage import PROFILE_COV, upload_to_codecov


class TestUploadToCodecov(unittest.TestCase):
    def setUp(self):
        self.mock_context = MockContext()

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_basic_linux(self, mock_gitlab_section, mock_platform_system, mock_get_distro):
        """Test basic upload_to_codecov functionality on Linux"""
        # Setup mocks
        mock_get_distro.return_value = "ubuntu"
        mock_platform_system.return_value = "Linux"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Call the function
        upload_to_codecov(self.mock_context)

        # Verify the correct command was executed
        self.mock_context.run.assert_called_once_with(f"codecov -f {PROFILE_COV} -F ubuntu", warn=True, timeout=2 * 60)

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_windows(self, mock_gitlab_section, mock_platform_system, mock_get_distro):
        """Test upload_to_codecov functionality on Windows"""
        # Setup mocks
        mock_get_distro.return_value = "windows"
        mock_platform_system.return_value = "Windows"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Call the function
        upload_to_codecov(self.mock_context)

        # Verify the correct command was executed with Windows binary
        self.mock_context.run.assert_called_once_with(
            f"codecov.exe -f {PROFILE_COV} -F windows", warn=True, timeout=2 * 60
        )

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_with_extra_tags(self, mock_gitlab_section, mock_platform_system, mock_get_distro):
        """Test upload_to_codecov with extra tags"""
        # Setup mocks
        mock_get_distro.return_value = "centos"
        mock_platform_system.return_value = "Linux"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Call the function with extra tags
        extra_tags = ["tag1", "tag2"]
        upload_to_codecov(self.mock_context, extra_tag=extra_tags)

        # Verify the correct command was executed with extra tags
        self.mock_context.run.assert_called_once_with(
            f"codecov -f {PROFILE_COV} -F centos -F tag1 -F tag2", warn=True, timeout=2 * 60
        )

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_with_empty_extra_tags(self, mock_gitlab_section, mock_platform_system, mock_get_distro):
        """Test upload_to_codecov with empty extra_tags list"""
        # Setup mocks
        mock_get_distro.return_value = "alpine"
        mock_platform_system.return_value = "Linux"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Call the function with empty extra_tags
        upload_to_codecov(self.mock_context, extra_tag=[])

        # Verify the correct command was executed with only the distro tag
        self.mock_context.run.assert_called_once_with(f"codecov -f {PROFILE_COV} -F alpine", warn=True, timeout=2 * 60)

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_with_pull_cache(self, mock_gitlab_section, mock_platform_system, mock_get_distro):
        """Test upload_to_codecov with pull_coverage_cache flag"""
        # Setup mocks
        mock_get_distro.return_value = "debian"
        mock_platform_system.return_value = "Linux"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Mock the apply_missing_coverage function
        with patch('tasks.coverage.apply_missing_coverage') as mock_apply:
            with patch('tasks.coverage.get_main_parent_commit') as mock_get_commit:
                mock_get_commit.return_value = "abc123"

                # Call the function with pull_coverage_cache=True
                upload_to_codecov(self.mock_context, pull_coverage_cache=True)

                # Verify apply_missing_coverage was called
                mock_apply.assert_called_once_with(self.mock_context, from_commit_sha="abc123", keep_temp_files=False)

                # Verify the codecov command was still executed
                self.mock_context.run.assert_called_once_with(
                    f"codecov -f {PROFILE_COV} -F debian", warn=True, timeout=2 * 60
                )

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_with_push_cache(self, mock_gitlab_section, mock_platform_system, mock_get_distro):
        """Test upload_to_codecov with push_coverage_cache flag"""
        # Setup mocks
        mock_get_distro.return_value = "rhel"
        mock_platform_system.return_value = "Linux"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Mock the upload_coverage_to_s3 function
        with patch('tasks.coverage.upload_coverage_to_s3') as mock_upload:
            # Call the function with push_coverage_cache=True
            upload_to_codecov(self.mock_context, push_coverage_cache=True)

            # Verify upload_coverage_to_s3 was called
            mock_upload.assert_called_once_with(self.mock_context)

            # Verify the codecov command was still executed
            self.mock_context.run.assert_called_once_with(
                f"codecov -f {PROFILE_COV} -F rhel", warn=True, timeout=2 * 60
            )

    def test_upload_to_codecov_conflicting_flags(self):
        """Test that upload_to_codecov raises an error when both flags are True"""
        # Call the function with both flags True - should raise Exit
        with self.assertRaises(Exit) as context:
            upload_to_codecov(self.mock_context, pull_coverage_cache=True, push_coverage_cache=True)

        # Verify the error message
        self.assertIn("Can't use both --pull-missing-coverage and --push-coverage-cache flags", str(context.exception))

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_with_debug_cache(self, mock_gitlab_section, mock_platform_system, mock_get_distro):
        """Test upload_to_codecov with debug_cache flag"""
        # Setup mocks
        mock_get_distro.return_value = "fedora"
        mock_platform_system.return_value = "Linux"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Mock the apply_missing_coverage function
        with patch('tasks.coverage.apply_missing_coverage') as mock_apply:
            with patch('tasks.coverage.get_main_parent_commit') as mock_get_commit:
                mock_get_commit.return_value = "def456"

                # Call the function with debug_cache=True
                upload_to_codecov(self.mock_context, pull_coverage_cache=True, debug_cache=True)

                # Verify apply_missing_coverage was called with debug_cache=True
                mock_apply.assert_called_once_with(self.mock_context, from_commit_sha="def456", keep_temp_files=True)

                # Verify the codecov command was still executed
                self.mock_context.run.assert_called_once_with(
                    f"codecov -f {PROFILE_COV} -F fedora", warn=True, timeout=2 * 60
                )

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_with_custom_coverage_file(
        self, mock_gitlab_section, mock_platform_system, mock_get_distro
    ):
        """Test upload_to_codecov with custom coverage file"""
        # Setup mocks
        mock_get_distro.return_value = "suse"
        mock_platform_system.return_value = "Linux"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Call the function with custom coverage file
        custom_coverage_file = "custom_coverage.out"
        upload_to_codecov(self.mock_context, coverage_file=custom_coverage_file)

        # Verify the correct command was executed with custom coverage file
        self.mock_context.run.assert_called_once_with(
            f"codecov -f {custom_coverage_file} -F suse", warn=True, timeout=2 * 60
        )

    @patch('tasks.coverage.get_distro')
    @patch('tasks.coverage.platform.system')
    @patch('tasks.coverage.gitlab_section')
    def test_upload_to_codecov_windows_with_custom_coverage_file(
        self, mock_gitlab_section, mock_platform_system, mock_get_distro
    ):
        """Test upload_to_codecov on Windows with custom coverage file"""
        # Setup mocks
        mock_get_distro.return_value = "windows"
        mock_platform_system.return_value = "Windows"
        mock_gitlab_section.return_value.__enter__ = MagicMock()

        # Mock the context run method to capture the command
        self.mock_context.run = MagicMock()

        # Call the function with custom coverage file on Windows
        custom_coverage_file = "windows_coverage.out"
        upload_to_codecov(self.mock_context, coverage_file=custom_coverage_file)

        # Verify the correct command was executed with Windows binary and custom coverage file
        self.mock_context.run.assert_called_once_with(
            f"codecov.exe -f {custom_coverage_file} -F windows", warn=True, timeout=2 * 60
        )


if __name__ == '__main__':
    unittest.main()
