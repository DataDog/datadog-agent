import unittest
from unittest.mock import MagicMock, patch

import requests
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.libs.package.url import (
    DEB_TESTING_BUCKET_URL,
    RPM_TESTING_BUCKET_URL,
    _deb_get_filename_for_package,
    get_deb_package_url,
    get_rpm_package_url,
)


class TestGetRpmPackageUrl(unittest.TestCase):
    @patch('tempfile.mktemp')
    @patch('tasks.libs.common.download.download')
    @patch('requests.get')
    def test_get_rpm_package_url_amd64(self, mock_get, mock_download, mock_mktemp):
        # Setup mocks
        # Mock tempfile.mktemp
        mock_mktemp.return_value = "/tmp/fake_temp_file"

        # Mock download to do nothing - we'll just verify it was called with the correct parameters
        def download_side_effect(url, filename):
            # Do nothing but pass through the call
            pass

        mock_download.side_effect = download_side_effect

        # Mock requests.get
        mock_response = MagicMock()
        mock_response.text = """<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="primary">
    <location href="repodata/primary.xml.gz"/>
  </data>
</repomd>"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Create a context mock with run method
        ctx = Context()
        ctx.run = MagicMock()
        ctx.run.return_value = MagicMock(
            stdout="""<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common">
  <package type="rpm">
    <name>datadog-agent</name>
    <version>7.45.0</version>
    <location href="Packages/d/datadog-agent-7.45.0-1.x86_64.rpm"/>
  </package>
  <package type="rpm">
    <name>datadog-agent-dev</name>
    <version>7.45.0</version>
    <location href="Packages/d/datadog-agent-dev-7.45.0-1.x86_64.rpm"/>
  </package>
</metadata>"""
        )

        # Call the function
        pipeline_id = 123456
        package_name = "datadog-agent"
        arch = "amd64"

        result = get_rpm_package_url(ctx, pipeline_id, package_name, arch)

        # Verify results
        arch2 = "x86_64"  # Expected conversion for amd64
        packages_url = f"{RPM_TESTING_BUCKET_URL}/testing/pipeline-{pipeline_id}-a7/7/{arch2}"
        repomd_url = f"{packages_url}/repodata/repomd.xml"
        expected_url = f"{packages_url}/Packages/d/datadog-agent-7.45.0-1.x86_64.rpm"

        self.assertEqual(result, expected_url)

        # Verify that we called requests.get with the repomd_url
        mock_get.assert_any_call(repomd_url, timeout=None)

        # Verify gunzip was called
        ctx.run.assert_called_once_with("gunzip --stdout /tmp/fake_temp_file", hide=True)

    @patch('tempfile.mktemp')
    @patch('tasks.libs.common.download.download')
    @patch('requests.get')
    def test_get_rpm_package_url_arm64(self, mock_get, mock_download, mock_mktemp):
        # Setup mocks
        # Mock tempfile.mktemp
        mock_mktemp.return_value = "/tmp/fake_temp_file"

        # Mock download to do nothing - we'll just verify it was called with the correct parameters
        def download_side_effect(url, filename):
            # Do nothing but pass through the call
            pass

        mock_download.side_effect = download_side_effect

        # Mock requests.get
        mock_response = MagicMock()
        mock_response.text = """<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="primary">
    <location href="repodata/primary.xml.gz"/>
  </data>
</repomd>"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Create a context mock with run method
        ctx = Context()
        ctx.run = MagicMock()
        ctx.run.return_value = MagicMock(
            stdout="""<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common">
  <package type="rpm">
    <name>datadog-agent</name>
    <version>7.45.0</version>
    <location href="Packages/d/datadog-agent-7.45.0-1.aarch64.rpm"/>
  </package>
</metadata>"""
        )

        # Call the function
        pipeline_id = 987654
        package_name = "datadog-agent"
        arch = "arm64"

        result = get_rpm_package_url(ctx, pipeline_id, package_name, arch)

        # Verify results
        arch2 = "aarch64"  # Expected conversion for arm64
        packages_url = f"{RPM_TESTING_BUCKET_URL}/testing/pipeline-{pipeline_id}-a7/7/{arch2}"
        repomd_url = f"{packages_url}/repodata/repomd.xml"
        expected_url = f"{packages_url}/Packages/d/datadog-agent-7.45.0-1.aarch64.rpm"

        self.assertEqual(result, expected_url)

        # Verify that we called requests.get with the repomd_url
        mock_get.assert_any_call(repomd_url, timeout=None)

        # Verify gunzip was called
        ctx.run.assert_called_once_with("gunzip --stdout /tmp/fake_temp_file", hide=True)

    @patch('tempfile.mktemp')
    @patch('tasks.libs.common.download.download')
    @patch('requests.get')
    def test_get_rpm_package_url_no_primary_data(self, mock_get, mock_download, mock_mktemp):
        # Mock download to do nothing
        def download_side_effect(url, filename):
            pass

        mock_download.side_effect = download_side_effect

        # Mock requests.get
        mock_response = MagicMock()
        mock_response.text = """<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="other">
    <location href="repodata/other.xml.gz"/>
  </data>
</repomd>"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Create a context mock
        ctx = Context()

        # Call the function and expect an assert
        pipeline_id = 123456
        package_name = "datadog-agent"
        arch = "amd64"

        with self.assertRaises(AssertionError) as context:
            get_rpm_package_url(ctx, pipeline_id, package_name, arch)

        # Verify error message
        packages_url = f"{RPM_TESTING_BUCKET_URL}/testing/pipeline-{pipeline_id}-a7/7/x86_64"
        repomd_url = f"{packages_url}/repodata/repomd.xml"
        self.assertTrue(f"Could not find primary data in {repomd_url}" in str(context.exception))

    @patch('tempfile.mktemp')
    @patch('tasks.libs.common.download.download')
    @patch('requests.get')
    def test_get_rpm_package_url_no_location(self, mock_get, mock_download, mock_mktemp):
        # Mock download to do nothing
        def download_side_effect(url, filename):
            pass

        mock_download.side_effect = download_side_effect

        # Mock requests.get
        mock_response = MagicMock()
        mock_response.text = """<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="primary">
    <!-- Missing location element -->
  </data>
