"""
Release helper tasks
"""

import json
import os
import re
import sys
import tempfile
from collections import OrderedDict
from datetime import date
from time import sleep

from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab import Gitlab, get_gitlab_token
from tasks.libs.common.color import color_message
from tasks.libs.common.user_interactions import yes_no_question
from tasks.libs.common.utils import (
    DEFAULT_BRANCH,
    GITHUB_REPO_NAME,
    check_clean_branch_state,
    get_version,
    nightly_entry_for,
    release_entry_for,
)
from tasks.libs.types.version import Version
from tasks.modules import DEFAULT_MODULES
from tasks.pipeline import edit_schedule, run

# Generic version regex. Aims to match:
# - X.Y.Z
# - X.Y.Z-rc.t
# - X.Y.Z-devel
# - vX.Y(.Z) (security-agent-policies repo)
VERSION_RE = re.compile(r'(v)?(\d+)[.](\d+)([.](\d+))?(-devel)?(-rc\.(\d+))?')

# Regex matching rc version tag format like 7.50.0-rc.1
RC_VERSION_RE = re.compile(r'\d+[.]\d+[.]\d+-rc\.\d+')

UNFREEZE_REPO_AGENT = "datadog-agent"
UNFREEZE_REPOS = [UNFREEZE_REPO_AGENT, "omnibus-software", "omnibus-ruby", "datadog-agent-macos-build"]
RELEASE_JSON_FIELDS_TO_UPDATE = [
    "INTEGRATIONS_CORE_VERSION",
    "OMNIBUS_SOFTWARE_VERSION",
    "OMNIBUS_RUBY_VERSION",
    "MACOS_BUILD_VERSION",
]


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
    print("\nIf not run as part of finish task, commit this with:")
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
    Pinned to datadog-agent v{agent7_version}: `CHANGELOG <https://github.com/{GITHUB_REPO_NAME}/blob/{DEFAULT_BRANCH}/CHANGELOG.rst#{agent7_version.replace('.', '')}{agent6_version}>`_."""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nIf not run as part of finish task, commit this with:")
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
def update_changelog(ctx, new_version=None, target="all", upstream="origin"):
    """
    Quick task to generate the new CHANGELOG using reno when releasing a minor
    version (linux/macOS only).
    By default generates Agent and Cluster Agent changelogs.
    Use target == "agent" or target == "cluster-agent" to only generate one or the other.
    If new_version is omitted, a changelog since last tag on the current branch
    will be generated.
    """

    # Step 1 - generate the changelogs

    generate_agent = target in ["all", "agent"]
    generate_cluster_agent = target in ["all", "cluster-agent"]

    if new_version is not None:
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

        # let's check that the tag for the new version is present
        try:
            ctx.run(f"git tag --list | grep {new_version}")
        except Failure:
            print(f"Missing '{new_version}' git tag: mandatory to use 'reno'")
            raise

    if generate_agent:
        update_changelog_generic(ctx, new_version, "releasenotes", "CHANGELOG.rst")
    if generate_cluster_agent:
        update_changelog_generic(ctx, new_version, "releasenotes-dca", "CHANGELOG-DCA.rst")

    # Step 2 - commit changes

    update_branch = f"changelog-update-{new_version}"
    base_branch = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    print(color_message(f"Branching out to {update_branch}", "bold"))
    ctx.run(f"git checkout -b {update_branch}")

    print(color_message("Committing CHANGELOG.rst and CHANGELOG-DCA.rst", "bold"))
    print(
        color_message(
            "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
        )
    )
    ctx.run("git add CHANGELOG.rst CHANGELOG-DCA.rst")

    commit_message = f"'Changelog updates for {new_version} release'"

    ok = try_git_command(ctx, f"git commit -m {commit_message}")
    if not ok:
        raise Exit(
            color_message(
                f"Could not create commit. Please commit manually with:\ngit commit -m {commit_message}\n, push the {update_branch} branch and then open a PR.",
                "red",
            ),
            code=1,
        )

    # Step 3 - Push and create PR

    print(color_message("Pushing new branch to the upstream repository", "bold"))
    res = ctx.run(f"git push --set-upstream {upstream} {update_branch}", warn=True)
    if res.exited is None or res.exited > 0:
        raise Exit(
            color_message(
                f"Could not push branch {update_branch} to the upstream '{upstream}'. Please push it manually and then open a PR.",
                "red",
            ),
            code=1,
        )

    create_pr(f"Changelog update for {new_version} release", base_branch, update_branch, new_version, changelog_pr=True)


def update_changelog_generic(ctx, new_version, changelog_dir, changelog_file):
    if new_version is None:
        latest_version = current_version(ctx, 7)
        ctx.run(f"reno -q --rel-notes-dir {changelog_dir} report --ignore-cache --earliest-version {latest_version}")
        return
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

    gh = GithubAPI()
    pull_requests = gh.get_pulls(milestone=milestone, labels=['major_change'])
    if pull_requests is None:
        return
    if len(pull_requests) == 0:
        print(f"no major change for {milestone}")
        return

    for pr in pull_requests:
        print(f"#{pr.number}: {pr.title} ({pr.html_url})")


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


def build_compatible_version_re(allowed_major_versions, minor_version):
    """
    Returns a regex that matches only versions whose major version is
    in the provided list of allowed_major_versions, and whose minor version matches
    the provided minor version.
    """
    return re.compile(
        r'(v)?({})[.]({})([.](\d+))+(-devel)?(-rc\.(\d+))?(?!-\w)'.format(  # noqa: FS002
            "|".join(allowed_major_versions), minor_version
        )
    )


##
## Base functions to fetch candidate versions on other repositories
##


def _get_highest_repo_version(
    repo, version_prefix, version_re, allowed_major_versions=None, max_version: Version = None
):
    # If allowed_major_versions is not specified, search for all versions by using an empty
    # major version prefix.
    if not allowed_major_versions:
        allowed_major_versions = [""]

    highest_version = None

    gh = GithubAPI(repository=f'Datadog/{repo}')

    for major_version in allowed_major_versions:
        tags = gh.get_tags(f'{version_prefix}{major_version}')

        for tag in tags:
            match = version_re.search(tag.name)
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


def _get_jmxfetch_release_json_info(release_json, agent_major_version, is_first_rc=False):
    """
    Gets the JMXFetch version info from the previous entries in the release.json file.
    """

    release_json_version_data = _get_release_json_info_for_next_rc(release_json, agent_major_version, is_first_rc)

    jmxfetch_version = release_json_version_data['JMXFETCH_VERSION']
    jmxfetch_shasum = release_json_version_data['JMXFETCH_HASH']

    print(f"The JMXFetch version is {jmxfetch_version}")

    return jmxfetch_version, jmxfetch_shasum


def _get_windows_release_json_info(release_json, agent_major_version, is_first_rc=False):
    """
    Gets the Windows NPM driver info from the previous entries in the release.json file.
    """
    release_json_version_data = _get_release_json_info_for_next_rc(release_json, agent_major_version, is_first_rc)

    win_ddnpm_driver, win_ddnpm_version, win_ddnpm_shasum = _get_windows_driver_info(release_json_version_data, 'DDNPM')
    win_ddprocmon_driver, win_ddprocmon_version, win_ddprocmon_shasum = _get_windows_driver_info(
        release_json_version_data, 'DDPROCMON'
    )

    return (
        win_ddnpm_driver,
        win_ddnpm_version,
        win_ddnpm_shasum,
        win_ddprocmon_driver,
        win_ddprocmon_version,
        win_ddprocmon_shasum,
    )


def _get_windows_driver_info(release_json_version_data, driver_name):
    """
    Gets the Windows driver info from the release.json version data.
    """
    driver_key = f'WINDOWS_{driver_name}_DRIVER'
    version_key = f'WINDOWS_{driver_name}_VERSION'
    shasum_key = f'WINDOWS_{driver_name}_SHASUM'

    driver_value = release_json_version_data[driver_key]
    version_value = release_json_version_data[version_key]
    shasum_value = release_json_version_data[shasum_key]

    if driver_value not in ['release-signed', 'attestation-signed']:
        print(f"WARN: {driver_key} value '{driver_value}' is not valid")

    print(f"The windows {driver_name.lower()} version is {version_value}")

    return driver_value, version_value, shasum_value


def _get_release_json_info_for_next_rc(release_json, agent_major_version, is_first_rc=False):
    """
    Gets the version info from the previous entries in the release.json file.
    """

    # First RC should use the data from nightly section otherwise reuse the last RC info
    if is_first_rc:
        previous_release_json_version = nightly_entry_for(agent_major_version)
    else:
        previous_release_json_version = release_entry_for(agent_major_version)

    print(f"Using '{previous_release_json_version}' values")

    return release_json[previous_release_json_version]


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
    jmxfetch_shasum,
    security_agent_policies_version,
    macos_build_version,
    windows_ddnpm_driver,
    windows_ddnpm_version,
    windows_ddnpm_shasum,
    windows_ddprocmon_driver,
    windows_ddprocmon_version,
    windows_ddprocmon_shasum,
):
    """
    Adds a new entry to provided release_json object with the provided parameters, and returns the new release_json object.
    """

    print(f"Jmxfetch's SHA256 is {jmxfetch_shasum}")
    print(f"Windows DDNPM's SHA256 is {windows_ddnpm_shasum}")
    print(f"Windows DDPROCMON's SHA256 is {windows_ddprocmon_shasum}")

    new_version_config = OrderedDict()
    new_version_config["INTEGRATIONS_CORE_VERSION"] = integrations_version
    new_version_config["OMNIBUS_SOFTWARE_VERSION"] = omnibus_software_version
    new_version_config["OMNIBUS_RUBY_VERSION"] = omnibus_ruby_version
    new_version_config["JMXFETCH_VERSION"] = jmxfetch_version
    new_version_config["JMXFETCH_HASH"] = jmxfetch_shasum
    new_version_config["SECURITY_AGENT_POLICIES_VERSION"] = security_agent_policies_version
    new_version_config["MACOS_BUILD_VERSION"] = macos_build_version
    new_version_config["WINDOWS_DDNPM_DRIVER"] = windows_ddnpm_driver
    new_version_config["WINDOWS_DDNPM_VERSION"] = windows_ddnpm_version
    new_version_config["WINDOWS_DDNPM_SHASUM"] = windows_ddnpm_shasum
    new_version_config["WINDOWS_DDPROCMON_DRIVER"] = windows_ddprocmon_driver
    new_version_config["WINDOWS_DDPROCMON_VERSION"] = windows_ddprocmon_version
    new_version_config["WINDOWS_DDPROCMON_SHASUM"] = windows_ddprocmon_shasum

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


def _update_release_json(release_json, release_entry, new_version: Version, max_version: Version):
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
        check_for_rc,
    )

    omnibus_software_version = _fetch_dependency_repo_version(
        "omnibus-software",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        check_for_rc,
    )

    omnibus_ruby_version = _fetch_dependency_repo_version(
        "omnibus-ruby",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        check_for_rc,
    )

    macos_build_version = _fetch_dependency_repo_version(
        "datadog-agent-macos-build",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        check_for_rc,
    )

    # Part 2: repositories which have their own version scheme

    # jmxfetch version is updated directly by the AML team
    jmxfetch_version, jmxfetch_shasum = _get_jmxfetch_release_json_info(
        release_json, new_version.major, is_first_rc=(new_version.rc == 1)
    )

    # security agent policies are updated directly by the CWS team
    security_agent_policies_version = _get_release_version_from_release_json(
        release_json, new_version.major, VERSION_RE, "SECURITY_AGENT_POLICIES_VERSION"
    )
    print(TAG_FOUND_TEMPLATE.format("security-agent-policies", security_agent_policies_version))

    (
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
        windows_ddprocmon_driver,
        windows_ddprocmon_version,
        windows_ddprocmon_shasum,
    ) = _get_windows_release_json_info(release_json, new_version.major, is_first_rc=(new_version.rc == 1))

    # Add new entry to the release.json object and return it
    return _update_release_json_entry(
        release_json,
        release_entry,
        integrations_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version,
        jmxfetch_shasum,
        security_agent_policies_version,
        macos_build_version,
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
        windows_ddprocmon_driver,
        windows_ddprocmon_version,
        windows_ddprocmon_shasum,
    )


def update_release_json(new_version: Version, max_version: Version):
    """
    Updates the release entries in release.json to prepare the next RC or final build.
    """
    release_json = _load_release_json()

    release_entry = release_entry_for(new_version.major)
    print(f"Updating {release_entry} for {new_version}")

    # Update release.json object with the entry for the new version
    release_json = _update_release_json(release_json, release_entry, new_version, max_version)

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


def __tag_single_module(ctx, module, agent_version, commit, push, force_option, devel):
    """Tag a given module."""
    for tag in module.tag(agent_version):

        if devel:
            tag += "-devel"

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
def tag_modules(ctx, agent_version, commit="HEAD", verify=True, push=True, force=False, devel=False):
    """
    Create tags for Go nested modules for a given Datadog Agent version.
    The version should be given as an Agent 7 version.

    * --commit COMMIT will tag COMMIT with the tags (default HEAD)
    * --verify checks for correctness on the Agent version (on by default).
    * --push will push the tags to the origin remote (on by default).
    * --force will allow the task to overwrite existing tags. Needed to move existing tags (off by default).
    * --devel will create -devel tags (used after creation of the release branch)

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
            __tag_single_module(ctx, module, agent_version, commit, push, force_option, devel)

    print(f"Created module tags for version {agent_version}")


