from typing import Optional

from invoke import Context, task

from tasks.pipeline import update_circleci_config, update_gitlab_config


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