</repomd>"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Create a context mock
        ctx = Context()

        # Call the function and expect an assert
        pipeline_id = 123456
        package_name = "datadog-agent"
        arch = "amd64"

        with self.assertRaises(AssertionError) as context:
            get_rpm_package_url(ctx, pipeline_id, package_name, arch)

        # Verify error message
        packages_url = f"{RPM_TESTING_BUCKET_URL}/testing/pipeline-{pipeline_id}-a7/7/x86_64"
        repomd_url = f"{packages_url}/repodata/repomd.xml"
        self.assertTrue(f"Could not find location for primary data in {repomd_url}" in str(context.exception))

    @patch('tempfile.mktemp')
    @patch('tasks.libs.common.download.download')
    @patch('requests.get')
    def test_get_rpm_package_url_package_not_found(self, mock_get, mock_download, mock_mktemp):
        # Setup mocks
        # Mock tempfile.mktemp
        mock_mktemp.return_value = "/tmp/fake_temp_file"

        # Mock download to do nothing
        def download_side_effect(url, filename):
            pass

        mock_download.side_effect = download_side_effect

        # Mock requests.get
        mock_response = MagicMock()
        mock_response.text = """<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="primary">
    <location href="repodata/primary.xml.gz"/>
  </data>
</repomd>"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Create a context mock with run method
        ctx = Context()
        ctx.run = MagicMock()
        ctx.run.return_value = MagicMock(
            stdout="""<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common">
  <package type="rpm">
    <name>some-other-package</name>
    <version>1.0.0</version>
    <location href="Packages/s/some-other-package-1.0.0-1.x86_64.rpm"/>
  </package>
