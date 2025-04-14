import os
import re
import sys

from tasks.libs.ciproviders.gitlab_api import get_commit, get_pipeline
from tasks.libs.common.git import get_default_branch
from tasks.libs.common.utils import Color, color_message
from tasks.libs.notify.utils import DEPLOY_PIPELINES_CHANNEL, PIPELINES_CHANNEL, PROJECT_NAME, get_pipeline_type
from tasks.libs.pipeline.data import get_failed_jobs, get_jobs_skipped_on_pr
from tasks.libs.pipeline.notifications import (
    base_message,
    get_failed_tests,
)
from tasks.libs.types.types import SlackMessage


def should_send_message_to_author(git_ref: str, default_branch: str) -> bool:
    # Must match X.Y.Z, X.Y.x, W.X.Y-rc.Z
    # Must not match W.X.Y-rc.Z-some-feature
    release_ref_regex = re.compile(r"^[0-9]+\.[0-9]+\.(x|[0-9]+)$")
    release_ref_regex_rc = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]-rc.[0-9]+$")

    return not (git_ref == default_branch or release_ref_regex.match(git_ref) or release_ref_regex_rc.match(git_ref))