@task
def tag_version(ctx, agent_version, commit="HEAD", verify=True, push=True, force=False, devel=False):
    """
    Create tags for a given Datadog Agent version.
    The version should be given as an Agent 7 version.

    * --commit COMMIT will tag COMMIT with the tags (default HEAD)
    * --verify checks for correctness on the Agent version (on by default).
    * --push will push the tags to the origin remote (on by default).
    * --force will allow the task to overwrite existing tags. Needed to move existing tags (off by default).
    * --devel will create -devel tags (used after creation of the release branch)

    Examples:
    inv -e release.tag-version 7.27.0                 # Create tags and push them to origin
    inv -e release.tag-version 7.27.0-rc.3 --no-push  # Create tags locally; don't push them
    inv -e release.tag-version 7.29.0-rc.3 --force    # Create tags (overwriting existing tags with the same name), force-push them to origin
    """
    if verify:
        check_version(agent_version)

    # Always tag the main module
    force_option = __get_force_option(force)
    __tag_single_module(ctx, DEFAULT_MODULES["."], agent_version, commit, push, force_option, devel)
    print(f"Created tags for version {agent_version}")


@task
def tag_devel(ctx, agent_version, commit="HEAD", verify=True, push=True, force=False):
    tag_version(ctx, agent_version, commit, verify, push, force, devel=True)
    tag_modules(ctx, agent_version, commit, verify, push, force, devel=True)


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
def finish(ctx, major_versions="6,7", upstream="origin"):
    """
    Updates the release entry in the release.json file for the new version.

    Updates internal module dependencies with the new version.
    """

    if sys.version_info[0] < 3:
        return Exit(message="Must use Python 3 for this task", code=1)

    list_major_versions = parse_major_versions(major_versions)
    print(f"Finishing release for major version(s) {list_major_versions}")

    for major_version in list_major_versions:
        # NOTE: the release process assumes that at least one RC
        # was built before release.finish is used. It doesn't support
        # doing final version -> final version updates (eg. 7.32.0 -> 7.32.1
        # without doing at least 7.32.1-rc.1), as next_final_version won't
        # find the correct new version.
        # To support this, we'd have to support a --patch-version param in
        # release.finish
        new_version = next_final_version(ctx, major_version, False)
        update_release_json(new_version, new_version)

    # Update internal module dependencies
    update_modules(ctx, str(new_version))

    # Step 3: branch out, commit change, push branch

    final_branch = f"{new_version}-final"

    print(color_message(f"Branching out to {final_branch}", "bold"))
    ctx.run(f"git checkout -b {final_branch}")

    print(color_message("Committing release.json and Go modules updates", "bold"))
    print(
        color_message(
            "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
        )
    )
    ctx.run("git add release.json")
    ctx.run("git ls-files . | grep 'go.mod$' | xargs git add")

    commit_message = f"'Final updates for release.json and Go modules for {new_version} release'"

    ok = try_git_command(ctx, f"git commit -m {commit_message}")
    if not ok:
        raise Exit(
            color_message(
                f"Could not create commit. Please commit manually with:\ngit commit -m {commit_message}\n, push the {final_branch} branch and then open a PR against {final_branch}.",
                "red",
            ),
            code=1,
        )

    # Step 4: add release changelog preludes
    print(color_message("Adding Agent release changelog prelude", "bold"))
    add_prelude(ctx, str(new_version))

    print(color_message("Adding DCA release changelog prelude", "bold"))
    add_dca_prelude(ctx, str(new_version))

    ok = try_git_command(ctx, f"git commit -m 'Add preludes for {new_version} release'")
    if not ok:
        raise Exit(
            color_message(
                f"Could not create commit. Please commit manually, push the {final_branch} branch and then open a PR against {final_branch}.",
                "red",
            ),
            code=1,
        )

    # Step 5: push branch and create PR

    print(color_message("Pushing new branch to the upstream repository", "bold"))
    res = ctx.run(f"git push --set-upstream {upstream} {final_branch}", warn=True)
    if res.exited is None or res.exited > 0:
        raise Exit(
            color_message(
                f"Could not push branch {final_branch} to the upstream '{upstream}'. Please push it manually and then open a PR against {final_branch}.",
                "red",
            ),
            code=1,
        )

    current_branch = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    create_pr(
        f"Final updates for release.json and Go modules for {new_version} release + preludes",
        current_branch,
        final_branch,
        new_version,
    )


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

    github = GithubAPI(repository=GITHUB_REPO_NAME)

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

    # Check that the current and update branches are valid
    current_branch = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    update_branch = f"release/{new_highest_version}"

    check_clean_branch_state(ctx, github, update_branch)
    if not check_base_branch(current_branch, new_highest_version):
        raise Exit(
            color_message(
                f"The branch you are on is neither {DEFAULT_BRANCH} or the correct release branch ({new_highest_version.branch()}). Aborting.",
                "red",
            ),
            code=1,
        )

    # Step 1: Update release entries

    print(color_message("Updating release entries", "bold"))
    for major_version in list_major_versions:
        new_version = next_rc_version(ctx, major_version, patch_version)
        update_release_json(new_version, new_final_version)

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

    create_pr(
        f"[release] Update release.json and Go modules for {versions_string}",
        current_branch,
        update_branch,
        new_final_version,
    )


