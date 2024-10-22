from __future__ import annotations

import re

from invoke import exceptions
from invoke.context import Context
from invoke.tasks import task

from tasks.go import tidy
from tasks.libs.ciproviders.circleci import update_circleci_config
from tasks.libs.ciproviders.gitlab_api import update_gitlab_config
from tasks.libs.common.color import color_message
from tasks.modules import DEFAULT_MODULES

GO_VERSION_FILE = "./.go-version"

# list of references of Go versions
# each tuple is (path, pre_pattern, post_pattern, minor), where
# - path is the path of the file to update
# - pre_pattern and post_pattern delimit the version to update
# - is_bugfix is True if the version in the match is a bugfix version, False if it's a minor
GO_VERSION_REFERENCES: list[tuple[str, str, str, bool]] = [
    (GO_VERSION_FILE, "", "", True),  # the version is the only content of the file
    ("./tools/gdb/Dockerfile", "https://go.dev/dl/go", ".linux-amd64.tar.gz", True),
    ("./test/fakeintake/Dockerfile", "FROM golang:", "-alpine", True),
    ("./tasks/unit_tests/modules_tests.py", 'Go": "', '",', False),
    ("./devenv/scripts/Install-DevEnv.ps1", '$go_version = "', '"', True),
    ("./docs/dev/agent_dev_env.md", "[install Golang](https://golang.org/doc/install) version `", "`", True),
    ("./tasks/go.py", '"go version go', ' linux/amd64"', True),
    ("./README.md", "[Go](https://golang.org/doc/install) ", " or later", False),
    ("./test/fakeintake/docs/README.md", "[Golang ", "]", False),
    ("./cmd/process-agent/README.md", "`go >= ", "`", False),
    ("./pkg/logs/launchers/windowsevent/README.md", "install go ", "+,", False),
    ("./.wwhrd.yml", "raw.githubusercontent.com/golang/go/go", "/LICENSE", True),
    ("./docs/public/setup.md", "version `", "` or higher", True),
]

PATTERN_MAJOR_MINOR = r'1\.\d+'
PATTERN_MAJOR_MINOR_BUGFIX = r'1\.\d+\.\d+'


@task
def go_version(_):
    current_version = _get_repo_go_version()
    print(current_version)


@task(
    help={
        "version": "The version of Go to use",
        "image_tag": "Tag from buildimages with format v<build_id>_<commit_id>",
        "test": "Whether the image is a test image or not",
        "warn": "Don't exit in case of matching error, just warn.",
        "release_note": "Whether to create a release note or not. The default behaviour is to create a release note",
        "include_otel_modules": "Whether to update the version in go.mod files used by otel.",
    }
)
def update_go(
    ctx: Context,
    version: str,
    image_tag: str | None = None,
    test: bool = True,
    warn: bool = False,
    release_note: bool = True,
    include_otel_modules: bool = False,
):
    """
    Updates the version of Go and build images.
    """
    import semver

    if not semver.VersionInfo.isvalid(version):
        raise exceptions.Exit(f"The version {version} isn't valid.")

    current_version = _get_repo_go_version()
    current_major_minor = _get_major_minor_version(current_version)
    new_major_minor = _get_major_minor_version(version)

    is_minor_update = current_major_minor != new_major_minor
    if is_minor_update:
        print(color_message("WARNING: this is a change of minor version\n", "orange"))

    if image_tag:
        try:
            update_gitlab_config(".gitlab-ci.yml", image_tag, test=test)
        except RuntimeError as e:
            if warn:
                print(color_message(f"WARNING: {str(e)}", "orange"))
            else:
                raise

        try:
            update_circleci_config(".circleci/config.yml", image_tag, test=test)
        except RuntimeError as e:
            if warn:
                print(color_message(f"WARNING: {str(e)}", "orange"))
            else:
                raise

    _update_references(warn, version)
    _update_go_mods(warn, version, include_otel_modules)

    # check the installed go version before running `tidy_all`
    res = ctx.run("go version")
    if res and res.stdout.startswith(f"go version go{version} "):
        tidy(ctx)
    else:
        print(
            color_message(
                "WARNING: did not run `inv tidy` as the version of your `go` binary doesn't match the requested version",
                "orange",
            )
        )

    if release_note:
        releasenote_path = _create_releasenote(ctx, version)
        print(
            f"A default release note was created at {releasenote_path}, edit it if necessary, for example to list CVEs it fixes."
        )
    if is_minor_update:
        # Examples of minor updates with long descriptions:
        # releasenotes/notes/go1.16.7-4ec8477608022a26.yaml
        # releasenotes/notes/go1185-fd9d8b88c7c7a12e.yaml
        print("In particular as this is a minor update, the release note should describe user-facing changes.")

    print(
        color_message(
            "\nRemember to look for reference to the former version by yourself too, and update this task if you find any.",
            "green",
        )
    )


