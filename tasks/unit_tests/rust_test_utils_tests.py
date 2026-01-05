import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path
from unittest.mock import MagicMock, patch

from tasks.libs.testing.rust_test_utils import (
    _enhance_junit_error_message,
    discover_rust_tests,
)


class TestDiscoverRustTests(unittest.TestCase):
    """Tests for discover_rust_tests() function using Bazel query"""

    @patch('subprocess.run')
    def test_discover_single_rust_test(self, mock_run):
        """Test discovering a single Rust test via Bazel query"""
        # Mock bazel query output
        mock_query = MagicMock()
        mock_query.returncode = 0
        mock_query.stdout = "//pkg/collector/test:my_test\n"
        mock_query.stderr = ""

        mock_run.side_effect = [mock_query]

        result = discover_rust_tests(["./pkg"])

        self.assertEqual(len(result), 1)
        self.assertIn("my_test", result)
        self.assertEqual(result["my_test"], "pkg/collector/test")

    @patch('subprocess.run')
    def test_discover_multiple_rust_tests(self, mock_run):
        """Test discovering multiple Rust tests from different packages"""
        mock_query = MagicMock()
        mock_query.returncode = 0
        mock_query.stdout = "//pkg/module1:test_one\n//pkg/module2:test_two\n"
        mock_query.stderr = ""

        mock_run.side_effect = [mock_query]

        result = discover_rust_tests(["./pkg"])

        self.assertEqual(len(result), 2)
        self.assertIn("test_one", result)
        self.assertIn("test_two", result)
        self.assertEqual(result["test_one"], "pkg/module1")
        self.assertEqual(result["test_two"], "pkg/module2")

    @patch('subprocess.run')
    def test_discover_no_tests_exit_code_7(self, mock_run):
        """Test handling bazel query exit code 7 (no targets found)"""
        mock_query = MagicMock()
        mock_query.returncode = 7  # No targets found
        mock_query.stdout = ""
        mock_query.stderr = "ERROR: no targets found beneath 'pkg'"

        mock_run.side_effect = [mock_query]

        result = discover_rust_tests(["./pkg"])

        self.assertEqual(len(result), 0)

    @patch('subprocess.run')
    def test_discover_multiple_paths(self, mock_run):
        """Test discovering tests from multiple target paths"""
        # First query returns one test
        mock_query1 = MagicMock()
        mock_query1.returncode = 0
        mock_query1.stdout = "//pkg/test:test1\n"
        mock_query1.stderr = ""

        # Second query returns exit code 7 (no tests)
        mock_query2 = MagicMock()
        mock_query2.returncode = 7
        mock_query2.stdout = ""
        mock_query2.stderr = ""

        mock_run.side_effect = [mock_query1, mock_query2]

        result = discover_rust_tests(["./pkg", "./cmd"])

        self.assertEqual(len(result), 1)
        self.assertIn("test1", result)

    @patch('subprocess.run')
    def test_query_output_with_loading_messages(self, mock_run):
        """Test parsing bazel query output that includes loading messages"""
        mock_query = MagicMock()
        mock_query.returncode = 0
        mock_query.stdout = "Loading: 0 packages loaded\n//pkg/test:my_test\n"
        mock_query.stderr = ""

        mock_run.side_effect = [mock_query]

        result = discover_rust_tests(["./pkg"])

        self.assertEqual(len(result), 1)
        self.assertIn("my_test", result)

    @patch('subprocess.run')
    def test_query_error_handling(self, mock_run):
        """Test handling of bazel query errors (non-7 exit codes)"""
        mock_query = MagicMock()
        mock_query.returncode = 1  # Some other error
        mock_query.stdout = ""
        mock_query.stderr = "ERROR: some other error"

        mock_run.side_effect = [mock_query]

        # Should not raise, just return empty dict
        result = discover_rust_tests(["./pkg"])

        self.assertEqual(len(result), 0)

    @patch('subprocess.run')
    def test_path_normalization_trailing_slash(self, mock_run):
        """Test that trailing slashes in paths are handled correctly"""
        mock_query = MagicMock()
        mock_query.returncode = 0
        mock_query.stdout = "//pkg/test:my_test\n"
        mock_query.stderr = ""

        mock_run.side_effect = [mock_query]

        # Path with trailing slash should work
        result = discover_rust_tests(["./pkg/"])

        self.assertEqual(len(result), 1)
        # Verify the query was called with correct pattern (no double slashes)
        query_call = mock_run.call_args_list[0]
        query_cmd = query_call[0][0]
        self.assertIn("//pkg/...", query_cmd[2])
        self.assertNotIn("//pkg//", query_cmd[2])

    @patch('subprocess.run')
    def test_path_normalization_with_ellipsis(self, mock_run):
        """Test that paths ending with ... are normalized correctly"""
        mock_query = MagicMock()
        mock_query.returncode = 0
        mock_query.stdout = "//pkg/test:my_test\n"
        mock_query.stderr = ""

        mock_run.side_effect = [mock_query]

        # Path already with ... should work
        result = discover_rust_tests(["./pkg/..."])

        self.assertEqual(len(result), 1)
        # Verify the query was called with correct pattern
        query_call = mock_run.call_args_list[0]
        query_cmd = query_call[0][0]
        self.assertIn("//pkg/...", query_cmd[2])


