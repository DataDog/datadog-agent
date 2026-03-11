"""Linting-related tasks for release note validation.

This module validates that release notes in YAML files follow the expected
structure: known sections, correct types, and non-empty content.
Content is written in Markdown; no RST validation is performed.
"""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

import yaml

from tasks.libs.releasing.notes import CHANGELOG_SECTIONS as _ASSEMBLER_SECTIONS

if TYPE_CHECKING:
    from collections.abc import Iterable


# Known sections that can appear in a fragment.
# Derived from the assembler's CHANGELOG_SECTIONS to ensure the linter and
# assembler stay in sync. 'prelude' is handled separately during assembly.
CHANGELOG_SECTIONS = frozenset(key for key, _ in _ASSEMBLER_SECTIONS) | {"prelude"}


class LintError:
    """Represents a single linting error."""

    def __init__(self, line: int | None, level: str, message: str):
        self.line = line
        self.level = level
        self.message = message

    def __repr__(self) -> str:
        line_str = f"Line {self.line}" if self.line is not None else "Unknown line"
        return f"{line_str}: ({self.level.upper()}) {self.message}"


class ReleasenoteError:
    """Represents errors found in a release note section."""

    def __init__(self, section: str, errors: list[LintError]):
        self.section = section
        self.errors = errors


class ReleasenoteFileResult:
    """Represents all linting results for a single release note file."""

    def __init__(self, file_path: str, section_errors: list[ReleasenoteError]):
        self.file_path = file_path
        self.section_errors = section_errors

    @property
    def has_errors(self) -> bool:
        return any(
            e.level == "error"
            for section_error in self.section_errors
            for e in section_error.errors
        )

    @property
    def has_warnings(self) -> bool:
        return any(
            e.level == "warning"
            for section_error in self.section_errors
            for e in section_error.errors
        )

    def format_output(self) -> str:
        """Format errors and warnings for display."""
        if not self.section_errors:
            return ""

        lines = [f"{self.file_path}:"]
        for section_error in self.section_errors:
            for error in section_error.errors:
                line_str = (
                    f"Line {error.line}" if error.line is not None else "Unknown line"
                )
                lines.append(
                    f"  [{section_error.section}] {line_str}: ({error.level.upper()}) {error.message}"
                )
        return "\n".join(lines)


def validate_fragment_structure(content: dict) -> list[ReleasenoteError]:
    """Validate that the YAML content follows the expected fragment format.

    Checks:
    - Content is a dict
    - Only known sections are present
    - Each section contains a list of strings (or a string for 'prelude')
    - No empty sections
    """
    errors = []

    if not isinstance(content, dict):
        return [
            ReleasenoteError(
                section="yaml",
                errors=[
                    LintError(
                        line=None,
                        level="error",
                        message=f"Release note must be a YAML mapping, got {type(content).__name__}",
                    )
                ],
            )
        ]

    unknown_sections = set(content.keys()) - CHANGELOG_SECTIONS
    if unknown_sections:
        section_errors = [
            LintError(
                line=None,
                level="error",
                message=f"Unknown section '{s}'. Valid sections: {', '.join(sorted(CHANGELOG_SECTIONS))}",
            )
            for s in sorted(unknown_sections)
        ]
        errors.append(ReleasenoteError(section="structure", errors=section_errors))

    for section, section_content in content.items():
        if section not in CHANGELOG_SECTIONS:
            continue

        section_errors = []

        if section_content is None:
            section_errors.append(
                LintError(
                    line=None,
                    level="warning",
                    message=f"Section '{section}' is empty (null). Remove it or add content.",
                )
            )
        elif section == "prelude":
            if not isinstance(section_content, str):
                section_errors.append(
                    LintError(
                        line=None,
                        level="error",
                        message=f"Section 'prelude' must be a string, got {type(section_content).__name__}",
                    )
                )
            elif not section_content.strip():
                section_errors.append(
                    LintError(
                        line=None,
                        level="warning",
                        message="Section 'prelude' is empty or whitespace-only",
                    )
                )
        elif not isinstance(section_content, list):
            section_errors.append(
                LintError(
                    line=None,
                    level="error",
                    message=f"Section '{section}' must be a list of strings, got {type(section_content).__name__}",
                )
            )
        elif not section_content:
            section_errors.append(
                LintError(
                    line=None,
                    level="warning",
                    message=f"Section '{section}' is an empty list. Remove it or add content.",
                )
            )
        else:
            for i, item in enumerate(section_content):
                if not isinstance(item, str):
                    section_errors.append(
                        LintError(
                            line=None,
                            level="error",
                            message=f"Item {i} in section '{section}' must be a string, got {type(item).__name__}",
                        )
                    )
                elif not item.strip():
                    section_errors.append(
                        LintError(
                            line=None,
                            level="warning",
                            message=f"Item {i} in section '{section}' is empty or whitespace-only",
                        )
                    )

        if section_errors:
            errors.append(ReleasenoteError(section=section, errors=section_errors))

    return errors


def lint_releasenote_file(file_path: str | Path) -> ReleasenoteFileResult:
    """Lint a single release note YAML file.

    Parses the YAML file and validates the structure: known sections,
    correct types, non-empty content. Content is treated as Markdown.
    """
    file_path = Path(file_path)
    section_errors: list[ReleasenoteError] = []

    try:
        with open(file_path, encoding="utf-8") as f:
            content = yaml.safe_load(f)
    except yaml.YAMLError as e:
        section_errors.append(
            ReleasenoteError(
                section="yaml",
                errors=[
                    LintError(
                        line=None, level="error", message=f"YAML parsing error: {e}"
                    )
                ],
            )
        )
        return ReleasenoteFileResult(
            file_path=str(file_path), section_errors=section_errors
        )
    except OSError as e:
        section_errors.append(
            ReleasenoteError(
                section="file",
                errors=[
                    LintError(line=None, level="error", message=f"File read error: {e}")
                ],
            )
        )
        return ReleasenoteFileResult(
            file_path=str(file_path), section_errors=section_errors
        )

    if content is None:
        return ReleasenoteFileResult(
            file_path=str(file_path), section_errors=section_errors
        )

    structure_errors = validate_fragment_structure(content)
    section_errors.extend(structure_errors)

    return ReleasenoteFileResult(
        file_path=str(file_path), section_errors=section_errors
    )


def lint_releasenotes(
    files: Iterable[str | Path],
) -> tuple[list[ReleasenoteFileResult], list[ReleasenoteFileResult]]:
    """Lint multiple release note files.

    Returns a tuple of (errors, warnings) where each is a list of
    ReleasenoteFileResult objects for files with errors or warnings respectively.
    Files that have only warnings are not included in the errors list.
    """
    errors = []
    warnings = []
    for file_path in files:
        result = lint_releasenote_file(file_path)
        if result.has_errors:
            errors.append(result)
        elif result.has_warnings:
            warnings.append(result)
    return errors, warnings
