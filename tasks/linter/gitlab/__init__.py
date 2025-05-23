"""Linting-related tasks for files related to Gitlab CI"""

from .tasks import (
    gitlab_change_paths,
    gitlab_ci,
    gitlab_ci_jobs_codeowners,
    gitlab_ci_jobs_needs_rules,
    gitlab_ci_jobs_owners,
    gitlab_ci_shellcheck,
    job_change_path,
    list_parameters,
    ssm_parameters,
)

__all__ = [
    "gitlab_change_paths",
    "gitlab_ci",
    "gitlab_ci_jobs_codeowners",
    "gitlab_ci_jobs_needs_rules",
    "gitlab_ci_jobs_owners",
    "gitlab_ci_shellcheck",
    "job_change_path",
    "list_parameters",
    "ssm_parameters",
]