# replace the given pattern with the given string in the file
def _update_file(warn: bool, path: str, pattern: str, replace: str, expected_match: int = 1, dry_run: bool = False):
    # newline='' keeps the file's newline character(s)
    # meaning it keeps '\n' for most files and '\r\n' for windows specific files

    with open(path, newline='', encoding='utf-8') as reader:
        content = reader.read()

    if dry_run:
        matches = re.findall(pattern, content, flags=re.MULTILINE)
        nb_match = len(matches)
    else:
        content, nb_match = re.subn(pattern, replace, content, flags=re.MULTILINE)

    if nb_match != expected_match:
        msg = f"{path}: '{pattern}': expected {expected_match} matches but got {nb_match}"
        if warn:
            print(color_message(f"WARNING: {msg}", "orange"))
        else:
            raise exceptions.Exit(msg)

    if not dry_run:
        with open(path, "w", newline='') as writer:
            writer.write(content)


# returns the current go version
def _get_repo_go_version() -> str:
    with open(GO_VERSION_FILE) as reader:
        version = reader.read()
    return version.strip()


# extracts the minor version from the given string
# eg. if the string is "1.2.3", returns "1.2"
def _get_major_minor_version(version: str) -> str:
    import semver

    ver = semver.VersionInfo.parse(version)
    return f"{ver.major}.{ver.minor}"


def _get_pattern(pre_pattern: str, post_pattern: str, is_bugfix: bool) -> str:
    version_pattern = PATTERN_MAJOR_MINOR_BUGFIX if is_bugfix else PATTERN_MAJOR_MINOR
    pattern = rf'({re.escape(pre_pattern)}){version_pattern}({re.escape(post_pattern)})'
    return pattern


def _update_references(warn: bool, version: str, dry_run: bool = False):
    new_major_minor = _get_major_minor_version(version)
    for path, pre_pattern, post_pattern, is_bugfix in GO_VERSION_REFERENCES:
        pattern = _get_pattern(pre_pattern, post_pattern, is_bugfix)

        new_version = version if is_bugfix else new_major_minor
        replace = rf'\g<1>{new_version}\g<2>'

        _update_file(warn, path, pattern, replace, dry_run=dry_run)


def _update_go_mods(warn: bool, version: str, include_otel_modules: bool, dry_run: bool = False):
    for path, module in DEFAULT_MODULES.items():
        if not include_otel_modules and module.used_by_otel:
            # only update the go directives in go.mod files not used by otel
            # to allow them to keep using the modules
            continue
        mod_file = f"./{path}/go.mod"
        major_minor = _get_major_minor_version(version)
        major_minor_zero = f"{major_minor}.0"
        # $ only matches \n, not \r\n, so we need to use \r?$ to make it work on Windows
        _update_file(warn, mod_file, f"^go {PATTERN_MAJOR_MINOR_BUGFIX}\r?$", f"go {major_minor_zero}", dry_run=dry_run)


def _create_releasenote(ctx: Context, version: str):
    RELEASENOTE_TEMPLATE = """---
enhancements:
- |
    Agents are now built with Go ``{}``.
"""
    # hiding stderr too because `reno` displays some warnings about the config
    res = ctx.run(f'reno new "bump go to {version}"', hide='both')
    assert res, "Could not create release note"
    match = re.match("^Created new notes file in (.*)$", res.stdout, flags=re.MULTILINE)
    if not match:
        raise exceptions.Exit("Could not get created release note path")

    path = match.group(1)
    with open(path, "w") as writer:
        writer.write(RELEASENOTE_TEMPLATE.format(version))
    return path
