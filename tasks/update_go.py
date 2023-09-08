import re
from typing import Optional

from invoke import Context, exceptions, task

from .go import tidy_all
from .libs.common.color import color_message
from .modules import DEFAULT_MODULES
from .pipeline import update_circleci_config, update_gitlab_config

GO_VERSION_FILE = "./.go-version"


# replace the given pattern with the given string in the file
def _update_file(path: str, pattern: str, replace: str, expected_match: int = 1):
    # newline='' keeps the file's newline character(s)
    # meaning it keeps '\n' for most files and '\r\n' for windows specific files

    with open(path, "r", newline='') as reader:
        content = reader.read()

    content, nb_match = re.subn(pattern, replace, content, flags=re.MULTILINE)
    if nb_match != expected_match:
        print(
            color_message(
                f"WARNING: {path}: '{pattern}': expected {expected_match} matches but go {nb_match}", "orange"
            )
        )

    with open(path, "w", newline='') as writer:
        writer.write(content)


# returns the current go version
def _get_repo_go_version() -> str:
    with open(GO_VERSION_FILE, "r") as reader:
        version = reader.read()
    return version.strip()


# extracts the major version from the given string
# eg. if the string is "1.2.3", returns "1.2"
def _get_major_version(version: str) -> str:
    major, _, _ = version.rpartition(".")
    return major


def _update_go_version_file(version: str):
    _update_file(GO_VERSION_FILE, "[.0-9]+", version)


def _update_gdb_dockerfile(version: str):
    path = "./tools/gdb/Dockerfile"
    pattern = r'(https://go\.dev/dl/go)[.0-9]+(\.linux-amd64\.tar\.gz)'
    replace = rf'\g<1>{version}\g<2>'
    _update_file(path, pattern, replace)


def _update_install_devenv(version: str):
    path = "./devenv/scripts/Install-DevEnv.ps1"
    _update_file(path, '("Installing go )[.0-9]+"', rf'\g<1>{version}"')
    _update_file(
        path,
        r'(https://dl\.google\.com/go/go)[.0-9]+(\.windows-)',
        rf'\g<1>{version}\g<2>',
        2,
    )


def _update_agent_devenv(version: str):
    path = "./docs/dev/agent_dev_env.md"
    pattern = r"^(You must \[install Golang\]\(https://golang\.org/doc/install\) version )`[.0-9]+`"
    replace = rf"\g<1>`{version}`"
    _update_file(path, pattern, replace)


def _update_task_go(version: str):
    path = "./tasks/go.py"
    pattern = '("go version go)[.0-9]+( linux/amd64")'
    replace = rf'\g<1>{version}\g<2>'
    _update_file(path, pattern, replace)


def _update_readme(major: str):
    path = "./README.md"
    pattern = r'(\[Go\]\(https://golang\.org/doc/install\) )[.0-9]+( or later)'
    replace = rf'\g<1>{major}\g<2>'
    _update_file(path, pattern, replace)


def _update_go_mods(major: str):
    mod_files = [f"./{module}/go.mod" for module in DEFAULT_MODULES]
    for mod_file in mod_files:
        _update_file(mod_file, "^go [.0-9]+$", f"go {major}")


def _create_releasenote(ctx: Context, version: str):
    RELEASENOTE_TEMPLATE = """---
enhancements:
- |
    Agents are now built with Go ``{}``.
"""
    # hiding stderr too because `reno` displays some warnings about the config
    res = ctx.run(f'reno new "bump go to {version}"', hide='both')
    match = re.match("^Created new notes file in (.*)$", res.stdout, flags=re.MULTILINE)
    if not match:
        raise exceptions.Exit("Could not get created release note path")

    path = match.group(1)
    with open(path, "w") as writer:
        writer.write(RELEASENOTE_TEMPLATE.format(version))
    return path


@task(
    help={
        "version": "The version of Go to use",
        "image_tag": "Tag from buildimages with format v<build_id>_<commit_id>",
        "test_version": "Whether the image is a test image or not",
        "force_major": "Whether to apply changes like if it was a major version update",
    }
)
def update_go(
    ctx: Context,
    version: str,
    image_tag: str,
    test_version: Optional[bool] = False,
    force_major: Optional[bool] = False,
):
    """
    Updates the version of Go and build images.
    """
    if not re.match("[0-9]+.[0-9]+.[0-9]+", version):
        raise exceptions.Exit(
            "The version doesn't have an expected format, it should be 3 numbers separated with a dot."
        )

    # check the installed go version before running `tidy_all`
    res = ctx.run("go version")
    if not res.stdout.startswith(f"go version go{version} "):
        raise exceptions.Exit("The version of your `go` binary doesn't match the requested version")

    current_version = _get_repo_go_version()
    current_major = _get_major_version(current_version)
    new_major = _get_major_version(version)

    major_update = current_major != new_major or force_major
    if major_update:
        print(color_message("WARNING: this is a change of major version\n", "orange"))
        _update_readme(new_major)
        _update_go_mods(new_major)

    update_gitlab_config(".gitlab-ci.yml", image_tag, test_version=test_version)
    update_circleci_config(".circleci/config.yml", image_tag, test_version=test_version)
    _update_go_version_file(version)
    _update_gdb_dockerfile(version)
    _update_install_devenv(version)
    _update_agent_devenv(version)
    _update_task_go(version)

    tidy_all(ctx)

    releasenote_path = _create_releasenote(ctx, version)
    print(
        f"A default release note was created at {releasenote_path}, edit it if necessary, for example to list CVEs it fixes."
    )
    if major_update:
        # Examples of major updates with long descriptions:
        # releasenotes/notes/go1.16.7-4ec8477608022a26.yaml
        # releasenotes/notes/go1185-fd9d8b88c7c7a12e.yaml
        print("In particular as this is a major update, the release note should describe user-facing changes.")
    print()

    print(
        color_message(
            "Remember to look for reference to the former version by yourself too, and update this task if you find some.",
            "green",
        )
    )
