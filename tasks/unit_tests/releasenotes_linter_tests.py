import importlib.util
import os
import tempfile
import unittest

from tasks.libs.linter.releasenotes import (
    RENO_SECTIONS,
    ReleasenoteFileResult,
    RSTLintError,
    detect_markdown_patterns,
    lint_releasenote_file,
    lint_releasenotes,
    validate_reno_structure,
    validate_rst,
)

# Check if docutils is available for tests that depend on it
# Use find_spec to avoid importing the module (which would trigger mypy errors about missing stubs)
DOCUTILS_AVAILABLE = importlib.util.find_spec("docutils") is not None


class TestValidateRST(unittest.TestCase):
    """Tests for the validate_rst function."""

    def test_valid_rst_no_errors(self):
        """Valid RST should return no errors."""
        rst = """
This is valid RST with ``inline code`` and a
`link <https://example.com>`_.
"""
        errors = validate_rst(rst)
        self.assertEqual(len(errors), 0)

    def test_valid_rst_bullet_list(self):
        """Valid RST bullet lists should return no errors."""
        rst = """
- First item
- Second item with ``code``
- Third item
"""
        errors = validate_rst(rst)
        self.assertEqual(len(errors), 0)

    def test_empty_string_no_errors(self):
        """Empty string should return no errors."""
        errors = validate_rst("")
        self.assertEqual(len(errors), 0)

    def test_whitespace_only_no_errors(self):
        """Whitespace-only string should return no errors."""
        errors = validate_rst("   \n\t  \n  ")
        self.assertEqual(len(errors), 0)

    @unittest.skipUnless(DOCUTILS_AVAILABLE, "docutils not installed")
    def test_unknown_target_reference(self):
        """Unknown reference targets should be detected."""
        rst = "See the `config`_ section for details."
        errors = validate_rst(rst)
        # Should detect unknown target
        self.assertGreater(len(errors), 0)

    @unittest.skipUnless(DOCUTILS_AVAILABLE, "docutils not installed")
    def test_title_underline_mismatch(self):
        """Title with mismatched underline should be detected."""
        rst = """
Title
===

Some content.
"""
        errors = validate_rst(rst)
        # docutils should flag the underline being too short
        self.assertGreater(len(errors), 0)

    def test_valid_rst_directive(self):
        """Valid RST directives should not produce errors."""
        rst = """
.. note::

   This is a note directive.
"""
        errors = validate_rst(rst)
        self.assertEqual(len(errors), 0)

    @unittest.skipUnless(DOCUTILS_AVAILABLE, "docutils not installed")
    def test_error_contains_line_number(self):
        """Errors should contain line number information when available."""
        rst = """Line one
`broken reference`_
Line three
"""
        errors = validate_rst(rst)
        self.assertGreater(len(errors), 0)
        # At least one error should have a line number
        has_line = any(e.line is not None for e in errors)
        self.assertTrue(has_line)


