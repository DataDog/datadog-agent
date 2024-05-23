"""Definitions and utilities for configuring the Agent build.
"""

REPO_PATH = "github.com/DataDog/datadog-agent"

def with_repo_path(mapping, repo_path=REPO_PATH):
    """Create a copy of a mapping with a path to repo"""
    return {repo_path + k: v for k, v in mapping.items()}