class TestEnhanceJunitErrorMessage(unittest.TestCase):
    """Tests for _enhance_junit_error_message() function"""

    def setUp(self):
        """Set up test XML files"""
        self.temp_dir = Path(tempfile.mkdtemp())
        self.test_data_dir = Path(__file__).parent / "testdata" / "rust"

    def tearDown(self):
        """Clean up temp files"""
        if self.temp_dir.exists():
            shutil.rmtree(self.temp_dir)

    def test_enhance_error_with_full_output(self):
        """Test enhancing error message with full system-out content"""
        # Copy sample failing XML
        sample_xml = self.test_data_dir / "sample_junit_fail.xml"
        test_xml = self.temp_dir / "test.xml"
        shutil.copy2(sample_xml, test_xml)

        _enhance_junit_error_message(str(test_xml), "pkg/test", "test_name")

        # Verify changes
        tree = ET.parse(test_xml)
        root = tree.getroot()
        error = root.find('.//error')

        self.assertIsNotNone(error)
        self.assertEqual(error.get('message'), 'Rust test suite failed')
        self.assertIn('test result: FAILED', error.text)
        self.assertIn('assertion `left == right` failed', error.text)

    def test_set_classname_and_suite_name(self):
        """Test setting proper test suite and test case names"""
        sample_xml = self.test_data_dir / "sample_junit_fail.xml"
        test_xml = self.temp_dir / "test.xml"
        shutil.copy2(sample_xml, test_xml)

        source_path = "pkg/collector/module/rusthello"
        test_name = "rusthello_test"

        _enhance_junit_error_message(str(test_xml), source_path, test_name)

        tree = ET.parse(test_xml)
        root = tree.getroot()
        testsuite = root.find('.//testsuite')
        testcase = root.find('.//testcase')

        # Check that suite name is just the source path
        self.assertEqual(testsuite.get('name'), source_path)

        # Check that testcase name is just the test name
        self.assertEqual(testcase.get('name'), test_name)

        # Check that classname is the source path
        self.assertEqual(testcase.get('classname'), source_path)

    def test_handle_missing_system_out(self):
        """Test handling XML without system-out element"""
        xml_content = '''<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="test_suite" tests="1" failures="0" errors="1">
    <testcase name="test_case" time="0.001">
      <error message="test error"></error>
    </testcase>
  </testsuite>
</testsuites>'''

        test_xml = self.temp_dir / "test.xml"
        test_xml.write_text(xml_content)

        # Should not raise an exception
        _enhance_junit_error_message(str(test_xml), "pkg/test", "test_name")

        # Verify classname is still set
        tree = ET.parse(test_xml)
        testcase = tree.find('.//testcase')
        self.assertEqual(testcase.get('classname'), "pkg/test")

    def test_handle_passing_test(self):
        """Test enhancing XML for passing tests"""
        sample_xml = self.test_data_dir / "sample_junit_pass.xml"
        test_xml = self.temp_dir / "test.xml"
        shutil.copy2(sample_xml, test_xml)

        _enhance_junit_error_message(str(test_xml), "pkg/test", "test_name")

        # Should complete without errors
        tree = ET.parse(test_xml)
        testcase = tree.find('.//testcase')
        self.assertEqual(testcase.get('classname'), "pkg/test")


if __name__ == '__main__':
    unittest.main()
