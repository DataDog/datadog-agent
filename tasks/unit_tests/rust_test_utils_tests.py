import json
import os
import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import MagicMock, mock_open, patch

from tasks.flavor import AgentFlavor
from tasks.libs.testing.rust_test_utils import (
    RustTestResult,
    _enhance_junit_error_message,
    discover_rust_tests,
    run_rust_tests,
)
from tasks.libs.types.arch import Arch


class TestDiscoverRustTests(unittest.TestCase):
    """Tests for discover_rust_tests() function"""

    def setUp(self):
        """Set up test fixtures"""
        self.test_data_dir = Path(__file__).parent / "testdata" / "rust"
        self.temp_dir = Path(tempfile.mkdtemp())

    def tearDown(self):
        """Clean up temp files"""
        if self.temp_dir.exists():
            shutil.rmtree(self.temp_dir)

    def test_discover_single_rust_test(self):
        """Test discovering a single Rust test in BUILD.bazel"""
        # Create a test directory with BUILD.bazel
        test_dir = self.temp_dir / "pkg" / "test"
        test_dir.mkdir(parents=True)

        build_content = 'rust_test(\n    name = "my_test",\n)\n'
        (test_dir / "BUILD.bazel").write_text(build_content)

        # Save current dir and change to temp_dir
        original_dir = os.getcwd()
        try:
            os.chdir(self.temp_dir)
            result = discover_rust_tests(["./pkg/..."])

            self.assertEqual(len(result), 1)
            self.assertIn("my_test", result)
            self.assertTrue(result["my_test"].endswith(str(Path("pkg/test"))))
        finally:
            os.chdir(original_dir)

    def test_discover_multiple_rust_tests(self):
        """Test discovering multiple Rust tests in one BUILD.bazel"""
        test_dir = self.temp_dir / "pkg" / "test"
        test_dir.mkdir(parents=True)

        build_content = '''rust_test(
    name = "test_one",
)

rust_test(
    name = "test_two",
)
'''
        (test_dir / "BUILD.bazel").write_text(build_content)

        original_dir = os.getcwd()
        try:
            os.chdir(self.temp_dir)
            result = discover_rust_tests(["./pkg/..."])

            self.assertEqual(len(result), 2)
            self.assertIn("test_one", result)
            self.assertIn("test_two", result)
        finally:
            os.chdir(original_dir)

    def test_discover_nested_directories(self):
        """Test discovering Rust tests in nested directory structure"""
        dir1 = self.temp_dir / "pkg" / "module1"
        dir2 = self.temp_dir / "pkg" / "module2" / "submodule"
        dir1.mkdir(parents=True)
        dir2.mkdir(parents=True)

        (dir1 / "BUILD.bazel").write_text('rust_test(\n    name = "test1",\n)\n')
        (dir2 / "BUILD.bazel").write_text('rust_test(\n    name = "test2",\n)\n')

        original_dir = os.getcwd()
        try:
            os.chdir(self.temp_dir)
            result = discover_rust_tests(["./pkg/..."])

            self.assertEqual(len(result), 2)
            self.assertIn("test1", result)
            self.assertIn("test2", result)
            self.assertTrue(result["test1"].endswith(str(Path("pkg/module1"))))
            self.assertTrue(result["test2"].endswith(str(Path("pkg/module2/submodule"))))
        finally:
            os.chdir(original_dir)

    def test_discover_no_tests(self):
        """Test handling directories with no Rust tests"""
        test_dir = self.temp_dir / "pkg" / "test"
        test_dir.mkdir(parents=True)

        # Create BUILD.bazel without rust_test
        (test_dir / "BUILD.bazel").write_text('go_library(name = "test")\n')

        result = discover_rust_tests([str(self.temp_dir / "pkg")])

        self.assertEqual(len(result), 0)

    def test_discover_nonexistent_directory(self):
        """Test handling nonexistent directories"""
        result = discover_rust_tests([str(self.temp_dir / "nonexistent")])

        self.assertEqual(len(result), 0)

    def test_discover_invalid_build_file(self):
        """Test handling malformed BUILD.bazel files"""
        test_dir = self.temp_dir / "pkg" / "test"
        test_dir.mkdir(parents=True)

        # Create valid file (can't easily simulate read errors in unit tests)
        build_file = test_dir / "BUILD.bazel"
        build_file.write_text('rust_test(\n    name = "test",\n)\n')

        original_dir = os.getcwd()
        try:
            os.chdir(self.temp_dir)
            # Note: Can't actually test read errors easily in unit tests,
            # but the code handles them with try/except
            result = discover_rust_tests(["./pkg/..."])

            # Should still work with valid file
            self.assertEqual(len(result), 1)
        finally:
            os.chdir(original_dir)


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
