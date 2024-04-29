import os
from typing import Optional

from invoke import Context, task

from tasks.pipeline import update_circleci_config, update_gitlab_config, update_test_infra_def


@task(
    help={
        "image_tag": "tag from build_image with format v<build_id>_<commit_id>",
        "test_version": "Is a test image or not",
    }
)
def update(_: Context, image_tag: str, test_version: Optional[str] = True):
    """
    Update local files to run with new image_tag from agent-buildimages
    Use --no-test-version to commit without the _test_only suffixes
    """
    update_gitlab_config(".gitlab-ci.yml", image_tag, test_version=test_version)
    update_circleci_config(".circleci/config.yml", image_tag, test_version=test_version)


@task(help={"commit_sha": "commit sha from the test-infra-definitions repository"})
def update_test_infra_definitions(ctx: Context, commit_sha: str, go_mod_only: bool = False):
    """
    Update the test-infra-definition image version in the Gitlab CI as well as in the e2e go.mod
    """
    if not go_mod_only:
        update_test_infra_def(".gitlab/common/test_infra_version.yml", commit_sha[:12])

    os.chdir("test/new-e2e")
    ctx.run(f"go get github.com/DataDog/test-infra-definitions@{commit_sha}")
    ctx.run("go mod tidy")
