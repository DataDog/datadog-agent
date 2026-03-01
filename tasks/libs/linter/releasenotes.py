"""Linting-related tasks for release notes RST validation.

This module validates that release notes in YAML files use proper
reStructuredText (RST) formatting instead of Markdown, using docutils
as the reference RST parser, plus additional checks for common Markdown
patterns and reno format compliance.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import TYPE_CHECKING

import yaml

if TYPE_CHECKING:
    from collections.abc import Iterable


# Reno filenames must end with a 16-character lowercase hex UID: <slug>-<uid>.yaml
# See: https://docs.openstack.org/reno/latest/user/design.html
RENO_FILENAME_RE = re.compile(r'^.+-[0-9a-f]{16}\.yaml$')

# Known reno sections that can contain RST content
# See: https://docs.openstack.org/reno/latest/user/usage.html#editing-a-release-note
RENO_SECTIONS = frozenset(
    [
        'features',
        'issues',
        'upgrade',
        'deprecations',
        'critical',
        'security',
        'fixes',
        'other',
        'enhancements',
        'known_issues',
        'prelude',
    ]
)

# Markdown patterns that indicate contributor confusion
# Each tuple contains (pattern, description)
MARKDOWN_PATTERNS = [
    # Markdown links: [text](url) - exclude image syntax with negative lookbehind
    (re.compile(r'(?<!!)\[([^\]]+)\]\(([^)]+)\)'), 'Markdown link syntax. Use RST: `{0} <{1}>`_'),
    # Markdown bold with double underscores: __text__
    # Note: **text** is the same in both Markdown and RST, so we don't flag it
    (re.compile(r'__([^_]+)__'), 'Markdown bold syntax. Use RST: **{0}**'),
    # Markdown italic with underscores: _text_
    # Must have whitespace/punctuation before and after to avoid matching snake_case identifiers
    # Only flag when clearly used as italic markers (surrounded by whitespace or at boundaries)
    (re.compile(r'(?<=\s)_([^_\s][^_]*)_(?=\s|[.,;:!?\)]|$)'), 'Markdown italic syntax. Use RST: *{0}*'),
    # Markdown headers: # Title, ## Title, etc.
    (re.compile(r'^#{1,6}\s+.+$', re.MULTILINE), 'Markdown header syntax. Use RST title underlines instead'),
    # Markdown code blocks: ```lang or ``` (at end of line, since we process line by line)
    (re.compile(r'^```\w*$'), 'Markdown code block syntax. Use RST: .. code-block:: <lang>'),
    # Markdown horizontal rule: --- or *** or ___ (at start of line, 3+ chars)
    (re.compile(r'^[-*_]{3,}\s*$', re.MULTILINE), 'Markdown horizontal rule. Not typically needed in release notes'),
    # Markdown images: ![alt](url)
    (re.compile(r'!\[([^\]]*)\]\(([^)]+)\)'), 'Markdown image syntax. Use RST: .. image:: {1}'),
    # Markdown blockquote: > text (at start of line)
    (re.compile(r'^>\s+.+$', re.MULTILINE), 'Markdown blockquote. Use RST indentation or .. note:: directive'),
    # Single backticks for inline code (Markdown style)
    # In RST, inline code uses double backticks: ``code``
    # Single backticks in RST are for interpreted text (roles, references, links)
    # Exclude:
    # - RST double backticks: ``code`` (negative lookbehind/lookahead for `)
    # - RST links: `text <url>`_ (content has < or >)
    # - RST references: `text`_ (followed by _)
    (re.compile(r'(?<!`)`([^`<>]+)`(?![_`])'), 'Markdown inline code. Use RST double backticks: ``{0}``'),
]


class RSTLintError:
    """Represents a single RST linting error."""

    def __init__(self, line: int | None, level: str, message: str):
        self.line = line
        self.level = level
        self.message = message

    def __repr__(self) -> str:
        line_str = f"Line {self.line}" if self.line else "Unknown line"
        return f"{line_str}: ({self.level.upper()}) {self.message}"


class ReleasenoteError:
    """Represents errors found in a release note section."""

    def __init__(self, section: str, errors: list[RSTLintError]):
        self.section = section
        self.errors = errors


class ReleasenoteFileResult:
    """Represents all linting results for a single release note file."""

    def __init__(self, file_path: str, section_errors: list[ReleasenoteError]):
        self.file_path = file_path
        self.section_errors = section_errors

    @property
    def has_errors(self) -> bool:
        return len(self.section_errors) > 0

    def format_output(self) -> str:
        """Format errors for display."""
        if not self.has_errors:
            return ""

        lines = [f"{self.file_path}:"]
        for section_error in self.section_errors:
            for error in section_error.errors:
                line_str = f"Line {error.line}" if error.line else "Unknown line"
                lines.append(f"  [{section_error.section}] {line_str}: ({error.level.upper()}) {error.message}")
        return "\n".join(lines)


def detect_markdown_patterns(text: str) -> list[RSTLintError]:
    """Detect common Markdown patterns that indicate contributor confusion.

    Args:
        text: The text to check for Markdown patterns.

    Returns:
        A list of RSTLintError objects for detected Markdown patterns.
    """
    if not text or not text.strip():
        return []

    errors = []
    lines = text.split('\n')

    for pattern, description in MARKDOWN_PATTERNS:
        for line_num, line in enumerate(lines, start=1):
            for match in pattern.finditer(line):
                # Format the description with captured groups if available
                groups = match.groups()
                if groups:
                    try:
                        formatted_desc = description.format(*groups)
                    except (IndexError, KeyError):
                        formatted_desc = description
                else:
                    formatted_desc = description

                errors.append(
                    RSTLintError(
                        line=line_num,
                        level='error',
                        message=f'Markdown detected: {formatted_desc}',
                    )
                )

    return errors


def validate_rst(text: str) -> list[RSTLintError]:
    """Parse RST text and return list of errors found.

    Uses docutils to parse RST content and collect all parser warnings
    and errors as RSTLintError objects with line numbers.

    Args:
        text: The RST text to validate.

    Returns:
        A list of RSTLintError objects representing parsing issues.
    """
    if not text or not text.strip():
        return []

    errors = []

    # First check for Markdown patterns
    errors.extend(detect_markdown_patterns(text))

    # Lazy import docutils to avoid import errors when not installed
    try:
        from io import StringIO

        from docutils.core import publish_parts  # type: ignore[import-untyped]
        from docutils.utils import Reporter  # type: ignore[import-untyped]
    except ImportError:
        # docutils not installed, skip RST validation but keep Markdown detection
        return errors

    # Capture docutils warnings/errors via warning_stream
    # Using publish_parts with full processing to detect unresolved references
    warning_stream = StringIO()

    try:
        publish_parts(
            text,
            writer_name='html',
            settings_overrides={
                'report_level': Reporter.INFO_LEVEL,  # Capture INFO and above (for title underline issues)
                'halt_level': Reporter.SEVERE_LEVEL + 1,  # Never halt, collect all errors
                'warning_stream': warning_stream,
            },
        )
    except Exception as e:
        # If parsing completely fails, return a single error
        errors.append(RSTLintError(line=None, level='error', message=str(e)))
        return errors

    # Parse the warning stream for errors
    # Formats:
    #   <string>:LINE: (LEVEL/NUM) Message  (with line number)
    #   <string>: (LEVEL/NUM) Message       (without line number)
    #   /path/to/file:LINE: (LEVEL/NUM) Message  (with real path, e.g. from .. include::)
    warning_output = warning_stream.getvalue()
    if warning_output:
        level_map = {'INFO': 'info', 'WARNING': 'warning', 'ERROR': 'error', 'SEVERE': 'severe'}
        # Regex: source:optional_line: (LEVEL/NUM) message
        warning_pattern = re.compile(r'^[^:]+:(?:(\d+):)? \(([A-Z]+)/\d+\) (.+)')
        for line in warning_output.strip().split('\n'):
            if not line:
                continue
            match = warning_pattern.match(line)
            if match:
                line_num = int(match.group(1)) if match.group(1) else None
                level_str = match.group(2)
                message = match.group(3)
                level = level_map.get(level_str, 'warning')
                errors.append(RSTLintError(line=line_num, level=level, message=message))

    return errors


def validate_reno_structure(content: dict, file_path: str) -> list[ReleasenoteError]:
    """Validate that the YAML content follows reno format.

    Checks:
    - Only known reno sections are present
    - Each section contains a list of strings
    - No empty sections

    Args:
        content: The parsed YAML content.
        file_path: Path to the file (for error messages).

    Returns:
        A list of ReleasenoteError objects for structure issues.
    """
    errors = []

    if not isinstance(content, dict):
        return [
            ReleasenoteError(
                section='yaml',
                errors=[
                    RSTLintError(
                        line=None,
                        level='error',
                        message=f'Release note must be a YAML mapping, got {type(content).__name__}',
                    )
                ],
            )
        ]

    # Check for unknown sections
    unknown_sections = set(content.keys()) - RENO_SECTIONS
    if unknown_sections:
        section_errors = []
        for section in sorted(unknown_sections):
            section_errors.append(
                RSTLintError(
                    line=None,
                    level='error',
                    message=f"Unknown reno section '{section}'. Valid sections: {', '.join(sorted(RENO_SECTIONS))}",
                )
            )
        errors.append(ReleasenoteError(section='structure', errors=section_errors))

    # Check section content structure
    for section, section_content in content.items():
        if section not in RENO_SECTIONS:
            continue  # Already reported as unknown

        section_errors = []

        if section_content is None:
            section_errors.append(
                RSTLintError(
                    line=None,
                    level='warning',
                    message=f"Section '{section}' is empty (null). Remove it or add content.",
                )
            )
        elif section == 'prelude':
            # Prelude section contains a string, not a list
            if not isinstance(section_content, str):
                section_errors.append(
                    RSTLintError(
                        line=None,
                        level='error',
                        message=f"Section 'prelude' must be a string, got {type(section_content).__name__}",
                    )
                )
            elif not section_content.strip():
                section_errors.append(
                    RSTLintError(
                        line=None,
                        level='warning',
                        message="Section 'prelude' is empty or whitespace-only",
                    )
                )
        elif not isinstance(section_content, list):
            section_errors.append(
                RSTLintError(
                    line=None,
                    level='error',
                    message=f"Section '{section}' must be a list of strings, got {type(section_content).__name__}",
                )
            )
        else:
            for i, item in enumerate(section_content):
                if not isinstance(item, str):
                    section_errors.append(
                        RSTLintError(
                            line=None,
                            level='error',
                            message=f"Item {i} in section '{section}' must be a string, got {type(item).__name__}",
                        )
                    )
                elif not item.strip():
                    section_errors.append(
                        RSTLintError(
                            line=None,
                            level='warning',
                            message=f"Item {i} in section '{section}' is empty or whitespace-only",
                        )
                    )

        if section_errors:
            errors.append(ReleasenoteError(section=section, errors=section_errors))

    return errors


def lint_releasenote_file(file_path: str | Path) -> ReleasenoteFileResult:
    """Lint a single release note YAML file for RST formatting issues.

    Parses the YAML file, validates reno structure, extracts text content
    from each section, and validates RST formatting.

    Args:
        file_path: Path to the release note YAML file.

    Returns:
        A ReleasenoteFileResult containing any errors found.
    """
    file_path = Path(file_path)

    section_errors: list[ReleasenoteError] = []

    # Validate filename UID format for files under a notes/ directory
    if 'notes' in file_path.parts and not RENO_FILENAME_RE.match(file_path.name):
        section_errors.append(
            ReleasenoteError(
                section='filename',
                errors=[
                    RSTLintError(
                        line=None,
                        level='error',
                        message=f"Filename '{file_path.name}' does not match reno convention '<slug>-<16 hex chars>.yaml'",
                    )
                ],
            )
        )

    try:
        with open(file_path, encoding='utf-8') as f:
            content = yaml.safe_load(f)
    except yaml.YAMLError as e:
        section_errors.append(
            ReleasenoteError(
                section='yaml', errors=[RSTLintError(line=None, level='error', message=f'YAML parsing error: {e}')]
            )
        )
        return ReleasenoteFileResult(file_path=str(file_path), section_errors=section_errors)
    except OSError as e:
        section_errors.append(
            ReleasenoteError(
                section='file', errors=[RSTLintError(line=None, level='error', message=f'File read error: {e}')]
            )
        )
        return ReleasenoteFileResult(file_path=str(file_path), section_errors=section_errors)

    if content is None:
        # Empty file or only comments
        return ReleasenoteFileResult(file_path=str(file_path), section_errors=section_errors)

    # Validate reno structure first
    structure_errors = validate_reno_structure(content, str(file_path))
    section_errors.extend(structure_errors)

    # If content is not a dict, we can't iterate over sections - return early
    # (the structure error was already reported above)
    if not isinstance(content, dict):
        return ReleasenoteFileResult(file_path=str(file_path), section_errors=section_errors)

    # Validate RST content in each known section
    for section in RENO_SECTIONS:
        if section not in content:
            continue

        section_content = content[section]
        if not section_content:
            continue

        # Section content is a list of strings (RST text)
        if isinstance(section_content, list):
            for i, item in enumerate(section_content):
                if isinstance(item, str):
                    errors = validate_rst(item)
                    if errors:
                        # Adjust section name to include item index if multiple items
                        section_name = section if len(section_content) == 1 else f"{section}[{i}]"
                        section_errors.append(ReleasenoteError(section=section_name, errors=errors))
        elif isinstance(section_content, str):
            errors = validate_rst(section_content)
            if errors:
                section_errors.append(ReleasenoteError(section=section, errors=errors))

    return ReleasenoteFileResult(file_path=str(file_path), section_errors=section_errors)


def lint_releasenotes(files: Iterable[str | Path]) -> list[ReleasenoteFileResult]:
    """Lint multiple release note files for RST formatting issues.

    Args:
        files: Iterable of file paths to lint.

    Returns:
        A list of ReleasenoteFileResult objects, one per file.
    """
    results = []
    for file_path in files:
        result = lint_releasenote_file(file_path)
        if result.has_errors:
            results.append(result)
    return results
