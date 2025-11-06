"""
Invoke entrypoint, import here all the tasks we want to make available
"""

from invoke import Collection

from linter import (
    copyrights,
)

from licenses import (
    generate_licenses,
    lint_licenses,
)
