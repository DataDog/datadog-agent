"""Module regrouping all invoke tasks used for linting the `datadog-agent` repo"""

from .github import github_actions_shellcheck, releasenote

# Make gitlab helper methods and tasks accessible as they are used in unit test mocks
from .gitlab import (
    gitlab_change_paths,
    gitlab_ci,
    gitlab_ci_jobs_codeowners,
    gitlab_ci_jobs_needs_rules,
    gitlab_ci_jobs_owners,
    gitlab_ci_shellcheck,
    helpers,
    job_change_path,
    list_parameters,
    ssm_parameters,
    tasks,
)
from .go import go, update_go
from .misc import copyrights, filenames
from .python import python

__all__ = [
    "github_actions_shellcheck",
    "releasenote",
    "gitlab_change_paths",
    "gitlab_ci",
    "gitlab_ci_jobs_codeowners",
    "gitlab_ci_jobs_needs_rules",
    "gitlab_ci_jobs_owners",
    "gitlab_ci_shellcheck",
    "helpers",
    "job_change_path",
    "list_parameters",
    "ssm_parameters",
    "tasks",
    "go",
    "update_go",
    "copyrights",
    "filenames",
    "python",
]
