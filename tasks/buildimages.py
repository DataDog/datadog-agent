import os
from typing import Optional

from invoke import Context, task

from tasks.libs.ciproviders.gitlab_api import update_gitlab_config, update_test_infra_def
from tasks.libs.ciproviders.circleci import update_circleci_config


@task(
    help={
        "version": "tag from build_image with format v<build_id>_<commit_id>",
        "images": "The image(s) to update, comma separated. If empty, all images will be updated. Can be a incomplete pattern, e.g. 'deb,rpm' will update all deb and rpm images. deb_x64 will update only one image",
        "test": "Is a test image or not",
    }
)
def update(_: Context, version: str, images: Optional[str] = "", test: Optional[str] = True):
    """
    Update local files to use a new version of dedicated image from buildimages
    Use --no-test to commit without the _test_only suffixes
    """
    patterns = images.split(",")
    modified = update_gitlab_config(".gitlab-ci.yml", version, patterns, test_version=test)
    message = ", ".join(modified)
    if len(patterns) == 0 or any(p.startswith("circle") for p in patterns):
        update_circleci_config(".circleci/config.yml", version, test_version=test)
        message += ", circleci"
    print(f"Updated {message}")


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
