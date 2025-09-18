#!/usr/bin/env python3
"""
Unit tests for the coverage module functions.
"""

import unittest
from unittest.mock import MagicMock, patch

from invoke import Context

from tasks.libs.common.coverage import upload_codecov


class TestUploadCodecov(unittest.TestCase):
    """Test cases for the upload_codecov function."""

    def setUp(self):
        """Set up test fixtures."""
        self.mock_context = MagicMock(spec=Context)
        self.mock_context.run = MagicMock()

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_basic_linux(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with basic parameters on Linux."""
        # Setup mocks
        mock_get_distro.return_value = "ubuntu"
        mock_platform_system.return_value = "Linux"

        # Call the function
        upload_codecov(self.mock_context, "coverage.out", [])

        # Verify the correct command was executed
        self.mock_context.run.assert_called_once_with("codecov -f coverage.out -F ubuntu", warn=True, timeout=2 * 60)

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_windows(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov on Windows."""
        # Setup mocks
        mock_get_distro.return_value = "windows"
        mock_platform_system.return_value = "Windows"

        # Call the function
        upload_codecov(self.mock_context, "coverage.out", [])

        # Verify the correct command was executed with Windows binary
        self.mock_context.run.assert_called_once_with(
            "codecov.exe -f coverage.out -F windows", warn=True, timeout=2 * 60
        )

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_with_extra_tags(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with extra tags."""
        # Setup mocks
        mock_get_distro.return_value = "centos"
        mock_platform_system.return_value = "Linux"

        # Call the function with extra tags
        extra_tags = ["tag1", "tag2", "tag3"]
        upload_codecov(self.mock_context, "coverage.out", extra_tags)

        # Verify the correct command was executed with extra tags
        self.mock_context.run.assert_called_once_with(
            "codecov -f coverage.out -F centos -F tag1 -F tag2 -F tag3", warn=True, timeout=2 * 60
        )

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_with_empty_extra_tags(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with empty extra_tags list."""
        # Setup mocks
        mock_get_distro.return_value = "alpine"
        mock_platform_system.return_value = "Linux"

        # Call the function with empty extra_tags
        upload_codecov(self.mock_context, "coverage.out", [])

        # Verify the correct command was executed with only the distro tag
        self.mock_context.run.assert_called_once_with("codecov -f coverage.out -F alpine", warn=True, timeout=2 * 60)

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_with_single_extra_tag(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with a single extra tag."""
        # Setup mocks
        mock_get_distro.return_value = "debian"
        mock_platform_system.return_value = "Linux"

        # Call the function with single extra tag
        upload_codecov(self.mock_context, "coverage.out", ["test-tag"])

        # Verify the correct command was executed
        self.mock_context.run.assert_called_once_with(
            "codecov -f coverage.out -F debian -F test-tag", warn=True, timeout=2 * 60
        )

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_with_custom_coverage_file(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with a custom coverage file path."""
        # Setup mocks
        mock_get_distro.return_value = "rhel"
        mock_platform_system.return_value = "Linux"

        # Call the function with custom coverage file
        upload_codecov(self.mock_context, "/path/to/custom/coverage.out", ["custom-tag"])

        # Verify the correct command was executed
        self.mock_context.run.assert_called_once_with(
            "codecov -f /path/to/custom/coverage.out -F rhel -F custom-tag", warn=True, timeout=2 * 60
        )

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_with_special_characters_in_tags(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with special characters in tags."""
        # Setup mocks
        mock_get_distro.return_value = "fedora"
        mock_platform_system.return_value = "Linux"

        # Call the function with tags containing special characters
        extra_tags = ["test-tag", "tag_with_underscore", "tag-with-dash"]
        upload_codecov(self.mock_context, "coverage.out", extra_tags)

        # Verify the correct command was executed
        self.mock_context.run.assert_called_once_with(
            "codecov -f coverage.out -F fedora -F test-tag -F tag_with_underscore -F tag-with-dash",
            warn=True,
            timeout=2 * 60,
        )

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_context_run_parameters(self, mock_platform_system, mock_get_distro):
        """Test that context.run is called with correct parameters."""
        # Setup mocks
        mock_get_distro.return_value = "ubuntu"
        mock_platform_system.return_value = "Linux"

        # Call the function
        upload_codecov(self.mock_context, "coverage.out", ["test-tag"])

        # Verify context.run was called with correct parameters
        call_args = self.mock_context.run.call_args
        self.assertEqual(call_args[1]['warn'], True)
        self.assertEqual(call_args[1]['timeout'], 2 * 60)

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_multiple_calls(self, mock_platform_system, mock_get_distro):
        """Test multiple calls to upload_codecov."""
        # Setup mocks
        mock_get_distro.return_value = "centos"
        mock_platform_system.return_value = "Linux"

        # Call the function multiple times
        upload_codecov(self.mock_context, "coverage1.out", ["tag1"])
        upload_codecov(self.mock_context, "coverage2.out", ["tag2"])

        # Verify both calls were made correctly
        expected_calls = [
            unittest.mock.call("codecov -f coverage1.out -F centos -F tag1", warn=True, timeout=2 * 60),
            unittest.mock.call("codecov -f coverage2.out -F centos -F tag2", warn=True, timeout=2 * 60),
        ]
        self.mock_context.run.assert_has_calls(expected_calls)
        self.assertEqual(self.mock_context.run.call_count, 2)

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_with_none_extra_tags(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with None extra_tags (should be treated as empty list)."""
        # Setup mocks
        mock_get_distro.return_value = "ubuntu"
        mock_platform_system.return_value = "Linux"

        # Call the function with None extra_tags
        upload_codecov(self.mock_context, "coverage.out", None)

        # Verify the correct command was executed with only the distro tag
        self.mock_context.run.assert_called_once_with("codecov -f coverage.out -F ubuntu", warn=True, timeout=2 * 60)

    @patch('tasks.libs.common.coverage.get_distro')
    @patch('tasks.libs.common.coverage.platform.system')
    def test_upload_codecov_edge_case_empty_string_tags(self, mock_platform_system, mock_get_distro):
        """Test upload_codecov with empty string tags."""
        # Setup mocks
        mock_get_distro.return_value = "ubuntu"
        mock_platform_system.return_value = "Linux"

        # Call the function with empty string tags
        upload_codecov(self.mock_context, "coverage.out", ["", "valid-tag", ""])

        # Verify the correct command was executed (empty strings should be included)
        self.mock_context.run.assert_called_once_with(
            "codecov -f coverage.out -F ubuntu -F  -F valid-tag -F ", warn=True, timeout=2 * 60
        )
