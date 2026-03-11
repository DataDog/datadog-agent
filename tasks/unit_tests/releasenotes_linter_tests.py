import os
import shutil
import tempfile
import unittest

from tasks.libs.linter.releasenotes import (
    CHANGELOG_SECTIONS,
    LintError,
    ReleasenoteError,
    ReleasenoteFileResult,
    lint_releasenote_file,
    lint_releasenotes,
    validate_fragment_structure,
)


class TestValidateFragmentStructure(unittest.TestCase):
    """Tests for the validate_fragment_structure function."""

    def test_valid_structure(self):
        """Valid structure should have no errors."""
        content = {
            'features': ['Feature 1', 'Feature 2'],
            'fixes': ['Fix 1'],
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 0)

    def test_unknown_section(self):
        """Unknown sections should be detected."""
        content = {
            'features': ['Feature 1'],
            'unknown_section': ['Something'],
        }
        errors = validate_fragment_structure(content)
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
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(len(errors[0].errors), 2)

    def test_not_a_dict(self):
        """Non-dict content should be detected."""
        content = ['not', 'a', 'dict']
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertIn('must be a YAML mapping', errors[0].errors[0].message)

    def test_section_not_a_list(self):
        """Section that is not a list should be detected."""
        content = {
            'features': 'not a list',
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertIn('must be a list', errors[0].errors[0].message)

    def test_section_item_not_string(self):
        """Section items that are not strings should be detected."""
        content = {
            'features': ['Valid string', 123, {'nested': 'dict'}],
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(len(errors[0].errors), 2)

    def test_empty_section_warning(self):
        """Empty (null) sections should produce a warning."""
        content = {
            'features': None,
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].errors[0].level, 'warning')

    def test_empty_string_item_warning(self):
        """Empty string items should produce a warning."""
        content = {
            'features': ['Valid', '   ', ''],
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(len(errors[0].errors), 2)

    def test_all_sections_valid(self):
        """All known sections should be accepted."""
        content = {section: [f'Content for {section}'] for section in CHANGELOG_SECTIONS if section != 'prelude'}
        content['prelude'] = 'This is a prelude string'
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 0)

    def test_prelude_as_string_valid(self):
        """Prelude section with string content should be valid."""
        content = {
            'prelude': 'This is a valid prelude string.',
            'features': ['Feature 1'],
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 0)

    def test_prelude_as_list_invalid(self):
        """Prelude section with list content should be invalid."""
        content = {
            'prelude': ['Should not be a list'],
            'features': ['Feature 1'],
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'prelude')
        self.assertIn('must be a string', errors[0].errors[0].message)

    def test_prelude_empty_string_warning(self):
        """Empty prelude string should produce a warning."""
        content = {'prelude': '   '}
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'prelude')
        self.assertEqual(errors[0].errors[0].level, 'warning')

    def test_prelude_non_string_type_error(self):
        """Prelude section with non-string type should be an error."""
        content = {'prelude': 123}
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'prelude')
        self.assertEqual(errors[0].errors[0].level, 'error')
        self.assertIn('must be a string, got int', errors[0].errors[0].message)

    def test_empty_list_section_warning(self):
        """Empty list in a section should produce a warning."""
        content = {'features': []}
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0].section, 'features')
        self.assertEqual(errors[0].errors[0].level, 'warning')
        self.assertIn('empty list', errors[0].errors[0].message)

    def test_markdown_content_accepted(self):
        """Markdown content should be accepted without errors."""
        content = {
            'features': [
                'Added support for [new feature](https://docs.example.com).',
                'Fixed `some_function()` to handle edge cases correctly.',
            ],
            'fixes': ['Resolved **critical** bug in `old_module`.'],
        }
        errors = validate_fragment_structure(content)
        self.assertEqual(len(errors), 0)


class TestLintError(unittest.TestCase):
    """Tests for the LintError class."""

    def test_repr_with_line(self):
        """Repr should include line number when available."""
        error = LintError(line=5, level='warning', message='Test message')
        repr_str = repr(error)
        self.assertIn('Line 5', repr_str)
        self.assertIn('WARNING', repr_str)
        self.assertIn('Test message', repr_str)

    def test_repr_without_line(self):
        """Repr should handle missing line number."""
        error = LintError(line=None, level='error', message='Test message')
        repr_str = repr(error)
        self.assertIn('Unknown line', repr_str)
        self.assertIn('ERROR', repr_str)


class TestLintReleasenoteFile(unittest.TestCase):
    """Tests for the lint_releasenote_file function."""

    def setUp(self):
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.temp_dir)

    def _write_temp_file(self, content: str) -> str:
        path = os.path.join(self.temp_dir, 'test_note.yaml')
        with open(path, 'w', encoding='utf-8') as f:
            f.write(content)
        return path

    def test_valid_releasenote(self):
        """Valid release note with Markdown content should have no errors."""
        content = """---
features:
  - |
    Added support for [new feature](https://docs.example.com) in the agent.
fixes:
  - |
    Fixed a bug in the `old_feature` handling.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertFalse(result.has_errors)

    def test_empty_file(self):
        """Empty file should have no errors."""
        path = self._write_temp_file("")
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

    def test_markdown_content_accepted(self):
        """Markdown content (links, code, bold) should be accepted."""
        content = """---