</metadata>"""
        )

        # Call the function
        pipeline_id = 123456
        package_name = "datadog-agent"
        arch = "amd64"

        with self.assertRaises(Exit) as context:
            get_rpm_package_url(ctx, pipeline_id, package_name, arch)

        # Verify error
        packages_url = f"{RPM_TESTING_BUCKET_URL}/testing/pipeline-{pipeline_id}-a7/7/x86_64"
        primary_url = f"{packages_url}/repodata/primary.xml.gz"
        self.assertEqual(context.exception.code, 1)
        self.assertEqual(context.exception.message, f"Could not find package {package_name} in {primary_url}")

    @patch('requests.get')
    def test_get_rpm_package_url_http_error(self, mock_get):
        # Mock requests.get to raise an exception
        mock_response = MagicMock()
        mock_response.raise_for_status.side_effect = requests.exceptions.HTTPError("404 Not Found")
        mock_get.return_value = mock_response

        # Create a context mock
        ctx = Context()

        # Call the function and expect an exception
        pipeline_id = 123456
        package_name = "datadog-agent"
        arch = "amd64"

        with self.assertRaises(requests.exceptions.HTTPError):
            get_rpm_package_url(ctx, pipeline_id, package_name, arch)


class TestGetDebPackageUrl(unittest.TestCase):
    @patch('tasks.libs.package.url._deb_get_filename_for_package')
    def test_get_deb_package_url_amd64(self, mock_get_filename):
        # Setup the mock
        mock_get_filename.return_value = "pool/main/d/datadog-agent/datadog-agent_7.45.0-1_amd64.deb"

        # Call the function with test parameters
        ctx = Context()
        pipeline_id = 123456
        package_name = "datadog-agent"
        arch = "amd64"

        # Execute the function
        result = get_deb_package_url(ctx, pipeline_id, package_name, arch)

        # Assert the result
        expected_url = f"{DEB_TESTING_BUCKET_URL}/pool/main/d/datadog-agent/datadog-agent_7.45.0-1_amd64.deb"
        self.assertEqual(result, expected_url)

        # Assert the mock was called with the correct parameters
        expected_packages_url = (
            f"{DEB_TESTING_BUCKET_URL}/dists/pipeline-{pipeline_id}-a7-x86_64/7/binary-{arch}/Packages"
        )
        mock_get_filename.assert_called_once_with(expected_packages_url, package_name)

    @patch('tasks.libs.package.url._deb_get_filename_for_package')
    def test_get_deb_package_url_arm64(self, mock_get_filename):
        # Setup the mock
        mock_get_filename.return_value = "pool/main/d/datadog-agent/datadog-agent_7.45.0-1_arm64.deb"

        # Call the function with test parameters
        ctx = Context()
        pipeline_id = 987654
        package_name = "datadog-agent"
        arch = "arm64"

        # Execute the function
        result = get_deb_package_url(ctx, pipeline_id, package_name, arch)

        # Assert the result
        expected_url = f"{DEB_TESTING_BUCKET_URL}/pool/main/d/datadog-agent/datadog-agent_7.45.0-1_arm64.deb"
        self.assertEqual(result, expected_url)

        # Assert the mock was called with the correct parameters
        expected_packages_url = (
            f"{DEB_TESTING_BUCKET_URL}/dists/pipeline-{pipeline_id}-a7-{arch}/7/binary-{arch}/Packages"
        )
        mock_get_filename.assert_called_once_with(expected_packages_url, package_name)


class TestDebGetFilenameForPackage(unittest.TestCase):
    @patch('requests.get')
    def test_deb_get_filename_for_package(self, mock_get):
        # Mock the response
        mock_response = MagicMock()
        mock_response.text = """Package: datadog-agent
Version: 7.45.0-1
Architecture: amd64
Filename: pool/main/d/datadog-agent/datadog-agent_7.45.0-1_amd64.deb
Size: 12345
MD5sum: abcd1234

Package: datadog-agent-dev
Version: 7.45.0-1
Architecture: amd64
Filename: pool/main/d/datadog-agent-dev/datadog-agent-dev_7.45.0-1_amd64.deb
Size: 54321
MD5sum: efgh5678
"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Test parameters
        package_name = "datadog-agent"
        packages_url = f"{DEB_TESTING_BUCKET_URL}/dists/pipeline-123-a7-x86_64/7/binary-amd64/Packages"

        # Execute the function
        result = _deb_get_filename_for_package(packages_url, package_name)

        # Assert results
        self.assertEqual(result, "pool/main/d/datadog-agent/datadog-agent_7.45.0-1_amd64.deb")
        mock_get.assert_called_once_with(packages_url, timeout=None)
        mock_response.raise_for_status.assert_called_once()

    @patch('requests.get')
    def test_deb_get_filename_for_package_not_found(self, mock_get):
        # Mock the response
        mock_response = MagicMock()
        mock_response.text = """Package: some-other-package
Version: 1.0.0
Architecture: amd64
Filename: pool/main/s/some-other-package/some-other-package_1.0.0_amd64.deb
Size: 12345
MD5sum: abcd1234
"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Test parameters
        package_name = "datadog-agent"
        packages_url = f"{DEB_TESTING_BUCKET_URL}/dists/pipeline-123-a7-x86_64/7/binary-amd64/Packages"

        # Execute the function and assert it raises the expected exception
        with self.assertRaises(Exit) as context:
            _deb_get_filename_for_package(packages_url, package_name)

        self.assertEqual(context.exception.code, 1)
        self.assertEqual(context.exception.message, f"Could not find filename for {package_name} in {packages_url}")

    @patch('requests.get')
    def test_deb_get_filename_for_package_missing_filename(self, mock_get):
        # Mock the response with a package that has no Filename field
        mock_response = MagicMock()
        mock_response.text = """Package: datadog-agent
Version: 7.45.0-1
Architecture: amd64
Size: 12345
MD5sum: abcd1234
"""
        mock_response.raise_for_status = MagicMock()
        mock_get.return_value = mock_response

        # Test parameters
        package_name = "datadog-agent"
        packages_url = f"{DEB_TESTING_BUCKET_URL}/dists/pipeline-123-a7-x86_64/7/binary-amd64/Packages"

        # Execute the function and assert it raises the expected exception
        with self.assertRaises(Exit) as context:
            _deb_get_filename_for_package(packages_url, package_name)

        self.assertEqual(context.exception.code, 1)
        self.assertEqual(context.exception.message, f"Could not find filename for {package_name} in {packages_url}")