def create_pr(title, base_branch, target_branch, version, changelog_pr=False):
    print(color_message("Creating PR", "bold"))

    github = GithubAPI(repository=GITHUB_REPO_NAME)

    # Find milestone based on what the next final version is. If the milestone does not exist, fail.
    milestone_name = str(version)

    milestone = github.get_milestone_by_name(milestone_name)

    if not milestone or not milestone.number:
        raise Exit(
            color_message(
                f"""Could not find milestone {milestone_name} in the Github repository. Response: {milestone}
Make sure that milestone is open before trying again.""",
                "red",
            ),
            code=1,
        )

    pr = github.create_pr(
        pr_title=title,
        pr_body="",
        base_branch=base_branch,
        target_branch=target_branch,
    )

    if not pr:
        raise Exit(
            color_message(f"Could not create PR in the Github repository. Response: {pr}", "red"),
            code=1,
        )

    print(color_message(f"Created PR #{pr.number}", "bold"))

    labels = [
        "changelog/no-changelog",
        "qa/no-code-change",
        "team/agent-build-and-releases",
        "team/agent-release-management",
        "category/release_operations",
    ]

    if changelog_pr:
        labels.append("backport/main")

    updated_pr = github.update_pr(
        pull_number=pr.number,
        milestone_number=milestone.number,
        labels=labels,
    )

    if not updated_pr or not updated_pr.number or not updated_pr.html_url:
        raise Exit(
            color_message(f"Could not update PR in the Github repository. Response: {updated_pr}", "red"),
            code=1,
        )

    print(color_message(f"Set labels and milestone for PR #{updated_pr.number}", "bold"))
    print(color_message(f"Done preparing release PR. The PR is available here: {updated_pr.html_url}", "bold"))


