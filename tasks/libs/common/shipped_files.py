"""
Detection of changes that cannot impact any Agent delivery (packages or images).
"""

from __future__ import annotations

import re

# Paths that are never used to build an Agent package or image.
# Any path not matched here is considered shipped: only add paths you are sure about,
# a wrong match lets a change skip the changelog and QA checks.
_NON_SHIPPED_PATTERNS = (
    # Tests
    r'_test\.go$',
    r'(^|/)testdata/',
    r'^test/',
    r'^pkg/security/tests/',
    r'^rtloader/test/',
    # Documentation, except the markdown files published along with images or packages
    r'^(?!Dockerfiles/|omnibus/|packages/|packaging/).*\.md$',
    r'^docs?/',
    r'^releasenotes(-dca)?/',
    # CI configuration that only runs tests or linters
    r'^\.github/',
    r'^\.gitlab/(test|build/lint|build/source_test)/',
    r'^\.gitlab/JOBOWNERS$',
    r'^\.ddqa/',
    r'^flakes\.yaml$',
    r'^renovate\.json$',
    # Invoke tasks that only run tests, linters or PR automation
    r'^tasks/unit_tests/',
    r'^tasks/(e2e_framework|kernel_matrix_testing|static_quality_gates)/',
    r'^tasks/libs/dynamic_test/',
    r'^tasks/(dyntest|github_tasks|kmt|linter|new_e2e_tests)\.py$',
    # Developer and AI tooling
    r'^\.(agents|claude|claude-plugins|codex|cursor|devcontainer|opencode|run|vscode|zed)/',
    r'^\.(editorconfig|git-blame-ignore-revs|gitignore|golangci\.yml|lychee\.toml|markdown-link-check|mcp\.json|mockery\.yaml|pre-commit-config\.yaml|wwhrd\.yml)$',
    r'^devenv/',
    r'^tools/(agent_QA|gdb|go-update)/',
)
_NON_SHIPPED_RE = re.compile('|'.join(f'(?:{pattern})' for pattern in _NON_SHIPPED_PATTERNS))


def is_shipped_file(path: str) -> bool:
    """
    Returns True if the file may be used to build an Agent package or image
    """
    return _NON_SHIPPED_RE.search(path) is None


def touches_shipped_files(files: list[str]) -> bool:
    """
    Returns True if any of the files may be used to build an Agent package or image
    """
    return any(is_shipped_file(file) for file in files)