class TestDetectMarkdownPatterns(unittest.TestCase):
    """Tests for Markdown pattern detection."""

    def test_markdown_link(self):
        """Markdown links should be detected."""
        text = "Check [this link](https://example.com) for more info"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertIn('Markdown link syntax', errors[0].message)
        self.assertIn('`this link <https://example.com>`_', errors[0].message)

    def test_markdown_image(self):
        """Markdown images should be detected."""
        text = "![alt text](image.png)"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertIn('Markdown image syntax', errors[0].message)

    def test_markdown_bold_asterisks_not_flagged(self):
        """Markdown bold with asterisks (**text**) is same in RST, should not be flagged."""
        text = "This is **bold** text"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 0)

    def test_markdown_bold_underscores(self):
        """Markdown bold with underscores should be detected."""
        text = "This is __bold__ text"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertIn('Markdown bold syntax', errors[0].message)

    def test_markdown_italic_underscores(self):
        """Markdown italic with underscores should be detected."""
        text = "This is _italic_ text"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertIn('Markdown italic syntax', errors[0].message)

    def test_snake_case_not_flagged(self):
        """Snake_case identifiers should not be flagged as Markdown italic."""
        text = "Use ``datadog_agent.obfuscate_sql_exec_plan`` function"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 0)

    def test_markdown_header(self):
        """Markdown headers should be detected."""
        text = "# This is a header"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertIn('Markdown header syntax', errors[0].message)

    def test_markdown_header_level2(self):
        """Markdown level 2 headers should be detected."""
        text = "## This is a level 2 header"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)

    def test_markdown_code_block(self):
        """Markdown code blocks should be detected."""
        text = "```python\nprint('hello')\n```"
        errors = detect_markdown_patterns(text)
        self.assertGreater(len(errors), 0)
        self.assertTrue(any('Markdown code block' in e.message for e in errors))

    def test_markdown_blockquote(self):
        """Markdown blockquotes should be detected."""
        text = "> This is a quote"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertIn('Markdown blockquote', errors[0].message)

    def test_single_backticks_inline_code(self):
        """Single backticks for inline code should be detected."""
        text = "Use `code` here"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertIn('Markdown inline code', errors[0].message)
        self.assertIn('``code``', errors[0].message)

    def test_double_backticks_not_flagged(self):
        """RST double backticks should not be flagged."""
        text = "Use ``code`` here"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 0)

    def test_rst_link_not_flagged(self):
        """RST links should not be flagged."""
        text = "See `the documentation <https://example.com>`_ for details"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 0)

    def test_rst_reference_not_flagged(self):
        """RST references should not be flagged."""
        text = "See `config`_ for details"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 0)

    def test_valid_rst_not_flagged(self):
        """Valid RST should not be flagged as Markdown."""
        text = "Use ``code`` and `link <https://example.com>`_ here"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 0)

    def test_rst_bold_not_flagged(self):
        """RST bold (**text**) is same as Markdown, should not be flagged."""
        # **bold** is valid in both RST and Markdown with same meaning
        text = "This is **bold** in both formats"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 0)

    def test_multiple_patterns_same_line(self):
        """Multiple Markdown patterns on same line should all be detected."""
        text = "Check [link](url) and __bold__"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 2)

    def test_line_numbers_correct(self):
        """Line numbers should be correctly reported."""
        text = "Line 1 is fine\n[link](url) on line 2\nLine 3 is fine"
        errors = detect_markdown_patterns(text)
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].line, 2)