@task
def build_rc(ctx, major_versions="6,7", patch_version=False, k8s_deployments=False):
    """
    To be done after the PR created by release.create-rc is merged, with the same options
    as release.create-rc.

    k8s_deployments - when set to True the child pipeline deploying to subset of k8s staging clusters will be triggered.

    Tags the new RC versions on the current commit, and creates the build pipeline for these
    new tags.
    """
    if sys.version_info[0] < 3:
        return Exit(message="Must use Python 3 for this task", code=1)

    gitlab = Gitlab(project_name=GITHUB_REPO_NAME, api_token=get_gitlab_token())
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
        rc_build=True,
        rc_k8s_deployments=k8s_deployments,
    )


@task(help={'key': "Path to an existing release.json key, separated with double colons, eg. 'last_stable::6'"})
def set_release_json(_, key, value):
    release_json = _load_release_json()
    path = key.split('::')
    current_node = release_json
    for key_idx in range(len(path)):
        key = path[key_idx]
        if key not in current_node:
            raise Exit(code=1, message=f"Couldn't find '{key}' in release.json")
        if key_idx == len(path) - 1:
            current_node[key] = value
            break
        else:
            current_node = current_node[key]
    _save_release_json(release_json)


@task(help={'key': "Path to the release.json key, separated with double colons, eg. 'last_stable::6'"})
def get_release_json_value(_, key):
    release_json = _get_release_json_value(key)
    print(release_json)