features:
  - |
    Check [this link](https://example.com) for more info.
    Use `code_here` inline or **bold text** as needed.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertFalse(result.has_errors)

    def test_unknown_section_error(self):
        """Unknown sections should be detected."""
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
        self.assertTrue(any('Unknown section' in e.message for se in result.section_errors for e in se.errors))

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
        path = self._write_temp_file("true")
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
invalid_section:
  - Something.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        output = result.format_output()
        self.assertIn(path, output)
        self.assertIn('[structure]', output)

    def test_format_output_empty_when_no_errors(self):
        """Format output should be empty when no errors."""
        content = """---
features:
  - |
    Valid Markdown content here.
"""
        path = self._write_temp_file(content)
        result = lint_releasenote_file(path)
        self.assertEqual(result.format_output(), "")


class TestLintReleasenotes(unittest.TestCase):
    """Tests for the lint_releasenotes function."""

    def setUp(self):
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.temp_dir)

    def _write_temp_file(self, name: str, content: str) -> str:
        path = os.path.join(self.temp_dir, name)
        with open(path, 'w', encoding='utf-8') as f:
            f.write(content)
        return path

    def test_multiple_files_some_with_errors(self):
        """Should return only files with errors in the errors list."""
        valid_content = """---
features:
  - |
    Valid Markdown content.
"""
        invalid_content = """---
invalid_section:
  - Something.
"""
        valid_path = self._write_temp_file('valid.yaml', valid_content)
        invalid_path = self._write_temp_file('invalid.yaml', invalid_content)

        errors, warnings = lint_releasenotes([valid_path, invalid_path])

        self.assertEqual(len(errors), 1)
        self.assertIn('invalid.yaml', errors[0].file_path)
        self.assertEqual(len(warnings), 0)

    def test_warning_only_files_not_in_errors(self):
        """Files with only warnings should appear in warnings, not errors."""
        warning_content = """---
features:
  - ""
"""
        path = self._write_temp_file('warn.yaml', warning_content)

        errors, warnings = lint_releasenotes([path])

        self.assertEqual(len(errors), 0)
        self.assertEqual(len(warnings), 1)
        self.assertIn('warn.yaml', warnings[0].file_path)

    def test_empty_file_list(self):
        """Empty file list should return empty results."""
        errors, warnings = lint_releasenotes([])
        self.assertEqual(len(errors), 0)
        self.assertEqual(len(warnings), 0)

    def test_all_valid_files(self):
        """All valid files should return empty results."""
        content = """---
features:
  - |
    Valid Markdown content.
"""
        path1 = self._write_temp_file('file1.yaml', content)
        path2 = self._write_temp_file('file2.yaml', content)

        errors, warnings = lint_releasenotes([path1, path2])
        self.assertEqual(len(errors), 0)
        self.assertEqual(len(warnings), 0)


class TestReleasenoteFileResult(unittest.TestCase):
    """Tests for ReleasenoteFileResult class."""

    def test_has_errors_true(self):
        """has_errors should be True when there are error-level entries."""
        result = ReleasenoteFileResult(
            file_path='test.yaml',
            section_errors=[
                ReleasenoteError(section='features', errors=[LintError(line=1, level='error', message='test')])
            ],
        )
        self.assertTrue(result.has_errors)

    def test_has_errors_false_no_entries(self):
        """has_errors should be False when there are no section errors."""
        result = ReleasenoteFileResult(file_path='test.yaml', section_errors=[])
        self.assertFalse(result.has_errors)

    def test_has_errors_false_warnings_only(self):
        """has_errors should be False when section errors contain only warnings."""
        result = ReleasenoteFileResult(
            file_path='test.yaml',
            section_errors=[
                ReleasenoteError(section='features', errors=[LintError(line=1, level='warning', message='test')])
            ],
        )
        self.assertFalse(result.has_errors)

    def test_has_warnings_true(self):
        """has_warnings should be True when there are warning-level entries."""
        result = ReleasenoteFileResult(
            file_path='test.yaml',
            section_errors=[
                ReleasenoteError(section='features', errors=[LintError(line=1, level='warning', message='test')])
            ],
        )
        self.assertTrue(result.has_warnings)

    def test_has_warnings_false(self):
        """has_warnings should be False when there are no warnings."""
        result = ReleasenoteFileResult(file_path='test.yaml', section_errors=[])
        self.assertFalse(result.has_warnings)


if __name__ == '__main__':
    unittest.main()
