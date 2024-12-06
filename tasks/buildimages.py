from __future__ import annotations

import os

import yaml
from invoke import Context, Exit, task

from tasks.libs.ciproviders.circleci import update_circleci_config
from tasks.libs.ciproviders.gitlab_api import ReferenceTag, update_gitlab_config, update_test_infra_def
from tasks.libs.common.color import color_message


@task(
    help={
        "tag": "tag from build_image with format v<build_id>_<commit_id>",
        "images": "The image(s) to update, comma separated. If empty, updates all images. It support incomplete pattern, e.g. 'deb,rpm' will update all deb and rpm images. deb_x64 will update only one image. Use the --list-images flag to list all available images",
        "test": "Is a test image or not",
        "list_images": "List all available images",
    }
)
def update(_: Context, tag: str = "", images: str = "", test: bool = True, list_images: bool = False):
    """
    Update local files to use a new version of dedicated images from agent-buildimages
    Use --no-test to commit without the _test_only suffixes
    Use --list-images to list all available images
    """
    if list_images:
        print("List of available images:")
        modified = update_gitlab_config(".gitlab-ci.yml", "", update=False)
        modified.append("CIRCLECI_RUNNER")
    else:
        print("Updating images:")
        modified = update_gitlab_config(".gitlab-ci.yml", tag, images, test=test)
        if images == "" or "circle" in images:
            update_circleci_config(".circleci/config.yml", tag, test=test)
            modified.append("CIRCLECI_RUNNER")
    print(f"  {', '.join(modified)}")


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


@task(
    help={
        "old_build_image_tag": "The old build image tag",
        "new_build_image_tag": "The new build image tag",
        "old_go_version": "The old Go version",
        "new_go_version": "The new Go version",
        "test": "Flag to indicate if this is a test version",
    },
    autoprint=True,
)
def generate_pr_body(
    _: Context,
    old_build_image_tag: str,
    new_build_image_tag: str,
    old_go_version: str,
    new_go_version: str,
    test: bool = False,
):
    """
    Generate the PR body used for buildimages-update Github workflow
    """
    buildimages_workflow_url = "https://github.com/DataDog/datadog-agent/actions/workflows/buildimages-update.yml"
    test_version_str = "(test version)" if test else ""
    compare_url = f"https://github.com/DataDog/datadog-agent-buildimages/compare/{old_build_image_tag.split('-')[1]}...{new_build_image_tag.split('-')[1]}"
    pr_body = f"""This PR was automatically created by the [Update buildimages Github Workflow]({buildimages_workflow_url}).

### Buildimages update
This PR updates the current buildimages (`{old_build_image_tag}`) to `{new_build_image_tag}`{test_version_str}, [here is the full changelog]({compare_url}).
"""

    if old_go_version != new_go_version:
        old_go_version_url = f"https://go.dev/doc/devel/release#go{old_go_version}"
        new_go_version_url = f"https://go.dev/doc/devel/release#go{new_go_version}"
        pr_body += f"""

### Golang update
This PR updates the current Golang version ([`{old_go_version}`]({old_go_version_url})) to [`{new_go_version}`]({new_go_version_url}).
"""
    return pr_body


@task(
    help={
        "file_path": "path of the Gitlab configuration YAML file",
        "image_type": "The type of image to get the tag for (e.g. deb_x64, rpm_armhf, etc). You can use any value defined by or CI_IMAGE_<image_tyoe> in the gitlab-ci configuration variables. Get the DATADOG_AGENT_BUILDIMAGES version if image_type not specified",
    },
    autoprint=True,
)
def get_tag(_, file_path=".gitlab-ci.yml", image_type=None):
    """
    Print the current image tag of the given Gitlab configuration file (default: ".gitlab-ci.yml")
    """
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    with open(file_path) as gl:
        gitlab_ci = yaml.safe_load(gl)
    if "variables" not in gitlab_ci:
        raise Exit(
            f'[{color_message("ERROR", "red")}] - No variables in gitlab configuration file {file_path}',
            code=1,
        )

    if image_type is None:
        return gitlab_ci["variables"].get(
            "DATADOG_AGENT_BUILDIMAGES",
            f'{color_message("Not found", "red")}: DATADOG_AGENT_BUILDIMAGES is not defined in the configuration',
        )
    else:
        available_images = set()
        for key in gitlab_ci["variables"].keys():
            if key.startswith("CI_IMAGE"):
                image = key.removeprefix("CI_IMAGE_").removesuffix("_SUFFIX").casefold()
                available_images.add(image)
                if image_type.casefold() == image:
                    return gitlab_ci["variables"][key]
        raise Exit(
            f'{color_message("ERROR", "red")}: {image_type} is not defined in the configuration. Available images: {", ".join(available_images)}',
            code=1,
        )
