import unittest
import unittest.mock

from invoke.exceptions import Exit

from tasks.update_python import (
    _get_major_minor_version,
    _validate_sha256,
    _validate_version_string,
)


class TestUpdatePython(unittest.TestCase):
    def test_get_major_minor_version(self):
        """Test extracting major.minor from full version."""
        self.assertEqual(_get_major_minor_version("3.13.7"), "3.13")
        self.assertEqual(_get_major_minor_version("3.14.0"), "3.14")
        self.assertEqual(_get_major_minor_version("2.7.18"), "2.7")

    def test_validate_version_string(self):
        """Test version string validation."""
        # Valid versions
        self.assertTrue(_validate_version_string("3.13.7"))
        self.assertTrue(_validate_version_string("3.14.0"))
        self.assertTrue(_validate_version_string("2.7.18"))

        # Invalid versions
        self.assertFalse(_validate_version_string("3.13"))
        self.assertFalse(_validate_version_string("3.13.7.1"))
        self.assertFalse(_validate_version_string("abc"))
        self.assertFalse(_validate_version_string("3.13.x"))
        self.assertFalse(_validate_version_string(""))

    def test_validate_sha256(self):
        """Test SHA256 hash validation."""
        # Valid SHA256 (64 hex characters)
        valid_hash = "6c9d80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b2"
        self.assertTrue(_validate_sha256(valid_hash))
        self.assertTrue(_validate_sha256(valid_hash.upper()))

        # Invalid SHA256
        self.assertFalse(_validate_sha256("abc"))
        self.assertFalse(
            _validate_sha256("6c9d80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b")
        )  # Too short
        self.assertFalse(
            _validate_sha256("6c9d80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b2a")
        )  # Too long
        self.assertFalse(
            _validate_sha256("zzzz80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b2")
        )  # Invalid chars
        self.assertFalse(_validate_sha256(""))


class TestGetCurrentPythonVersion(unittest.TestCase):
    @unittest.mock.patch('tasks.update_python.Path')
    def test_get_current_python_version(self, mock_path):
        """Test reading current Python version from omnibus file."""
        from tasks.update_python import _get_current_python_version

        # Mock file content
        mock_file = unittest.mock.MagicMock()
        mock_file.read_text.return_value = '''name "python3"

default_version "3.13.7"

unless windows?
  dependency "libffi"
end
'''
        mock_path.return_value = mock_file

        version = _get_current_python_version()
        self.assertEqual(version, "3.13.7")

    @unittest.mock.patch('tasks.update_python.Path')
    def test_get_current_python_version_not_found(self, mock_path):
        """Test error when version not found in file."""
        from tasks.update_python import _get_current_python_version

        # Mock file without version
        mock_file = unittest.mock.MagicMock()
        mock_file.read_text.return_value = 'name "python3"\n# No version here'
        mock_path.return_value = mock_file

        with self.assertRaises(Exit):
            _get_current_python_version()


class TestOmnibusUpdate(unittest.TestCase):
    @unittest.mock.patch('tasks.update_python.Path')
    def test_update_omnibus_python_version_and_sha(self, mock_path):
        """Test updating version and SHA256 in omnibus file."""
        from tasks.update_python import _update_omnibus_python

        original_content = '''name "python3"

default_version "3.13.7"

source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
       :sha256 => "6c9d80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b2"
'''

        # Mock file operations
        mock_file = unittest.mock.MagicMock()
        mock_file.read_text.return_value = original_content
        mock_path.return_value = mock_file

        # Update to new version
        _update_omnibus_python("3.13.9", "c4c066af19c98fb7835d473bebd7e23be84f6e9874d47db9e39a68ee5d0ce35c", warn=False)

        # Get the written content
        written_content = mock_file.write_text.call_args[0][0]

        # Verify version was updated
        self.assertIn('default_version "3.13.9"', written_content)
        self.assertNotIn('default_version "3.13.7"', written_content)

        # Verify SHA256 was updated
        self.assertIn(':sha256 => "c4c066af19c98fb7835d473bebd7e23be84f6e9874d47db9e39a68ee5d0ce35c"', written_content)
        self.assertNotIn('6c9d80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b2', written_content)


