import re
from typing import Optional

from invoke import Context, exceptions, task

from .go import tidy_all
from .libs.common.color import color_message
from .modules import DEFAULT_MODULES
from .pipeline import update_circleci_config, update_gitlab_config

GO_VERSION_FILE = "./.go-version"


@task(
    help={
        "version": "The version of Go to use",
        "image_tag": "Tag from buildimages with format v<build_id>_<commit_id>",
        "test_version": "Whether the image is a test image or not",
        "warn": "Don't exit in case of matching error, just warn.",
    }
)
def update_go(
    ctx: Context,
    version: str,
    image_tag: str,
    test_version: Optional[bool] = False,
    warn: Optional[bool] = False,
):
    """
    Updates the version of Go and build images.
    """
    import semver

    if not semver.Version.is_valid(version):
        raise exceptions.Exit(f"The version {version} isn't valid.")

    current_version = _get_repo_go_version()
    current_major = _get_major_version(current_version)
    new_major = _get_major_version(version)

    major_update = current_major != new_major
    if major_update:
        print(color_message("WARNING: this is a change of major version\n", "orange"))

    try:
        update_gitlab_config(".gitlab-ci.yml", image_tag, test_version=test_version)
    except RuntimeError as e:
        if warn:
            print(color_message(f"WARNING: {str(e)}", "orange"))
        else:
            raise

    try:
        update_circleci_config(".circleci/config.yml", image_tag, test_version=test_version)
    except RuntimeError as e:
        if warn:
            print(color_message(f"WARNING: {str(e)}", "orange"))
        else:
            raise

    _update_readme(warn, new_major)
    _update_go_mods(warn, new_major)
    _update_go_version_file(warn, version)
    _update_gdb_dockerfile(warn, version)
    _update_install_devenv(warn, version)
    _update_agent_devenv(warn, version)
    _update_task_go(warn, version)

    # check the installed go version before running `tidy_all`
    res = ctx.run("go version")
    if res.stdout.startswith(f"go version go{version} "):
        tidy_all(ctx)
    else:
        print(
            color_message(
                "WARNING: did not run `inv tidy-all` as the version of your `go` binary doesn't match the request version",
                "orange",
            )
        )

    releasenote_path = _create_releasenote(ctx, version)
    print(
        f"A default release note was created at {releasenote_path}, edit it if necessary, for example to list CVEs it fixes."
    )
    if major_update:
        # Examples of major updates with long descriptions:
        # releasenotes/notes/go1.16.7-4ec8477608022a26.yaml
        # releasenotes/notes/go1185-fd9d8b88c7c7a12e.yaml
        print("In particular as this is a major update, the release note should describe user-facing changes.")

    print(
        color_message(
            "\nRemember to look for reference to the former version by yourself too, and update this task if you find any.",
            "green",
        )
    )


# replace the given pattern with the given string in the file
def _update_file(warn: bool, path: str, pattern: str, replace: str, expected_match: int = 1):
    # newline='' keeps the file's newline character(s)
    # meaning it keeps '\n' for most files and '\r\n' for windows specific files

    with open(path, "r", newline='') as reader:
        content = reader.read()

    content, nb_match = re.subn(pattern, replace, content, flags=re.MULTILINE)
    if nb_match != expected_match:
        msg = f"{path}: '{pattern}': expected {expected_match} matches but go {nb_match}"
        if warn:
            print(color_message(f"WARNING: {msg}", "orange"))
        else:
            raise exceptions.Exit(msg)

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
    import semver

    ver = semver.Version.parse(version)
    return f"{ver.major}.{ver.minor}"


def _update_go_version_file(warn: bool, version: str):
    _update_file(warn, GO_VERSION_FILE, "[.0-9]+", version)


def _update_gdb_dockerfile(warn: bool, version: str):
    path = "./tools/gdb/Dockerfile"
    pattern = r'(https://go\.dev/dl/go)[.0-9]+(\.linux-amd64\.tar\.gz)'
    replace = rf'\g<1>{version}\g<2>'
    _update_file(warn, path, pattern, replace)


def _update_install_devenv(warn: bool, version: str):
    path = "./devenv/scripts/Install-DevEnv.ps1"
    _update_file(warn, path, '("Installing go )[.0-9]+"', rf'\g<1>{version}"')
    _update_file(
        warn,
        path,
        r'(https://dl\.google\.com/go/go)[.0-9]+(\.windows-)',
        rf'\g<1>{version}\g<2>',
        2,
    )


def _update_agent_devenv(warn: bool, version: str):
    path = "./docs/dev/agent_dev_env.md"
    pattern = r"^(You must \[install Golang\]\(https://golang\.org/doc/install\) version )`[.0-9]+`"
    replace = rf"\g<1>`{version}`"
    _update_file(warn, path, pattern, replace)


def _update_task_go(warn: bool, version: str):
    path = "./tasks/go.py"
    pattern = '("go version go)[.0-9]+( linux/amd64")'
    replace = rf'\g<1>{version}\g<2>'
    _update_file(warn, path, pattern, replace)


def _update_readme(warn: bool, major: str):
    path = "./README.md"
    pattern = r'(\[Go\]\(https://golang\.org/doc/install\) )[.0-9]+( or later)'
    replace = rf'\g<1>{major}\g<2>'
    _update_file(warn, path, pattern, replace)


def _update_go_mods(warn: bool, major: str):
    mod_files = [f"./{module}/go.mod" for module in DEFAULT_MODULES]
    for mod_file in mod_files:
        _update_file(warn, mod_file, "^go [.0-9]+$", f"go {major}")


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
