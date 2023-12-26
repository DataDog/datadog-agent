import re
from typing import List, Optional, Tuple

from invoke import Context, exceptions, task

from .go import tidy_all
from .libs.common.color import color_message
from .modules import DEFAULT_MODULES
from .pipeline import update_circleci_config, update_gitlab_config

GO_VERSION_FILE = "./.go-version"

# list of references of Go versions
# each tuple is (path, pre_pattern, post_pattern, minor), where
# - path is the path of the file to update
# - pre_pattern and post_pattern delimit the version to update
# - minor is whether the version in the match is a minor version of a bugfix version
GO_VERSION_REFERENCES: List[Tuple[str, str, str, bool]] = [
    (GO_VERSION_FILE, "", "", False),  # the version is the only content of the file
    ("./tools/gdb/Dockerfile", "https://go.dev/dl/go", ".linux-amd64.tar.gz", False),
    ("./test/fakeintake/Dockerfile", "FROM golang:", "-alpine", False),
    ("./devenv/scripts/Install-DevEnv.ps1", '$go_version = "', '"', False),
    ("./docs/dev/agent_dev_env.md", "[install Golang](https://golang.org/doc/install) version `", "`", False),
    ("./tasks/go.py", '"go version go', ' linux/amd64"', False),
    ("./README.md", "[Go](https://golang.org/doc/install) ", " or later", True),
    ("./test/fakeintake/docs/README.md", "[Golang ", "]", True),
    ("./cmd/process-agent/README.md", "`go >= ", "`", True),
    ("./pkg/logs/launchers/windowsevent/README.md", "install go ", "+,", True),
]


@task
def go_version(_):
    current_version = _get_repo_go_version()
    print(current_version)


@task(
    help={
        "version": "The version of Go to use",
        "image_tag": "Tag from buildimages with format v<build_id>_<commit_id>",
        "test_version": "Whether the image is a test image or not",
        "warn": "Don't exit in case of matching error, just warn.",
        "release_note": "Whether to create a release note or not. The default behaviour is to create a release note",
        "include_otel_modules": "Whether to update the version in go.mod files used by otel.",
    }
)
def update_go(
    ctx: Context,
    version: str,
    image_tag: str,
    test_version: Optional[bool] = False,
    warn: Optional[bool] = False,
    release_note: Optional[bool] = True,
    include_otel_modules: Optional[bool] = False,
):
    """
    Updates the version of Go and build images.
    """
    import semver

    if not semver.VersionInfo.isvalid(version):
        raise exceptions.Exit(f"The version {version} isn't valid.")

    current_version = _get_repo_go_version()
    current_major = _get_minor_version(current_version)
    new_minor = _get_minor_version(version)

    minor_update = current_major != new_minor
    if minor_update:
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

    _update_references(warn, version, new_minor)
    _update_go_mods(warn, new_minor, include_otel_modules)

    # check the installed go version before running `tidy_all`
    res = ctx.run("go version")
    if res.stdout.startswith(f"go version go{version} "):
        tidy_all(ctx)
    else:
        print(
            color_message(
                "WARNING: did not run `inv tidy-all` as the version of your `go` binary doesn't match the requested version",
                "orange",
            )
        )

    if release_note:
        releasenote_path = _create_releasenote(ctx, version)
        print(
            f"A default release note was created at {releasenote_path}, edit it if necessary, for example to list CVEs it fixes."
        )
    if minor_update:
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
def _update_file(warn: bool, path: str, pattern: str, replace: str, expected_match: int = 1):
    # newline='' keeps the file's newline character(s)
    # meaning it keeps '\n' for most files and '\r\n' for windows specific files

    with open(path, "r", newline='') as reader:
        content = reader.read()

    content, nb_match = re.subn(pattern, replace, content, flags=re.MULTILINE)
    if nb_match != expected_match:
        msg = f"{path}: '{pattern}': expected {expected_match} matches but got {nb_match}"
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


# extracts the minor version from the given string
# eg. if the string is "1.2.3", returns "1.2"
def _get_minor_version(version: str) -> str:
    import semver

    ver = semver.VersionInfo.parse(version)
    return f"{ver.major}.{ver.minor}"


def _get_pattern(pre_pattern: str, post_pattern: str, minor: bool) -> Tuple[str, str]:
    version_pattern = r'1\.\d+' if minor else r'1\.\d+\.\d+'
    pattern = rf'({re.escape(pre_pattern)}){version_pattern}({re.escape(post_pattern)})'
    return pattern


def _update_references(warn: bool, version: str, new_minor: str):
    for path, pre_pattern, post_pattern, minor in GO_VERSION_REFERENCES:
        pattern = _get_pattern(pre_pattern, post_pattern, minor)
        new_version = new_minor if minor else version
        replace = rf'\g<1>{new_version}\g<2>'
        _update_file(warn, path, pattern, replace)


def _update_go_mods(warn: bool, major: str, include_otel_modules: bool):
    for path, module in DEFAULT_MODULES.items():
        if not include_otel_modules and module.used_by_otel:
            # only update the go directives in go.mod files not used by otel
            # to allow them to keep using the modules
            continue
        mod_file = f"./{path}/go.mod"
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
