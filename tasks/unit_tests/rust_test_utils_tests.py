import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path
from unittest.mock import MagicMock, patch

from tasks.libs.testing.rust_test_utils import _enhance_junit_error_message, run_rust_tests
from tasks.libs.types.arch import Arch


class TestRunRustTests(unittest.TestCase):
    """Tests for run_rust_tests() function with single Bazel command"""

    @patch('subprocess.run')
    @patch('invoke.context.Context.run')
    def test_run_with_rust_tests(self, mock_ctx_run, mock_subprocess_run):
        """Test running Rust tests when tests are found"""
        # Mock query to return one test
        mock_query = MagicMock()
        mock_query.returncode = 0
        mock_query.stdout = "//pkg/test:my_test\n"
        mock_query.stderr = ""

        mock_subprocess_run.return_value = mock_query

        # Mock test execution
        mock_test_result = MagicMock()
        mock_test_result.exited = 0
        mock_ctx_run.return_value = mock_test_result

        # Mock XML file existence and content
        with patch('os.path.exists', return_value=True):
            with patch('shutil.copy2'):
                with patch('tasks.libs.testing.rust_test_utils._enhance_junit_error_message'):
                    with patch('xml.etree.ElementTree.parse') as mock_parse:
                        mock_tree = MagicMock()
                        mock_root = MagicMock()
                        mock_testsuite = MagicMock()
                        mock_testsuite.get.side_effect = lambda key, default=0: 0
                        mock_root.find.return_value = mock_testsuite
                        mock_tree.getroot.return_value = mock_root
                        mock_parse.return_value = mock_tree

                        ctx = MagicMock()
                        ctx.run = mock_ctx_run

                        result = run_rust_tests(ctx, ["./pkg"], Arch.local())

                        self.assertTrue(result.success)
                        self.assertEqual(result.test_count, 1)
                        self.assertEqual(len(result.failures), 0)

    @patch('subprocess.run')
    def test_no_rust_tests_found(self, mock_subprocess_run):
        """Test when no Rust tests are found"""
        # Mock query to return no tests (exit code 7)
        mock_query = MagicMock()
        mock_query.returncode = 7
        mock_query.stdout = ""
        mock_query.stderr = "ERROR: no targets found"

        mock_subprocess_run.return_value = mock_query

        ctx = MagicMock()
        result = run_rust_tests(ctx, ["./pkg"], Arch.local())

        self.assertTrue(result.success)
        self.assertEqual(result.test_count, 0)
        self.assertEqual(len(result.failures), 0)

    @patch('subprocess.run')
    @patch('invoke.context.Context.run')
    def test_multiple_target_paths(self, mock_ctx_run, mock_subprocess_run):
        """Test with multiple target paths"""
        # First query returns one test, second returns empty
        mock_query1 = MagicMock()
        mock_query1.returncode = 0
        mock_query1.stdout = "//pkg/test:test1\n"
        mock_query1.stderr = ""

        mock_query2 = MagicMock()
        mock_query2.returncode = 7
        mock_query2.stdout = ""
        mock_query2.stderr = ""

        mock_subprocess_run.side_effect = [mock_query1, mock_query2]

        mock_test_result = MagicMock()
        mock_test_result.exited = 0
        mock_ctx_run.return_value = mock_test_result

        with patch('os.path.exists', return_value=True):
            with patch('shutil.copy2'):
                with patch('tasks.libs.testing.rust_test_utils._enhance_junit_error_message'):
                    with patch('xml.etree.ElementTree.parse') as mock_parse:
                        mock_tree = MagicMock()
                        mock_root = MagicMock()
                        mock_testsuite = MagicMock()
                        mock_testsuite.get.side_effect = lambda key, default=0: 0
                        mock_root.find.return_value = mock_testsuite
                        mock_tree.getroot.return_value = mock_root
                        mock_parse.return_value = mock_tree

                        ctx = MagicMock()
                        ctx.run = mock_ctx_run

                        result = run_rust_tests(ctx, ["./pkg", "./cmd"], Arch.local())

                        self.assertTrue(result.success)
                        self.assertEqual(result.test_count, 1)

    @patch('subprocess.run')
    @patch('invoke.context.Context.run')
    def test_test_failure_detected(self, mock_ctx_run, mock_subprocess_run):
        """Test that failures are detected from JUnit XML"""
        # Mock query
        mock_query = MagicMock()
        mock_query.returncode = 0
        mock_query.stdout = "//pkg/test:failing_test\n"
        mock_query.stderr = ""

        mock_subprocess_run.return_value = mock_query

        # Mock test execution (Bazel returns non-zero)
        mock_test_result = MagicMock()
        mock_test_result.exited = 1
        mock_ctx_run.return_value = mock_test_result

        with patch('os.path.exists', return_value=True):
            with patch('shutil.copy2'):
                with patch('tasks.libs.testing.rust_test_utils._enhance_junit_error_message'):
                    with patch('xml.etree.ElementTree.parse') as mock_parse:
                        # Mock XML showing failures
                        mock_tree = MagicMock()
                        mock_root = MagicMock()
                        mock_testsuite = MagicMock()

                        def get_attr(key, default=0):
                            if key == 'failures':
                                return 1
                            return 0

                        mock_testsuite.get = get_attr
                        mock_root.find.return_value = mock_testsuite
                        mock_tree.getroot.return_value = mock_root
                        mock_parse.return_value = mock_tree

                        ctx = MagicMock()
                        ctx.run = mock_ctx_run

                        result = run_rust_tests(ctx, ["./pkg"], Arch.local())

                        self.assertFalse(result.success)
                        self.assertEqual(result.test_count, 1)
                        self.assertEqual(len(result.failures), 1)
                        self.assertIn("pkg/test:failing_test", result.failures)

    def test_windows_and_macos_skip(self):
        """Test that Windows and macOS are skipped"""
        with patch('sys.platform', 'win32'):
            ctx = MagicMock()
            result = run_rust_tests(ctx, ["./pkg"], Arch.local())
            self.assertTrue(result.success)
            self.assertEqual(result.test_count, 0)

        with patch('sys.platform', 'darwin'):
            ctx = MagicMock()
            result = run_rust_tests(ctx, ["./pkg"], Arch.local())
            self.assertTrue(result.success)
            self.assertEqual(result.test_count, 0)


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