class TestBazelUpdate(unittest.TestCase):
    @unittest.mock.patch('tasks.update_python.Path')
    def test_update_bazel_python_version_and_sha(self, mock_path):
        """Test updating version and SHA256 in Bazel file."""
        from tasks.update_python import _update_bazel_python

        original_content = '''http_archive = use_repo_rule("//third_party/bazel/tools/build_defs/repo:http.bzl", "http_archive")

PYTHON_VERSION = "3.13.7"

http_archive(
    name = "cpython",
    sha256 = "6c9d80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b2",
    strip_prefix = "Python-{}".format(PYTHON_VERSION),
)
'''

        # Mock file operations
        mock_file = unittest.mock.MagicMock()
        mock_file.read_text.return_value = original_content
        mock_path.return_value = mock_file

        # Update to new version
        _update_bazel_python("3.13.9", "c4c066af19c98fb7835d473bebd7e23be84f6e9874d47db9e39a68ee5d0ce35c", warn=False)

        # Get the written content
        written_content = mock_file.write_text.call_args[0][0]

        # Verify version was updated
        self.assertIn('PYTHON_VERSION = "3.13.9"', written_content)
        self.assertNotIn('PYTHON_VERSION = "3.13.7"', written_content)

        # Verify SHA256 was updated
        self.assertIn('sha256 = "c4c066af19c98fb7835d473bebd7e23be84f6e9874d47db9e39a68ee5d0ce35c"', written_content)
        self.assertNotIn('6c9d80839cfa20024f34d9a6dd31ae2a9cd97ff5e980e969209746037a5153b2', written_content)


class TestGoTestUpdate(unittest.TestCase):
    @unittest.mock.patch('tasks.update_python.Path')
    def test_update_test_python_version(self, mock_path):
        """Test updating expected Python version in Go tests."""
        from tasks.update_python import _update_test_python

        original_content = '''package common

const (
	ExpectedPythonVersion2 = "2.7.18"
	// ExpectedPythonVersion3 is the expected python 3 version
	// Bump this version when the version in omnibus/config/software/python3.rb changes
	ExpectedPythonVersion3 = "3.13.7"
)
'''

        # Mock file operations
        mock_file = unittest.mock.MagicMock()
        mock_file.read_text.return_value = original_content
        mock_path.return_value = mock_file

        # Update to new version
        _update_test_python("3.13.9", warn=False)

        # Get the written content
        written_content = mock_file.write_text.call_args[0][0]

        # Verify version was updated
        self.assertIn('ExpectedPythonVersion3 = "3.13.9"', written_content)
        self.assertNotIn('ExpectedPythonVersion3 = "3.13.7"', written_content)

        # Verify Python 2 version unchanged
        self.assertIn('ExpectedPythonVersion2 = "2.7.18"', written_content)


class TestGetLatestPythonVersion(unittest.TestCase):
    def test_get_latest_python_version(self):
        """Test fetching latest Python version from python.org."""
        from tasks.update_python import _get_latest_python_version

        # Mock HTML response from python.org FTP
        mock_response = unittest.mock.MagicMock()
        mock_response.text = '''
<html>
<body>
<a href="3.13.0/">3.13.0/</a>
<a href="3.13.1/">3.13.1/</a>
<a href="3.13.7/">3.13.7/</a>
<a href="3.13.8/">3.13.8/</a>
<a href="3.13.9/">3.13.9/</a>
<a href="3.14.0/">3.14.0/</a>
</body>
</html>
'''

        # Mock httpx at the point it's imported (inside the function)
        with unittest.mock.patch('httpx.get', return_value=mock_response) as mock_get:
            # Should return latest 3.13.x version
            version = _get_latest_python_version("3.13")
            self.assertEqual(version, "3.13.9")

            # Verify correct URL was called
            mock_get.assert_called_with("https://www.python.org/ftp/python/", timeout=30, verify=True)

    def test_get_latest_python_version_not_found(self):
        """Test when no matching version is found."""
        from tasks.update_python import _get_latest_python_version

        # Mock response with no matching versions
        mock_response = unittest.mock.MagicMock()
        mock_response.text = '<html><body><a href="3.14.0/">3.14.0/</a></body></html>'

        # Mock httpx at the point it's imported (inside the function)
        with unittest.mock.patch('httpx.get', return_value=mock_response):
            # Should return None when no 3.13.x found
            version = _get_latest_python_version("3.13")
            self.assertIsNone(version)
