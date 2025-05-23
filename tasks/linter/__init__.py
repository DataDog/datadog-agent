"""Module regrouping all invoke tasks used for linting the `datadog-agent` repo"""

from .gitlab import (
    gitlab_change_paths,  # noqa: F401
    gitlab_ci,  # noqa: F401
    gitlab_ci_jobs_codeowners,  # noqa: F401
    gitlab_ci_jobs_needs_rules,  # noqa: F401
    gitlab_ci_jobs_owners,  # noqa: F401
    job_change_path,  # noqa: F401
    list_parameters,  # noqa: F401
    ssm_parameters,  # noqa: F401
)
from .go import go, update_go  # noqa: F401
from .misc import copyrights, filenames  # noqa: F401
from .old import *  # noqa: F403
from .python import python  # noqa: F401