def _get_release_json_value(key):
    release_json = _load_release_json()

    path = key.split('::')

    for element in path:
        if element not in release_json:
            raise Exit(code=1, message=f"Couldn't find '{key}' in release.json")

        release_json = release_json.get(element)

    return release_json


def create_and_update_release_branch(ctx, repo, release_branch, base_directory="~/dd", upstream="origin"):
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
            # Step 1.1 - In datadog-agent repo update base_branch and nightly builds entries
            rj = _load_release_json()

            rj["base_branch"] = release_branch

            for nightly in ["nightly", "nightly-a7"]:
                for field in RELEASE_JSON_FIELDS_TO_UPDATE:
                    rj[nightly][field] = f"{release_branch}"

            _save_release_json(rj)

            # Step 1.2 - In datadog-agent repo update gitlab-ci.yaml jobs
            with open(".gitlab-ci.yml", "r") as gl:
                file_content = gl.readlines()

            with open(".gitlab-ci.yml", "w") as gl:
                for line in file_content:
                    if re.search(r"compare_to: main", line):
                        gl.write(line.replace("main", f"{release_branch}"))
                    else:
                        gl.write(line)

            # Step 1.3 - Commit new changes
            ctx.run("git add release.json .gitlab-ci.yml")
            ok = try_git_command(ctx, f"git commit -m 'Update release.json and .gitlab-ci.yml with {release_branch}'")
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
def unfreeze(ctx, base_directory="~/dd", major_versions="6,7", upstream="origin"):
    """
    Performs set of tasks required for the main branch unfreeze during the agent release cycle.
    That includes:
    - creates a release branch in datadog-agent, datadog-agent-macos, omnibus-ruby and omnibus-software repositories,
    - updates release.json on new datadog-agent branch to point to newly created release branches in nightly section
    - updates entries in .gitlab-ci.yml which depend on local branch name

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

    # Step 0: checks

    print(color_message("Checking repository state", "bold"))
    ctx.run("git fetch")

    github = GithubAPI(repository=GITHUB_REPO_NAME)
    check_clean_branch_state(ctx, github, release_branch)

    if not yes_no_question(
        f"This task will create new branches with the name '{release_branch}' in repositories: {', '.join(UNFREEZE_REPOS)}. Is this OK?",
        color="orange",
        default=False,
    ):
        raise Exit(color_message("Aborting.", "red"), code=1)

    for repo in UNFREEZE_REPOS:
        create_and_update_release_branch(ctx, repo, release_branch, base_directory=base_directory, upstream=upstream)


def _update_last_stable(_, version, major_versions="6,7"):
    """
    Updates the last_release field(s) of release.json
    """
    release_json = _load_release_json()
    list_major_versions = parse_major_versions(major_versions)
    # If the release isn't a RC, update the last stable release field
    for major in list_major_versions:
        version.major = major
        release_json['last_stable'][str(major)] = str(version)
    _save_release_json(release_json)


@task
def cleanup(ctx):
    """
    Perform the post release cleanup steps
    Currently this:
      - Updates the scheduled nightly pipeline to target the new stable branch
      - Updates the release.json last_stable fields
    """
    gh = GithubAPI()
    latest_release = gh.latest_release()
    match = VERSION_RE.search(latest_release)
    if not match:
        raise Exit(f'Unexpected version fetched from github {latest_release}', code=1)
    version = _create_version_from_match(match)
    _update_last_stable(ctx, version)
    edit_schedule(ctx, 2555, ref=version.branch())


@task
def check_omnibus_branches(ctx):
    base_branch = _get_release_json_value('base_branch')
    if base_branch == 'main':
        omnibus_ruby_branch = 'datadog-5.5.0'
        omnibus_software_branch = 'master'
    else:
        omnibus_ruby_branch = base_branch
        omnibus_software_branch = base_branch

    def _check_commit_in_repo(repo_name, branch, release_json_field):
        with tempfile.TemporaryDirectory() as tmpdir:
            ctx.run(
                f'git clone --depth=50 https://github.com/DataDog/{repo_name} --branch {branch} {tmpdir}/{repo_name}',
                hide='stdout',
            )
            for version in ['nightly', 'nightly-a7']:
                commit = _get_release_json_value(f'{version}::{release_json_field}')
                if ctx.run(f'git -C {tmpdir}/{repo_name} branch --contains {commit}', warn=True, hide=True).exited != 0:
                    raise Exit(
                        code=1,
                        message=f'{repo_name} commit ({commit}) is not in the expected branch ({branch}). The PR is not mergeable',
                    )
                else:
                    print(f'[{version}] Commit {commit} was found in {repo_name} branch {branch}')

    _check_commit_in_repo('omnibus-ruby', omnibus_ruby_branch, 'OMNIBUS_RUBY_VERSION')
    _check_commit_in_repo('omnibus-software', omnibus_software_branch, 'OMNIBUS_SOFTWARE_VERSION')

    return True


@task
def update_build_links(_ctx, new_version):
    """
    Updates Agent release candidates build links on https://datadoghq.atlassian.net/wiki/spaces/agent/pages/2889876360/Build+links

    new_version - should be given as an Agent 7 RC version, ie. '7.50.0-rc.1' format.

    Notes:
    Attlasian credentials are required to be available as ATLASSIAN_USERNAME and ATLASSIAN_PASSWORD as environment variables.
    ATLASSIAN_USERNAME is typically an email address.
    ATLASSIAN_PASSWORD is a token. See: https://id.atlassian.com/manage-profile/security/api-tokens
    """
    from atlassian import Confluence
    from atlassian.confluence import ApiError

    BUILD_LINKS_PAGE_ID = 2889876360

    match = RC_VERSION_RE.match(new_version)
    if not match:
        raise Exit(
            color_message(
                f"{new_version} is not a valid Agent RC version number/tag. \nCorrect example: 7.50.0-rc.1",
                "red",
            ),
            code=1,
        )

    username = os.getenv("ATLASSIAN_USERNAME")
    password = os.getenv("ATLASSIAN_PASSWORD")

    if username is None or password is None:
        raise Exit(
            color_message(
                "No Atlassian credentials provided. Run inv --help update-build-links for more details.",
                "red",
            ),
            code=1,
        )

    confluence = Confluence(url="https://datadoghq.atlassian.net/", username=username, password=password)

    content = confluence.get_page_by_id(page_id=BUILD_LINKS_PAGE_ID, expand="body.storage")

    title = content["title"]
    current_version = title[-11:]
    body = content["body"]["storage"]["value"]

    title = title.replace(current_version, new_version)

    patterns = _create_build_links_patterns(current_version, new_version)

    for key in patterns:
        body = body.replace(key, patterns[key])

    print(color_message(f"Updating QA Build links page with {new_version}", "bold"))

    try:
        confluence.update_page(BUILD_LINKS_PAGE_ID, title, body=body)
    except ApiError as e:
        raise Exit(
            color_message(
                f"Failed to update confluence page. Reason: {e.reason}",
                "red",
            ),
            code=1,
        )
    print(color_message("Build links page updated", "green"))


def _create_build_links_patterns(current_version, new_version):
    patterns = {}

    current_minor_version = current_version[1:]
    new_minor_version = new_version[1:]

    patterns[current_minor_version] = new_minor_version
    patterns[current_minor_version.replace("rc.", "rc-")] = new_minor_version.replace("rc.", "rc-")
    patterns[current_minor_version.replace("-rc", "~rc")] = new_minor_version.replace("-rc", "~rc")

    return patterns


@task
def get_active_release_branch(_ctx):
    """
    Determine what is the current active release branch for the Agent.
    If release started and code freeze is in place - main branch is considered active.
    If release started and code freeze is over - release branch is considered active.
    """
    gh = GithubAPI()
    latest_release = gh.latest_release()
    version = _create_version_from_match(VERSION_RE.search(latest_release))
    next_version = version.next_version(bump_minor=True)
    release_branch = gh.get_branch(next_version.branch())
    if release_branch:
        print(f"{release_branch.name}")
    else:
        print("main")