class TestValidateRenoStructure(unittest.TestCase):
    """Tests for reno structure validation."""

    def test_valid_structure(self):
        """Valid reno structure should have no errors."""
        content = {
            'features': ['Feature 1', 'Feature 2'],
            'fixes': ['Fix 1'],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 0)

    def test_unknown_section(self):
        """Unknown sections should be detected."""
        content = {
            'features': ['Feature 1'],
            'unknown_section': ['Something'],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'structure')
        self.assertIn('unknown_section', errors[0].errors[0].message)

    def test_multiple_unknown_sections(self):
        """Multiple unknown sections should all be detected."""
        content = {
            'features': ['Feature 1'],
            'bad_section': ['Something'],
            'another_bad': ['Something else'],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(len(errors[0].errors), 2)

    def test_not_a_dict(self):
        """Non-dict content should be detected."""
        content = ['not', 'a', 'dict']
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertIn('must be a YAML mapping', errors[0].errors[0].message)

    def test_section_not_a_list(self):
        """Section that is not a list should be detected."""
        content = {
            'features': 'not a list',
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertIn('must be a list', errors[0].errors[0].message)

    def test_section_item_not_string(self):
        """Section items that are not strings should be detected."""
        content = {
            'features': ['Valid string', 123, {'nested': 'dict'}],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(len(errors[0].errors), 2)  # Two non-string items

    def test_empty_section_warning(self):
        """Empty (null) sections should produce a warning."""
        content = {
            'features': None,
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].errors[0].level, 'warning')

    def test_empty_string_item_warning(self):
        """Empty string items should produce a warning."""
        content = {
            'features': ['Valid', '   ', ''],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(len(errors[0].errors), 2)  # Two empty items

    def test_all_reno_sections_valid(self):
        """All known reno sections should be accepted."""
        content = {section: [f'Content for {section}'] for section in RENO_SECTIONS if section != 'prelude'}
        # Prelude is a string, not a list
        content['prelude'] = 'This is a prelude string'
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 0)

    def test_prelude_as_string_valid(self):
        """Prelude section with string content should be valid."""
        content = {
            'prelude': 'This is a valid prelude string with content.',
            'features': ['Feature 1'],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 0)

    def test_prelude_as_list_invalid(self):
        """Prelude section with list content should be invalid."""
        content = {
            'prelude': ['This should not be a list'],
            'features': ['Feature 1'],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'prelude')
        self.assertIn('must be a string', errors[0].errors[0].message)

    def test_prelude_empty_string_warning(self):
        """Empty prelude string should produce a warning."""
        content = {
            'prelude': '   ',  # whitespace-only
            'features': ['Feature 1'],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'prelude')
        self.assertEqual(errors[0].errors[0].level, 'warning')
        self.assertIn('empty or whitespace-only', errors[0].errors[0].message)

    def test_prelude_non_string_type_error(self):
        """Prelude section with non-string type should be an error."""
        content = {
            'prelude': 123,  # number instead of string
            'features': ['Feature 1'],
        }
        errors = validate_reno_structure(content, 'test.yaml')
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'prelude')
        self.assertEqual(errors[0].errors[0].level, 'error')
        self.assertIn('must be a string, got int', errors[0].errors[0].message)


class TestRSTLintError(unittest.TestCase):
    """Tests for the RSTLintError class."""

    def test_repr_with_line(self):
        """Repr should include line number when available."""
        error = RSTLintError(line=5, level='warning', message='Test message')
        repr_str = repr(error)
        self.assertIn('Line 5', repr_str)
        self.assertIn('WARNING', repr_str)
        self.assertIn('Test message', repr_str)

    def test_repr_without_line(self):
        """Repr should handle missing line number."""
        error = RSTLintError(line=None, level='error', message='Test message')
        repr_str = repr(error)
        self.assertIn('Unknown line', repr_str)
        self.assertIn('ERROR', repr_str)


class TestLintReleasenoteFile(unittest.TestCase):
    """Tests for the lint_releasenote_file function."""

    def setUp(self):
        """Create a temporary directory for test files."""
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        """Clean up temporary files."""
        import shutil

        shutil.rmtree(self.temp_dir)

    def _write_temp_file(self, content: str) -> str:
        """Write content to a temporary file and return its path."""
        path = os.path.join(self.temp_dir, 'test_note.yaml')
        with open(path, 'w') as f:
            f.write(content)
        return path

    def test_valid_releasenote(self):
        """Valid release note should have no errors."""
        content = """---
features:
  - |
    Add support for ``new_feature`` in the agent.
    See `the documentation <https://docs.example.com>`_ for details.
fixes:
  - |
    Fix a bug in the ``old_feature`` handling.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertFalse(result.has_errors)

    def test_empty_file(self):
        """Empty file should have no errors."""
        content = ""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertFalse(result.has_errors)

    def test_comments_only(self):
        """File with only comments should have no errors."""
        content = """# This is a comment
# Another comment
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertFalse(result.has_errors)

    def test_markdown_link_error(self):
        """Markdown links in release note should be detected."""
        content = """---
features:
  - |
    Check [this link](https://example.com) for more info.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertTrue(result.has_errors)
        self.assertTrue(any('Markdown' in e.message for se in result.section_errors for e in se.errors))

    def test_unknown_section_error(self):
        """Unknown reno sections should be detected."""
        content = """---
features:
  - |
    Valid feature.
invalid_section:
  - |
    This section is not valid.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertTrue(result.has_errors)
        self.assertTrue(any('Unknown reno section' in e.message for se in result.section_errors for e in se.errors))

    def test_multiple_sections_with_errors(self):
        """Errors in multiple sections should all be reported."""
        content = """---
features:
  - |
    Check [link](url) here.
fixes:
  - |
    Fixed [bug](url) here.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertTrue(result.has_errors)
        # Should have errors in both sections
        sections_with_errors = {e.section for e in result.section_errors}
        self.assertIn('features', sections_with_errors)
        self.assertIn('fixes', sections_with_errors)

    def test_multiple_items_in_section(self):
        """Multiple items in a section should be tracked with index."""
        content = """---
features:
  - |
    First feature is valid with ``code``.
  - |
    Second feature has [invalid](link) syntax.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertTrue(result.has_errors)
        # Should have error in second item only
        self.assertEqual(len(result.section_errors), 1)
        self.assertEqual(result.section_errors[0].section, 'features[1]')

    def test_invalid_yaml(self):
        """Invalid YAML should be reported as an error."""
        content = """---
features:
  - |
    Invalid indentation
   - nested
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertTrue(result.has_errors)
        self.assertEqual(result.section_errors[0].section, 'yaml')

    def test_scalar_yaml_content(self):
        """Scalar YAML content (not a dict) should be reported without crashing."""
        # This tests the fix for: TypeError when content is a scalar like True or 123
        content = "true"
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertTrue(result.has_errors)
        self.assertIn('must be a YAML mapping', result.section_errors[0].errors[0].message)

    def test_nonexistent_file(self):
        """Nonexistent file should be reported as an error."""
        result = lint_releasenote_file('/nonexistent/path/file.yaml')
        self.assertTrue(result.has_errors)
        self.assertEqual(result.section_errors[0].section, 'file')

    def test_format_output(self):
        """Format output should produce readable string."""
        content = """---
features:
  - |
    Check [link](url) for settings.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        output = result.format_output()
        self.assertIn(path, output)
        self.assertIn('[features]', output)

    def test_format_output_empty_when_no_errors(self):
        """Format output should be empty when no errors."""
        content = """---
features:
  - |
    Valid ``code`` here.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertEqual(result.format_output(), "")


class TestLintReleasenotes(unittest.TestCase):
    """Tests for the lint_releasenotes function."""

    def setUp(self):
        """Create a temporary directory for test files."""
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        """Clean up temporary files."""
        import shutil

        shutil.rmtree(self.temp_dir)

    def _write_temp_file(self, name: str, content: str) -> str:
        """Write content to a temporary file and return its path."""
        path = os.path.join(self.temp_dir, name)
        with open(path, 'w') as f:
            f.write(content)
        return path

    def test_multiple_files_some_with_errors(self):
        """Should return only files with errors."""
        valid_content = """---
features:
  - |
    Valid ``code`` content.
"""
        invalid_content = """---
features:
  - |
    Invalid [link](url) content.
"""
        valid_path = self._write_temp_file('valid.yaml', valid_content)
        invalid_path = self._write_temp_file('invalid.yaml', invalid_content)

        results = lint_releasenotes([valid_path, invalid_path])

        # Only invalid file should be in results
        self.assertEqual(len(results), 1)
        self.assertIn('invalid.yaml', results[0].file_path)

    def test_empty_file_list(self):
        """Empty file list should return empty results."""
        results = lint_releasenotes([])
        self.assertEqual(len(results), 0)

    def test_all_valid_files(self):
        """All valid files should return empty results."""
        content = """---
features:
  - |
    Valid ``code`` content.
"""
        path1 = self._write_temp_file('file1.yaml', content)
        path2 = self._write_temp_file('file2.yaml', content)

        results = lint_releasenotes([path1, path2])
        self.assertEqual(len(results), 0)


class TestReleasenoteFileResult(unittest.TestCase):
    """Tests for ReleasenoteFileResult class."""

    def test_has_errors_true(self):
        """has_errors should be True when there are section errors."""
        from tasks.libs.linter.releasenotes import ReleasenoteError

        result = ReleasenoteFileResult(
            file_path='test.yaml',
            section_errors=[
                ReleasenoteError(section='features', errors=[RSTLintError(line=1, level='warning', message='test')])
            ],
        )
        self.assertTrue(result.has_errors)

    def test_has_errors_false(self):
        """has_errors should be False when there are no section errors."""
        result = ReleasenoteFileResult(file_path='test.yaml', section_errors=[])
        self.assertFalse(result.has_errors)


if __name__ == '__main__':
    unittest.main()
