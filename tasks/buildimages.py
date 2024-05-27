from __future__ import annotations

import os

from invoke import Context, task

from tasks.pipeline import update_circleci_config, update_gitlab_config, update_test_infra_def


@task(
    help={
        "image_tag": "tag from build_image with format v<build_id>_<commit_id>",
        "test_version": "Is a test image or not",
    }
)
def update(_: Context, image_tag: str, test_version: str | None = True):
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


@task
def generate_pr_body(
    _: Context,
    old_build_image_tag: str,
    new_build_image_tag: str,
    old_go_version: str,
    new_go_version: str,
    test_version: bool = False,
):
    """
    Generate the PR body used for buildimages-update Github workflow
    """
    pr_body = f"""This PR was automatically created by the [Update buildimages Github Workflow](https://github.com/DataDog/datadog-agent/actions/workflows/buildimages-update.yml).

### Buildimages update
This PR updates the current buildimages (`{old_build_image_tag}`) to `{new_build_image_tag}`{'(test version)' if test_version else ''}, here is the full changelog between:
https://github.com/DataDog/datadog-agent-buildimages/compare/{old_build_image_tag.split("-")[1]}...{new_build_image_tag.split("-")[1]}
"""

    if old_go_version != new_go_version:
        pr_body += f"""

### Golang update
This PR updates the current Golang version ([`{old_go_version}`](https://go.dev/doc/devel/release#go{old_go_version})) to [`{new_go_version}`](https://go.dev/doc/devel/release#go{new_go_version}).
"""
    print(pr_body)
