"""
Release helper tasks
"""

import hashlib
import json
import re
import sys
from collections import OrderedDict
from datetime import date
from time import sleep

from invoke import Failure, task
from invoke.exceptions import Exit

from .libs.common.color import color_message
from .libs.common.github_api import GithubAPI, get_github_token
from .libs.common.gitlab import Gitlab, get_gitlab_token
from .libs.common.remote_api import APIError
from .libs.common.user_interactions import yes_no_question
from .libs.version import Version
from .modules import DEFAULT_MODULES
from .pipeline import run
from .utils import DEFAULT_BRANCH, get_version, nightly_entry_for, release_entry_for

# Generic version regex. Aims to match:
# - X.Y.Z
# - X.Y.Z-rc.t
# - X.Y.Z-devel
# - vX.Y(.Z) (security-agent-policies repo)
VERSION_RE = re.compile(r'(v)?(\d+)[.](\d+)([.](\d+))?(-devel)?(-rc\.(\d+))?')

REPOSITORY_NAME = "DataDog/datadog-agent"

UNFREEZE_REPO_AGENT = "datadog-agent"
UNFREEZE_REPOS = [UNFREEZE_REPO_AGENT, "omnibus-software", "omnibus-ruby", "datadog-agent-macos-build"]


@task
def add_prelude(ctx, version):
    res = ctx.run(f"reno new prelude-release-{version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Release on: {date.today()}

    - Please refer to the `{version} tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-{version.replace('.', '')}>`_ for the list of changes on the Core Checks
"""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nCommit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


@task
def add_dca_prelude(ctx, agent7_version, agent6_version=""):
    """
    Release of the Cluster Agent should be pinned to a version of the Agent.
    """
    res = ctx.run(f"reno --rel-notes-dir releasenotes-dca new prelude-release-{agent7_version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    if agent6_version != "":
        agent6_version = (
            f"--{agent6_version.replace('.', '')}"  # generate the right hyperlink to the agent's changelog.
        )

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Released on: {date.today()}
    Pinned to datadog-agent v{agent7_version}: `CHANGELOG <https://github.com/{REPOSITORY_NAME}/blob/{DEFAULT_BRANCH}/CHANGELOG.rst#{agent7_version.replace('.', '')}{agent6_version}>`_."""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nCommit this with:")
    print(f"git commit -m \"Add prelude for {agent7_version} release\"")


@task
def add_installscript_prelude(ctx, version):
    res = ctx.run(f"reno --rel-notes-dir releasenotes-installscript new prelude-release-{version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Released on: {date.today()}"""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nCommit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


@task
def update_changelog(ctx, new_version, target="all"):
    """
    Quick task to generate the new CHANGELOG using reno when releasing a minor
    version (linux/macOS only).
    By default generates Agent and Cluster Agent changelogs.
    Use target == "agent" or target == "cluster-agent" to only generate one or the other.
    """
    generate_agent = target in ["all", "agent"]
    generate_cluster_agent = target in ["all", "cluster-agent"]

    new_version_int = list(map(int, new_version.split(".")))

    if len(new_version_int) != 3:
        print(f"Error: invalid version: {new_version_int}")
        raise Exit(1)

    # let's avoid losing uncommitted change with 'git reset --hard'
    try:
        ctx.run("git diff --exit-code HEAD", hide="both")
    except Failure:
        print("Error: You have uncommitted change, please commit or stash before using update_changelog")
        return

    # make sure we are up to date
    ctx.run("git fetch")

    # let's check that the tag for the new version is present (needed by reno)
    try:
        ctx.run(f"git tag --list | grep {new_version}")
    except Failure:
        print(f"Missing '{new_version}' git tag: mandatory to use 'reno'")
        raise

    if generate_agent:
        update_changelog_generic(ctx, new_version, "releasenotes", "CHANGELOG.rst")
    if generate_cluster_agent:
        update_changelog_generic(ctx, new_version, "releasenotes-dca", "CHANGELOG-DCA.rst")


def update_changelog_generic(ctx, new_version, changelog_dir, changelog_file):
    new_version_int = list(map(int, new_version.split(".")))

    # removing releasenotes from bugfix on the old minor.
    branching_point = f"{new_version_int[0]}.{new_version_int[1]}.0-devel"
    previous_minor = f"{new_version_int[0]}.{new_version_int[1] - 1}"
    if previous_minor == "7.15":
        previous_minor = "6.15"  # 7.15 is the first release in the 7.x series
    log_result = ctx.run(
        f"git log {branching_point}...remotes/origin/{previous_minor}.x --name-only --oneline | grep {changelog_dir}/notes/ || true"
    )
    log_result = log_result.stdout.replace('\n', ' ').strip()
    if len(log_result) > 0:
        ctx.run(f"git rm --ignore-unmatch {log_result}")

    # generate the new changelog
    ctx.run(
        f"reno --rel-notes-dir {changelog_dir} report --ignore-cache --earliest-version {branching_point} --version {new_version} --no-show-source > /tmp/new_changelog.rst"
    )

    ctx.run(f"git checkout HEAD -- {changelog_dir}")

    # mac's `sed` has a different syntax for the "-i" paramter
    # GNU sed has a `--version` parameter while BSD sed does not, using that to do proper detection.
    try:
        ctx.run("sed --version", hide='both')
        sed_i_arg = "-i"
    except Failure:
        sed_i_arg = "-i ''"
    # check whether there is a v6 tag on the same v7 tag, if so add the v6 tag to the release title
    v6_tag = ""
    if new_version_int[0] == 7:
        v6_tag = _find_v6_tag(ctx, new_version)
        if v6_tag:
            ctx.run(f"sed {sed_i_arg} -E 's#^{new_version}#{new_version} / {v6_tag}#' /tmp/new_changelog.rst")
    # remove the old header from the existing changelog
    ctx.run(f"sed {sed_i_arg} -e '1,4d' {changelog_file}")

    # merging to <changelog_file>
    ctx.run(f"cat {changelog_file} >> /tmp/new_changelog.rst && mv /tmp/new_changelog.rst {changelog_file}")

    # commit new CHANGELOG
    ctx.run(f"git add {changelog_file}")

    print("\nCommit this with:")
    print(f"git commit -m \"Update {changelog_file} for {new_version}\"")


@task
def update_installscript_changelog(ctx, new_version):
    """
    Quick task to generate the new CHANGELOG-INSTALLSCRIPT using reno when releasing a minor
    version (linux/macOS only).
    """
    new_version_int = list(map(int, new_version.split(".")))

    if len(new_version_int) != 3:
        print(f"Error: invalid version: {new_version_int}")
        raise Exit(1)

    # let's avoid losing uncommitted change with 'git reset --hard'
    try:
        ctx.run("git diff --exit-code HEAD", hide="both")
    except Failure:
        print("Error: You have uncommitted changes, please commit or stash before using update-installscript-changelog")
        return

    # make sure we are up to date
    ctx.run("git fetch")

    # let's check that the tag for the new version is present (needed by reno)
    try:
        ctx.run(f"git tag --list | grep installscript-{new_version}")
    except Failure:
        print(f"Missing 'installscript-{new_version}' git tag: mandatory to use 'reno'")
        raise

    # generate the new changelog
    ctx.run(
        f"reno --rel-notes-dir releasenotes-installscript report             --ignore-cache             --version installscript-{new_version}             --no-show-source > /tmp/new_changelog-installscript.rst"
    )

    # reseting git
    ctx.run("git reset --hard HEAD")

    # mac's `sed` has a different syntax for the "-i" paramter
    sed_i_arg = "-i"
    if sys.platform == 'darwin':
        sed_i_arg = "-i ''"
    # remove the old header from the existing changelog
    ctx.run(f"sed {sed_i_arg} -e '1,4d' CHANGELOG-INSTALLSCRIPT.rst")

    if sys.platform != 'darwin':
        # sed on darwin doesn't support `-z`. On mac, you will need to manually update the following.
        ctx.run(
            "sed -z {0} -e 's/installscript-{1}\\n===={2}/{1}\\n{2}/' /tmp/new_changelog-installscript.rst".format(  # noqa: FS002
                sed_i_arg, new_version, '=' * len(new_version)
            )
        )

    # merging to CHANGELOG-INSTALLSCRIPT.rst
    ctx.run(
        "cat CHANGELOG-INSTALLSCRIPT.rst >> /tmp/new_changelog-installscript.rst && mv /tmp/new_changelog-installscript.rst CHANGELOG-INSTALLSCRIPT.rst"
    )

    # commit new CHANGELOG-INSTALLSCRIPT
    ctx.run("git add CHANGELOG-INSTALLSCRIPT.rst")

    print("\nCommit this with:")
    print(f"git commit -m \"[INSTALLSCRIPT] Update CHANGELOG-INSTALLSCRIPT for {new_version}\"")


@task
def _find_v6_tag(ctx, v7_tag):
    """
    Returns one of the v6 tags that point at the same commit as the passed v7 tag.
    If none are found, returns the empty string.
    """
    v6_tag = ""

    print(f"Looking for a v6 tag pointing to same commit as tag '{v7_tag}'...")
    # Find commit at which the v7_tag points
    commit = ctx.run(f"git rev-list --max-count=1 {v7_tag}", hide='out').stdout.strip()
    try:
        v6_tags = ctx.run(f"git tag --points-at {commit} | grep -E '^6\\.'", hide='out').stdout.strip().split("\n")
    except Failure:
        print(f"Found no v6 tag pointing at same commit as '{v7_tag}'.")
    else:
        v6_tag = v6_tags[0]
        if len(v6_tags) > 1:
            print(f"Found v6 tags '{v6_tags}', picking {v6_tag}'")
        else:
            print(f"Found v6 tag '{v6_tag}'")

    return v6_tag


@task
def list_major_change(_, milestone):
    """
    List all PR labeled "major_changed" for this release.
    """

    github_token = get_github_token()

    response = _query_github_api(
        github_token,
        f"https://api.github.com/search/issues?q=repo:datadog/datadog-agent+label:major_change+milestone:{milestone}",
    )
    results = response.json()
    if not results["items"]:
        print(f"no major change for {milestone}")
        return

    for pr in results["items"]:
        print(f"#{pr['number']}: {pr['title']} ({pr['html_url']})")


#
# release.json manipulation invoke tasks section
#

##
## I/O functions
##


def _load_release_json():
    with open("release.json", "r") as release_json_stream:
        return json.load(release_json_stream, object_pairs_hook=OrderedDict)


def _save_release_json(release_json):
    with open("release.json", "w") as release_json_stream:
        # Note, no space after the comma
        json.dump(release_json, release_json_stream, indent=4, sort_keys=False, separators=(',', ': '))


##
## Utils
##


def _create_version_from_match(match):
    groups = match.groups()
    version = Version(
        major=int(groups[1]),
        minor=int(groups[2]),
        patch=int(groups[4]) if groups[4] and groups[4] != 0 else None,
        devel=True if groups[5] else False,
        rc=int(groups[7]) if groups[7] and groups[7] != 0 else None,
        prefix=groups[0] if groups[0] else "",
    )
    return version


def _stringify_config(config_dict):
    """
    Takes a config dict of the following form:
    {
        "xxx_VERSION": Version(major: x, minor: y, patch: z, rc: t, prefix: "pre"),
        "xxx_HASH": "hashvalue",
        ...
    }

    and transforms all VERSIONs into their string representation (using the Version object's __str__).
    """
    return {key: str(value) for key, value in config_dict.items()}


def _query_github_api(auth_token, url):
    import requests

    # Basic auth doesn't seem to work with private repos, so we use token auth here
    headers = {"Authorization": f"token {auth_token}"}
    response = requests.get(url, headers=headers)
    return response


def build_compatible_version_re(allowed_major_versions, minor_version):
    """
    Returns a regex that matches only versions whose major version is
    in the provided list of allowed_major_versions, and whose minor version matches
    the provided minor version.
    """
    return re.compile(
        r'(v)?({})[.]({})([.](\d+))?(-devel)?(-rc\.(\d+))?'.format(  # noqa: FS002
            "|".join(allowed_major_versions), minor_version
        )
    )


##
## Base functions to fetch candidate versions on other repositories
##


def _get_highest_repo_version(
    auth, repo, version_prefix, version_re, allowed_major_versions=None, max_version: Version = None
):
    # If allowed_major_versions is not specified, search for all versions by using an empty
    # major version prefix.
    if not allowed_major_versions:
        allowed_major_versions = [""]

    highest_version = None

    for major_version in allowed_major_versions:
        url = f"https://api.github.com/repos/DataDog/{repo}/git/matching-refs/tags/{version_prefix}{major_version}"

        tags = _query_github_api(auth, url).json()

        for tag in tags:
            match = version_re.search(tag["ref"])
            if match:
                this_version = _create_version_from_match(match)
                if max_version:
                    # Get the max version that corresponds to the major version
                    # of the current tag
                    this_max_version = max_version.clone()
                    this_max_version.major = this_version.major
                    if this_version > this_max_version:
                        continue
                if this_version > highest_version:
                    highest_version = this_version

        # The allowed_major_versions are listed in order of preference
        # If something matching a given major version exists, no need to
        # go through the next ones.
        if highest_version:
            break

    if not highest_version:
        raise Exit(f"Couldn't find any matching {repo} version.", 1)

    return highest_version


def _get_release_version_from_release_json(release_json, major_version, version_re, release_json_key=None):
    """
    If release_json_key is None, returns the highest version entry in release.json.
    If release_json_key is set, returns the entry for release_json_key of the highest version entry in release.json.
    """

    release_version = None
    release_component_version = None

    # Get the release entry for the given Agent major version
    release_entry_name = release_entry_for(major_version)
    release_json_entry = release_json.get(release_entry_name, None)

    # Check that the release entry exists, otherwise fail
    if release_json_entry:
        release_version = release_entry_name

        # Check that the component's version is defined in the release entry
        if release_json_key is not None:
            match = version_re.match(release_json_entry.get(release_json_key, ""))
            if match:
                release_component_version = _create_version_from_match(match)
            else:
                print(
                    f"{release_entry_name} does not have a valid {release_json_key} ({release_json_entry.get(release_json_key, '')}), ignoring"
                )

    if not release_version:
        raise Exit(f"Couldn't find any matching {release_version} version.", 1)

    if release_json_key is not None:
        return release_component_version

    return release_version


##
## Variables needed for the repository version fetch functions
##

# COMPATIBLE_MAJOR_VERSIONS lists the major versions of tags
# that can be used with a given Agent version
# This is here for compatibility and simplicity reasons, as in most repos
# we don't create both 6 and 7 tags for a combined Agent 6 & 7 release.
# The order matters, eg. when fetching matching tags for an Agent 6 entry,
# tags starting with 6 will be preferred to tags starting with 7.
COMPATIBLE_MAJOR_VERSIONS = {6: ["6", "7"], 7: ["7"]}


# Message templates for the below functions
# Defined here either because they're long and would make the code less legible,
# or because they're used multiple times.
DIFFERENT_TAGS_TEMPLATE = (
    "The latest version of {} ({}) does not match the version used in the previous release entry ({})."
)
RC_TAG_QUESTION_TEMPLATE = "The {} tag found is an RC tag: {}. Are you sure you want to use it?"
TAG_FOUND_TEMPLATE = "The {} tag is {}"


##
## Repository version fetch functions
## The following functions aim at returning the correct version to use for a given
## repository, after compatibility & user confirmations
## The version object returned by such functions should be ready to be used to fill
## the release.json entry.
##


def _fetch_dependency_repo_version(
    repo_name,
    new_agent_version,
    max_agent_version,
    allowed_major_versions,
    compatible_version_re,
    github_token,
    check_for_rc,
):
    """
    Fetches the latest tag on a given repository whose version scheme matches the one used for the Agent,
    with the following constraints:
    - the tag must have a major version that's in allowed_major_versions
    - the tag must match compatible_version_re (the main usage is to restrict the compatible tags to the
      ones with the same minor version as the Agent)?

    If check_for_rc is true, a warning will be emitted if the latest version that satisfies
    the constraints is an RC. User confirmation is then needed to check that this is desired.
    """

    # Get the highest repo version that's not higher than the Agent version we're going to build
    # We don't want to use a tag on dependent repositories that is supposed to be used in a future
    # release of the Agent (eg. if 7.31.1-rc.1 is tagged on integrations-core while we're releasing 7.30.0).
    version = _get_highest_repo_version(
        github_token,
        repo_name,
        new_agent_version.prefix,
        compatible_version_re,
        allowed_major_versions,
        max_version=max_agent_version,
    )

    if check_for_rc and version.is_rc():
        if not yes_no_question(RC_TAG_QUESTION_TEMPLATE.format(repo_name, version), "orange", False):
            raise Exit("Aborting release.json update.", 1)

    print(TAG_FOUND_TEMPLATE.format(repo_name, version))
    return version


def _confirm_independent_dependency_repo_version(repo, latest_version, highest_release_json_version):
    """
    Checks if the two versions of a repository we found (from release.json and from the available repo tags)
    are different. If they are, asks the user for confirmation before updating the version.
    """

    if latest_version == highest_release_json_version:
        return highest_release_json_version

    print(color_message(DIFFERENT_TAGS_TEMPLATE.format(repo, latest_version, highest_release_json_version), "orange"))
    if yes_no_question(f"Do you want to update {repo} to {latest_version}?", "orange", False):
        return latest_version

    return highest_release_json_version


def _fetch_independent_dependency_repo_version(
    repo_name, release_json, agent_major_version, github_token, release_json_key
):
    """
    Fetches the latest tag on a given repository whose version scheme doesn't match the one used for the Agent:
    - first, we get the latest version used in release entries of the matching Agent major version
    - then, we fetch the latest version available in the repository
    - if the above two versions are different, emit a warning and ask for user confirmation before updating the version.
    """

    previous_version = _get_release_version_from_release_json(
        release_json,
        agent_major_version,
        VERSION_RE,
        release_json_key=release_json_key,
    )
    # NOTE: This assumes that the repository doesn't change the way it prefixes versions.
    version = _get_highest_repo_version(github_token, repo_name, previous_version.prefix, VERSION_RE)

    version = _confirm_independent_dependency_repo_version(repo_name, version, previous_version)
    print(TAG_FOUND_TEMPLATE.format(repo_name, version))

    return version


def _get_windows_ddnpm_release_json_info(release_json, agent_major_version, is_first_rc=False):
    """
    Gets the Windows NPM driver info from the previous entries in the release.json file.
    """

    # First RC should use the data from nightly section otherwise reuse the last RC info
    if is_first_rc:
        previous_release_json_version = nightly_entry_for(agent_major_version)
    else:
        previous_release_json_version = release_entry_for(agent_major_version)

    print(f"Using '{previous_release_json_version}' DDNPM values")
    release_json_version_data = release_json[previous_release_json_version]

    win_ddnpm_driver = release_json_version_data['WINDOWS_DDNPM_DRIVER']
    win_ddnpm_version = release_json_version_data['WINDOWS_DDNPM_VERSION']
    win_ddnpm_shasum = release_json_version_data['WINDOWS_DDNPM_SHASUM']

    if win_ddnpm_driver not in ['release-signed', 'attestation-signed']:
        print(f"WARN: WINDOWS_DDNPM_DRIVER value '{win_ddnpm_driver}' is not valid")

    print(f"The windows ddnpm version is {win_ddnpm_version}")

    return win_ddnpm_driver, win_ddnpm_version, win_ddnpm_shasum


##
## release_json object update function
##


def _update_release_json_entry(
    release_json,
    release_entry,
    integrations_version,
    omnibus_software_version,
    omnibus_ruby_version,
    jmxfetch_version,
    security_agent_policies_version,
    macos_build_version,
    windows_ddnpm_driver,
    windows_ddnpm_version,
    windows_ddnpm_shasum,
):
    """
    Adds a new entry to provided release_json object with the provided parameters, and returns the new release_json object.
    """
    import requests

    jmxfetch = requests.get(
        f"https://oss.sonatype.org/service/local/repositories/releases/content/com/datadoghq/jmxfetch/{jmxfetch_version}/jmxfetch-{jmxfetch_version}-jar-with-dependencies.jar"
    ).content
    jmxfetch_sha256 = hashlib.sha256(jmxfetch).hexdigest()

    print(f"Jmxfetch's SHA256 is {jmxfetch_sha256}")
    print(f"Windows DDNPM's SHA256 is {windows_ddnpm_shasum}")

    new_version_config = OrderedDict()
    new_version_config["INTEGRATIONS_CORE_VERSION"] = integrations_version
    new_version_config["OMNIBUS_SOFTWARE_VERSION"] = omnibus_software_version
    new_version_config["OMNIBUS_RUBY_VERSION"] = omnibus_ruby_version
    new_version_config["JMXFETCH_VERSION"] = jmxfetch_version
    new_version_config["JMXFETCH_HASH"] = jmxfetch_sha256
    new_version_config["SECURITY_AGENT_POLICIES_VERSION"] = security_agent_policies_version
    new_version_config["MACOS_BUILD_VERSION"] = macos_build_version
    new_version_config["WINDOWS_DDNPM_DRIVER"] = windows_ddnpm_driver
    new_version_config["WINDOWS_DDNPM_VERSION"] = windows_ddnpm_version
    new_version_config["WINDOWS_DDNPM_SHASUM"] = windows_ddnpm_shasum

    # Necessary if we want to maintain the JSON order, so that humans don't get confused
    new_release_json = OrderedDict()

    # Add all versions from the old release.json
    for key, value in release_json.items():
        new_release_json[key] = value

    # Then update the entry
    new_release_json[release_entry] = _stringify_config(new_version_config)

    return new_release_json


##
## Main functions
##


def _update_release_json(release_json, release_entry, new_version: Version, max_version: Version, github_token):
    """
    Updates the provided release.json object by fetching compatible versions for all dependencies
    of the provided Agent version, constructing the new entry, adding it to the release.json object
    and returning it.
    """

    allowed_major_versions = COMPATIBLE_MAJOR_VERSIONS[new_version.major]

    # Part 1: repositories which follow the Agent version scheme

    # For repositories which follow the Agent version scheme, we want to only get
    # tags with the same minor version, to avoid problems when releasing a patch
    # version while a minor version release is ongoing.
    compatible_version_re = build_compatible_version_re(allowed_major_versions, new_version.minor)

    # If the new version is a final version, set the check_for_rc flag to true to warn if a dependency's version
    # is an RC.
    check_for_rc = not new_version.is_rc()

    integrations_version = _fetch_dependency_repo_version(
        "integrations-core",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        github_token,
        check_for_rc,
    )

    omnibus_software_version = _fetch_dependency_repo_version(
        "omnibus-software",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        github_token,
        check_for_rc,
    )

    omnibus_ruby_version = _fetch_dependency_repo_version(
        "omnibus-ruby",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        github_token,
        check_for_rc,
    )

    macos_build_version = _fetch_dependency_repo_version(
        "datadog-agent-macos-build",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        github_token,
        check_for_rc,
    )

    # Part 2: repositories which have their own version scheme
    jmxfetch_version = _fetch_independent_dependency_repo_version(
        "jmxfetch", release_json, new_version.major, github_token, "JMXFETCH_VERSION"
    )

    # security agent policies are updated directly by the CWS team
    security_agent_policies_version = _get_release_version_from_release_json(
        release_json, new_version.major, VERSION_RE, "SECURITY_AGENT_POLICIES_VERSION"
    )
    print(TAG_FOUND_TEMPLATE.format("security-agent-policies", security_agent_policies_version))

    windows_ddnpm_driver, windows_ddnpm_version, windows_ddnpm_shasum = _get_windows_ddnpm_release_json_info(
        release_json, new_version.major, is_first_rc=(new_version.rc == 1)
    )

    # Add new entry to the release.json object and return it
    return _update_release_json_entry(
        release_json,
        release_entry,
        integrations_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version,
        security_agent_policies_version,
        macos_build_version,
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
    )


def update_release_json(github_token, new_version: Version, max_version: Version):
    """
    Updates the release entries in release.json to prepare the next RC or final build.
    """
    release_json = _load_release_json()

    release_entry = release_entry_for(new_version.major)
    print(f"Updating {release_entry} for {new_version}")

    # Update release.json object with the entry for the new version
    release_json = _update_release_json(release_json, release_entry, new_version, max_version, github_token)

    _save_release_json(release_json)


def check_version(agent_version):
    """Check Agent version to see if it is valid."""
    version_re = re.compile(r'7[.](\d+)[.](\d+)(-rc\.(\d+))?')
    if not version_re.match(agent_version):
        raise Exit(message="Version should be of the form 7.Y.Z or 7.Y.Z-rc.t")


@task
def update_modules(ctx, agent_version, verify=True):
    """
    Update internal dependencies between the different Agent modules.
    * --verify checks for correctness on the Agent Version (on by default).

    Examples:
    inv -e release.update-modules 7.27.0
    """
    if verify:
        check_version(agent_version)

    for module in DEFAULT_MODULES.values():
        for dependency in module.dependencies:
            dependency_mod = DEFAULT_MODULES[dependency]
            ctx.run(f"go mod edit -require={dependency_mod.dependency_path(agent_version)} {module.go_mod_path()}")


def __get_force_option(force: bool) -> str:
    """Get flag to pass to git tag depending on if we want forcing or not."""
    force_option = ""
    if force:
        print(color_message("--force option enabled. This will allow the task to overwrite existing tags.", "orange"))
        result = yes_no_question("Please confirm the use of the --force option.", color="orange", default=False)
        if result:
            print("Continuing with the --force option.")
            force_option = " --force"
        else:
            print("Continuing without the --force option.")
    return force_option


def __tag_single_module(ctx, module, agent_version, commit, push, force_option):
    """Tag a given module."""
    for tag in module.tag(agent_version):
        ok = try_git_command(
            ctx,
            f"git tag -m {tag} {tag} {commit}{force_option}",
        )
        if not ok:
            message = f"Could not create tag {tag}. Please rerun the task to retry creating the tags (you may need the --force option)"
            raise Exit(color_message(message, "red"), code=1)
        print(f"Created tag {tag}")
        if push:
            ctx.run(f"git push origin {tag}{force_option}")
            print(f"Pushed tag {tag}")


@task
def tag_modules(ctx, agent_version, commit="HEAD", verify=True, push=True, force=False):
    """
    Create tags for Go nested modules for a given Datadog Agent version.
    The version should be given as an Agent 7 version.

    * --commit COMMIT will tag COMMIT with the tags (default HEAD)
    * --verify checks for correctness on the Agent version (on by default).
    * --push will push the tags to the origin remote (on by default).
    * --force will allow the task to overwrite existing tags. Needed to move existing tags (off by default).

    Examples:
    inv -e release.tag-modules 7.27.0                 # Create tags and push them to origin
    inv -e release.tag-modules 7.27.0-rc.3 --no-push  # Create tags locally; don't push them
    inv -e release.tag-modules 7.29.0-rc.3 --force    # Create tags (overwriting existing tags with the same name), force-push them to origin

    """
    if verify:
        check_version(agent_version)

    force_option = __get_force_option(force)
    for module in DEFAULT_MODULES.values():
        # Skip main module; this is tagged at tag_version via __tag_single_module.
        if module.should_tag and module.path != ".":
            __tag_single_module(ctx, module, agent_version, commit, push, force_option)

    print(f"Created module tags for version {agent_version}")


@task
def tag_version(ctx, agent_version, commit="HEAD", verify=True, push=True, force=False):
    """
    Create tags for a given Datadog Agent version.
    The version should be given as an Agent 7 version.

    * --commit COMMIT will tag COMMIT with the tags (default HEAD)
    * --verify checks for correctness on the Agent version (on by default).
    * --push will push the tags to the origin remote (on by default).
    * --force will allow the task to overwrite existing tags. Needed to move existing tags (off by default).

    Examples:
    inv -e release.tag-version 7.27.0                 # Create tags and push them to origin
    inv -e release.tag-version 7.27.0-rc.3 --no-push  # Create tags locally; don't push them
    inv -e release.tag-version 7.29.0-rc.3 --force    # Create tags (overwriting existing tags with the same name), force-push them to origin
    """
    if verify:
        check_version(agent_version)

    # Always tag the main module
    force_option = __get_force_option(force)
    __tag_single_module(ctx, DEFAULT_MODULES["."], agent_version, commit, push, force_option)
    print(f"Created tags for version {agent_version}")


def current_version(ctx, major_version) -> Version:
    return _create_version_from_match(VERSION_RE.search(get_version(ctx, major_version=major_version)))


def next_final_version(ctx, major_version, patch_version) -> Version:
    previous_version = current_version(ctx, major_version)

    # Set the new version
    if previous_version.is_devel():
        # If the previous version was a devel version, use the same version without devel
        # (should never happen during regular releases, we always do at least one RC)
        return previous_version.non_devel_version()
    if previous_version.is_rc():
        # If the previous version was an RC version, use the same version without RC
        return previous_version.next_version(rc=False)

    # Else, the latest version was a final release, so we use the next release
    # (eg. 7.32.1 from 7.32.0).
    if patch_version:
        return previous_version.next_version(bump_patch=True, rc=False)
    else:
        return previous_version.next_version(bump_minor=True, rc=False)


def next_rc_version(ctx, major_version, patch_version=False) -> Version:
    # Fetch previous version from the most recent tag on the branch
    previous_version = current_version(ctx, major_version)

    if previous_version.is_rc():
        # We're already on an RC, only bump the RC version
        new_version = previous_version.next_version(rc=True)
    else:
        if patch_version:
            new_version = previous_version.next_version(bump_patch=True, rc=True)
        else:
            # Minor version bump, we're doing a standard release:
            # - if the previous tag is a devel tag, use it without the devel tag
            # - otherwise (should not happen during regular release cycles), bump the minor version
            if previous_version.is_devel():
                new_version = previous_version.non_devel_version()
                new_version = new_version.next_version(rc=True)
            else:
                new_version = previous_version.next_version(bump_minor=True, rc=True)

    return new_version


def check_base_branch(branch, release_version):
    """
    Checks if the given branch is either the default branch or the release branch associated
    with the given release version.
    """
    return branch == DEFAULT_BRANCH or branch == release_version.branch()


def check_uncommitted_changes(ctx):
    """
    Checks if there are uncommitted changes in the local git repository.
    """
    modified_files = ctx.run("git --no-pager diff --name-only HEAD | wc -l", hide=True).stdout.strip()

    # Return True if at least one file has uncommitted changes.
    return modified_files != "0"


def check_local_branch(ctx, branch):
    """
    Checks if the given branch exists locally
    """
    matching_branch = ctx.run(f"git --no-pager branch --list {branch} | wc -l", hide=True).stdout.strip()

    # Return True if a branch is returned by git branch --list
    return matching_branch != "0"


def check_upstream_branch(github, branch):
    """
    Checks if the given branch already exists in the upstream repository
    """
    try:
        github_branch = github.get_branch(branch)
    except APIError as e:
        if e.status_code == 404:
            return False
        raise e

    # Return True if the branch exists
    return github_branch and github_branch.get('name', False)


def parse_major_versions(major_versions):
    return sorted(int(x) for x in major_versions.split(","))


def try_git_command(ctx, git_command):
    """
    Try a git command that should be retried (after user confirmation) if it fails.
    Primarily useful for commands which can fail if commit signing fails: we don't want the
    whole workflow to fail if that happens, we want to retry.
    """

    do_retry = True

    while do_retry:
        res = ctx.run(git_command, warn=True)
        if res.exited is None or res.exited > 0:
            print(
                color_message(
                    f"Failed to run \"{git_command}\" (did the commit/tag signing operation fail?)",
                    "orange",
                )
            )
            do_retry = yes_no_question("Do you want to retry this operation?", color="orange", default=True)
            continue

        return True

    return False


@task
def finish(ctx, major_versions="6,7"):
    """
    Updates the release entry in the release.json file for the new version.

    Updates internal module dependencies with the new version.
    """

    if sys.version_info[0] < 3:
        return Exit(message="Must use Python 3 for this task", code=1)

    list_major_versions = parse_major_versions(major_versions)
    print(f"Finishing release for major version(s) {list_major_versions}")

    github_token = get_github_token()

    for major_version in list_major_versions:
        # NOTE: the release process assumes that at least one RC
        # was built before release.finish is used. It doesn't support
        # doing final version -> final version updates (eg. 7.32.0 -> 7.32.1
        # without doing at least 7.32.1-rc.1), as next_final_version won't
        # find the correct new version.
        # To support this, we'd have to support a --patch-version param in
        # release.finish
        new_version = next_final_version(ctx, major_version, False)
        update_release_json(github_token, new_version, new_version)

    # Update internal module dependencies
    update_modules(ctx, str(new_version))


@task(help={'upstream': "Remote repository name (default 'origin')"})
def create_rc(ctx, major_versions="6,7", patch_version=False, upstream="origin"):
    """
    Updates the release entries in release.json to prepare the next RC build.
    If the previous version of the Agent (determined as the latest tag on the
    current branch) is not an RC:
    - by default, updates the release entries for the next minor version of
      the Agent.
    - if --patch-version is specified, updates the release entries for the next
      patch version of the Agent.

    This changes which tags will be considered on the dependency repositories (only
    tags that match the same major and minor version as the Agent).

    If the previous version of the Agent was an RC, updates the release entries for RC + 1.

    Examples:
    If the latest tag on the branch is 7.31.0, and invoke release.create-rc --patch-version
    is run, then the task will prepare the release entries for 7.31.1-rc.1, and therefore
    will only use 7.31.X tags on the dependency repositories that follow the Agent version scheme.

    If the latest tag on the branch is 7.32.0-devel or 7.31.0, and invoke release.create-rc
    is run, then the task will prepare the release entries for 7.32.0-rc.1, and therefore
    will only use 7.32.X tags on the dependency repositories that follow the Agent version scheme.

    Updates internal module dependencies with the new RC.

    Commits the above changes, and then creates a PR on the upstream repository with the change.

    Notes:
    This requires a Github token (either in the GITHUB_TOKEN environment variable, or in the MacOS keychain),
    with 'repo' permissions.
    This also requires that there are no local uncommitted changes, that the current branch is 'main' or the
    release branch, and that no branch named 'release/<new rc version>' already exists locally or upstream.
    """
    if sys.version_info[0] < 3:
        return Exit(message="Must use Python 3 for this task", code=1)

    github = GithubAPI(repository=REPOSITORY_NAME, api_token=get_github_token())

    list_major_versions = parse_major_versions(major_versions)

    # Get the version of the highest major: useful for some logging & to get
    # the version to use for Go submodules updates
    new_highest_version = next_rc_version(ctx, max(list_major_versions), patch_version)
    # Get the next final version of the highest major: useful to know which
    # milestone to target, as well as decide which tags from dependency repositories
    # can be used.
    new_final_version = next_final_version(ctx, max(list_major_versions), patch_version)

    # Get a string representation of the RC, eg. "6/7.32.0-rc.1"
    versions_string = f"{'/'.join([str(n) for n in list_major_versions[:-1]] + [str(new_highest_version)])}"

    print(color_message(f"Preparing RC for agent version(s) {list_major_versions}", "bold"))

    # Step 0: checks

    print(color_message("Checking repository state", "bold"))
    ctx.run("git fetch")

    if check_uncommitted_changes(ctx):
        raise Exit(
            color_message(
                "There are uncomitted changes in your repository. Please commit or stash them before trying again.",
                "red",
            ),
            code=1,
        )

    # Check that the current and update branches are valid
    current_branch = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    update_branch = f"release/{new_highest_version}"

    if not check_base_branch(current_branch, new_highest_version):
        raise Exit(
            color_message(
                f"The branch you are on is neither {DEFAULT_BRANCH} or the correct release branch ({new_highest_version.branch()}). Aborting.",
                "red",
            ),
            code=1,
        )

    if check_local_branch(ctx, update_branch):
        raise Exit(
            color_message(
                f"The branch {update_branch} already exists locally. Please remove it before trying again.",
                "red",
            ),
            code=1,
        )

    if check_upstream_branch(github, update_branch):
        raise Exit(
            color_message(
                f"The branch {update_branch} already exists upstream. Please remove it before trying again.",
                "red",
            ),
            code=1,
        )

    # Find milestone based on what the next final version is. If the milestone does not exist, fail.
    milestone_name = str(new_final_version)

    milestone = github.get_milestone_by_name(milestone_name)

    if not milestone or not milestone.get("number"):
        raise Exit(
            color_message(
                f"""Could not find milestone {milestone_name} in the Github repository. Response: {milestone}
Make sure that milestone is open before trying again.""",
                "red",
            ),
            code=1,
        )

    # Step 1: Update release entries

    print(color_message("Updating release entries", "bold"))
    for major_version in list_major_versions:
        new_version = next_rc_version(ctx, major_version, patch_version)
        update_release_json(github.api_token, new_version, new_final_version)

    # Step 2: Update internal module dependencies

    print(color_message("Updating Go modules", "bold"))
    update_modules(ctx, str(new_highest_version))

    # Step 3: branch out, commit change, push branch

    print(color_message(f"Branching out to {update_branch}", "bold"))
    ctx.run(f"git checkout -b {update_branch}")

    print(color_message("Committing release.json and Go modules updates", "bold"))
    print(
        color_message(
            "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
        )
    )
    ctx.run("git add release.json")
    ctx.run("git ls-files . | grep 'go.mod$' | xargs git add")

    ok = try_git_command(ctx, f"git commit -m 'Update release.json and Go modules for {versions_string}'")
    if not ok:
        raise Exit(
            color_message(
                f"Could not create commit. Please commit manually, push the {update_branch} branch and then open a PR against {current_branch}.",
                "red",
            ),
            code=1,
        )

    print(color_message("Pushing new branch to the upstream repository", "bold"))
    res = ctx.run(f"git push --set-upstream {upstream} {update_branch}", warn=True)
    if res.exited is None or res.exited > 0:
        raise Exit(
            color_message(
                f"Could not push branch {update_branch} to the upstream '{upstream}'. Please push it manually and then open a PR against {current_branch}.",
                "red",
            ),
            code=1,
        )

    print(color_message("Creating PR", "bold"))

    # Step 4: create PR

    pr = github.create_pr(
        pr_title=f"[release] Update release.json and Go modules for {versions_string}",
        pr_body="",
        base_branch=current_branch,
        target_branch=update_branch,
    )

    if not pr or not pr.get("number"):
        raise Exit(
            color_message(f"Could not create PR in the Github repository. Response: {pr}", "red"),
            code=1,
        )

    print(color_message(f"Created PR #{pr['number']}", "bold"))

    # Step 5: add milestone and labels to PR

    updated_pr = github.update_pr(
        pull_number=pr["number"],
        milestone_number=milestone["number"],
        labels=["changelog/no-changelog", "qa/skip-qa", "team/agent-platform", "team/agent-core"],
    )

    if not updated_pr or not updated_pr.get("number") or not updated_pr.get("html_url"):
        raise Exit(
            color_message(f"Could not update PR in the Github repository. Response: {updated_pr}", "red"),
            code=1,
        )

    print(color_message(f"Set labels and milestone for PR #{updated_pr['number']}", "bold"))
    print(
        color_message(
            f"Done preparing RC {versions_string}. The PR is available here: {updated_pr['html_url']}", "bold"
        )
    )


@task
def build_rc(ctx, major_versions="6,7", patch_version=False):
    """
    To be done after the PR created by release.create-rc is merged, with the same options
    as release.create-rc.

    Tags the new RC versions on the current commit, and creates the build pipeline for these
    new tags.
    """
    if sys.version_info[0] < 3:
        return Exit(message="Must use Python 3 for this task", code=1)

    gitlab = Gitlab(project_name=REPOSITORY_NAME, api_token=get_gitlab_token())
    list_major_versions = parse_major_versions(major_versions)

    # Get the version of the highest major: needed for tag_version and to know
    # which tag to target when creating the pipeline.
    new_version = next_rc_version(ctx, max(list_major_versions), patch_version)

    # Get a string representation of the RC, eg. "6/7.32.0-rc.1"
    versions_string = f"{'/'.join([str(n) for n in list_major_versions[:-1]] + [str(new_version)])}"

    # Step 0: checks

    print(color_message("Checking repository state", "bold"))
    # Check that the base branch is valid
    current_branch = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    if not check_base_branch(current_branch, new_version):
        raise Exit(
            color_message(
                f"The branch you are on is neither {DEFAULT_BRANCH} or the correct release branch ({new_version.branch()}). Aborting.",
                "red",
            ),
            code=1,
        )

    latest_commit = ctx.run("git --no-pager log --no-color -1 --oneline").stdout.strip()

    if not yes_no_question(
        f"This task will create tags for {versions_string} on the current commit: {latest_commit}. Is this OK?",
        color="orange",
        default=False,
    ):
        raise Exit(color_message("Aborting.", "red"), code=1)

    # Step 1: Tag versions

    print(color_message(f"Tagging RC for agent version(s) {list_major_versions}", "bold"))
    print(
        color_message("If commit signing is enabled, you will have to make sure each tag gets properly signed.", "bold")
    )
    # tag_version only takes the highest version (Agent 7 currently), and creates
    # the tags for all supported versions
    # TODO: make it possible to do Agent 6-only or Agent 7-only tags?
    tag_version(ctx, str(new_version), force=False)
    tag_modules(ctx, str(new_version), force=False)

    print(color_message(f"Waiting until the {new_version} tag appears in Gitlab", "bold"))
    gitlab_tag = None
    while not gitlab_tag:
        gitlab_tag = gitlab.find_tag(str(new_version)).get("name", None)
        sleep(5)

    print(color_message("Creating RC pipeline", "bold"))

    # Step 2: Run the RC pipeline

    run(
        ctx,
        git_ref=gitlab_tag,
        use_release_entries=True,
        major_versions=major_versions,
        repo_branch="beta",
        deploy=True,
    )


@task(help={'key': "Path to the release.json key, separated with double colons, eg. 'last_stable::6'"})
def get_release_json_value(_, key):

    release_json = _load_release_json()

    path = key.split('::')

    for element in path:
        if element not in release_json:
            raise Exit(code=1, message=f"Couldn't find '{key}' in release.json")

        release_json = release_json.get(element)

    print(release_json)


def create_release_branch(ctx, repo, release_branch, base_directory="~/dd", upstream="origin"):
    # Perform branch out in all required repositories
    with ctx.cd(f"{base_directory}/{repo}"):
        # Step 1 - Create a local branch out from the default branch

        print(color_message(f"Working repository: {repo}", "bold"))
        main_branch = ctx.run(f"git remote show {upstream} | grep \"HEAD branch\" | sed 's/.*: //'").stdout.strip()
        ctx.run(f"git checkout {main_branch}")
        ctx.run("git pull")
        print(color_message(f"Branching out to {release_branch}", "bold"))
        ctx.run(f"git checkout -b {release_branch}")

        if repo == UNFREEZE_REPO_AGENT:
            rj = _load_release_json()
            rj["base_branch"] = release_branch
            _save_release_json(rj)
            ctx.run("git add release.json")
            ok = try_git_command(ctx, f"git commit -m 'Set base_branch to {release_branch}'")
            if not ok:
                raise Exit(
                    color_message(
                        f"Could not create commit. Please commit manually and push the commit to the {release_branch} branch.",
                        "red",
                    ),
                    code=1,
                )

        # Step 2 - Push newly created release branch to the remote repository

        print(color_message("Pushing new branch to the upstream repository", "bold"))
        res = ctx.run(f"git push --set-upstream {upstream} {release_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {release_branch} to the upstream '{upstream}'. Please push it manually.",
                    "red",
                ),
                code=1,
            )


@task(help={'upstream': "Remote repository name (default 'origin')"})
def unfreeze(ctx, base_directory="~/dd", major_versions="6,7", upstream="origin", redo=False):
    """
    Performs set of tasks required for the main branch unfreeze during the agent release cycle.
    That includes:
    - creates a release branch in datadog-agent, omnibus-ruby and omnibus-software repositories,
    - pushes an empty commit on the datadog-agent main branch,
    - creates devel tags in the datadog-agent repository on the empty commit from the last step.

    Notes:
    base_directory - path to the directory where dd repos are cloned, defaults to ~/dd, but can be overwritten.
    This requires a Github token (either in the GITHUB_TOKEN environment variable, or in the MacOS keychain),
    with 'repo' permissions.
    This also requires that there are no local uncommitted changes, that the current branch is 'main' or the
    release branch, and that no branch named 'release/<new rc version>' already exists locally or upstream.
    """
    if sys.version_info[0] < 3:
        return Exit(message="Must use Python 3 for this task", code=1)

    list_major_versions = parse_major_versions(major_versions)

    current = current_version(ctx, max(list_major_versions))
    next = current.next_version(bump_minor=True)
    next.devel = True

    # Strings with proper branch/tag names
    release_branch = current.branch()
    devel_tag = str(next)

    # Step 0: checks

    print(color_message("Checking repository state", "bold"))
    ctx.run("git fetch")

    if check_uncommitted_changes(ctx):
        raise Exit(
            color_message(
                "There are uncomitted changes in your repository. Please commit or stash them before trying again.",
                "red",
            ),
            code=1,
        )

    if not yes_no_question(
        f"This task will create new branches with the name '{release_branch}' in repositories: {', '.join(UNFREEZE_REPOS)}. Is this OK?",
        color="orange",
        default=False,
    ):
        raise Exit(color_message("Aborting.", "red"), code=1)

    # Step 1: Create release branch
    for repo in UNFREEZE_REPOS:
        create_release_branch(ctx, repo, release_branch, base_directory=base_directory)

    print(color_message("Creating empty commit for devel tags", "bold"))
    with ctx.cd(f"{base_directory}/datadog-agent"):
        ctx.run("git checkout main")
        ok = try_git_command(ctx, "git commit --allow-empty -m 'Empty commit for next release devel tags'")
        if not ok:
            raise Exit(
                color_message(
                    "Could not create commit. Please commit manually, push the commit manually to the main branch.",
                    "red",
                ),
                code=1,
            )

        print(color_message("Pushing new commit", "bold"))
        res = ctx.run(f"git push {upstream}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push commit to the upstream '{upstream}'. Please push it manually.",
                    "red",
                ),
                code=1,
            )

    # Step 3: Create tags for next version
    print(color_message(f"Creating devel tags for agent version(s) {list_major_versions}", "bold"))
    print(
        color_message("If commit signing is enabled, you will have to make sure each tag gets properly signed.", "bold")
    )

    tag_version(ctx, devel_tag, tag_modules=False, push=True, force=redo)
